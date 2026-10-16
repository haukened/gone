# Behind a reverse proxy

Any proxy that terminates TLS works: nginx, Traefik, HAProxy, Caddy, or a Cloudflare Tunnel. If you're starting from scratch, [Docker Compose with Caddy](compose.md) is a ready-made setup.

## The rules

1. **Serve Gone over HTTPS.** Browsers won't encrypt or decrypt on a plain-HTTP page, so Gone turns off sending there.
2. **Forward to Gone's port** (`8080` by default, set by `GONE_ADDR`), and don't expose that port to other machines.
3. **Tell Gone which proxies to trust**, so rate limits apply to each visitor rather than to the proxy.

## Client addresses and rate limits

Rate limits key on the client address: IPv4 `/32`, IPv6 `/64`. Behind a proxy, every request appears to come from the proxy, so all visitors would share one budget.

Set `GONE_TRUSTED_PROXIES` to the proxy's address or network, for example `172.18.0.2` or `10.0.0.0/8`. Gone then reads the real client from `X-Forwarded-For`, but only on requests that arrive from a trusted address.
- Prefixes broader than `/8` (IPv4) or `/32` (IPv6) are rejected, and so are IPv4-mapped IPv6 addresses.
- If `X-Forwarded-For` arrives from an untrusted peer, Gone ignores it and logs a one-time warning.

> [!WARNING]
> Only list proxies you control. A trusted proxy can make any request look like it came from any address.

Limits are kept in memory per process. With several replicas, the effective budget is the configured budget times the number of replicas.

## Example: nginx

```nginx
server {
    listen 443 ssl;
    server_name gone.example.com;
    # ssl_certificate ... ;

    client_max_body_size 11m;   # a little above GONE_MAX_BYTES (10 MiB)

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

Run Gone with `GONE_ADDR=127.0.0.1:8080` and `GONE_TRUSTED_PROXIES=127.0.0.1`.

## Things to check

- **Body size:** the proxy must allow uploads of at least `GONE_MAX_BYTES` (10 MiB by default) plus a little overhead.
- **Headers:** don't strip `X-Gone-*` request or response headers. The browser and CLI use them to claim, confirm and manage secrets.
- **Caching:** don't cache `/api/` responses. Gone sends `Cache-Control: no-store` on them.
- **Metrics:** keep the metrics port (`GONE_METRICS_ADDR`) off the public proxy. See [Metrics](../metrics.md).
