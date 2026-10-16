# Gone documentation

Start with the [README](../README.md) for what Gone is and a one-command quick start.

## Running Gone

| Page | For |
| ---- | --- |
| [Running Gone with Docker](deploy/docker.md) | The container image: tags, data, health checks, verifying the image |
| [Docker Compose with Caddy](deploy/compose.md) | A ready-made HTTPS setup with automatic certificates |
| [Behind a reverse proxy](deploy/reverse-proxy.md) | nginx, Traefik, a Cloudflare Tunnel or any other TLS proxy, and `GONE_TRUSTED_PROXIES` |
| [Configuration](configuration.md) | Every environment variable, with defaults and limits |
| [Branding your instance](branding.md) | Your name, logo, accent colour and footer links, with screenshots of each |
| [Metrics](metrics.md) | The token-protected metrics listener, Prometheus and the JSON snapshot |

## Using Gone

| Page | For |
| ---- | --- |
| [The `gone` CLI](cli.md) | Sending, opening, managing and requesting secrets from a terminal or script, with exit codes and JSON formats |
| [HTTP API](api.md) and [OpenAPI spec](openapi.yaml) | Writing your own client |
| [Protocol](protocol.md) | The encryption format and link format, byte by byte, with shared test vectors |

## How it works

| Page | For |
| ---- | --- |
| [Security and architecture](security.md) | How the design keeps its promises, the threat model, what the server stores, security headers |
| [SECURITY.md](../SECURITY.md) | Reporting a vulnerability |

## Working on Gone

| Page | For |
| ---- | --- |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | How a change gets in, and the code, test and documentation requirements |
| [Development](development.md) | Building, running and testing from source |
| [Translations](../CONTRIBUTING.md#translations) and [the catalog conventions](../web/messages/README.md) | Fixing a translation or adding a language |
| [Roadmap](ROADMAP.md) | What has been built, phase by phase, and what's next |
| [Rejected features](rejected.md) | Features we've said no to, why, and when we'd look again |
| [Technical decisions](decisions.md) | How Gone is built, and the alternatives we turned down |
| [DESIGN.md](../DESIGN.md) and [PRODUCT.md](../PRODUCT.md) | The visual system and the product principles |
