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

Visit http://localhost:8080, paste a secret, pick an expiry, copy the generated link, send it. The recipient opens the link, the secret displays once, and the server deletes it immediately.

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
1. You type a secret in the web form and choose how long it should live (its TTL).
2. Your browser encrypts it locally before it ever leaves your machine.
3. The server stores only the encrypted blob plus when it should expire.
4. You get a link like: `https://example/secret/abcd#v1:ENC_KEY_MATERIAL`.
5. You send that full URL (including everything after the `#`) to someone.
6. When they open it, the server gives their browser the encrypted blob once, deletes it, and the browser decrypts it locally using the part after `#`.
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
| `GONE_MAX_BYTES` | Absolute max secret size (bytes). | `1048576` |
| `GONE_TTL_OPTIONS` | Comma list of selectable TTLs. | `5m,30m,1h,2h,4h,8h,24h` |
| `GONE_METRICS_ADDR` | Optional metrics listener address. | (empty) |
| `GONE_METRICS_TOKEN` | Optional bearer token required for metrics. | (empty) |

Derived automatically:
* MinTTL / MaxTTL = smallest / largest in `GONE_TTL_OPTIONS` (accepted range is any duration inside that span, not just the listed ones).
* SQLite DSN → `<GONE_DATA_DIR>/gone.db` (WAL mode, FULL sync enforced).

TTL Format: comma‑separated Go durations using `s`, `m`, `h` (e.g. `30s,5m,90m,2h`).

---

## 4. Metrics (Optional)
Disabled unless `GONE_METRICS_ADDR` is set. If `GONE_METRICS_TOKEN` is non‑empty you must supply `Authorization: Bearer <token>`.

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

>[! NOTE]
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
| `task cover` | Run tests with coverage output. |

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
GONE_MAX_BYTES=$((1024*1024)) \
GONE_METRICS_ADDR=127.0.0.1:9090 \
GONE_METRICS_TOKEN=localtok \
./bin/gone
```

Or just:
```sh
task run
```

---

## 7. Storage & Persistence
* Metadata (IDs, expiry, consumed state) → SQLite (WAL, FULL sync).
* Ciphertext: inline if ≤ `GONE_INLINE_MAX_BYTES`; otherwise filesystem blob under `blobs/` in data dir.
* Expirations cleared by janitor + immediate deletion on consume.

---

## 8. Security & Architecture (Deep Dive)
This section is intentionally lower in the file—most users can stop above.

### Encryption & One‑Time Retrieval (Protocol v1)
1. Browser creates random AES‑GCM key + nonce (Web Crypto API).
2. Encrypts plaintext with AAD `gone:v1`.
3. Sends ciphertext + nonce (`X-Gone-Nonce`) + version (`X-Gone-Version`). Key never leaves browser.
4. Server stores ciphertext + metadata only.
5. Response returns secret ID + expiry.
6. Share link: `https://host/secret/{id}#v1:<base64url-key>`.
7. First GET streams ciphertext and deletes record atomically.
8. Browser decrypts locally; reload fails (already deleted).

Properties:
* Compromise yields only ciphertext & nonces.
* Must possess both path ID and fragment key.
* Atomic consume prevents replay.
* AES‑GCM integrity + fixed AAD protect against tamper.

### Threat Model Snapshot
Defended:
* TLS transport assumed.
* No server knowledge of keys / plaintext.
* Atomic single consumption.
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
GNU Affero General Public License v3.0 – see `LICENSE`.