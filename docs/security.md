# Security and architecture

Gone makes two promises:
- the server never sees a secret's plaintext or key;
- a link opens a secret at most once.

This page explains how the design keeps them, what is and isn't defended, and what the server stores. The normative byte-level specification, with the envelope format, sanitization rules and shared test vectors, is [protocol.md](protocol.md). To report a vulnerability, see [SECURITY.md](../SECURITY.md).

## Sending a secret (protocol v1)

1. The browser makes a random AES-256-GCM key and nonce with WebCrypto.
2. It encrypts the plaintext with the additional data `gone:v1`.
   - A text-only secret is raw UTF-8.
   - A secret with files is a `GONE2` envelope: magic bytes, a JSON header listing the message length and each file's name, type and size, then the message bytes, then the file bytes. File names and types are therefore encrypted too.
3. It uploads the ciphertext with the nonce (`X-Gone-Nonce`) and version (`X-Gone-Version`). The key never leaves the browser.
4. The server stores the ciphertext and its expiry. It returns the secret's ID, the expiry and a random manage token, and keeps only the token's SHA-256 hash.
5. The sender gets two links:
   - **Share link:** `https://host/secret/{id}#v1:<base64url key>`.
   - **Manage link** (for the sender only): `https://host/manage/{id}#<manage token>`.

   Everything after `#` is a URL fragment, which browsers never send to a server.

## Opening it

1. The first `GET /api/secret/{id}` atomically **claims** the secret. It streams the ciphertext with a random claim token (`X-Gone-Claim`) and a lease expiry (`X-Gone-Claim-Expires`).
2. Any other `GET` without that token gets `404`, exactly as if the secret were gone. If the download is interrupted, the browser retries with the token and receives the same bytes, until the lease lapses.
3. The browser checks the byte count and decrypts locally. AES-GCM authenticates every byte.
4. The browser then sends `DELETE /api/secret/{id}` with the claim token. The server deletes the row and any blob, and answers `204`.
5. If no confirmation arrives before `GONE_CLAIM_LEASE` lapses, the janitor deletes the secret anyway. It is never served to a second party.

The sender's manage page sends the manage token in an `X-Gone-Manage` header:
- `GET /api/secret/{id}/status` asks whether it's still waiting;
- `POST /api/secret/{id}/revoke` deletes it now.

Properties:
- A server compromise yields only ciphertext and nonces.
- Opening a secret needs both the ID in the path and the key in the fragment.
- Only the claim's holder can fetch it again, which prevents replay. The server stores only a SHA-256 hash of the claim token.
- AES-GCM and the fixed additional data detect tampering.
- **Manage tokens:**
  - They are 256 random bits.
  - They travel only in the URL fragment and the `X-Gone-Manage` header, never in a path, query or log.
  - The server stores only a SHA-256 hash and compares it in constant time.
  - The custom header forces a CORS preflight that the server never grants, so other sites can't revoke your secrets.
- **The fragment doesn't stay in the address bar.** Once the secret, manage and reply pages have read it, they replace the URL with the same path and no fragment (`history.replaceState`), so the link key, manage token and fill token don't linger in the address bar, tab history or session restore. The page keeps its copy in memory, and a reload shows the "link is incomplete" page instead of claiming anything. Pasting the full link into the same tab afterwards sets the page up again (the pages listen for `hashchange` while the link is unusable), without starting a second claim. Copy a link from the result page, not from the address bar.
- **Leaving a page removes the secret from it.** When a page with a revealed secret is hidden or unloaded (`pagehide`, which also fires when the browser puts it in the back-forward cache), the page zeroes its plaintext, revokes the file URLs, empties the message and file list and shows the "gone" view, so a page restored from the cache holds nothing. A secret with downloaded files not yet saved also asks the browser to confirm before leaving.
- **No tombstones.** Status and revoke answer `404` alike for opened, revoked, expired, unknown and wrong-token secrets. The server's answers reveal nothing beyond "still waiting".

## Optional passphrase (protocol v2)

