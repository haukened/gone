#!/usr/bin/env bash
# Static checks for the Helm chart, without a cluster: helm lint, every
# scenario in charts/gone/ci rendered and validated against the Kubernetes
# schemas, and the settings the chart must refuse. Needs helm and
# kubeconform on PATH. KUBE_VERSIONS overrides the versions checked.
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
chart=charts/gone
versions=${KUBE_VERSIONS:-1.35.0 1.36.0 1.37.0}
crds='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
apis=(--api-versions monitoring.coreos.com/v1/ServiceMonitor --api-versions gateway.networking.k8s.io/v1/HTTPRoute)

failed=0
pass() { echo "PASS $1"; }
fail() { echo "FAIL $1"; failed=1; }

if helm lint --strict "$chart" >/dev/null; then pass "helm lint"; else fail "helm lint"; helm lint --strict "$chart"; fi
for values in "$chart"/ci/*-values.yaml; do
	name=$(basename "$values" -values.yaml)
	# helm lint can't be told about CRDs, so the ServiceMonitor check logs
	# its refusal there; the render below passes the CRD APIs.
	if ! helm lint --strict "$chart" -f "$values" >/dev/null 2>&1; then fail "helm lint $name"; continue; fi
	for v in $versions; do
		if helm template t "$chart" -f "$values" "${apis[@]}" |
			kubeconform -strict -summary -kubernetes-version "$v" -schema-location default -schema-location "$crds" >/dev/null; then
			pass "$name on Kubernetes $v"
		else
			fail "$name on Kubernetes $v"
		fi
	done
done

refuses() { # name, wanted message, helm args...
	local name=$1 want=$2; shift 2
	local out
	if out=$(helm template t "$chart" "$@" 2>&1); then fail "$name: rendered"; return; fi
	if [[ $out == *"$want"* ]]; then pass "$name"; else fail "$name: $out"; fi
}
for p in authelia authentik tailscale custom; do
	refuses "refuses auth.provider=$p" "plain-header sign-in isn't supported on Kubernetes" --set auth.provider="$p"
done
refuses "refuses GONE_AUTH in extraEnv" "set sign-in through auth, not extraEnv" --set-json 'extraEnv=[{"name":"GONE_AUTH","value":"custom"}]'
refuses "refuses GONE_AUTH_HEADER in extraEnv" "set sign-in through auth, not extraEnv" --set-json 'extraEnv=[{"name":"GONE_AUTH_HEADER","value":"X-User"}]'
refuses "refuses GONE_DATA_DIR in extraEnv" "the chart sets this itself" --set-json 'extraEnv=[{"name":"GONE_DATA_DIR","value":"/data"}]'
refuses "refuses replicaCount=2" "Gone runs one replica" --set replicaCount=2
refuses "refuses an unknown provider" "unknown provider" --set auth.provider=okta
refuses "refuses an unknown value" "additional properties" --set replicas=2
refuses "refuses metrics without a token" "needs metrics.token.existingSecret" --set metrics.enabled=true
refuses "refuses a route without a Gateway" "needs httpRoute.parentRefs" --set httpRoute.enabled=true

if [ "$failed" = 0 ]; then echo "ALL PASSED"; else echo "SOME CHECKS FAILED"; fi
exit "$failed"
