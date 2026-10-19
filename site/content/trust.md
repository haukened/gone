---
title: Security and trust
description: What Gone protects, what it can't, and how to check both yourself.
---

<!-- Draft for review. -->

# Security and trust

**Gone is built so you don't have to take its word for anything.** This page says what it protects, what it can't, and how to check both.

## What Gone protects

- **The secret, from the server.** The server stores only ciphertext and never sees a key. Someone who steals the server, its database or its backups gets nothing they can read.
- **One open.** The first download claims the secret atomically; anyone else gets the same "not found" as for a secret that never existed. It's deleted as soon as the recipient's browser confirms it arrived, or when a short lease runs out.
- **A leaked link,** when you added a strong passphrase. Someone with the link alone can't read the secret.
- **Requests.** A reply can only be read by the browser that asked for it: it's encrypted to a key that never leaves that browser.
- **A shared screen.** An opened message stays covered until the recipient presses Show.
- **Your traffic and your visitors.** Gone requires HTTPS, sends a strict Content-Security-Policy, loads nothing from other sites, and never logs visitors' addresses, secret IDs or tokens.

## What it can't protect

- **A compromised device.** Malware or a malicious browser extension on the sender's or recipient's computer can see what they see.
- **Where the link goes.** Anyone who gets the link before the recipient can open it, which uses it up. If the recipient finds it gone, the link leaked: send a new secret.
- **A secret already delivered.** Revoking stops a secret that hasn't been opened; it can't recall one that has.
- **The web page's code.** In a browser, the code that encrypts comes from the server you're using, so you trust that server to send Gone's real code. Run your own instance, or use the CLI, which encrypts on your machine with code you installed.

The full threat model is in [Security and architecture](/docs/security/#threat-model).

## Check it yourself

- **The protocol is written down** in a [normative specification](/docs/protocol/), with test vectors that the browser and CLI code are tested against.
- **The code is open,** under AGPL-3.0.
- **Every release carries signed build provenance.** Before you run Gone, you can check that a binary, package, image or Helm chart was built by Gone's release workflow from a tagged commit: `gh attestation verify <file> --repo haukened/gone`. The install script does this for you when the GitHub CLI is signed in.
- **The project's security practices are scored** by the [OpenSSF Scorecard](https://scorecard.dev/viewer/?uri=github.com/haukened/gone).

## Report a vulnerability

Please report security issues privately through GitHub's [private vulnerability reporting](https://github.com/haukened/gone/security/advisories/new), not in a public issue. [The security policy](/docs/security-policy/) says what's in scope and what to expect.

## This website

This site is static files on GitHub Pages. Its pages load nothing from other sites, set no cookies and run no analytics, and carry their own Content-Security-Policy. GitHub Pages can't send security headers, so the site can't forbid other sites from framing it, as a Gone instance does.
