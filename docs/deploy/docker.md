# Running Gone with Docker

Gone ships as a multi-arch container image (`linux/amd64`, `linux/arm64`): `ghcr.io/haukened/gone`.

> [!IMPORTANT]
> **Gone needs HTTPS.** Browsers only allow the encryption Gone uses (WebCrypto) on secure pages. Over plain `http://`, Gone shows a warning, turns off sending, and won't open links. The one exception is `http://localhost` on the machine running Gone, which browsers treat as secure. To serve other machines, put Gone behind a proxy that terminates TLS: see [Docker Compose with Caddy](compose.md) or [Behind a reverse proxy](reverse-proxy.md).

## Try it on this machine

```sh
docker run --rm -p 8080:8080 ghcr.io/haukened/gone:latest
```

Visit http://localhost:8080, type a secret or attach files, pick an expiry, and copy the link. The recipient opens the link, the secret shows once, and the server deletes it as soon as their browser confirms it received everything.

This only works from the same machine. Another device visiting `http://<this-ip>:8080` gets an insecure page, so sending is turned off there.

## Tags

| Tag | What it is |
| --- | ---------- |
| `vX.Y.Z` (for example `v3.10.3`) | One release. Pin this in production. |
| `latest` | The newest release. Pre-release tags never move `latest`. |

## Data

Everything Gone stores lives in `/data` inside the container: the SQLite database (`gone.db`) and a `blobs/` directory for larger secrets. Mount a volume there to keep secrets across restarts:

```sh
docker run -d --name gone \
  -p 127.0.0.1:8080:8080 \
  -v gone-data:/data \
  ghcr.io/haukened/gone:v3.10.3
```

The image runs as UID `65532` with no shell or package manager, and works with a read-only root filesystem (`--read-only`), since it writes only to `/data`. Back up the volume while Gone is stopped, or accept that a live copy may hold secrets that expire soon after. Everything in it is ciphertext.

Settings are environment variables (`-e GONE_...`). The full list is in [Configuration](../configuration.md).

## Health

The image has a Docker `HEALTHCHECK` that runs `goned healthcheck` every 30 seconds. It is a one-shot probe, not a second server. It calls the running server's `/readyz` on `GONE_ADDR`, using loopback when the address is `:port` or unspecified. It exits `0` when ready and `1` otherwise, and `docker ps` shows the result.

Kubernetes and other orchestrators can probe the endpoints directly:
- `/healthz` (`200 ok` while the process is up);
- `/readyz` (`200 ready` when the database and blob directory are usable, else `503`; the result is cached for 2 seconds).

## Metrics

Metrics are off by default. To turn them on, publish a second port on the host's loopback address and set both variables:

```sh
docker run --rm \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:9090:9090 \
  -e GONE_METRICS_ADDR=0.0.0.0:9090 \
  -e GONE_METRICS_TOKEN="$(openssl rand -base64 24)" \
  ghcr.io/haukened/gone:latest
```

The token should be at least 16 characters (a future release will require it); `openssl rand -base64 24` makes a good one (keep it, since Prometheus needs the same value). `GONE_METRICS_ADDR=0.0.0.0:9090` is the address *inside* the container, which Docker needs in order to forward the published port. The `127.0.0.1:` in `-p` is what keeps it off the network. Without Docker, bind `GONE_METRICS_ADDR=127.0.0.1:9090` directly.

See [Metrics](../metrics.md) for the Prometheus setup and the list of metrics.

## Verifying the image

Every release image is built by the release workflow with an SBOM and SLSA provenance, and GitHub signs an attestation for its digest. Check that an image came from this repository's workflow before running it:

```sh
gh attestation verify oci://ghcr.io/haukened/gone:v3.10.3 --repo haukened/gone
```

## Building the image yourself

The `Dockerfile` uses [Docker Hardened Images](https://dhi.io): `dhi.io/golang` to build and the distroless `dhi.io/static` to run. Both are pinned by digest and kept current by Dependabot. Pulling them needs a free Docker Hub account:

```sh
docker login dhi.io
docker build -t gone .
```

The release workflow builds the published images the same way, so it needs the repository secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` to pull the base images.
