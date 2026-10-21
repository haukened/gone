# Upgrade and maintain

Keeping a Gone instance running: upgrading it, what it keeps on disk, why it needs no backups, and how to tell it's healthy. To set one up, start with [Self-hosting Gone](deploy/README.md).

## Upgrading

Every release is listed on the [releases page](https://github.com/haukened/gone/releases). Releases follow [Semantic Versioning](https://semver.org/), and each release's notes summarise its changes and any upgrade steps.

1. Read the release notes for the versions you're skipping.
2. [Verify](verify.md) the new release.
3. Upgrade the way you installed it:

| Installed with | Upgrade |
| --- | --- |
| [Docker Compose](deploy/compose.md) | Change the Gone image tag in `compose.yaml`, then `docker compose pull && docker compose up -d` |
| [Docker](deploy/docker.md) | Pull the new tag, then recreate the container with the same volume and settings |
| [Kubernetes](deploy/kubernetes.md) | `helm upgrade` to the new chart version. The pod is replaced, not rolled, so expect a few seconds of downtime |
| [Linux packages](deploy/packages.md) | Install the new release's package the same way as the first. It keeps `/etc/default/gone` and restarts the service |

On startup, Gone brings its database up to date by itself, so there is nothing to run by hand. Secrets waiting at the time of the upgrade stay openable.

**Going back:** an older release refuses to start on a database a newer one has changed (`database schema is newer than this binary supports`). If an upgrade goes wrong, fix forward, or start the older release with an empty data directory. That loses the secrets that were waiting, which their senders can send again.

## What Gone stores

Everything is in the data directory, `GONE_DATA_DIR` (`/data` in the image, `/var/lib/gone` from a package):

- `gone.db`, an SQLite database (with its `-wal` and `-shm` files): each waiting secret's ciphertext or a pointer to it, sizes, times, and hashes of its tokens. Requests, and running totals for metrics.
- `blobs/`: the ciphertext of secrets too large to keep in the database.

No keys, plaintext, IP addresses or identities are ever written. A secret's row and blob are deleted when it's opened, revoked or expires; the janitor removes expired ones within a minute. The details are in [What the server stores](security.md#what-the-server-stores).

## Backups

**Gone's data doesn't need backing up, and we recommend not backing it up.**

- Everything in it is short-lived. A secret lasts at most your longest expiry (`GONE_TTL_OPTIONS`, 24 hours by default), and a reply to a request at most twice that.
- Losing the data directory loses only the secrets still waiting. Their senders see them disappear and can send them again.
- **Restoring a backup brings secrets back.** A secret opened or revoked after the backup was taken becomes openable again until it expires, by anyone who still has its link. That breaks Gone's promise that a link opens a secret once.

What's worth keeping is your configuration: `compose.yaml` and `.env`, your Helm values file, or `/etc/default/gone`, and any branding files. Keep those in version control or your usual configuration backups.

If you must back up the data directory anyway (to satisfy a policy, say), keep each copy for less than your longest expiry, and treat a restore as an incident: everyone who sent a secret since the backup should assume it can be opened again.

## Health

- `/healthz` answers `200 ok` while the process is up.
- `/readyz` answers `200 ready` when the database and blob directory are usable, and `503` otherwise. The result is cached for 2 seconds.
- The container image's `HEALTHCHECK` runs `goned healthcheck`, which calls `/readyz`, so `docker ps` shows whether Gone is healthy. The Helm chart uses the same two endpoints for its probes.

Point uptime monitoring at `/readyz` through your proxy, so it also checks HTTPS.

## Logs

Gone logs to standard error as text lines (`docker logs`, `journalctl -u gone`, or `kubectl logs`). It never logs client addresses, secret IDs, tokens or plaintext. Expect a few lines at startup (the listen address and rate limits), a pair of lines for each secret created or opened, tagged with a random ID for that request rather than the secret's, a `janitor` line every minute, and warnings for problems such as a short metrics token.

For counts and timings, turn on [Metrics](metrics.md).

## Disk

Disk use grows with the number of secrets waiting at once, up to `GONE_MAX_BYTES` each, and shrinks as they're opened or expire. SQLite reuses freed space, so the database file doesn't shrink on its own after a busy period. That's normal. Gone runs with `synchronous=FULL`, so a confirmed open survives a power cut. It works on a slow disk, but each secret then takes longer to create and open.
