# Sign-in interop checks

Runs Gone behind real sign-in proxies and checks what [docs/sign-in.md](../../../docs/sign-in.md) promises. No vendor accounts are needed: everything runs locally in Docker with test-only credentials. It isn't part of CI.

| Stack | Proxy | Gone setting | Checks |
| ----- | ----- | ------------ | ------ |
| oidc | Dex (identity provider) and oauth2-proxy | `GONE_AUTH=oidc` | A real ID token works through the proxy and straight to Gone; a forged token and no token are refused; the open paths need no sign-in |
| authelia | Authelia behind Caddy `forward_auth` | `GONE_AUTH=authelia` | A logged-in session can create; no session, a forged `Remote-User` through Caddy, and a forged `Remote-User` sent straight to Gone are all refused |

Run it with `task interop-signin`, which builds `bin/goned` first. It needs Docker, `curl`, `jq` and `openssl`, and uses `127.0.0.1` ports 14180, 15556, 18081, 18082 and 18443 and the Docker network `172.31.251.0/24`. Set `KEEP=1` to leave the containers running afterwards.

Pomerium, Cloudflare Access, Google IAP, Teleport, authentik and Tailscale are covered by the unit tests in `internal/auth`, which mint tokens in each provider's documented format. Pomerium and authentik need a browser login, and the others need an account.
