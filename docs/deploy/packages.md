# Running Gone from a package

Each release has a `gone-server` package for Debian, Ubuntu, Fedora and RHEL, for amd64 and arm64. It installs `goned` with a hardened systemd service. Use it to run Gone directly on a machine, a VM or a Proxmox container instead of in Docker. The CLI is the separate `gone` package (see [The `gone` CLI](../cli.md#install)).

> [!IMPORTANT]
> **Gone needs HTTPS.** Browsers only allow the encryption Gone uses on secure pages, so the service listens on `127.0.0.1:8080` and expects a TLS proxy in front of it, such as Caddy, nginx or a Cloudflare Tunnel. See [Behind a reverse proxy](reverse-proxy.md).

## Install

Download the package for your system from the [release page](https://github.com/haukened/gone/releases), check it, and install it:

```sh
TAG=v3.13.0
PKG=gone-server_${TAG#v}_amd64.deb             # or gone-server-${TAG#v}-1.x86_64.rpm, or arm64 / aarch64
curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/${PKG}"
gh attestation verify "$PKG" --repo haukened/gone

sudo apt install "./$PKG"                       # Debian, Ubuntu
sudo dnf install "./$PKG"                       # Fedora, RHEL
```

The package doesn't start Gone. It needs its settings and a TLS proxy first, so installing only prints the next step.

## Configure and start

Settings live in `/etc/default/gone` as `GONE_*` variables, the same ones listed in [Configuration](../configuration.md). The file starts with Gone listening on localhost and the common settings commented out. If your proxy runs on the same machine, set:

```sh
GONE_TRUSTED_PROXIES=127.0.0.1
```

so rate limits apply to each visitor rather than to the proxy. Then start Gone, and have it start at boot:

```sh
sudo systemctl enable --now gone
systemctl status gone
journalctl -u gone        # logs
```

After editing `/etc/default/gone`, run `sudo systemctl restart gone`. `GONE_DATA_DIR` is set by the service, so leave it out of the file.

## Data and backups

Gone keeps its database and stored secrets in `/var/lib/gone`, which only the service (and root) can read. Everything in it is ciphertext and expires on its own. Don't back it up: restoring a copy makes secrets opened since then openable again until they expire. Back up `/etc/default/gone` instead; see [Backups](../operating.md#backups).

## Upgrades and removal

- **Upgrading:** install the newer release's package the same way. The packages aren't in an apt or dnf repository yet, so `apt upgrade` and `dnf upgrade` won't find new versions. An upgrade keeps your `/etc/default/gone`, and restarts Gone if it's enabled.
- **Removing** the package stops and disables the service. It keeps `/var/lib/gone`, like other services' data. Delete that directory yourself if you no longer need it.

## How the service is locked down

The service file is `/usr/lib/systemd/system/gone.service`.
- **No user to manage:** it uses systemd's `DynamicUser`. systemd gives the service an unprivileged user of its own when it starts, and keeps `/var/lib/gone` owned by it. So the package creates no user, and Gone never runs as root.
- **Hardened:** it can't gain privileges or capabilities, and sees the filesystem read-only apart from its data directory and a private `/tmp`. Home directories, devices and other processes are hidden from it, kernel settings are read-only, and it can only open IP and local sockets. Its system calls are limited to systemd's `@system-service` set.
- **Checked:** `systemd-analyze security gone` shows the details.

If you need to change the service, use `sudo systemctl edit gone` rather than editing the file, so upgrades keep your changes.
