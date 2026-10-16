# Technical decisions

How Gone is built, and the alternatives we turned down: the choice, what we didn't do, why, and what would make us look again. Features we've said no to are in [rejected.md](rejected.md); this page is about the how, not the what.

Each phase of [the roadmap](ROADMAP.md) also records the decisions made while building it. This page collects the ones a contributor is most likely to question. A "Never" here means the alternative conflicts with one of Gone's guarantees or with a [non-goal](ROADMAP.md#non-goals).

## Encryption

| Decision | Instead of | Why | Revisit when |
| -------- | ---------- | --- | ------------ |
| Encryption runs on the browser's built-in WebCrypto, with a separate Go implementation for the CLI. Shared [test vectors](protocol.md#9-test-vectors) keep the two in step. | WebAssembly, for example the Go code compiled for the browser | WebAssembly would add about 2 MB to every page, need a `'wasm-unsafe-eval'` hole in the Content Security Policy, give up hardware-accelerated AES-GCM, and leave secrets in a garbage-collected heap that can't be wiped. | Never, while the strict CSP stands |
| Passphrases are stretched with PBKDF2-SHA-256 at 600,000 iterations. The format reserves IDs for other functions ([protocol.md §4.2](protocol.md#42-protocol-version-2-passphrase)). | Argon2id | Argon2id resists guessing better, but browsers only offer it through WebAssembly. Reconfirmed on 2026-10-16. | Browsers offer Argon2 natively |
| A wrong link key and a wrong passphrase give the same error. | Telling the two apart | AES-GCM can't tell them apart; both just fail to authenticate. | Never |

## Server

| Decision | Instead of | Why | Revisit when |
| -------- | ---------- | --- | ------------ |
| Nothing is kept once a secret is gone. Status answers only "pending" or `404`. | Tombstones, or an "opened" or "revoked" state | A row kept after deletion leaks that the ID existed and when it was opened. This is why there are no [read receipts](rejected.md#sending-and-receiving). | Never |
| SQLite through the pure-Go `modernc.org/sqlite`, with ciphertext over 8 KiB in files | `mattn/go-sqlite3` (CGO), or object storage | A fully static binary with `CGO_ENABLED=0`, and one file plus a folder to back up. | Instances outgrow it in practice |
| `synchronous=FULL` | Relaxing it for speed | A claim lost to a power cut would put the secret back to unclaimed, so it could be opened a second time. | Never |
| An ordered write gate. Contention answers `503 busy`. | Letting SQLite's busy handler race | Writers weren't served in order, and a timeout surfaced as a `500`. | — |
| Rate limits are per process, in memory. | A shared store, such as Redis | Gone runs as one process. Large floods belong at the edge anyway. | Gone is commonly run with several replicas |
| Configuration from environment variables only | Config files or command-line flags | One way to configure, and it suits containers. | Never |

## Web client

| Decision | Instead of | Why | Revisit when |
| -------- | ---------- | --- | ------------ |
| Vanilla JavaScript in small classic scripts, with no build step. Each script publishes one `window.gone*` namespace. | Front-end frameworks, a bundler, TypeScript or ES modules | Nothing to audit beyond what ships, and it works under a strict CSP. The repo's only npm dependency is Playwright in `test/e2e`, which is test-only and never shipped. | Never, for shipped code |
| Everything is served from the operator's own server. | Third-party scripts, fonts, analytics or CDNs | Nothing loads from anywhere else, so no outside party sees who uses an instance. | Never |
| Our own QR encoder (`web/js/qr*.js`), checked against node-qrcode and Nayuki | A QR library | No new dependency, and nothing outside `'self'` in the CSP. | — |
| Translations in inlang's message format, rendered by our own small runtime. Pages embed only the `js.*` and `common.*` messages. | Paraglide | Paraglide needs ES modules or a bundler. The catalog format keeps it possible later. | The web client moves to ES modules |
| The language switches in place, from catalogs fetched on demand. | Reloading the page | A reload would lose a link you just made or a secret you just opened. | Never |
| A custom, keyboard-accessible language menu | The browser's native `<select>` | The native list didn't fit the design. | — |

## Command line

| Decision | Instead of | Why | Revisit when |
| -------- | ---------- | --- | ------------ |
| Saved requests in a gob file, written atomically under a lock, with permissions checked like `ssh` | SQLite | SQLite grew the binary from 10.2 to 16.7 MB. The format is deliberately not something to open, edit or copy-paste. | — |
| `gone` (client) and `goned` (server) are separate binaries. | One binary with server subcommands | The client carries no server code or web assets, and the name users type is the short one. | — |

## Dependencies

The server compiles in 16 external modules. Each new one needs a written reason in its PR.

| Decision | Instead of | Why | Revisit when |
| -------- | ---------- | --- | ------------ |
| Prometheus text format written by hand | `client_golang` | It would add about 10 modules for a small text format. | — |
| Locale matching written by hand | A locale-matching library | Matching five locales takes a few dozen lines. | Gone supports many more locales |
| Plain validation code | `go-playground/validator` | It was the only reason `x/crypto` was in the server's build. Dropping it removed six modules. | — |