A sender can add a passphrase. The link keeps the same shape with a `v2:` prefix, but the AES-GCM key also depends on the passphrase. See [protocol.md §4.2](protocol.md#42-protocol-version-2-passphrase).

1. The browser picks a random 32-byte link key, a 16-byte salt and a 12-byte nonce.
2. It stretches the passphrase with PBKDF2-SHA-256 at 600,000 iterations. The passphrase is first normalized to NFC and encoded as UTF-8.
3. HKDF-SHA-256 combines the stretched passphrase with the link key to make the AES-GCM key.
4. The upload is a 21-byte header (KDF ID, iteration count, salt) followed by the ciphertext. The header is bound into the additional data (`gone:v2` plus the header), so it can't be altered. The server stores the body as opaque bytes and never sees the passphrase.
5. The recipient's page asks for the passphrase before fetching anything. A wrong passphrase can be retried against the one download as often as needed, with no second request. The secret is confirmed, and so deleted, only after it decrypts. While the page holds that download, which is already claimed and can't be fetched again, leaving it (closing the tab, reloading, following a link) makes the browser ask first (`beforeunload`); if the person leaves anyway, the held bytes are zeroed.

Generated passphrases are five words from the [EFF short wordlist](https://www.eff.org/dice), about 51 bits of entropy. The list is © Electronic Frontier Foundation, used under [CC BY 3.0 US](https://creativecommons.org/licenses/by/3.0/us/).

## Requesting a secret (protocol v3)

A request runs the other way. See [protocol.md §4.3](protocol.md#43-protocol-version-3-request-reply) and [§8.6](protocol.md#86-secret-requests).

1. The requester's browser makes an ECDH P-256 key pair. It stores the private key in IndexedDB as a non-extractable WebCrypto key, which survives closing the browser but whose bytes no page script can read. The label the requester types also stays in that browser.
2. The server gets only a TTL. It returns a request ID plus two random tokens: a manage token, which the requester keeps, and a fill token. It stores only their SHA-256 hashes.
3. The reply link is `/reply/{id}#v3:<public key>.<fill token>`. The person holding the secret encrypts to that public key with a fresh ephemeral key (ECDH, then HKDF-SHA-256, then AES-256-GCM) and uploads one reply.
4. The requester's page checks status without writing anything. It then claims the reply with the manage token, decrypts it locally and confirms it, exactly like opening a secret. The browser then forgets the key.

## Threat model

**Defended**
- **Transport:** TLS is required. Browsers only allow WebCrypto on secure pages, so Gone refuses to send or open over plain HTTP, except on `localhost`.
- **The server never sees keys or plaintext.**
- **One open:** an atomic single claim, then deletion after confirmed receipt or when the lease lapses.
- **Sender revoke** of a waiting secret, authorized by a token only the sender holds.
- **Request replies:** a reply can't be read by anyone but the requester. It is encrypted to a key that never leaves the requester's browser, and only the requester's manage token can claim it, so the request ID alone can't burn it.
- **A leaked link,** when the sender added a strong passphrase and the link leaks before the recipient opens it.
- **Expiry:** secrets are deleted on time.
- **Shoulder-surfing and screen sharing:** an opened message stays covered until the recipient presses **Show**, and it isn't in the page at all while covered.
- **Metrics:** they live on a separate listener behind a bearer token (16 or more characters recommended; shorter ones warn now and will be refused in a future release), compared in constant time (as digests), and never include IDs or ciphertext.
- **Logs:** client addresses, IDs, tokens and plaintext are never logged. This includes the web server's own messages (handler panics, TLS and accept errors), which are rewritten to drop any IP:port before they are logged at warn level.
- **Cheap unauthenticated routes:** `/readyz` caches its result for 2 seconds and only opens the blob directory, and `Accept-Language` is parsed only up to 16 entries, so neither can be used to make the server do much work per request.
- **Reply rows are not secrets:** a request's reply can only be claimed, acknowledged, inspected or cancelled through the request routes with the request's own tokens. The secret routes answer `404` for it.

**Partially defended**
- **ID guessing:** IDs are 128-bit random, and reads are rate limited per client.
- **Floods:** per-client create and read budgets blunt single-source floods. Distributed floods need protection upstream.
- **Offline passphrase guessing:** anyone with the link can download the ciphertext once and guess the passphrase offline. PBKDF2 slows each guess, but only a strong passphrase, such as the generated five words, keeps a leaked secret safe.

**Out of scope**
- Malicious browser extensions, and anything else with control of the sender's or recipient's browser.
- Sophisticated timing side channels.
- **Burning a secret:** a passphrase can't stop someone with the link from opening it, which uses it up even if they never learn the passphrase. The real recipient then finds it gone, which is a sign the link leaked.
- **Link hygiene:** where links get pasted, and fragment leaks. This includes the manage link: anyone holding it can revoke that secret, though never read it.
- **Who answers a request:** anyone with the reply link can send the one reply, so send it only to the person who has the secret. The person replying should check the link came from who they expect, since anyone can make one.
- **The requester's browser profile:** anyone with access to it can open replies to that browser's requests. Losing its storage (clearing site data, a private window) loses the request, and the reply can't be opened anywhere else. The CLI's `requests.gob` file has the same property; see [the CLI guide](cli.md).
- **Recalling delivered bytes:** revoke wins over an unconfirmed claim, but it can't recall ciphertext already downloaded.
- **Large-scale distributed denial of service.**

**Operator policies** ([configuration.md](configuration.md#policies)): `GONE_REQUIRE_PASSPHRASE` is enforced by the server, which refuses any secret without a passphrase. It can't check the passphrase's strength, only that one was used. Replies to requests are exempt: a reply link holds no key that can open a reply, so the leaked-link risk a passphrase guards against doesn't apply ([why](decisions.md#server)). `GONE_MAX_FILES` is not a security control: the file count is inside the ciphertext, so only the web page and the CLI honour it, and a custom client can ignore it. `GONE_MAX_BYTES` is the limit the server enforces.

**Operator branding** ([branding.md](branding.md)): the logo is read once at startup, checked to be a PNG or an SVG, and served from memory at `/brand/logo`. An SVG gets its own policy, `default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox`, so it can't run scripts or load anything even when opened directly; the pages show it with `<img>`, where SVG scripts never run. The accent arrives as a same-origin stylesheet, so the page CSP is unchanged. The operator's files and links are trusted like the rest of the configuration: footer links must be `https://` (or `mailto:` for contact), and the name is escaped like any other text.

**QR codes** are offered only for share links, never for request or reply links. A QR code hides the address it opens, so it's easy to swap for a phishing link. That matters most for a link that asks someone to type a secret into it.

**Operating tips**
- Keep expiries short for the most sensitive secrets.
- Keep the metrics listener on loopback or a private network.
- Back up the data directory while Gone is stopped, or accept that a live copy may hold secrets that expire soon after. Everything in it is ciphertext.

## What the server stores

Everything lives in `GONE_DATA_DIR`:
- **SQLite database** (`gone.db`), in WAL mode with `synchronous=FULL`:
  - `secrets`: ID, protocol version, nonce, size, created and expiry times, inline ciphertext (or a flag that it's in a blob), the claim token's hash and lease expiry, the manage token's hash, and whether the row is a request reply.
  - `requests`: ID, the fill and manage tokens' hashes, TTL, and created and expiry times.
  - `metrics_counters` and `metrics_summaries`: running totals only, with no IDs.
- **Blobs:** ciphertext larger than `GONE_INLINE_MAX_BYTES` is stored as a file under `blobs/`.

Rows and blobs are deleted on confirmed receipt, revoke, cancel or expiry. An expired secret or reply is deleted, blob included, as soon as anyone asks for it, and otherwise by the janitor within a minute. Nothing about a secret is kept after that.

## Releases

Release binaries and the `ghcr.io/haukened/gone` image are built only from a semver tag (`vMAJOR.MINOR.PATCH`, optionally with a `-prerelease` suffix) whose commit is already on `main`. Any other tag fails the release workflows before anything is built. The full Go test suite, including the browser-crypto interop tests, runs and must pass before the build, in a job with a read-only token. Release builds use no shared CI cache, and only a stable tag moves `:latest`. Each image and archive carries GitHub-signed SLSA build provenance. Before running an image, check that this repository's release workflow built it from that tag: `gh attestation verify oci://ghcr.io/haukened/gone:vX.Y.Z --repo haukened/gone --signer-workflow haukened/gone/.github/workflows/release.yaml --source-ref refs/tags/vX.Y.Z`. Then deploy by digest, or by the `vX.Y.Z` tag, rather than following `:latest`.

## Development-only features

The pages have design-review preview modes (`?preview=secret`, `?preview=result`, `?preview=pending` and the like) that render a sample state with no request. They would let anyone craft a link that shows attacker-chosen text inside the real interface, so they exist only in development builds (`task dev`, build tag `dev`). They live in `web/dev/`, which `task minify` never copies and the production build (`task prod`, the container image) never embeds. The page scripts read no `preview` parameter themselves. A release's `web/dist` and binary contain no preview code, and a Go test on the embedded assets and an end-to-end test check it.

## Security headers

Every response gets:
- `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`
- `Referrer-Policy: no-referrer`
- `X-Content-Type-Options: nosniff`
- `X-Robots-Tag: noindex`

A Go test (`internal/httpx/csp_doc_test.go`) reads the Content-Security-Policy line above and fails if it differs from the header the server sets.

`X-Robots-Tag: noindex` keeps every page out of search engines, so a secret, manage or reply link that leaks onto a public page is never listed in search results. It covers the home page too: an instance isn't found through search, only through the links people share.

The CSP blocks all inline script and anything from another origin. `frame-ancestors 'none'` replaces `X-Frame-Options`. Nothing loads from anywhere but the operator's server: no CDNs, fonts or analytics.

Caching:
- Pages and API responses send `Cache-Control: no-store`.
- Static assets carry a content hash in their URL (`?v=`) and are cached for a year as `immutable`. Without the hash, they're cached for 5 minutes.
