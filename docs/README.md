# Gone API Documentation

This directory contains the OpenAPI specification (`openapi.yaml`) for the Gone one-time secret sharing service.

## Design Goals
- **Minimal surface**: Only two core endpoints (`POST /api/secret`, `GET /api/secret/{id}`) plus health probes.
- **Streaming-friendly**: Ciphertext is sent and returned as raw `application/octet-stream` rather than JSON-wrapped.
- **Deterministic deletion**: Retrieval consumes the secret immediately (metadata row hard-deleted, blob deleted-on-close).
- **Explicit limits**: Service enforces `MaxBytes`, and TTL must fall within configured `[MinTTL, MaxTTL]`.
- **Opaque IDs**: 128-bit random, 32 lowercase hex; never guessable or sequential.

## Endpoints Overview
| Method | Path | Purpose |
| ------ | ---- | ------- |
| POST | `/api/secret` | Create a secret (returns ID & expiry) |
| GET | `/api/secret/{id}` | Consume secret once (returns ciphertext) |
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check |

## Creation Workflow
1. Client encrypts plaintext locally, producing ciphertext, version, nonce.
2. Client sends ciphertext body with headers:
   - `X-Gone-Version` (uint8)
   - `X-Gone-Nonce` (base64url)
   - `X-Gone-TTL` (Go duration, e.g. `15m`)
   - `Content-Length` (required; no chunked uploads accepted initially)
3. Server validates size & TTL, issues ID, stores inline or external depending on size.
4. Response: `201` with JSON `{ "id": "<32-hex>", "expires_at": "RFC3339" }`.

## Consumption Workflow
1. Client `GET /api/secret/{id}`.
2. Server validates ID format.
3. If found and not expired, metadata row is atomically hard-deleted; blob (if external) is streamed and deleted on close.
4. Response: `200` with ciphertext body and headers `X-Gone-Version`, `X-Gone-Nonce`, `Content-Length`.
5. Subsequent requests return `404`.

## Error Mapping
| Condition | Status | Example Body |
| --------- | ------ | ------------ |
| Invalid ID | 400 | `{ "error": "invalid id" }` |
| TTL out of range | 400 | `{ "error": "ttl invalid" }` |
| Size > MaxBytes | 413 | `{ "error": "size exceeded" }` |
| Not found / consumed / expired | 404 | `{ "error": "not found" }` |
| Internal failure | 500 | `{ "error": "internal" }` |

## Security Headers (planned)
- `Cache-Control: no-store`
- `Pragma: no-cache`
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`
- `Content-Security-Policy: default-src 'none'`

## Future Extensions (Non-Breaking)
- Optional JSON POST mode with metadata wrapper.
- Rate limiting headers.
- Metrics endpoint (`/metrics`).
- ETag / If-None-Match for creation response caching (unlikely needed).

## Non-Goals
- Secret re-use or updates.
- Listing secrets.
- Multi-consume semantics.
- Server-side encryption (handled entirely client-side).

## Implementation Notes
- Inline vs external threshold determined by `inlineMax` in store; not exposed via API.
- `X-Gone-Nonce` length not strictly enforced beyond being non-empty; cryptographic validation remains client responsibility.
- Duration regex in spec mirrors Go `time.ParseDuration` subset.

Refer to `openapi.yaml` for machine-readable schema.
