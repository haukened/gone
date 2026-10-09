# Gone API Documentation

This directory contains the OpenAPI specification (`openapi.yaml`) for the Gone one-time secret sharing service. This page summarizes the same API in prose. The encryption format, link fragment, and client rules are specified in [protocol.md](protocol.md). The command-line client is documented in [cli.md](cli.md).

## Design Goals
- **Minimal surface**: Three secret operations (create, claim, acknowledge), two sender operations (status, revoke), secret requests (create, check, reply, status, claim, acknowledge, cancel), and health probes.
- **Zero knowledge**: The server only ever sees ciphertext. Encryption, decryption, and the key stay in the browser.
- **Streaming-friendly**: Ciphertext is sent and returned as raw `application/octet-stream`, not wrapped in JSON.
- **Deterministic deletion**: Retrieval is two-phase. `GET` claims the secret and `DELETE` acknowledges receipt, so a secret is deleted only after the recipient has it, or when the claim lease lapses. It is never served to a second party.
- **Explicit limits**: The service enforces `MaxBytes` (default 10 MiB, covering the message and all attachments combined), and TTL must fall within the configured `[MinTTL, MaxTTL]`.
- **Opaque IDs**: 128-bit random, 32 lowercase hex characters; never guessable or sequential.

## Endpoints Overview
| Method | Path | Purpose |
| ------ | ---- | ------- |
| POST | `/api/secret` | Create a secret (returns ID, expiry & manage token) |
| GET | `/api/secret/{id}` | Claim a secret, or re-fetch an existing claim (returns ciphertext + claim token) |
| DELETE | `/api/secret/{id}` | Acknowledge receipt; permanently deletes the secret |
| GET | `/api/secret/{id}/status` | Sender: is the secret still waiting? (`X-Gone-Manage`) |
| POST | `/api/secret/{id}/revoke` | Sender: delete the secret before it is opened (`X-Gone-Manage`) |
| POST | `/api/request` | Requester: open a secret request (`X-Gone-TTL`, no body; returns ID, expiry, manage token & fill token) |
| GET | `/api/request/{id}` | Reply page: can this request still be answered? (`X-Gone-Fill`) |
| PUT | `/api/request/{id}/reply` | Reply page: send the one reply, encrypted to the requester (`X-Gone-Fill`) |
| GET | `/api/request/{id}/status` | Requester: waiting or ready? (`X-Gone-Manage`) |
| GET | `/api/request/{id}/reply` | Requester: claim the reply (`X-Gone-Manage`, plus `X-Gone-Claim` on a retry) |
| DELETE | `/api/request/{id}/reply` | Requester: acknowledge the reply; permanently deletes it (`X-Gone-Claim`) |
| POST | `/api/request/{id}/revoke` | Requester: cancel the request, or delete an unopened reply (`X-Gone-Manage`) |
| GET | `/healthz` | Liveness check (`200`, text `ok`) |
| GET | `/readyz` | Readiness check: the database answers a ping and the blob directory is readable (`200`, text `ready`; otherwise `503` `not ready`) |

Other methods on `/api/secret/{id}` return `405` with `Allow: GET, DELETE`. The same applies to `/status` (`Allow: GET`) and `/revoke` (`Allow: POST`), and to `/api/request/{id}/reply` (`Allow: GET, PUT, DELETE`). Unknown `/api/` paths return a JSON `404`.

Metrics are **not** served on the public listener. When both `GONE_METRICS_ADDR` and `GONE_METRICS_TOKEN` are set, a separate listener serves a JSON snapshot that requires `Authorization: Bearer <token>`. If either is missing, metrics are disabled.

