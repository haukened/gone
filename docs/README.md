# Gone documentation

Start with the [README](../README.md) for what Gone is and a one-command quick start.

## Use the CLI

| Page | For |
| ---- | --- |
| [The `gone` CLI](cli.md) | Installing `gone`, a first secret, scripts and AI agents, and staying safe on the command line |
| [CLI reference](cli-reference.md) | Every command and flag, which servers a link may reach, exit codes and JSON output |

## Self-host

| Page | For |
| ---- | --- |
| [Self-hosting Gone](deploy/README.md) | What you run, choosing a method, and a checklist before you open it to others |
| [Docker Compose with Caddy](deploy/compose.md) | The recommended setup: HTTPS with an automatic certificate, in one file |
| [Running Gone with Docker](deploy/docker.md) | The container image: tags, data, health checks |
| [Running Gone on Kubernetes](deploy/kubernetes.md) | The Helm chart: install, Gateway API routing, storage, one replica, and which sign-in providers work there |
| [Running Gone from a package](deploy/packages.md) | The `gone-server` .deb and .rpm: a hardened systemd service for running Gone without Docker |
| [Behind a reverse proxy](deploy/reverse-proxy.md) | nginx, Traefik, a Cloudflare Tunnel or any other TLS proxy, and `GONE_TRUSTED_PROXIES` |
| [Sign-in for private instances](sign-in.md) | Limiting who can create secrets to people your sign-in proxy has signed in |
| [Branding your instance](branding.md) | Your name, logo, accent colour and footer links, with screenshots of each |
| [Metrics](metrics.md) | The token-protected metrics listener, Prometheus and the JSON snapshot |
| [Upgrade and maintain](operating.md) | Upgrading, what Gone stores, why it needs no backups, health checks and logs |

## Security

| Page | For |
| ---- | --- |
| [Security model](security.md) | What Gone protects and what it can't, how the design keeps its promises, what the server stores, security headers |
| [Verify a release](verify.md) | Checking that an archive, package, image or chart was built by this repository |
| [Technical decisions](decisions.md) | How Gone is built, and the alternatives we turned down |
| [SECURITY.md](../SECURITY.md) | Reporting a vulnerability |

## Reference

| Page | For |
| ---- | --- |
| [Configuration](configuration.md) | Every environment variable, with defaults and limits |
| [CLI reference](cli-reference.md) | Every `gone` command and flag |
| [HTTP API](api.md) and [OpenAPI spec](openapi.yaml) | Writing your own client |
| [Protocol](protocol.md) | The encryption format and link format, byte by byte, with shared test vectors |

## The project

| Page | For |
| ---- | --- |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | How a change gets in, and the code, test and documentation requirements |
| [Development](development.md) | Building, running and testing from source |
| [Translations](../CONTRIBUTING.md#translations) and [the catalog conventions](../web/messages/README.md) | Fixing a translation or adding a language |
| [Roadmap](ROADMAP.md) | What has been built, phase by phase, and what's next |
| [Rejected features](rejected.md) | Features we've said no to, why, and when we'd look again |
| [DESIGN.md](../DESIGN.md) and [PRODUCT.md](../PRODUCT.md) | The visual system and the product principles |
| [TRADEMARKS.md](../TRADEMARKS.md) | Using the Gone name and logo |
