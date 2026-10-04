---
description: Default behavior for this repository
applyTo: '**'
---
# GitHub Copilot Instructions for Gone

## CRITICAL: Always follow these instructions when contributing to this repository.
- Don't run codacy_cli_analyze on markdown files. Codacy does not support markdown files.

## Project Purpose

Gone is a minimal Go service for one-time secret sharing. Its goal is to provide a secure, simple, and efficient way to share secrets that can only be accessed once.

## You Must always:
- After generating code, review it carefully for security and correctness.
- All functions must have comprehensive GODoc comments, including parameters and return values.
- Write unit tests for any new functionality. (See Testing section below.)
- When tasks are complete, run:
  - `go fmt ./...` to format the code.
  - `task test` to run the Go and JavaScript tests.
  - `task lint` (golangci-lint: vet, staticcheck, revive, errcheck and more) and `task lint-web` (ESLint and stylelint). CI runs all three.
  - gosec, which golangci-lint does not run here: `go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 ./...` (the same scan CI runs weekly in `sec.yaml`).

## Project Structure
The project follows a minimal Go layout to keep code organized and maintainable:

- `cmd/goned/`: The server entry point (`goned`): wiring, storage setup, templates, readiness.
- `cmd/gone/`: The command-line client entry point (`gone`).
- `internal/app/`: The secret service: create, claim, acknowledge, status, revoke.
- `internal/domain/`: Protocol constants and validation shared by server and client (versions, nonces, TTLs, IDs).
- `internal/envelope/`: Go implementation of the encryption format, GONE2 envelope, and link fragments (see `docs/protocol.md`).
- `internal/store/`: Secret storage: SQLite index (`store/sqlite`, with migrations) and filesystem blobs (`store/filesystem`).
- `internal/httpx/`: HTTP handlers, middleware, and security headers.
- `internal/ratelimit/`: Per-client token-bucket rate limiting.
- `internal/janitor/`: Background deletion of expired secrets and lapsed claims.
- `internal/metrics/`: In-memory counters, persisted to SQLite and served on the optional metrics listener.
- `internal/config/`: Environment variable parsing and validation.
- `internal/cli/`, `internal/client/`, `internal/passgen/`: CLI commands, its HTTP API client, and the passphrase wordlist generator.
- `internal/sqlrows/`: Small SQL row helpers.
- `web/`: Templates, CSS (`web/css/`), and vanilla JS (`web/js/`) for the web interface.
- `test/`: Go integration tests; `test/js/` JS unit tests; `test/interop/` browser-crypto/CLI interop; `test/vectors/` shared test vectors.
- `docs/`: API (`README.md`, `openapi.yaml`), protocol, CLI guide, and roadmap.
- `scripts/`: Release tooling (`release.sh`).

## Testing Requirements
- Use Go's standard `testing` package.
- Write unit tests for all new functions and methods.
- Unit tests should be placed in a separate `_test.go` file.
  - Example: `store.go` -> `store_test.go` in the same folder.
- Place tests in the same package as the code being tested.
- Use table-driven tests where appropriate.
- Integration tests should be placed in the `test/` directory.
- Run tests with `go test ./...` and ensure all tests pass before finalizing changes.

## Coding Style
- Write idiomatic Go code following standard Go conventions.
- Use minimal dependencies to keep the project lightweight and maintainable.
- Prioritize security in all code, especially around secret handling and storage.

## Focus Areas for Copilot
- Safe file I/O operations: ensure files are handled securely and errors are properly managed.
- SQLite transactions: use transactions correctly to maintain data integrity.
- Secure HTTP headers: always include appropriate security headers in HTTP responses.
- Minimal use of `net/http`: keep HTTP handling simple and straightforward without adding unnecessary complexity.

## Important Notes
- Avoid introducing frameworks or heavy dependencies.
- Do not add unnecessary complexity; keep the codebase minimal and easy to understand.
- Security and simplicity are paramount in all suggestions.

