---
    description: Default behavior for this repository
    applyTo: '**'
---
---
# GitHub Copilot Instructions for Gone

## You Must always:
- After generating code, review it carefully for security and correctness.
- Write unit tests for any new functionality.
- When tasks are complete, run:
  - `go fmt ./...` to format the code.
  - `go vet ./...` to check for issues.
  - `go test ./...` to run all tests and ensure everything passes.
  - `gosec ./...` to check for security vulnerabilities.

## Project Purpose

Gone is a minimal Go service for one-time secret sharing. Its goal is to provide a secure, simple, and efficient way to share secrets that can only be accessed once.

## Project Structure
The project follows a minimal Go layout to keep code organized and maintainable:

- `cmd/gone/`: The main entry point for the binary.
- `internal/store/`: Contains code for secret storage and database interactions.
- `internal/httpx/`: Contains HTTP handlers and related logic.
- `web/`: Static assets (HTML, CSS, JS) for the web interface.
- `scripts/`: Janitor jobs and maintenance scripts.
- `test/`: Integration and unit tests.

**Copilot must always place new code in the correct directory according to its purpose, and keep the structure minimal. Do not introduce unnecessary subdirectories or complexity.**

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
- Go 1.22+
- net/http
- SQLite (WAL mode)
- Filesystem blobs for secret storage
- Vanilla JavaScript with WebCrypto API for client-side cryptography

## Build & Run
- Build with: `go build ./cmd/gone`
- Run with: `./gone`
- Optional environment variables:
  - `GONE_ADDR`: network address to bind (default `:8080`)
  - `GONE_DATA_DIR`: directory for filesystem blobs
  - `GONE_DSN`: SQLite data source name
  - `GONE_MAX_BYTES`: maximum secret size in bytes
  - `GONE_MIN_TTL`: minimum time-to-live for secrets
  - `GONE_MAX_TTL`: maximum time-to-live for secrets

## Testing
- Use Go's standard `testing` package
- Run tests with: `go test ./...`
- Integration tests located in `test/` directory

## Linting & Formatting
- Use `go fmt` for formatting
- Use `go vet` for static analysis
- Use `gosec` for security scanning
- Optionally, use `staticcheck` for additional linting

## Deployment
- Containerized deployment preferred
- Run container as non-root user
- Use multi-stage Docker builds for minimal image size
- Mount persistent storage at `/var/lib/gone`
- Expose health endpoints at `/healthz` and `/readyz`

## Code Ownership & Contribution
- Trunk-based development model
- Feature branches with Pull Request reviews
- Use squash merges
- Follow conventional commits for commit messages

## Performance Goals
- 95th percentile latency under 50ms at 100 requests per second
- Use SQLite in WAL mode for concurrency
- Ensure fsync on data directory to guarantee durability

## Observability
- Structured JSON logs using `slog`
- Do not log request bodies to protect secrets
- Metrics counters for:
  - Secrets created
  - Secrets consumed
  - Secrets expired
  - Orphaned secrets cleaned by janitor
- Janitor job timing metrics

## Domain Logic
- HTTP API endpoints:
  - `POST /api/secret` to create a secret
  - `GET /api/secret/{id}` to retrieve and consume a secret
- Database schema includes a `secrets` table with fields:
  - `id` (primary key)
  - `data` (blob or reference to filesystem blob)
  - `created_at`
  - `expires_at`
  - `consumed_at` nullable

## Style & Structure
- Follow the described module layout strictly
- Use error wrapping with `%w` for error propagation
- Define sentinel error `ErrNotFound` for missing secrets
- Avoid panics in HTTP handlers; handle errors gracefully
---