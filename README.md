[![Build](https://github.com/haukened/gone/actions/workflows/build.yaml/badge.svg)](https://github.com/haukened/gone/actions/workflows/build.yaml)
[![Security Scan](https://github.com/haukened/gone/actions/workflows/sec.yaml/badge.svg)](https://github.com/haukened/gone/actions/workflows/sec.yaml)
[![Codacy Badge](https://app.codacy.com/project/badge/Grade/f632a2010c7748199f7c2cb8317feffa)](https://app.codacy.com/gh/haukened/gone/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Codacy Badge](https://app.codacy.com/project/badge/Coverage/f632a2010c7748199f7c2cb8317feffa)](https://app.codacy.com/gh/haukened/gone/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage)

![GitHub License](https://img.shields.io/github/license/haukened/gone)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/haukened/gone)
![GitHub last commit](https://img.shields.io/github/last-commit/haukened/gone)


# gone

Go + One = Gone — a tiny service for sharing a secret exactly once.

Gone lets you paste a sensitive value (password, token, wifi key), generate a one‑time link, and send that link. The first person to open it sees the secret; after that it’s gone for good.

---

## 1. Quick Start (90‑second demo)

Run with Docker:

```sh
docker run --rm -p 8080:8080 ghcr.io/haukened/gone:latest
```

Visit http://localhost:8080, paste a secret and/or attach files, pick an expiry, copy the generated link, send it. The recipient opens the link, the secret displays once, and the server deletes it as soon as their browser confirms it received everything.

Want metrics? (optional)
```sh
docker run --rm \
	-p 8080:8080 -p 9090:9090 \
	-e GONE_METRICS_ADDR=0.0.0.0:9090 \
	-e GONE_METRICS_TOKEN=tok \
	ghcr.io/haukened/gone:latest
```

Fetch metrics snapshot:
```sh
curl -H 'Authorization: Bearer tok' http://localhost:9090/
```

---

## 2. Basic Usage
1. You type a secret and/or attach files (any type, up to 10 files; message + files share the `GONE_MAX_BYTES` cap) in the web form and choose how long it should live (its TTL).
2. Your browser encrypts it locally before it ever leaves your machine.
3. The server stores only the encrypted blob plus when it should expire.
4. You get a link like: `https://example/secret/abcd#v1:ENC_KEY_MATERIAL`.
5. You send that full URL (including everything after the `#`) to someone.
6. When they open it, the server hands their browser the encrypted blob, the browser decrypts it locally using the part after `#`, then tells the server to delete it.
7. A refresh or second visit won’t work—the secret is already gone.

Guarantees (simple terms):
* Server never learns the plaintext.
* Link works only one time.
* Expired or used links are dead.

---

## 3. Configuration
Environment variables only (no flags, no config files):

| Variable | Description | Default |
|----------|-------------|---------|
| `GONE_ADDR` | Listen address (`host:port` or `:port`). | `:8080` |
| `GONE_DATA_DIR` | Data directory (SQLite DB + blobs). | `/data` |
| `GONE_INLINE_MAX_BYTES` | Max ciphertext size stored inline in SQLite. | `8192` |
| `GONE_MAX_BYTES` | Absolute max secret size in bytes (message + attachments combined). | `10485760` (10 MiB) |
| `GONE_TTL_OPTIONS` | Comma list of selectable TTLs. | `5m,30m,1h,2h,4h,8h,24h` |
| `GONE_CLAIM_LEASE` | How long an opened secret is reserved for the recipient's browser to finish downloading and confirm deletion. If it never confirms (closed tab, network drop), the secret is deleted when the lease lapses. Max `15m`. | `2m` |
| `GONE_METRICS_ADDR` | Optional metrics listener address. | (empty) |
| `GONE_METRICS_TOKEN` | Bearer token for the metrics endpoint. Metrics stay disabled unless both this and `GONE_METRICS_ADDR` are set. | (empty) |

Derived automatically:
* MinTTL / MaxTTL = smallest / largest in `GONE_TTL_OPTIONS` (accepted range is any duration inside that span, not just the listed ones).
* SQLite DSN → `<GONE_DATA_DIR>/gone.db` (WAL mode, FULL sync enforced).

TTL Format: comma‑separated Go durations using `s`, `m`, `h` (e.g. `30s,5m,90m,2h`).

---

## 4. Metrics (Optional)
Disabled unless both `GONE_METRICS_ADDR` and `GONE_METRICS_TOKEN` are set (an address without a token logs a warning and leaves metrics off). When enabled, clients must supply `Authorization: Bearer <token>`.

JSON snapshot example:
```json
{
	"counters": {
		"secrets_created_total": 123,
		"secrets_consumed_total": 118,
		"secrets_expired_deleted_total": 47
	},
	"summaries": {
		"janitor_deleted_per_cycle": {"count": 42, "sum": 420, "min": 1, "max": 25}
	}
}
```

Definitions:
| Name | Type | Meaning |
|------|------|---------|
| `secrets_created_total` | counter | Secrets stored |
| `secrets_consumed_total` | counter | Secrets consumed & deleted |
| `secrets_expired_deleted_total` | counter | Expired secrets janitor removed |
| `secrets_claims_expired_total` | counter | Opened secrets deleted because the recipient never confirmed receipt within the claim lease |
| `janitor_deleted_per_cycle` | summary | Distribution of expirations per janitor run |

Persistence notes:
* In‑memory metrics flushed periodically to SQLite; snapshot merges persisted + current deltas.
* Graceful stop attempts a final flush.

Enable + fetch quickly:
```sh
GONE_METRICS_ADDR=127.0.0.1:9090 GONE_METRICS_TOKEN=tok \
	go run ./cmd/gone &
curl -H 'Authorization: Bearer tok' http://127.0.0.1:9090/
```

---

## 5. API Specification
OpenAPI file: `docs/openapi.yaml` (enumerates every emitted status code).
Importable into Postman / Insomnia / ReDoc.

---

>[!NOTE]
> Everything below this line is advanced detail for operators and developers.
> Read on if you're curious, but most people can stop here.

---

## 6. Build & Run (Local Dev)
This project uses [Task](https://taskfile.dev) (`Taskfile.yml`) to coordinate building the binary and (for production) minifying and embedding static assets.

Please first install Task: https://taskfile.dev/docs/installation

Core tasks:
| Task | What it does |
|------|---------------|
| `task dev` | Clean + build development binary (no minified assets, no `-tags=prod`). |
| `task prod` | Full production build: clean, minify assets into `web/dist`, build with `-tags=prod`. |
| `task run` | Convenience: rebuild dev binary and run with a temporary data dir. |
| `task cover` | Run Go tests with coverage output. |
| `task test` | Run Go and JavaScript unit tests. |
| `task test-js` | Run JavaScript unit tests (`node --test`, Node 20+, no npm install). |
| `task cover-js` | Run JavaScript unit tests with coverage output. |

Development build:
```sh
task dev
./bin/gone
```

Production build (minified assets embedded):
```sh
task prod
./bin/gone
```

Run with overrides (development example):
```sh
GONE_ADDR=127.0.0.1:8080 \
GONE_DATA_DIR=$(pwd)/data \
GONE_TTL_OPTIONS="5m,30m,1h" \
GONE_MAX_BYTES=$((10*1024*1024)) \
GONE_METRICS_ADDR=127.0.0.1:9090 \
GONE_METRICS_TOKEN=localtok \
./bin/gone
```

Or just:
```sh
task run
```

Gone is pure Go (SQLite via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)), so it builds with `CGO_ENABLED=0` into a fully static binary; `task prod` does this by default. Requires Go 1.27+.

### Container image
The `Dockerfile` uses [Docker Hardened Images](https://dhi.io): `dhi.io/golang` (builder) and the distroless `dhi.io/static` runtime (no shell or package manager; runs as UID `65532`). Both are pinned by digest and kept current by Dependabot.

Pulling DHI images needs a (free) Docker Hub account:
```sh
docker login dhi.io
docker build -t gone .
```

The release workflow publishes multi-arch (`linux/amd64`, `linux/arm64`) images with SBOM and provenance attestations, and requires the repository secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` to pull the base images.

---

## 7. Storage & Persistence
* Metadata (IDs, expiry, consumed state) → SQLite (WAL, FULL sync).
* Ciphertext: inline if ≤ `GONE_INLINE_MAX_BYTES`; otherwise filesystem blob under `blobs/` in data dir.
* Expirations and lapsed claims cleared by janitor; deletion is immediate once the recipient acknowledges receipt.

---

## 8. Security & Architecture (Deep Dive)
This section is intentionally lower in the file—most users can stop above.

### Encryption & One‑Time Retrieval (Protocol v1)
1. Browser creates random AES‑GCM key + nonce (Web Crypto API).
2. Encrypts plaintext with AAD `gone:v1`. A text‑only secret is raw UTF‑8; a secret with files is a `GONE2` envelope (magic, JSON header listing the message length and each file's name/type/size, then message bytes, then file bytes) so file names and types are encrypted too.
3. Sends ciphertext + nonce (`X-Gone-Nonce`) + version (`X-Gone-Version`). Key never leaves browser.
4. Server stores ciphertext + metadata only.
5. Response returns secret ID + expiry.
6. Share link: `https://host/secret/{id}#v1:<base64url-key>`.
7. First `GET /api/secret/{id}` atomically *claims* the secret and streams the ciphertext with a random claim token (`X-Gone-Claim`) and lease expiry (`X-Gone-Claim-Expires`). Any other GET without that token gets `404`, exactly as if the secret were gone.
8. If the download is interrupted, the browser retries the GET with `X-Gone-Claim` to receive the same ciphertext again (until the lease lapses).
9. Browser checks the byte count, decrypts locally (AES‑GCM authenticates every byte), then sends `DELETE /api/secret/{id}` with `X-Gone-Claim`; the server deletes the record and blob (`204`).
10. If no acknowledgement arrives before `GONE_CLAIM_LEASE` lapses, the janitor deletes the secret anyway. It is never served to a second party.

Properties:
* Compromise yields only ciphertext & nonces.
* Must possess both path ID and fragment key.
* Atomic claim (only the token holder can re-fetch) prevents replay; only a SHA‑256 hash of the claim token is stored.
* AES‑GCM integrity + fixed AAD protect against tamper.

### Threat Model Snapshot
Defended:
* TLS transport assumed.
* No server knowledge of keys / plaintext.
* Atomic single claim; deletion after confirmed receipt or lease expiry.
* Timely expiry deletion.

Out of Scope (current):
* Malicious browser extensions.
* Brute force ID enumeration (future: lightweight rate limiting).
* Sophisticated timing side channels.
* URL hygiene / accidental fragment leakage.
* Large scale DoS floods.

Operational Tips:
* Keep TTLs short for higher sensitivity.
* Restrict metrics listener to loopback or secured network.
* Backups should exclude transient expired blobs (or run quiescent snapshot).

### Future Hardening Ideas
* Optional separate KMS‑sealed metadata.
* Link burn confirmation UX.
* Structured audit events (without sensitive payload) to external sink.

---

## 9. Security Headers
Middleware sets:
* `Cache-Control: no-store`
* `Referrer-Policy: no-referrer`
* `X-Content-Type-Options: nosniff`
* `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`

Notes:
* CSP blocks inline code; only same‑origin static assets permitted (images allow data URIs).
* `frame-ancestors 'none'` removes need for X-Frame-Options.
* Dynamic pages: forced no‑store; static assets may be cached briefly.

---

## 10. Debug / Timing Instrumentation
Enable client timing logs either:
* Append `?debug=timing` to a page URL, or
* DevTools: `localStorage.setItem('goneDebugTiming','1')` then refresh.

Disable by removing parameter & clearing the key.

---

## 11. Roadmap (Excerpt)
* Rate limiting / abuse guard
* Optional Prometheus exposition
* CSP tightening & documentation
* Graceful shutdown coordination improvements

---

## 12. License
Copyright © 2025-2026 The Gone Project Contributors.

GNU Affero General Public License v3.0 – see `LICENSE`.