## Tech Stack
- Go 1.27+
- net/http
- SQLite (WAL mode) via pure-Go `modernc.org/sqlite` (no CGO; builds with `CGO_ENABLED=0`)
- Filesystem blobs for secret storage
- Vanilla JavaScript with WebCrypto API for client-side cryptography

## Build & Run
- Build the server with: `go build ./cmd/goned`, run with `./goned`
- Build the CLI with: `go build ./cmd/gone`
- `task dev`, `task prod`, and `task run` wrap the common builds (see the README task table).
- Configuration is environment variables only. The full list with defaults and limits is the configuration table in `README.md` §3. Main ones: `GONE_ADDR`, `GONE_DATA_DIR` (the SQLite database `gone.db` and blobs live here), `GONE_MAX_BYTES`, `GONE_TTL_OPTIONS` (min/max TTL are derived from it), `GONE_CLAIM_LEASE`, `GONE_RATE_*`, `GONE_TRUSTED_PROXIES`, and the metrics pair `GONE_METRICS_ADDR` / `GONE_METRICS_TOKEN`. The CLI reads `GONE_SERVER`.

## Testing
- Use Go's standard `testing` package
- Run tests with: `task test` (Go and JS) or `go test ./...`
- Integration tests located in `test/` directory
- Protocol changes must update `test/vectors/`; both Go and JS suites check them.

## Linting & Formatting
- Use `go fmt` for formatting
- `task lint` runs golangci-lint with `.golangci.yml` (vet, staticcheck, revive, errcheck and more)
- `task lint-web` runs ESLint and stylelint

## Deployment
- Containerized deployment preferred
- Run container as non-root user (UID 65532)
- Use multi-stage Docker builds for minimal image size
- Base images are Docker Hardened Images (`dhi.io/golang` builder, distroless `dhi.io/static` runtime), pinned by digest
- Mount persistent storage at `/data` (the image's `VOLUME` and the `GONE_DATA_DIR` default)
- Expose health endpoints at `/healthz` and `/readyz`

## Code Ownership & Contribution
- Trunk-based development model
- Feature branches with Pull Request reviews
- Merge PRs with merge commits (the GitHub default)
- Follow conventional commits for commit messages

## Performance Goals
- 95th percentile latency under 50ms at 100 requests per second
- Use SQLite in WAL mode for concurrency
- Ensure fsync on data directory to guarantee durability

## Observability
- Structured logs with `log/slog` (default text handler, key=value pairs)
- Do not log request bodies, client addresses, keys, or tokens
- Metrics counters (see the README metrics table): created, consumed, revoked, expired/deleted by the janitor, lapsed claims, and rate-limited requests; plus a janitor deletions-per-cycle summary

## Domain Logic
- HTTP API endpoints (full reference: `docs/README.md` and `docs/openapi.yaml`):
  - `POST /api/secret` creates a secret and returns its ID, expiry, and manage token
  - `GET /api/secret/{id}` claims a secret (returns ciphertext and a claim token); repeating it with `X-Gone-Claim` re-fetches during the lease
  - `DELETE /api/secret/{id}` with `X-Gone-Claim` acknowledges receipt and deletes it
  - `GET /api/secret/{id}/status` and `POST /api/secret/{id}/revoke` with `X-Gone-Manage` serve the sender
- The `secrets` table (`internal/store/sqlite/migrate.go`) has `id`, `version`, `nonce_b64u`, `inline` (ciphertext when small) or `external` (blob flag), `size`, `created_at`, `expires_at`, and, added by migrations, `claim_hash`, `claimed_until`, and `manage_hash`. Only hashes of claim and manage tokens are stored.
- Schema changes go through the `PRAGMA user_version` migration runner, one step per version.

## Style & Structure
- Follow the described module layout strictly
- Use error wrapping with `%w` for error propagation
- Define sentinel error `ErrNotFound` for missing secrets
- Avoid panics in HTTP handlers; handle errors gracefully
