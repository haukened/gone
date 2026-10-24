---
title: Gone
seoTitle: "Gone: share passwords and secrets with one-time links"
description: "Send a password, API key or file as a link that opens once. Encrypted in your browser, deleted after one read. Free, open source and self-hostable."

hero:
  eyebrow: Open source · Self-hosted · Zero-knowledge
  title: Share a secret exactly
  accent: once
  lead: Paste a password, a token or a file, and get a link that works exactly once. It's encrypted in your browser and deleted after one read.
  # The way in for most visitors: the public instance, in the browser.
  cta:
    primary: Send a secret
    secondary: Ask someone for a secret
    secondaryPath: /request
    note: Free, no account, nothing to install. Opens app.getgone.dev in your browser.
  image:
    light: docs/img/send-light.jpg
    dark: docs/img/send-dark.jpg
    alt: "Gone's send page: a message box holding a staging database login, with options for files, expiry and a passphrase, and a Create one-time link button"

# The CLI install box. {tag} is the latest release (v3.12.0) and {version}
# the same without the "v". A tab with "since" appears only once the latest
# release is at least that version.
install:
  title: Install the gone CLI
  tabs:
    - id: unix
      label: Linux & macOS
      commands:
        - curl -fsSL https://getgone.dev/install.sh | sh
      hint: Installs into ~/.local/bin without sudo, after checking the download against the release's checksums.
      link:
        text: Read the script first
        url: https://github.com/haukened/gone/blob/main/install.sh
    - id: packages
      label: Debian & Fedora
      since: v3.13.0
      commands:
        - sudo apt install ./gone_{version}_amd64.deb
        - sudo dnf install ./gone-{version}-1.x86_64.rpm
      hint: Download the package for your system from the release first; arm64 packages are there too.
      link:
        text: Packages for {tag}
        url: https://github.com/haukened/gone/releases/tag/{tag}
    - id: windows
      label: Windows
      download:
        text: Download for Windows
        url: https://github.com/haukened/gone/releases/download/{tag}/gone_{tag}_windows_amd64.zip
      hint: Unzip it and put gone.exe somewhere on your PATH. An Arm build is on the release page.
      link:
        text: All downloads for {tag}
        url: https://github.com/haukened/gone/releases/tag/{tag}
    - id: source
      label: From source
      commands:
        - go install github.com/haukened/gone/v3/cmd/gone@latest
      hint: Needs Go 1.27 or later.
  more:
    text: Every install option, and how to verify a download
    url: /docs/cli/#install

promises:
  - icon: lock
    title: Only they can read it
    text: Your browser encrypts the secret before it leaves your device, and the key never reaches the server. Not the server, not its operator, not us.
  - icon: eye
    title: It opens once
    text: The first person to open the link sees the secret. Anyone after them finds nothing.
  - icon: inbox
    title: Then nothing is kept
    text: No copy, no history, and no record that it was ever opened.

how:
  title: How it works
  steps:
    - title: You write the secret
      text: Your browser, or the gone CLI, encrypts it with a new random key using AES-256-GCM.
    - title: You send the link
      text: "The key is in the part after the #, which browsers never send to a server. The server only ever holds ciphertext."
    - title: They open it, once
      text: Their browser downloads the ciphertext, decrypts it, and confirms it arrived. Only then does the server delete it. Unopened secrets expire.
  link:
    text: The security model in detail
    url: /docs/security/
  image:
    light: docs/img/open-light.jpg
    dark: docs/img/open-dark.jpg
    alt: Gone's Open page, the secret waiting behind frosted glass until the recipient presses Open

cli:
  title: Or from your terminal
  text: The gone CLI works with secrets sent from the browser, and the other way round. Messages come from standard input, never from arguments, and scripts and AI agents can handle secrets by reference so the plaintext never appears in their output.
  commands:
    - command: printf 'hunter2' | gone send --ttl 30m
      note: prints the link, and a manage link to revoke it
    - command: gone get 'https://app.getgone.dev/secret/AbC#v1:…'
      note: opens a link, once
    - command: gone request --label 'VPN key'
      note: asks someone for a secret
    - command: printf 'hunter2' | gone reply '<request-link>'
      note: answers someone's request
  link:
    text: The CLI guide
    url: /docs/cli/

features:
  title: Small, with what you need
  items:
    - icon: clip
      title: Files
      text: Up to ten files per secret, encrypted like the message.
    - icon: lock
      title: A passphrase
      text: A second lock, typed or five generated words, sent separately.
    - icon: inbox
      title: Ask for a secret
      text: Send a link; the reply is encrypted to a key that never leaves your browser.
    - icon: qr
      title: A QR code
      text: For handing a link over in person.
    - icon: eye
      title: Covered until shown
      text: An opened secret stays hidden on a shared screen until you press Show.
    - icon: arrow
      title: A CLI that fits in
      text: Works with browser links in both directions, and with scripts.
    - icon: globe
      title: Five languages
      text: English, Spanish, French, German and Brazilian Portuguese.
    - icon: check
      title: Operator controls
      text: Require a passphrase, limit files, add your name and logo, or require sign-in through your own proxy. All free.
    - icon: waiting
      title: Metrics
      text: Prometheus metrics on their own listener, behind a token.

run:
  title: Run your own server
  lead: One static binary, one SQLite file, no external services. Every feature is free on every instance.
  options:
    - title: Docker Compose with Caddy
      badge: Recommended
      text: HTTPS with an automatic certificate, in one file.
      url: /docs/self-host/compose/
    - title: Docker
      text: The container image, behind your own proxy.
      url: /docs/self-host/docker/
    - title: Kubernetes
      since: v3.13.0
      text: A Helm chart, routed through the Gateway API.
      url: /docs/self-host/kubernetes/
    - title: Linux package
      since: v3.13.0
      text: A hardened systemd service from a .deb or .rpm.
      url: /docs/self-host/packages/

open:
  title: Built in the open
  facts:
    - title: Open source
      text: Every line, under the AGPL-3.0 licence.
    - title: Signed releases
      text: Every download carries build provenance you can verify.
    - title: Scored
      text: Security practices checked by the OpenSSF Scorecard.
  link:
    text: How Gone keeps its promises
    url: /trust/
---