## Creation Workflow
1. The client builds the plaintext (see [Payload Format](#payload-format)) and encrypts it locally with AES-256-GCM, producing ciphertext, a version, and a nonce.
2. The client sends the ciphertext body with these headers:
   - `X-Gone-Version` (canonical decimal, no sign or leading zeros; `1`, or `2` for a passphrase-protected secret)
   - `X-Gone-Nonce` (exactly 16 unpadded base64url characters encoding the 12-byte GCM IV)
   - `X-Gone-TTL` (Go duration, e.g. `15m`)
   - `Content-Length` (required; chunked uploads are rejected)

   Each `X-Gone-*` header must appear exactly once.
3. The server validates size and TTL, issues an ID, and stores the ciphertext inline in SQLite (≤ `GONE_INLINE_MAX_BYTES`) or as a filesystem blob.
4. Response: `201` with JSON `{ "id": "<32-hex>", "expires_at": "RFC3339", "manage_token": "<43-char base64url>" }`. The server keeps only a SHA-256 hash of the manage token.
5. The client builds the share link `/secret/{id}#v1:<base64url-key>` (or `#v2:` with a passphrase). The key lives only in the URL fragment, which browsers never send to the server. The passphrase never appears in the link.
6. The client also builds the sender's private manage link `/manage/{id}#<manage_token>`. It cannot decrypt anything; it can only check on or revoke the secret.

## Sender Workflow (Status + Revoke)
The manage page reads the token from the URL fragment and sends it only in the `X-Gone-Manage` header.

1. **Status.** `GET /api/secret/{id}/status` returns `200` with `{ "state": "pending", "created_at", "expires_at" }` while the secret is waiting, including while a recipient's claim lease is active. Checking status never claims or deletes anything.
2. **Revoke.** `POST /api/secret/{id}/revoke` deletes the secret and returns `204`. Revoke wins over an active claim: the recipient's acknowledgement then fails. A recipient who already downloaded the ciphertext still holds it, so revoke is only reliable before the link is opened.
3. **No tombstones.** Once a secret is opened, revoked, or expired, nothing about it remains. Status and revoke return the same `404` for all of these cases, for unknown IDs, and for a wrong token. The sender cannot tell "opened" from "expired" or "revoked", and nobody else can learn anything.

## Request Workflow
A secret request runs the other way: the person who needs a secret makes a link, and the person who has it answers. The cryptography is protocol v3 ([protocol.md](protocol.md) §4.3, §8.6).

1. **Create.** The requester's browser makes an ECDH P-256 key pair and keeps the private key, non-extractable, in IndexedDB. It sends `POST /api/request` with `X-Gone-TTL` and no body. Response: `201` with `{ "id", "expires_at", "manage_token", "fill_token" }`. The server stores only SHA-256 hashes of both tokens, and never sees the public key or the requester's label.
2. **Reply link.** The browser builds `/reply/{id}#v3:<base64url public key>.<fill_token>` for the requester to send to whoever has the secret.
3. **Check.** The reply page validates the fragment, then sends `GET /api/request/{id}` with `X-Gone-Fill`. `200 { "state": "open", "expires_at" }` while it can be answered, `404` otherwise. The check is read-only.
4. **Reply.** The reply page encrypts the message and files to the public key and sends `PUT /api/request/{id}/reply` with `X-Gone-Version: 3`, `X-Gone-Nonce`, `X-Gone-Fill` and `Content-Length`. Response: `201 { "expires_at" }`. Each request takes exactly one reply; the reply is then kept for the request's TTL again, counted from when it was sent.
5. **Status.** The requester's page polls `GET /api/request/{id}/status` with `X-Gone-Manage`: `{ "state": "waiting" | "ready", "created_at", "expires_at" }`. Status is read-only, so polling costs no writes.
6. **Open.** `GET /api/request/{id}/reply` with `X-Gone-Manage` claims the reply exactly like a secret claim (same headers and lease), and `DELETE /api/request/{id}/reply` with `X-Gone-Claim` deletes it. `GET /api/secret/{id}` never returns a reply, so the ID alone can't burn it.
7. **Cancel.** `POST /api/request/{id}/revoke` with `X-Gone-Manage` deletes an open request or an unopened reply (`204`).
8. **No tombstones.** Expired, cancelled, answered, opened and unknown requests, and wrong tokens, all return the same `404`.

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

File names, types, and sizes are therefore encrypted along with the contents. Decoders sanitize file names and map MIME types onto an allowlist (see [protocol.md §6](protocol.md#6-sanitization)). The web client allows up to 10 files of any type. The message and all files share the `MaxBytes` limit, which includes the 16-byte GCM tag. Encryption uses AES-256-GCM with AAD `gone:v1`. With a passphrase (protocol v2), the AES key also depends on the passphrase through PBKDF2-SHA-256 and HKDF, and the body starts with a 21-byte authenticated header that the server stores without parsing (see [protocol.md §4.2](protocol.md#42-protocol-version-2-passphrase)).

## Error Mapping
All errors are JSON `{ "error": "<message>" }`.

| Condition | Status | Body `error` |
| --------- | ------ | ------------ |
| `X-Correlation-ID` present but not a UUID (any route) | 400 | `invalid correlation id` |
| Invalid ID format | 400 | `invalid id` |
| Malformed claim token (DELETE without/with bad `X-Gone-Claim`, or bad token on GET retry) | 400 | `invalid claim` |
| Missing, repeated, or malformed `X-Gone-Manage` on status/revoke | 400 | `invalid manage token` |
| Missing, repeated, or malformed `X-Gone-Fill` on a request check or reply | 400 | `invalid fill token` |
| Missing, empty, or repeated `X-Gone-Version` / `X-Gone-Nonce` / `X-Gone-TTL` | 400 | `missing required headers` |
| Non-canonical or unsupported version (on a reply, anything but `3`) | 400 | `invalid version` |
| Nonce not 16 base64url characters (12 bytes) | 400 | `invalid nonce` |
| Unparseable TTL | 400 | `invalid ttl` |
| TTL outside `[MinTTL, MaxTTL]` | 400 | `ttl invalid` |
| Unparseable `Content-Length` | 400 | `invalid content length` |
| Not found / expired / claimed by someone else / wrong token / lease lapsed / opened or revoked (status, revoke) | 404 | `not found` |
| Method not allowed | 405 | `method not allowed` |
| Missing `Content-Length` on create | 411 | `content length required` |
| Size > `MaxBytes` | 413 | `size exceeded` |
| Per-client rate limit exceeded (sets `Retry-After`) | 429 | `rate limited` |
| Internal failure | 500 | `internal` |
| Storage overloaded: no write turn within 5s (sets `Retry-After`; nothing changed, safe to retry) | 503 | `busy` |

## Rate Limiting
Each client address has two token buckets: one for creating secrets (`POST /api/secret`, `GONE_RATE_CREATE`) and one for reading them (`GET` and `DELETE /api/secret/{id}`, plus `/status` and `/revoke`, `GONE_RATE_READ`). Requests follow the same split: `POST /api/request` and `PUT /api/request/{id}/reply` (which uploads a body) use the create bucket, and every other `/api/request/` call uses the read bucket. Both buckets hold up to `GONE_RATE_BURST` tokens and refill steadily at the configured rate. Health probes, pages, and static assets are not limited.

- Over budget: `429` with `Retry-After: <seconds>` and `{ "error": "rate limited" }`. The request body is never read.
- Clients are keyed by IPv4 `/32` or IPv6 `/64`.
- The client address is the TCP peer. `X-Forwarded-For` is honored only when the peer is in `GONE_TRUSTED_PROXIES`; it is then walked right to left, skipping trusted hops, and the first untrusted address is used.
- Counters are per process. With several replicas, the effective budget is multiplied by the replica count.
- Each IPv6 `/48` (or IPv4 `/24`) may hold at most 256 tracked clients, and the table holds at most 100,000. Beyond either cap, clients share overflow buckets grouped by that parent network, so one large allocation cannot crowd out other users.
- Rejections are counted in the `rate_limited_create_total` and `rate_limited_read_total` metrics.
- Request activity is counted in `requests_created_total`, `requests_filled_total`, `requests_opened_total`, `requests_cancelled_total` and `requests_expired_total`.
- Client addresses are never logged; only the correlation ID and scope are.

## Security Headers
Every response carries:
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`
- `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`
- `X-Correlation-ID`: the request's value if it parses as a UUID (returned in canonical form), or a generated UUID v4. A value that isn't a UUID is rejected with `400` on every route.

Responses default to `Cache-Control: no-store` and `Pragma: no-cache`; static assets under `/static/` override it with `Cache-Control: public, max-age=300` (the `Pragma: no-cache` header still appears on them).

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
- `X-Gone-Nonce` must decode (base64url, unpadded) to exactly 12 bytes; anything else is `400 invalid nonce`.
- The duration regex in the spec mirrors a subset of Go's `time.ParseDuration`.

Refer to `openapi.yaml` for the machine-readable schema.
