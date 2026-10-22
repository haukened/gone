# Self-hosting Gone

Gone is one program, `goned`, that serves the web app and the API and keeps its data in a single directory. Running your own instance means running `goned`, giving it somewhere to store data, and putting HTTPS in front of it. This page explains what you need and how to choose a way to run it.

## What you run

- **`goned`:** a single static binary, or the `ghcr.io/haukened/gone` image built from it. It listens on `:8080` by default. It needs no database server, cache or other service.
- **A data directory** (`GONE_DATA_DIR`): an SQLite database and, for larger secrets, ciphertext files. Everything in it is ciphertext and expires on its own. It needs to be on local disk; see [What Gone stores](../operating.md#what-gone-stores).
- **An HTTPS proxy in front.** Browsers only allow the encryption Gone uses on secure pages, so Gone must be served over HTTPS. The proxy terminates TLS and forwards to `goned`. Caddy, nginx, Traefik, a Cloudflare Tunnel or a Kubernetes Gateway all work.
- **One instance.** Gone keeps its state on local disk, so it runs as exactly one process. A second copy would lose or corrupt secrets.

It is small: a running instance uses about 15 MB of memory, and its performance target is 100 requests a second with a 95th-percentile latency under 50 ms. Disk use is bounded by how many secrets are waiting at once.

## Choose a way to run it

| Method | Good for | You provide |
| --- | --- | --- |
| **[Docker Compose with Caddy](compose.md)** (recommended) | A server with a public DNS name. Caddy gets and renews the certificate for you. | Docker with Compose, a DNS name, ports 80 and 443 |
| [Docker](docker.md) | A host that already runs a reverse proxy, or trying Gone on your own machine | Docker, and a TLS proxy ([Behind a reverse proxy](reverse-proxy.md)) |
| [Kubernetes](kubernetes.md) | A cluster with Gateway API, using the Helm chart | Kubernetes 1.34+, Helm 3.8+, a Gateway that terminates TLS |
| [Linux packages](packages.md) | A VM or machine without Docker; a hardened systemd service | Debian, Ubuntu, Fedora or RHEL, and a TLS proxy |

All four run the same `goned` with the same settings, so you can move between them: settings are the `GONE_*` variables in [Configuration](../configuration.md).

## Before you open it to others

- **HTTPS only, and Gone reachable only through the proxy.** Don't publish Gone's port directly. The Compose file and the package already listen only where the proxy can reach them.
- **Set `GONE_TRUSTED_PROXIES` to your proxy's address,** and only that, so rate limits apply per visitor rather than to the proxy as a whole. See [Client addresses and rate limits](reverse-proxy.md#client-addresses-and-rate-limits).
- **Pin a release** (`ghcr.io/haukened/gone:vX.Y.Z`, a package version, or a chart version) and [verify it](../verify.md) before you deploy it.
- **Choose your expiry options** (`GONE_TTL_OPTIONS`) and size limit (`GONE_MAX_BYTES`). The longest expiry is also how long a secret can sit on your disk.
- **Decide who may create secrets.** By default anyone can. To limit it to your organisation, put Gone behind your sign-in proxy: see [Sign-in for private instances](../sign-in.md). To require a passphrase on every secret, see [Policies](../configuration.md#policies).
- **Keep metrics private.** If you turn them on, keep the listener on loopback or a private network; see [Metrics](../metrics.md).
- **Plan upgrades, and decide on backups.** Gone needs no backups of its data, and a restored backup can bring back secrets that were already opened. See [Upgrade and maintain](../operating.md).
- **Protect against floods upstream.** Gone limits each client, but a distributed flood has to be stopped before it reaches the server, for example at a CDN or the proxy.

Optional: show your own name, logo and links with [Branding](../branding.md).
