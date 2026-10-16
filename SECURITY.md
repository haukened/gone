# Security Policy

## Supported Versions

Only the latest release is supported. Security fixes go into a new release, not into older ones, so update to the [latest release](https://github.com/haukened/gone/releases/latest) before reporting.

| Version            | Supported          |
| ------------------ | ------------------ |
| Latest release     | :white_check_mark: |
| Any older release  | :x:                |

## Reporting a Vulnerability

Please report vulnerabilities privately through GitHub's [private vulnerability reporting](https://github.com/haukened/gone/security/advisories/new). Only the maintainers can see the report. Do not open a public issue, pull request, or discussion about it.

Please include:
- the affected version (`gone version`, or the container image tag);
- what an attacker can do, and what they need first (for example, a leaked link, a position on the network, or access to the server);
- steps or a proof of concept to reproduce it.

## What to expect

- **Acknowledgement within 7 days** of your report.
- An assessment, and a fix or mitigation plan, as soon as we've confirmed the issue. Medium or higher severity issues are fixed within 60 days of becoming publicly known.
- We'll agree a disclosure date with you, publish a GitHub Security Advisory (and request a CVE where appropriate) when the fix is released, and name the vulnerability in that release's notes.
- We're happy to credit you in the advisory unless you'd rather stay anonymous.

## Scope

Gone's security model is described in [docs/security.md](docs/security.md). In short: the server must never be able to read a secret, a link must open a secret at most once, and a request's reply must open only in the browser (or `gone` CLI) that made the request. Problems with the default container image, the web client, the `gone` CLI, secret requests, the token-protected metrics listener, and the protocol in [docs/protocol.md](docs/protocol.md) are all in scope.
