# Sign-in for private instances

You can limit who creates secrets and requests on your instance to people your sign-in proxy has signed in. Opening a secret, replying to a request and the manage links never need sign-in, so recipients and the people you ask for secrets don't need an account. With `GONE_AUTH` unset, Gone is open to everyone, as it always has been.

Gone doesn't sign anyone in itself. Your existing proxy does that (Cloudflare Access, oauth2-proxy, Pomerium, Authelia and others). Gone then checks the proof the proxy adds to each request.

## How Gone trusts the proxy

Proxies hand over identity in one of two ways, and Gone treats them differently:

| Kind | What the proxy sends | What Gone checks | Safe when |
| ---- | -------------------- | ---------------- | --------- |
| **Signed token** | A JWT signed with the proxy's or identity provider's key | The signature against the provider's published keys, the algorithm, issuer, audience, expiry, and a non-empty subject | Always, even if someone reaches Gone without going through the proxy |
| **Plain header** | A header such as `Remote-User: alice` | That the request came from an address in `GONE_TRUSTED_PROXIES`, and the header's value | Only Gone's network lets nothing but the proxy reach it, and the proxy removes the header when a visitor sends it |

Prefer a signed provider when your proxy offers one. A plain-header provider refuses to start without `GONE_TRUSTED_PROXIES`, and every provider prints a warning at startup when it's missing.

List only your proxy's own address in `GONE_TRUSTED_PROXIES`, never a whole network. With Docker's published ports, a connection from the internet can arrive from the bridge's gateway address, so trusting the bridge subnet would trust a header any visitor sends.

**What Gone doesn't do:**
- It doesn't store, log or show who signed in. It only decides "signed in or not", and a refusal is counted in the `sign_in_denied_total` metric with nothing about the visitor.
- It doesn't keep accounts, users or groups.
- It doesn't filter by user or group. Allow or deny people in your proxy.
- It doesn't redirect anyone. See [What visitors see](#what-visitors-see).

## Providers

| `GONE_AUTH` | Product | Kind | Header Gone reads | You set |
| ----------- | ------- | ---- | ----------------- | ------- |
| `cloudflare` | Cloudflare Access | signed (RS256) | `Cf-Access-Jwt-Assertion` | `GONE_AUTH_TEAM`, `GONE_AUTH_AUDIENCE` |
| `google-iap` | Google Cloud Identity-Aware Proxy | signed (ES256) | `X-Goog-Iap-Jwt-Assertion` | `GONE_AUTH_AUDIENCE` |
| `pomerium` | Pomerium | signed (ES256) | `X-Pomerium-Jwt-Assertion` | `GONE_AUTH_HOST` (optionally `GONE_AUTH_JWKS_URL`) |
| `teleport` | Teleport Application Access | signed (RS256) | `Teleport-Jwt-Assertion` | `GONE_AUTH_HOST`, `GONE_AUTH_ISSUER`, `GONE_AUTH_AUDIENCE` |
| `oidc` | Any OpenID Connect provider's ID token, passed on by oauth2-proxy, Vouch or a similar proxy | signed (RS256 or ES256) | `Authorization: Bearer`, or `GONE_AUTH_HEADER` | `GONE_AUTH_ISSUER`, `GONE_AUTH_AUDIENCE` (optionally `GONE_AUTH_ALG`, `GONE_AUTH_HEADER`, `GONE_AUTH_JWKS_URL`) |
| `authelia` | Authelia | plain | `Remote-User` | `GONE_TRUSTED_PROXIES` |
| `authentik` | authentik proxy outpost | plain | `X-Authentik-Username` | `GONE_TRUSTED_PROXIES` |
| `tailscale` | Tailscale Serve | plain | `Tailscale-User-Login` | `GONE_TRUSTED_PROXIES` (usually `127.0.0.1`) |
| `custom` | Anything else that sets a header: Okta Access Gateway, F5 BIG-IP APM, Microsoft Entra application proxy's header-based sign-on, or nginx or Caddy renaming a header | plain | `GONE_AUTH_HEADER` | `GONE_AUTH_HEADER`, `GONE_TRUSTED_PROXIES` (optionally `GONE_AUTH_FORMAT`, `GONE_AUTH_VALUE`) |

