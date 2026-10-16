# Configuration

The server, `goned`, is configured only through environment variables. There are no command-line flags and no config files. An invalid value stops it at startup with an error that names the variable.

## Server variables

| Variable | Description | Default |
| -------- | ----------- | ------- |
| `GONE_ADDR` | Listen address: an IP literal and port (`127.0.0.1:8080`, `[::1]:8080`) or `:port`. Hostnames such as `localhost` are rejected. | `:8080` |
| `GONE_DATA_DIR` | Data directory for the SQLite database and blobs. Created with mode `0700` if missing. `""`, `.`, `/` and any path with a `..` part are rejected. | `/data` |
| `GONE_MAX_BYTES` | Largest secret in bytes, message and attachments combined. Must be above 0. | `10485760` (10 MiB) |
| `GONE_INLINE_MAX_BYTES` | Ciphertext up to this size is stored in SQLite; larger ciphertext is stored as a file under `blobs/`. Must be above 0. | `8192` |
| `GONE_REQUIRE_PASSPHRASE` | `true` to accept only passphrase-protected secrets. The server refuses others with `403`. Replies to requests are exempt. See [Policies](#policies). | `false` |
| `GONE_MAX_FILES` | Files a secret or reply may carry in the web page and the CLI, from 0 to 10. `0` turns files off. Advisory: see [Policies](#policies). | `10` |
| `GONE_TTL_OPTIONS` | Comma list of the expiry choices offered. Needs at least two different positive values. The web forms preselect the shortest one. | `5m,30m,1h,2h,4h,8h,24h` |
| `GONE_CLAIM_LEASE` | How long an opened secret is reserved while the recipient's browser downloads it and confirms. If it never confirms (closed tab, dropped network), the secret is deleted when the lease lapses. Above 0, at most `15m`. | `2m` |
| `GONE_RATE_CREATE` | Per-client budget for creating secrets and requests and for sending a reply, as `N/s`, `N/m` or `N/h` (`N` up to 1,000,000). `0` turns it off. | `10/m` |
| `GONE_RATE_READ` | Per-client budget for everything else on the API: opening and confirming secrets, status and delete calls, and checking, claiming or cancelling requests. `0` turns it off. | `30/m` |
| `GONE_RATE_BURST` | Requests a client may make back to back before the rate applies (1–1000). | `10` |
| `GONE_TRUSTED_PROXIES` | Comma list of proxy IPs or CIDRs whose `X-Forwarded-For` header is trusted. Prefixes broader than `/8` (IPv4) or `/32` (IPv6), and IPv4-mapped IPv6 addresses, are rejected. See [Behind a reverse proxy](deploy/reverse-proxy.md). | (empty) |
| `GONE_METRICS_ADDR` | Address for the separate metrics listener, in the same form as `GONE_ADDR`. Use a loopback address such as `127.0.0.1:9090`; see [Metrics](metrics.md). | (empty) |
| `GONE_METRICS_TOKEN` | Bearer token for the metrics listener, at least 16 characters recommended. A shorter token still works and logs a warning at startup, but a future release will refuse it. Generate one with `openssl rand -base64 24`. Metrics stay off unless both this and `GONE_METRICS_ADDR` are set. See [Metrics](metrics.md). | (empty) |
| `GONE_BRAND_NAME` | Your instance's name, shown in the header in place of the Gone wordmark. Up to 40 characters; commas are fine. See [Branding](#branding). | (empty) |
| `GONE_BRAND_LOGO` | Path to your logo, an SVG or PNG file of at most 256 KiB, shown in the header before the name. Read once at startup. | (empty) |
| `GONE_BRAND_ACCENT` | Your accent colour for the light and dark themes, as `#rrggbb/#rrggbb`. Each must have at least 4.5:1 contrast on its theme's background. | (empty) |
| `GONE_CONTACT_URL` | Footer link to your contact page: an `https://` URL or a `mailto:` address. | (empty) |
| `GONE_IMPRINT_URL` | Footer link to your imprint (legal notice) page, `https://` only. | (empty) |
| `GONE_PRIVACY_URL` | Footer link to your privacy policy, `https://` only. | (empty) |

### Expiry format

`GONE_TTL_OPTIONS` takes Go durations with `s`, `m` or `h`, such as `30s,5m,90m,2h`. Day and week units (`d`, `w`) are not accepted; write `24h` or `168h`.

The smallest and largest options set the accepted range. The API accepts any duration inside it, not just the listed ones; the web form offers only the listed ones and preselects the shortest, so a longer lifetime is always a deliberate choice. There is no setting for the preselected option.

### Policies

Two settings let an operator narrow what people may send. Clients read them from `GET /api/policy` before they encrypt anything.

| Setting | Who enforces it | What happens |
| ------- | --------------- | ------------ |
| `GONE_REQUIRE_PASSPHRASE=true` | The server | A secret without a passphrase (protocol v1) gets `403 passphrase required` before its body is read. The web page always shows the passphrase field and won't send without one; the CLI stops before reading anything and names the `--passphrase-*` flags. The server can tell a passphrase was used, not how strong it is. |
| `GONE_MAX_FILES=N` | The web page and the CLI | The web page offers N files, or hides file attachments at `0`; the CLI refuses more `--file` flags than that. The server can't check it, because the file count is encrypted, so a custom client can ignore it. Bound what a secret may carry with `GONE_MAX_BYTES`, which the server enforces. |

Replies to [requests](api.md#request-workflow) never need a passphrase. A reply link carries no decryption key, only the requester's public key, so a leaked reply link can't open a reply; see [decisions.md](decisions.md#server) for the full reasoning. Replies do follow `GONE_MAX_FILES`.

### Branding

The six branding settings put your name, logo and accent colour on an unmodified copy of Gone and link your contact, imprint and privacy pages from the footer. Each is optional and works alone; unbranded instances look exactly as before. [Branding your instance](branding.md) shows each setting with screenshots of the result.

In short:
- **Header:** the logo and name replace the Gone wordmark. Footer links alone don't change it.
- **Accent:** your two colours replace Gone's violet in heading highlights, focus rings and progress bars. Each must reach 4.5:1 contrast on its theme's background, or the server won't start.
- **Footer:** your links come first. The version line reads "Powered by Gone", and it, the copyright notice and the project links **can't be removed**.
- **Everything else stays Gone:** page titles, the favicon, the About page and the page text.

### Derived settings

- The database is `<GONE_DATA_DIR>/gone.db`. It runs in WAL mode with `synchronous=FULL`, so a confirmed claim survives a power cut.
- Blobs are stored in `<GONE_DATA_DIR>/blobs/`.

## CLI variables

The `gone` command-line client reads one variable:

| Variable | Description |
| -------- | ----------- |
| `GONE_SERVER` | Server for `gone send` and `gone request` when `--server` isn't given. It is checked before the server saved with `gone set server`. `get`, `status`, `revoke` and `reply` open a link only if its server is this one (or the one from `--server`, the config or the default) or is allowed with `--allow-origin`; the `request` subcommands use the saved request's server. See [cli.md](cli.md#which-servers-a-link-may-reach). |

See [the CLI guide](cli.md) for the full precedence and every flag.

## Example

```sh
GONE_ADDR=127.0.0.1:8080 \
GONE_DATA_DIR=/srv/gone \
GONE_TTL_OPTIONS=5m,30m,1h \
GONE_MAX_BYTES=$((1024*1024)) \
GONE_TRUSTED_PROXIES=127.0.0.1 \
goned
```
