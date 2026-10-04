# Gone

<!-- impeccable:product-schema 1 -->

## Platform

web (primary), plus the `gone` command-line client for terminals and scripts

## Users

Primary: people who self-host Gone for a team and use it mid-task to hand a credential (password, API token, wifi key, recovery code, small file) to a colleague or customer — usually while on a call or in chat. Recipients are mixed: some technical, many not, often on a phone, opening a link from someone else's server for the first time.

## Product Purpose

Share a secret exactly once. The sender's browser encrypts the message and attachments; the server stores only ciphertext; the key travels in the URL fragment and never reaches the server. The first recipient to open the link decrypts it locally, the server deletes it once the browser acknowledges receipt, and every later open finds nothing. Success: the secret arrives, is read once, and leaves no copy behind in chat logs, email, or tickets.

## Positioning

Zero-knowledge, one-time, self-hostable, tiny. Gone's honesty is structural — the server cannot read what it stores — so the interface earns trust through restraint: calm, precise, nothing hidden, like a well-made instrument rather than a marketing site.

## Operating Context

- Sender: desktop browser, mid-conversation, wants a link in seconds. Pastes text and/or drops files, picks an expiry, copies the link, pastes it into chat/email.
- Recipient: any device, frequently mobile, arrives cold from a link; must understand that opening reveals and burns the secret, then copy what they need.
- Operator: self-hosts via a single container behind their own domain; configures size and TTL limits.

## Capabilities and Constraints

- Create: text secret and/or up to 10 file attachments, server-configured max size, expiry (TTL) chosen from server-configured bounds, optional passphrase (typed or five generated words).
- Result: a shareable link containing the decryption key in the fragment, plus a private manage link for the sender. With a passphrase, a reminder to send it through a different channel.
- Receive: confirm-to-reveal, passphrase prompt for v2 links (retries against one download), decrypt locally, show text, download attachments; states for expired, already opened/not found, wrong passphrase, and decryption failure.
- Manage: the sender's manage link shows whether the secret is still waiting and can delete it before it is opened. It can never reveal the secret.
- Technical: Go html/template pages, vanilla JS with WebCrypto, strict CSP (self-only scripts/styles/fonts, no inline), no third-party assets or CDNs, no innerHTML, works over HTTPS; warns when served insecurely.
- Pages: send (home), result, receive, manage, about, error.

## Brand Commitments

- The product name "Gone" is binding. Everything else (marks, icons, palette, type) is open.

## Evidence on Hand

- README.md and about page copy explaining the mechanism. No logo, no illustrations, no testimonials, no usage statistics — none may be invented.

## Product Principles

1. Restraint is the trust signal: nothing decorative competes with the task or hints at hidden behavior.
2. Make the irreversible obvious: revealing and burning are one-way and must be stated plainly before they happen.
3. Seconds, not steps: the sender's path from paste to copied link stays as short as it is today or shorter.
4. The recipient may be non-technical and on a phone: plain language, large targets, no jargon required.
5. Self-contained by construction: nothing loads from anywhere but the operator's server.

## Accessibility & Inclusion

WCAG 2.2 AA minimum: full keyboard operation, visible focus, live-region announcements for async states (encrypting, uploading, copied, errors), reduced-motion support, light and dark themes honoring system preference, touch targets ≥ 24px (aim 44px on mobile).