Every variable is described in [Configuration](configuration.md#sign-in). A setting that doesn't apply to your provider stops the server, so a typo can't go unnoticed.

**Signed providers fetch keys.** Gone downloads the provider's public keys when it starts and every hour after, and again when it sees a key it doesn't know (at most once every 5 minutes). It's Gone's only outbound connection. Until the keys load, nobody counts as signed in, and creating is refused.

**Not built in yet:** AWS Application Load Balancer sign-in, Azure App Service authentication, and a dedicated Microsoft Entra provider. Entra application proxy works today through `custom`. [Ask](https://github.com/haukened/gone/issues/new/choose) if you need one of the others.

## Paths to leave open

Recipients and the people you ask for secrets must reach these paths without signing in. Configure your proxy to let them through, and to require sign-in for everything else:

- `/secret/`, `/manage/` and `/reply/`, with anything after them
- `/request/` followed by an ID (but not `/request` itself, which is where requests are made)
- `/api/secret/` and `/api/request/` with a trailing slash and anything after it (but not `/api/secret` and `/api/request`, where secrets and requests are created)
- `/api/policy`, `/static/`, `/i18n/`, `/healthz` and `/readyz`

If your exemptions are broader than this, Gone still refuses to create anything without sign-in.

Some proxies protect a whole application and can't exempt paths: Teleport, Tailscale Serve, and Google IAP unless you route the open paths to a second backend without IAP. With those, every recipient needs access through the same proxy too.

## What visitors see

When sign-in is on and a visitor isn't signed in:
- **API:** `POST /api/secret` and `POST /api/request` return `401` with `{"error":"sign in required"}`. Nothing else changes.
- **The send and request pages** show **Sign in required** instead of the form. If you set `GONE_AUTH_SIGNIN_URL`, the page adds a **Sign in with your identity provider** button that links to it. Gone never redirects by itself.
- **The `gone` CLI** stops with "this server requires signing in through its web page". It can't create secrets or requests on a sign-in instance yet; opening, replying, status and revoke work as usual.

`{return}` in `GONE_AUTH_SIGNIN_URL` becomes the page's own path (`/` or `/request`), so the proxy can send the visitor back. Normally the proxy signs visitors in before Gone sees them, so this page only shows when the paths left open are broader than intended.

## Recipes

### Cloudflare Access

1. In Cloudflare Zero Trust, add a self-hosted application for your Gone hostname with a policy for the people allowed to create.
2. Add a second application, or path rules, with the **Bypass** action for the [paths to leave open](#paths-to-leave-open).
3. Copy the application's **Application Audience (AUD) Tag**.

```sh
GONE_AUTH=cloudflare
GONE_AUTH_TEAM=acme            # acme.cloudflareaccess.com
GONE_AUTH_AUDIENCE=4714c1358e65fe4b408ad6d432a5f878f08194bdb4752441fd56faefa9b2b6f2
GONE_TRUSTED_PROXIES=127.0.0.1 # cloudflared on the same host
```

### oauth2-proxy with an OpenID Connect provider

oauth2-proxy passes the identity provider's ID token on as `Authorization: Bearer`, and Gone verifies it against the provider's keys. The same works for Keycloak, which recommends oauth2-proxy.

```sh
oauth2-proxy \
  --provider=oidc --oidc-issuer-url=https://idp.example.com/realms/acme \
  --client-id=gone --client-secret-file=/run/secrets/oidc \
  --upstream=http://127.0.0.1:8080 --http-address=127.0.0.1:4180 \
  --pass-authorization-header=true \
  --cookie-refresh=4m \
  --skip-auth-route='^/(secret|manage|reply)/' --skip-auth-route='^/request/.' \
  --skip-auth-route='^/api/(secret|request)/' --skip-auth-route='^/api/policy$' \
  --skip-auth-route='^/(static|i18n)/' --skip-auth-route='^/(healthz|readyz)$'
```

```sh
GONE_AUTH=oidc
GONE_AUTH_ISSUER=https://idp.example.com/realms/acme
GONE_AUTH_AUDIENCE=gone
GONE_AUTH_SIGNIN_URL=/oauth2/start?rd={return}
GONE_TRUSTED_PROXIES=127.0.0.1
```

Set `--cookie-refresh` shorter than your provider's ID token lifetime, so the token Gone checks has not expired. Gone reads the keys from the issuer's `/.well-known/openid-configuration`. Write `GONE_AUTH_ISSUER` exactly as that document's `issuer` does, trailing slash included: tokens carry it verbatim.

### Pomerium

Turn on `pass_identity_headers` for the Gone route. Add a second route for the [paths to leave open](#paths-to-leave-open) with `allow_public_unauthenticated_access: true`.

```sh
GONE_AUTH=pomerium
GONE_AUTH_HOST=gone.example.com
```

### Authelia behind Caddy

Use Caddy 2.11.2 or later. Earlier 2.10 and 2.11 releases passed a visitor's own `Remote-User` header through `forward_auth` ([CVE-2026-30851](https://github.com/caddyserver/caddy/security/advisories/GHSA-7r4p-vjf4-gxv4)).

```caddyfile
gone.example.com {
	forward_auth authelia:9091 {
		uri /api/authz/forward-auth
		copy_headers Remote-User Remote-Groups Remote-Email Remote-Name
	}
	reverse_proxy gone:8080
}
```

In Authelia's `access_control`, give the open paths `policy: bypass` and the rest of the domain `one_factor` or `two_factor`:

```yaml
access_control:
  rules:
    - domain: gone.example.com
      resources: ['^/(secret|manage|reply)/', '^/request/.', '^/api/(secret|request)/', '^/api/policy$', '^/(static|i18n)/', '^/(healthz|readyz)$']
      policy: bypass
    - domain: gone.example.com
      policy: one_factor
```

```sh
GONE_AUTH=authelia
GONE_TRUSTED_PROXIES=172.20.0.10   # Caddy's address
GONE_AUTH_SIGNIN_URL=https://auth.example.com/?rd=https://gone.example.com{return}
```

### Any proxy that sets a header (custom)

With nginx in front of an auth service (`auth_request`), pass the signed-in user on under a name of your choice:

```nginx
location / {
	auth_request /oauth2/auth;
	auth_request_set $email $upstream_http_x_auth_request_email;
	proxy_set_header X-Gone-User $email;
	proxy_pass http://127.0.0.1:8080;
}
```

```sh
GONE_AUTH=custom
GONE_AUTH_HEADER=X-Gone-User
GONE_AUTH_FORMAT=email
GONE_TRUSTED_PROXIES=127.0.0.1
```

- **`GONE_AUTH_FORMAT`:** `any` (the default) accepts any value that isn't empty; `email` accepts only a bare address.
- **Proof the request came through the proxy:** to have the proxy send a fixed value as well, set `GONE_AUTH_FORMAT=exact` and the same long random value in `GONE_AUTH_VALUE`. This proves the request passed through the proxy even if the network lets other traffic reach Gone. Generate one with `openssl rand -base64 24`.
- **Header names:** use letters, digits and dashes only. nginx drops headers containing underscores.
