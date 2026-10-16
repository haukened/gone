# Rejected features

Features and approaches we have considered and decided against, with the reason and what would make us look again. It exists so the same idea doesn't get re-argued from scratch, and so a "no" comes with a reason rather than silence.

A "no" here isn't forever unless it says **Never**. If a revisit trigger has happened, or you have a reason the entry doesn't address, open an issue that names the entry and what changed. Design choices inside features that did ship are explained where they live: [ROADMAP.md](ROADMAP.md) (per-phase decisions), [protocol.md](protocol.md) and [cli.md](cli.md).

**Never** means the feature conflicts with Gone's core promises:
- the server never sees a secret's plaintext or key;
- a link opens a secret at most once;
- nothing is kept after a secret is gone;
- nothing loads from anywhere but the operator's server.

---

## Scope

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| Accounts, login, teams, admin dashboards | v3 roadmap, 2026-10-02 | A record of who sent what contradicts "leaves no copy behind". Anyone can send without signing up. | Never |
| Webhooks, email or push notifications (including "tell me when it's read") | v3 roadmap, 2026-10-02 | They need stored contact details and outbound delivery. The manage page answers "was it opened?" on demand instead. | Never |
| Secrets that open more than once, or can be edited or listed | 2025-09 | The claim-and-confirm design depends on a single open; it is the product. | Never |
| Server-side encryption | 2025-09 | The server would hold the key. | Never |
| Audit trail or audit events | 2026-10-05 | A log of secret activity, even without payloads, is a record that a secret existed after it's gone. Operators get aggregate counts from [metrics](metrics.md). | Never |
| Paid tier or gated features | 2026-10-05 | Security shouldn't live behind paywalls. | Never |
| Markdown, syntax highlighting or rich text | 2026-10-05 | Pastebin territory, and more code rendering untrusted input. PrivateBin covers it. | Never |
| Compliance paperwork (SOC 2 and similar) | 2026-10-05 | A self-hosted, single-maintainer project can't certify an operator's deployment. | An organization sponsors it |
| Instance security page (SBOM, live CVE lookup, a "still secure?" badge) | 2026-10-15 | A page a server serves about itself proves nothing: a compromised instance would show green. Without a visible verdict it duplicates Dependabot and CI, and operators already update. | Dishonest instances become a real concern; then a `gone verify <url>` CLI check against the signed release, not a page |

## Encryption

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| WebAssembly in the browser (for example Go compiled to WASM) | v3 roadmap, 2026-10-02 | About 2 MB per page load, a `'wasm-unsafe-eval'` hole in the CSP, no hardware-accelerated AES-GCM, and secrets left in a garbage-collected heap that can't be wiped. A JS and a Go implementation checked against [shared test vectors](protocol.md#9-test-vectors) is safer and lighter. | Never, while the strict CSP stands |
| Argon2id for passphrases | v3.0.0 (PBKDF2 chosen); confirmed 2026-10-16 | Argon2id resists guessing better, but browsers only offer it through WASM. PBKDF2-SHA-256 at 600,000 iterations is built into WebCrypto. The format reserves KDF IDs, so a later one can be added within v2 ([protocol.md §4.2](protocol.md#42-protocol-version-2-passphrase)). | WebCrypto ships Argon2 natively |
| A limit on passphrase attempts | v3.0.0 | Anyone holding the downloaded ciphertext can guess offline anyway, so a client-side cap adds friction without adding security. | Never |
| Telling "wrong link key" apart from "wrong passphrase" | v3.0.0 | AES-GCM can't tell them apart; both just fail to authenticate. | Never |

## Server

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| Tombstones, or an "opened"/"revoked" status | v3.0.0 | Keeping a row after a secret is gone leaks that the ID existed and when it was opened. Status answers only "still waiting" or `404`. | Never |
| Rate limits shared across replicas | v3.0.0 | Limits are per process, in memory. A shared store is a new dependency for a deployment shape nobody runs yet. Edge protection is the answer to large floods anyway. | Gone is commonly run with several replicas |
| Object storage (S3) or database replicas | 2026-10-05 | One SQLite file plus a blob directory fits the load by a wide margin and keeps deletion simple and certain. | Sustained load, or `503 busy` responses in practice |
| Config files or command-line flags for the server | v1 | Environment variables only: one way to configure, and it suits containers. | Never |

## Web client

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| Front-end frameworks, a bundler or a JS build step | v3 roadmap, 2026-10-02 | The UI stays small vanilla scripts under a strict CSP, with nothing to audit beyond what ships. The only npm dependency in the repo is Playwright in `test/e2e`, which is test-only and never shipped. | Never |
| TypeScript | v3 | Same reason: it needs a build step. JSDoc covers the types that matter. | Never |
| Third-party scripts, fonts or CDNs | v1 | Nothing loads from anywhere but the operator's server. | Never |
| QR codes for recipient, request or reply links | v3.8.0 | A QR code hides the address it opens, so it's easy to swap for a phishing link. That matters most for a link that asks someone to type a secret into it. The QR code is offered only for the share link, and only on request. | Never |
| Inverted (light-on-dark) QR codes | v3.8.0 | Many scanners can't read them. The code is always black on white, even in dark mode. | Never |
| A native `<select>` language picker | v3.10.0 | It doesn't match the design. The picker is a custom, keyboard-accessible menu. | Never |
| Reloading the page to change language | v3.10.0 | A reload would lose a link you just made or a secret you just opened. The language switches in place. | Never |
| Paraglide for translations | v3.10.0 | It needs ES modules or a bundler. The catalogs use inlang's message format, so inlang tools work and Paraglide stays possible. | The web client moves to ES modules |

## Command line

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| `gone mcp` (an MCP server) | 2026-10-16 | Coding assistants have a shell and already use the CLI, which handles secrets by reference (`--message-file`, `--message-out`) so the plaintext stays out of their context. An MCP server would mostly serve chat apps, where users paste secrets into the conversation anyway. | Agent clients without a shell become common |
| The message as a command-line argument | v3.0.0 | It would land in shell history and the process list. Messages come from standard input or `--message-file`. | Never |
| `--force` to overwrite files | v3.0.0 | `gone get` never overwrites; it picks a free name, which is safer and just as scriptable. | Never |
| `--mode` for `--message-out` | v3.6.0 | Written files are always `0600`, like attachments. | Never |
| SQLite for the CLI's saved requests | v3.7.0 | It grew the binary from 10.2 to 16.7 MB. A gob file, written atomically under a lock, is enough. | Never |

## Dependencies

| Feature | Decided | Why | Revisit when |
| ------- | ------- | --- | ------------ |
| The Prometheus Go client (`client_golang`) | v3.9.0 | It would add about 10 modules to a server that compiles in 16. The text format is small enough to write by hand. | Never |
| A QR code library | v3.8.0 | No new dependency, and nothing outside `'self'` in the CSP. Gone's own encoder is checked against two reference implementations. | Never |
| A locale-matching library | v3.10.0 | Matching five locales takes a few dozen lines; a library would be new modules. | Gone supports many more locales |
