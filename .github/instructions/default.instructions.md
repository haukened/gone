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