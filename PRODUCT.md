# Gone

<!-- impeccable:product-schema 1 -->

## Platform

web (primary), plus the `gone` command-line client for terminals and scripts

## Users

Primary: people who self-host Gone for a team and use it mid-task to hand a credential (password, API token, wifi key, recovery code, small file) to a colleague or customer — usually while on a call or in chat. Recipients are mixed: some technical, many not, often on a phone, opening a link from someone else's server for the first time.

Documentation readers: the largest group is people using the `gone` CLI, so the docs landing leads with the CLI. Operators remain the primary users of the product itself.

## Product Purpose

Share a secret exactly once. The sender's browser encrypts the message and attachments; the server stores only ciphertext; the key travels in the URL fragment and never reaches the server. The first recipient to open the link decrypts it locally, the server deletes it once the browser acknowledges receipt, and every later open finds nothing. Success: the secret arrives, is read once, and leaves no copy behind in chat logs, email, or tickets.

## Positioning

Zero-knowledge, one-time, self-hostable, tiny. Gone's honesty is structural — the server cannot read what it stores — so the interface earns trust through restraint: calm, precise, nothing hidden, like a well-made instrument rather than a marketing site.

## Operating Context

- Sender: desktop browser, mid-conversation, wants a link in seconds. Pastes text and/or drops files, picks an expiry, copies the link, pastes it into chat/email.
- Recipient: any device, frequently mobile, arrives cold from a link; must understand that opening reveals and burns the secret, then copy what they need.
- Operator: self-hosts via a single container behind their own domain; configures size and TTL limits, and optionally requires a passphrase, limits attachments, brands the instance, or requires sign-in through their own proxy to create.

## Capabilities and Constraints

- Create: text secret and/or up to 10 file attachments, server-configured max size, expiry (TTL) chosen from server-configured bounds with the shortest preselected, optional passphrase (typed or five generated words).
- Result: a shareable link containing the decryption key in the fragment, plus a private manage link for the sender. An opt-in QR code of the share link for handing it over in person (send result only; never for request or reply links, since a code hides the address it opens). With a passphrase, a reminder to send it through a different channel.
- Receive: the page removes the key from the address bar once it has read it (a re-pasted link is read again); confirm-to-reveal, passphrase prompt for v2 links (retries against one download), decrypt locally, then the text stays covered until **Show** (for screen sharing) with **Copy** alongside and an "always show on this device" option; download attachments; states for expired, already opened/not found, wrong passphrase, and decryption failure. Leaving the page while a passphrase-protected download is waiting for the right passphrase warns first, since leaving loses the secret.
- Manage: the sender's manage link shows whether the secret is still waiting and can delete it before it is opened. It can never reveal the secret.
- Request: the requester makes a request link (an optional label that stays in their browser, and how long to wait). Their browser keeps a non-extractable private key in IndexedDB; whoever holds the link sends one reply encrypted to it. The requester's page lists the requests saved in that browser, checks them while it's open, and opens a reply once. A reply can only be opened in the browser that asked; there is no way to move it elsewhere.
- Operator controls: a passphrase required on every secret (enforced by the server; replies to requests exempt) and a file limit (honoured by the web UI and CLI); the operator's name, logo, accent colour and footer links, with "Powered by Gone" kept; and for private instances, sign-in through the operator's proxy to create secrets and requests, shown as a "Sign in required" page with an optional sign-in button. Opening and replying never need sign-in, and Gone never records who signed in. Every page carries `X-Robots-Tag: noindex`.
- Technical: Go html/template pages, vanilla JS with WebCrypto, strict CSP (self-only scripts/styles/fonts, no inline), no third-party assets or CDNs, no innerHTML, requires HTTPS (or localhost); on an insecure page it warns and turns off sending.
- Languages: English, Spanish, French, German and Brazilian Portuguese. The first visit follows the browser's languages; a picker in the header switches language in place, without a reload, and remembers the choice in a cookie. Non-English text is machine-translated and says so.
- Pages: send (home), result, receive, manage, request (new request and the list), request detail (waiting, open the reply, cancel), reply (answer a request), about, error.

## Brand Commitments

- The product name "Gone" and the "G" mark (`docs/brand/`, `web/img/`) are the brand, covered by [TRADEMARKS.md](TRADEMARKS.md). Icons, palette and type follow [DESIGN.md](DESIGN.md).
- An operator may show their own name, logo and accent colour in the header, and link their contact, imprint and privacy pages in the footer (`GONE_BRAND_*`, `GONE_*_URL`). Page titles, the About page, messages and the favicon still say Gone, and the footer always says "Powered by Gone".

## Evidence on Hand

- README.md and about page copy explaining the mechanism, and the brand mark and wordmark in `docs/brand/`. No illustrations, no testimonials, no usage statistics — none may be invented.

## Product Principles

1. Restraint is the trust signal: nothing decorative competes with the task or hints at hidden behavior.
2. Make the irreversible obvious: revealing and burning are one-way and must be stated plainly before they happen.
3. Seconds, not steps: the sender's path from paste to copied link stays as short as it is today or shorter. Requests live on their own page, so they add nothing to it.
4. The recipient may be non-technical and on a phone: plain language, large targets, no jargon required.
5. Self-contained by construction: nothing loads from anywhere but the operator's server.

## Accessibility & Inclusion

WCAG 2.2 AA minimum: full keyboard operation, visible focus, live-region announcements for async states (encrypting, uploading, copied, errors), reduced-motion support, light and dark themes honoring system preference, touch targets ≥ 24px (aim 44px on mobile).
