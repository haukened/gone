---
title: Gone
description: Share a secret exactly once. Encrypted in your browser, deleted after one read. Open source and self-hosted.
---

<!-- Draft for review. Layout notes are in HTML comments; the generator will turn the sections into the home page template. -->

# Share a secret exactly *once*.

Encrypted in your browser. Deleted after one read. Gone is a small, open-source service for handing over a password, a token or a key, on a server you run yourself.

<!-- Install box: tabs, each with a copy button. Kubernetes and Packages show only once the latest release includes them (v3.13). -->

| Tab | Command | Under it |
| --- | --- | --- |
| CLI | `curl -fsSL https://raw.githubusercontent.com/haukened/gone/main/install.sh \| sh` | Linux and macOS, no sudo. Checks the download before installing it. [Read the script first](https://github.com/haukened/gone/blob/main/install.sh) |
| Docker | `docker run --rm -p 8080:8080 ghcr.io/haukened/gone:latest` | Then open http://localhost:8080. For other machines, put it behind HTTPS. |
| Kubernetes | `helm install gone oci://ghcr.io/haukened/charts/gone` | |
| Packages | `sudo apt install ./gone-server_….deb` | Debian, Ubuntu, Fedora and RHEL, with a systemd service. |

**Buttons:** [Try it on the public instance](https://gone.hauken.us) · [Read the docs](/docs/)

*The public instance is open to anyone and run by the maintainer, with no uptime guarantee.*

## Three promises

1. **Only the person you send it to can read it.** Your browser encrypts the secret before it leaves your device, and the key never reaches the server. Not the server, not its operator, not us.
2. **It opens once.** The first person to open the link sees the secret. Anyone after them finds nothing.
3. **Then nothing is kept.** No copy, no history, no record that it was opened.

## How it works

1. **You write the secret.** Your browser (or the `gone` CLI) encrypts it with a new random key, using AES-256-GCM.
2. **You send the link.** The key is in the part after the `#`, which browsers never send to a server. The server only ever holds ciphertext.
3. **They open it, once.** Their browser downloads the ciphertext, decrypts it locally, and confirms it arrived. Only then does the server delete it. If they never open it, it expires.

## Features

- **Files,** up to ten per secret.
- **A passphrase** as a second lock, typed or five generated words.
- **Ask for a secret.** Send someone a link; their reply is encrypted to a key that never leaves your browser.
- **A QR code** for handing a link over in person.
- **Covered until shown,** so an opened secret is safe on a shared screen.
- **A CLI that works with the browser** in both directions, and lets scripts handle secrets by reference.
- **Five languages** in the app: English, Spanish, French, German and Brazilian Portuguese.
- **Operator controls:** require a passphrase, limit files, add your name and logo, or require sign-in through your own proxy. All free.
- **Metrics** for Prometheus, behind a token.

<!-- Screenshots: Send, Open, Revealed, Request, from docs/img/*-{light,dark}.jpg, with the README's captions. -->

## Most tools say they're secure. Here's how to check.

Look for the key after the `#` in the link, watch what your browser sends, and ask whether the server could read what it stores. Gone is built to pass every one of those checks, and to let you see that it does.

[Choosing a secret-sharing tool](/choose/)

## Run it your way

Docker, Docker Compose with Caddy (HTTPS in one file), Kubernetes, or a Linux package with a systemd service. One static binary, one SQLite file, no external services.

<!-- Each links to its guide in /docs/. -->

## Built in the open

Open source under AGPL-3.0 · Every release carries signed build provenance you can verify · OpenSSF Scorecard · [How Gone keeps its promises](/trust/)

<!-- Footer: GitHub · Releases · Docs · Security · Privacy · License (AGPL-3.0) · Trademarks. Small print: "This site sets no cookies and runs no analytics." -->
