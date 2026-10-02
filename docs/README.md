# Gone API Documentation

This directory contains the OpenAPI specification (`openapi.yaml`) for the Gone one-time secret sharing service. This page summarizes the same API in prose.

## Design Goals
- **Minimal surface**: Three secret operations (create, claim, acknowledge) plus health probes.
- **Zero knowledge**: The server only ever sees ciphertext. Encryption, decryption, and the key stay in the browser.
- **Streaming-friendly**: Ciphertext is sent and returned as raw `application/octet-stream`, not wrapped in JSON.
- **Deterministic deletion**: Retrieval is two-phase. `GET` claims the secret and `DELETE` acknowledges receipt, so a secret is deleted only after the recipient has it, or when the claim lease lapses. It is never served to a second party.
- **Explicit limits**: The service enforces `MaxBytes` (default 10 MiB, covering the message and all attachments combined), and TTL must fall within the configured `[MinTTL, MaxTTL]`.
- **Opaque IDs**: 128-bit random, 32 lowercase hex characters; never guessable or sequential.

## Endpoints Overview
| Method | Path | Purpose |
| ------ | ---- | ------- |
| POST | `/api/secret` | Create a secret (returns ID & expiry) |
| GET | `/api/secret/{id}` | Claim a secret, or re-fetch an existing claim (returns ciphertext + claim token) |
| DELETE | `/api/secret/{id}` | Acknowledge receipt; permanently deletes the secret |
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check (DB ping) |

Other methods on `/api/secret/{id}` return `405` with `Allow: GET, DELETE`. Unknown `/api/` paths return a JSON `404`.

Metrics are **not** served on the public listener. When both `GONE_METRICS_ADDR` and `GONE_METRICS_TOKEN` are set, a separate listener serves a JSON snapshot that requires `Authorization: Bearer <token>`. If either is missing, metrics are disabled.

