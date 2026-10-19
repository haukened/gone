# Gone

[Gone](https://github.com/haukened/gone) sends a secret that can be opened once. It's encrypted in the browser or CLI, and the server never sees the key.

```sh
helm install gone oci://ghcr.io/haukened/charts/gone --version <version> \
  --namespace gone --create-namespace
```

The chart's version is Gone's: chart `3.13.0` runs image `v3.13.0`. Verify it with:

```sh
gh attestation verify oci://ghcr.io/haukened/charts/gone:<version> --repo haukened/gone
```

## Things to know

- **HTTPS is required.** Route Gone through a Gateway that terminates TLS (`httpRoute`). Browsers only run Gone's encryption on HTTPS pages.
- **One replica, always.** SQLite and the blob directory have one writer, so there is no `replicaCount`, and upgrades stop the old pod first.
- **Persistence** is a 1 GiB ReadWriteOnce claim. Without it, a restart loses every secret.
- **Set `config.trustedProxies` to your pod network**, so rate limits apply per visitor rather than per Gateway pod.
- **Sign-in:** only signed providers (`cloudflare`, `google-iap`, `pomerium`, `teleport`, `oidc`) are supported on Kubernetes. Plain-header providers are refused, because any pod in the cluster could forge the header; the guide explains why and what to use instead.

Every value is documented in [`values.yaml`](values.yaml), and the full guide is [Running Gone on Kubernetes](https://github.com/haukened/gone/blob/main/docs/deploy/kubernetes.md).
