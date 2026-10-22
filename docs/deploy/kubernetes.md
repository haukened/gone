# Running Gone on Kubernetes

Gone has a Helm chart that runs it on Kubernetes, with Gateway API routing, one replica and its own storage.

The chart is published beside the image as an OCI artifact: `oci://ghcr.io/haukened/charts/gone`. Its version follows Gone's, so chart `3.13.0` runs image `v3.13.0`. It needs Kubernetes 1.34 or later and Helm 3.8 or later. It's tested on kind with Kubernetes 1.34 and 1.37 and Helm 4, routed through Envoy Gateway.

> [!IMPORTANT]
> **Gone needs HTTPS.** Browsers only allow the encryption Gone uses on secure pages, so serve it through a Gateway that terminates TLS. The one exception is `http://localhost`, which is why the port-forward below works for a first try.

## Install

```sh
helm install gone oci://ghcr.io/haukened/charts/gone --version 3.13.0 \
  --namespace gone --create-namespace
kubectl --namespace gone port-forward service/gone 8080:80
```

Then open http://localhost:8080 on the same machine.

The chart is signed by the release workflow. To check it came from this repository before you install it:

```sh
gh attestation verify oci://ghcr.io/haukened/charts/gone:3.13.0 --repo haukened/gone
```

Settings go in a values file (`helm install ... -f values.yaml`). Every setting is listed with its default in the chart's [`values.yaml`](../../charts/gone/values.yaml), and each one maps to a `GONE_*` variable in [Configuration](../configuration.md). The chart checks values against a schema, so a misspelt setting stops the install instead of being ignored.

## What the chart sets up