## Creation Workflow
1. The client builds the plaintext (see [Payload Format](#payload-format)) and encrypts it locally with AES-256-GCM, producing ciphertext, a version, and a nonce.
2. The client sends the ciphertext body with these headers:
   - `X-Gone-Version` (uint8; currently `1`)
   - `X-Gone-Nonce` (base64url, 12-byte GCM IV)
   - `X-Gone-TTL` (Go duration, e.g. `15m`)
   - `Content-Length` (required; chunked uploads are rejected)
3. The server validates size and TTL, issues an ID, and stores the ciphertext inline in SQLite (≤ `GONE_INLINE_MAX_BYTES`) or as a filesystem blob.
4. Response: `201` with JSON `{ "id": "<32-hex>", "expires_at": "RFC3339" }`.
5. The client builds the share link `/secret/{id}#v1:<base64url-key>`. The key lives only in the URL fragment, which browsers never send to the server.

## Consumption Workflow (Claim + Acknowledge)
1. **Claim.** The client sends `GET /api/secret/{id}` without an `X-Gone-Claim` header. If the secret exists, is unexpired, and is unclaimed, the server atomically marks it claimed and generates a random claim token. Only a SHA-256 hash of the token is stored.
2. **Response:** `200` with the ciphertext body and headers:
   - `X-Gone-Version`, `X-Gone-Nonce`
   - `X-Gone-Claim`: opaque claim token (43-char base64url)
   - `X-Gone-Claim-Expires`: RFC 3339 UTC lease deadline (`GONE_CLAIM_LEASE`, default `2m`, max `15m`)
   - `Content-Length`, `Content-Type: application/octet-stream`, `Cache-Control: no-store`
3. **Verify and decrypt.** The client checks that it received exactly `Content-Length` bytes, then decrypts. AES-GCM authentication detects any truncation or tampering.
4. **Retry (optional).** If the download was interrupted, the client repeats `GET /api/secret/{id}` with `X-Gone-Claim: <token>` while the lease is still valid. Only the token holder can re-fetch.
5. **Acknowledge.** The client sends `DELETE /api/secret/{id}` with `X-Gone-Claim: <token>`. The server permanently deletes the record and blob and returns `204 No Content`.
6. **Lease expiry.** If no acknowledgement arrives before the lease ends, the janitor deletes the secret (counted by `secrets_claims_expired_total`). It is never re-served to anyone else.
7. Any later request, whether a fresh claim of a claimed or deleted secret or a wrong token, returns `404`.

## Payload Format
The server treats the body as opaque ciphertext. Inside the encryption:
- **Text only:** the plaintext is the raw UTF-8 message.
- **With attachments (envelope v2):** `"GONE2\0\0\0"` magic (8 bytes), a big-endian `u32` header length, a JSON header `{ "v": 2, "msg": <message byte length>, "files": [{ "name", "type", "size" }] }`, then the message bytes, then each file's bytes in order.

File names, types, and sizes are therefore encrypted along with the contents. The web client allows up to 10 files of any type. The message and all files share the `MaxBytes` limit, which includes the 16-byte GCM tag. Encryption uses AES-256-GCM with AAD `gone:v1`.

## Error Mapping
All errors are JSON `{ "error": "<message>" }`.

| Condition | Status | Body `error` |
| --------- | ------ | ------------ |
| Invalid ID format | 400 | `invalid id` |
| Malformed claim token (DELETE without/with bad `X-Gone-Claim`, or bad token on GET retry) | 400 | `invalid claim` |
| Missing `X-Gone-Version` / `X-Gone-Nonce` / `X-Gone-TTL` | 400 | `missing required headers` |
| Unparseable version | 400 | `invalid version` |
| Unparseable TTL | 400 | `invalid ttl` |
| TTL outside `[MinTTL, MaxTTL]` | 400 | `ttl invalid` |
| Unparseable `Content-Length` | 400 | `invalid content length` |
| Not found / expired / claimed by someone else / wrong token / lease lapsed | 404 | `not found` |
| Method not allowed | 405 | `method not allowed` |
| Missing `Content-Length` on create | 411 | `content length required` |
| Size > `MaxBytes` | 413 | `size exceeded` |
| Per-client rate limit exceeded (sets `Retry-After`) | 429 | `rate limited` |
| Internal failure | 500 | `internal` |

## Rate Limiting
Each client address has two token buckets: one for creating secrets (`POST /api/secret`, `GONE_RATE_CREATE`) and one for reading them (`GET` and `DELETE /api/secret/{id}`, `GONE_RATE_READ`). Both buckets hold up to `GONE_RATE_BURST` tokens and refill steadily at the configured rate. Health probes, pages, and static assets are not limited.

- Over budget: `429` with `Retry-After: <seconds>` and `{ "error": "rate limited" }`. The request body is never read.
- Clients are keyed by IPv4 `/32` or IPv6 `/64`.
- The client address is the TCP peer. `X-Forwarded-For` is honored only when the peer is in `GONE_TRUSTED_PROXIES`; it is then walked right to left, skipping trusted hops, and the first untrusted address is used.
- Counters are per process. With several replicas, the effective budget is multiplied by the replica count.
- Each IPv6 `/48` (or IPv4 `/24`) may hold at most 256 tracked clients, and the table holds at most 100,000. Beyond either cap, clients share overflow buckets grouped by that parent network, so one large allocation cannot crowd out other users.
- Rejections are counted in the `rate_limited_create_total` and `rate_limited_read_total` metrics.
- Client addresses are never logged; only the correlation ID and scope are.

## Security Headers
Every response carries:
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`
- `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`
- `X-Correlation-ID` (echoed from the request, or a generated UUID v4)

Responses default to `Cache-Control: no-store` and `Pragma: no-cache`; static assets under `/static/` set their own caching policy.

## Future Extensions (Non-Breaking)
- Optional JSON POST mode with metadata wrapper.
- Prometheus exposition format for metrics.

## Non-Goals
- Secret re-use or updates.
- Listing secrets.
- Multi-consume semantics.
- Server-side encryption (handled entirely client-side).

## Implementation Notes
- The inline vs external threshold (`GONE_INLINE_MAX_BYTES`, default 8 KiB) is not exposed via the API.
- `X-Gone-Nonce` length is not strictly enforced beyond being non-empty; cryptographic validation is the client's responsibility.
- The duration regex in the spec mirrors a subset of Go's `time.ParseDuration`.

Refer to `openapi.yaml` for the machine-readable schema.
