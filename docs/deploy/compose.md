# Docker Compose with Caddy

[`deploy/compose.yaml`](../../deploy/compose.yaml) runs Gone behind [Caddy](https://caddyserver.com), which gets and renews a Let's Encrypt certificate on its own. It is the quickest way to run Gone for real.

## What you need

- A DNS name (an `A` or `AAAA` record) pointing at the server.
- Ports `80` and `443` open to the internet. Caddy needs both to get a certificate.
- Docker with the Compose plugin.

## Start it

```sh
mkdir gone && cd gone
curl -fsSLO https://raw.githubusercontent.com/haukened/gone/main/deploy/compose.yaml
curl -fsSLO https://raw.githubusercontent.com/haukened/gone/main/deploy/Caddyfile
GONE_DOMAIN=gone.example.com docker compose up -d
```

Then visit `https://gone.example.com`. To avoid typing the domain each time, put `GONE_DOMAIN=gone.example.com` in a `.env` file next to `compose.yaml`.

## What the files set up

- **Only Caddy is published.** Gone listens on a private Docker network (`172.31.255.0/28`) and is never reachable directly. Caddy redirects HTTP to HTTPS and proxies to `gone:8080`.
- **Rate limits per visitor.** Gone trusts Caddy's `X-Forwarded-For` header through `GONE_TRUSTED_PROXIES: 172.31.255.0/28`. Without it, every visitor would share Caddy's single budget. If that subnet collides with one of your networks, change it in both places in `compose.yaml`.
- **Hardened container.** Gone runs with a read-only root filesystem, no Linux capabilities, and `no-new-privileges`. It writes only to its `gone-data` volume.
- **Ordered start.** Caddy waits until Gone's health check passes.

## Changing settings

Add variables under the `gone` service's `environment:`. For example, to shorten the longest expiry and allow smaller secrets:

```yaml
    environment:
      GONE_TRUSTED_PROXIES: 172.31.255.0/28
      GONE_TTL_OPTIONS: 5m,30m,1h,4h
      GONE_MAX_BYTES: "1048576"
```

Every variable is listed in [Configuration](../configuration.md). Apply changes with `docker compose up -d`.

## Updating

```sh
docker compose pull && docker compose up -d
```

`compose.yaml` uses the `latest` tag. To control upgrades, pin a release instead, for example `ghcr.io/haukened/gone:v3.10.3`, and [verify it](docker.md#verifying-the-image) before you switch.
