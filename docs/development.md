# Development

How to build, run and test Gone from source. The pull request process and coding rules are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Requirements

- **Go 1.27.2 or newer.** Gone is pure Go: SQLite comes from [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite). It builds with `CGO_ENABLED=0` into a fully static binary.
- **[Task](https://taskfile.dev/docs/installation)**, which runs the build steps in `Taskfile.yml`.
- **Node 20 or newer**, for the JavaScript tests and linters. The web client itself needs no npm install.
- **[minify](https://github.com/tdewolff/minify)**, for production builds only.

## Build and run

| Task | What it does |
| ---- | ------------ |
| `task dev` | Builds development binaries, `bin/goned` (server) and `bin/gone` (CLI). Assets are served from `web/` on disk, so edits show on reload, the pseudo-language `en-XA` appears in the language picker, and the `?preview=` design modes from `web/dev/` work. |
| `task prod` | Production build: minifies the assets into `web/dist`, embeds them in `bin/goned` with `-tags=prod`, and builds `bin/gone`. |
| `task run` | Rebuilds the dev binary and runs it with a temporary data directory (`.tmp`). It sets `GONE_METRICS_ADDR=:9090`, but metrics stay off, with a warning, unless you also set `GONE_METRICS_TOKEN`. |
| `task release -- vX.Y.Z` | Cross-builds the release archives, `SHA256SUMS` and release notes into `dist/`. This is what the tag workflow publishes. |

Run with your own settings:

```sh
task dev
GONE_ADDR=127.0.0.1:8080 \
GONE_DATA_DIR="$(pwd)/.tmp" \
GONE_METRICS_ADDR=127.0.0.1:9090 \
GONE_METRICS_TOKEN=local-dev-token-0123 \
./bin/goned
```

Every variable is listed in [Configuration](configuration.md). The container image is covered in [Running Gone with Docker](deploy/docker.md#building-the-image-yourself).

## Tests

| Task | What it runs |
| ---- | ------------ |
| `task test` | Go tests (`go test ./...`), then the JavaScript unit tests. |
| `task test-js` | JavaScript unit tests in `test/js/`, with `node --test` and no dependencies. |
| `task cover` / `task cover-js` | The same, with coverage. Go and JS line coverage are kept at or near 100%. |
| `task e2e` | Browser end-to-end tests in `test/e2e/`: a production build in Chromium, Firefox and WebKit with Playwright. Run `task e2e-install` once first to install the browsers. |
| `task perf` | The latency target: p95 under 50 ms at 100 requests per second, against the real stack. |
| `task lint` | golangci-lint (configured in `.golangci.yml`) for the default and `dev` build tags. |
| `task lint-web` | ESLint and stylelint through `npx`, as CI runs them. |

### What each suite covers

- **Go unit tests** sit next to the code. **Integration tests** in `test/` run the real router and store in-process.
- **Fuzz targets** in `internal/httpx` (`FuzzParseSecretHeaders`, `FuzzParseFillHeaders`, `FuzzConsumeRoute`) are seeded from `test/vectors/server_headers.json`. Plain `go test` runs the seeds; fuzz one with `go test ./internal/httpx -run '^$' -fuzz FuzzParseSecretHeaders -fuzztime 60s`.
- **The OpenAPI drift test** (`test/openapi_drift_test.go`) reads `docs/openapi.yaml` and checks the real router against it: every documented operation is routed, the statuses it returns are documented, and the `Allow` and `Retry-After` headers are. Change the spec and the code together.
- **Shared test vectors** in `test/vectors/` are checked by both the Go and JavaScript suites, so the two implementations of the encryption format can't drift apart by a byte.
- **Interop tests** (`test/cli_interop_test.go`, `test/request_interop_test.go`) exchange secrets and requests between the CLI and the browser's crypto code, run in Node through `test/interop/interop.js`. They skip when `node` is missing, unless `GONE_REQUIRE_NODE=1` is set, as it is in CI.
- **JavaScript unit tests** load each script through `test/js/harness.js` with a fake DOM.
- **End-to-end tests** start the real `bin/goned` with its templates and CSP, and drive real browsers through sending, opening, managing, requesting, replying and switching languages.
  - Each test fails on any console error, uncaught exception, CSP violation, or error status for a page or asset.
  - Pass Playwright arguments after `--`, for example `task e2e -- --project=webkit request.spec.js`.
  - `GONE_E2E_PORT` changes the port; the default is `8199`.
- **The latency test** runs only with `GONE_PERF=1`, which `task perf` sets.
  - `GONE_PERF_DURATION` changes the run length (default `10s`).
  - `GONE_PERF_RPS` offers a different rate, to find a machine's limit.
  - Set `TMPDIR` to a directory on the disk you deploy on to benchmark that disk. CI runs it on a RAM disk, so CI checks the code, not the runner's disk.

## Debugging

To log client-side timings in the browser console:
- add `?debug=timing` to a page URL; or
- run `localStorage.setItem('goneDebugTiming', '1')` in DevTools, then reload.

Remove the parameter and the key to turn it off.

## Layout

| Path | What's there |
| ---- | ------------ |
| `cmd/goned`, `cmd/gone` | The server and CLI entry points |
| `internal/` | The Go packages: `httpx` (handlers), `app` (service), `store` (SQLite and blobs), `envelope` (Go encryption), `cli`, `client`, `i18n`, `metrics`, `ratelimit`, … |
| `web/` | Templates (`*.tmpl.html`), `js/`, `css/`, `messages/` (translations), fonts and images; `dev/` holds development-only scripts (the `?preview=` design modes) that production builds never contain |
| `test/` | Integration, interop, vector, perf and end-to-end tests |
| `docs/` | This documentation, the [protocol](protocol.md) and the [OpenAPI spec](openapi.yaml) |
| `deploy/` | The Compose and Caddy setup |