- **One Deployment with one pod.** Gone always runs as a single replica; see [One replica](#one-replica).
- **A PersistentVolumeClaim** (1 GiB, ReadWriteOnce) for the database and stored secrets.
- **A ClusterIP Service** on port 80, plus the metrics port when metrics are on.
- **Optional:** an HTTPRoute, a NetworkPolicy and a Prometheus Operator ServiceMonitor.
- **Pod hardening.** The pod meets the Pod Security "restricted" level: it runs as UID 65532 with a read-only root filesystem and no capabilities, no service account token, and no Service environment variables.

## Exposing Gone with a Gateway

The chart routes through the [Gateway API](https://gateway-api.sigs.k8s.io/) with an HTTPRoute. You bring the Gateway, its controller and its certificate. The chart has no Ingress template; if you use one, route `/` to the `gone` Service on port 80.

This is how the tests run, with [Envoy Gateway](https://gateway.envoyproxy.io/):

```yaml
# gateway.yaml: one HTTPS listener; the certificate Secret comes from
# cert-manager or anywhere else.
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: eg
  namespace: envoy-gateway-system
spec:
  gatewayClassName: eg
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      hostname: gone.example.com
      tls:
        certificateRefs:
          - name: gone-example-com-tls
      allowedRoutes:
        namespaces:
          from: All
```

```yaml
# values.yaml
httpRoute:
  enabled: true
  parentRefs:
    - name: eg
      namespace: envoy-gateway-system
      sectionName: https
  hostnames: [gone.example.com]
config:
  trustedProxies: [10.244.0.0/16] # your pod network; see the next section
```

**Request size:** Gone accepts secrets up to `GONE_MAX_BYTES` (10 MiB by default). Envoy Gateway passes that through as it is; the tests send a 9 MB file through it. If your Gateway limits request bodies, allow at least `GONE_MAX_BYTES` plus 1 MiB.

## Client addresses and rate limits

Rate limits apply per visitor. Behind a Gateway, every request reaches Gone from a Gateway pod, so Gone needs to know which addresses may tell it the real visitor's address (`X-Forwarded-For`). That's `config.trustedProxies`, the chart's name for `GONE_TRUSTED_PROXIES`.

On Kubernetes, the Gateway's pods change address whenever they restart or move, so list your cluster's **pod network** rather than single addresses. kind and many clusters use `10.244.0.0/16`, and `kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}'` shows each node's share.

**What this trusts:** any pod in the cluster can tell Gone which address a request came from, and so choose which visitor's rate limit it counts against. With sign-in off, or with a signed sign-in provider, that's all it affects. It's a far smaller risk than every visitor sharing one budget, which is what happens without it. In that case Gone logs `ignoring X-Forwarded-For from untrusted peer`.

## One replica

Gone always runs exactly one pod, and the chart has no `replicaCount`:
- **One writer.** SQLite and the blob directory allow one writer. Gone's own safeguards (the write queue, and the guard that stops expired-secret cleanup from deleting a file still being saved) work within one process.
- **No network filesystems.** SQLite's write-ahead log needs a local filesystem, which rules out ReadWriteMany and NFS volumes.
- **Rate limits are in memory**, so each extra pod would hand out a full extra budget.

A second pod would lose or corrupt secrets, so setting `replicaCount` to anything but 1 is refused with this explanation. Upgrades use the `Recreate` strategy: the old pod stops before the new one starts, so an upgrade or settings change means a few seconds of downtime.

## Storage

Gone keeps its data in `/data/gone` on the volume, not in the volume's root. Gone makes its data directory private (mode `0700`) at startup, and on most volumes the root directory belongs to root, so Gone can't change it and would exit with `chmod /data: operation not permitted`. The chart sets `fsGroup: 65532`, which lets Gone create and own the subdirectory.

- **Size:** `persistence.size` (1 GiB by default). Secrets are deleted once opened or expired, so the volume only holds those still waiting.
- **Your own claim:** set `persistence.existingClaim`. It must be ReadWriteOnce.
- **No persistence:** `persistence.enabled: false` uses an `emptyDir`, and every restart loses every secret. It's for trying the chart only.
- **Backups:** don't back up or snapshot the volume. Restoring it makes secrets opened since the copy openable again until they expire. Keep your values file instead; see [Backups](../operating.md#backups).
- **Uninstalling** with `helm uninstall` deletes the claim the chart made, and every secret with it.

## Branding

Branding values (`brand.name`, `brand.accent` and the footer links) work as described in [Branding](../branding.md). The logo is a file, so put it in a ConfigMap first:

```sh
kubectl --namespace gone create configmap gone-logo --from-file=logo.svg
```

```yaml
brand:
  name: Acme Secrets
  logo:
    existingConfigMap: gone-logo
    key: logo.svg
```

Gone reads the logo once at startup. After changing the ConfigMap, restart Gone with `kubectl rollout restart deployment/gone`. Changing any chart value restarts it automatically.

## Metrics

Metrics are served on a separate port (9090) and always need a bearer token, which goes in a Secret:

```sh
kubectl --namespace gone create secret generic gone-metrics \
  --from-literal=token="$(openssl rand -base64 24)"
```

```yaml
metrics:
  enabled: true
  token:
    existingSecret: gone-metrics
  serviceMonitor:
    enabled: true # needs the Prometheus Operator
```

The HTTPRoute only routes the web port, so metrics never go through the Gateway. [Metrics](../metrics.md) lists what's reported.

## NetworkPolicy

`networkPolicy.enabled` limits who can reach Gone:
- **Web port:** only the peers in `networkPolicy.from`, for example the Gateway's namespace.
- **Metrics port:** only the peers in `networkPolicy.metricsFrom`.
- **Outbound:** DNS only, plus HTTPS when a sign-in provider needs to fetch its signing keys.

A policy only works if your cluster's network plugin enforces it. Many do, some don't, and nothing warns you either way. `helm test` runs a pod in Gone's namespace, so include that namespace in `from` if you use it.

## Sign-in

On Kubernetes, the chart supports the providers that send a **signed token**: `cloudflare`, `google-iap`, `pomerium`, `teleport` and `oidc`. Gone checks the token's signature, so it doesn't matter which pod a request comes from. [Sign-in for private instances](../sign-in.md) describes each one; on Kubernetes they go under `auth`:

```yaml
auth:
  provider: oidc
  issuer: https://auth.example.com
  audience: gone
  signInURL: /oauth2/start?rd={return}
```

**Plain-header providers are not supported on Kubernetes:** `authelia`, `authentik`, `tailscale` and `custom`. The chart refuses them, including through `extraEnv`. If Gone finds itself in a Kubernetes pod with one of them set anyway, it logs a warning at startup.

### Why plain-header sign-in isn't supported

A plain-header provider proves nothing by itself. The proxy adds a header such as `Remote-User: alice`, and Gone believes it only because the request came from an address in `GONE_TRUSTED_PROXIES`. That works when you can name the proxy's one address and nothing else can reach Gone. On Kubernetes you can't do either:

1. **The proxy has no fixed address.** Gateway and proxy pods get a new address every time they restart, move to another node or scale. The only list that keeps working is the whole pod network.
2. **Trusting the pod network trusts every pod.** Any pod in the cluster can then call Gone's Service directly with `Remote-User: admin` and be treated as signed in. That includes another team's app, a debugging pod, or a dependency with a vulnerability. Nothing about the request looks different from the real proxy's.
3. **Node addresses don't help.** Traffic from a NodePort or LoadBalancer Service usually reaches Gone from a node's address (`externalTrafficPolicy: Cluster`). Trusting the nodes would trust anything sent to any node port.
4. **A NetworkPolicy can't be counted on.** In principle, a policy allowing only the proxy's pods would close the gap. But it only works if the network plugin enforces it, and when it doesn't, nothing fails or warns. Gone can't tell the difference, so sign-in would be open while appearing to work.
5. **Signed tokens don't have this problem.** A forged token fails its signature check no matter who sends it, so use a signed provider instead (below).

The one arrangement that is safe is the proxy running as a sidecar in Gone's own pod, with `GONE_TRUSTED_PROXIES=127.0.0.1`. Other pods can still reach Gone, but never from `127.0.0.1`. The chart doesn't build that, and it isn't tested. Gone still logs its warning, but starts, so that you can build it yourself.

### What to use instead

| You have | Use |
| -------- | --- |
| Authelia | Authelia's OpenID Connect provider, with oauth2-proxy in front of Gone passing the ID token: `auth.provider: oidc` |
| authentik | authentik's OAuth2/OpenID provider, with oauth2-proxy the same way: `oidc` |
| Tailscale | [tsidp](https://github.com/tailscale/tsidp), Tailscale's OpenID Connect provider (experimental), with oauth2-proxy: `oidc` |
| Another proxy that sets a header (`custom`) | Its OpenID Connect or signed-token option, if it has one, with `oidc` or the matching provider |
| Cloudflare Access, Google IAP, Pomerium, Teleport | The provider of the same name; these already sign their tokens |

The oauth2-proxy recipe in [Sign-in](../sign-in.md#oauth2-proxy-with-an-openid-connect-provider) applies unchanged. Note which providers are [built but not yet tested against the live product](../sign-in.md#providers).

## Testing the chart

From a checkout:
- **`task helm-lint`** runs `helm lint`, renders every scenario in `charts/gone/ci` and validates it against the Kubernetes schemas, and checks that the refused settings are refused.
- **`task helm-test`** builds Gone, starts a kind cluster and installs the chart. It runs `helm test`, sends and opens secrets with the CLI, restarts the pod to check secrets survive, checks a signed provider refuses unsigned creates, and routes through Envoy Gateway. Use `ENVOY=0` to skip Envoy and `KEEP=1` to keep the cluster.

`test/helm/install-tools.sh DIR` installs the pinned helm, kind, kubectl and kubeconform after checking their checksums. CI runs both tasks on every pull request.
