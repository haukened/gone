#!/usr/bin/env bash
# Installs the Helm chart on a kind cluster and checks it works: Pod
# Security "restricted", helm test, a secret sent and opened with the CLI,
# a secret that survives a pod restart, a signed sign-in provider refusing
# unsigned creates, and (ENVOY=1) routing through Envoy Gateway with
# trusted proxies. Needs Docker, kind, helm, kubectl, curl and a static
# bin/goned and bin/gone (task build-prod). CI runs it (build.yaml).
#
#   NODE_IMAGE  the kindest/node image; defaults to the newest tested
#   ENVOY=0     skip the Envoy Gateway checks
#   KEEP=1      leave the cluster running afterwards
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
for tool in docker kind helm kubectl curl; do
	command -v "$tool" >/dev/null || { echo "missing $tool"; exit 1; }
done
[ -x bin/goned ] && [ -x bin/gone ] || { echo "build bin/goned and bin/gone first: task build-prod"; exit 1; }

cluster=gone-helm
node=${NODE_IMAGE:-kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5}
envoy_version=v1.9.2
image=gone:helm-test
ns=gone
image_args=(--set image.repository=gone --set image.tag=helm-test --set image.pullPolicy=Never)

failed=0
check() { # name, want, got
	if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want $2, got $3"; failed=1; fi
}
# shellcheck disable=SC2329 # called by the EXIT trap below
cleanup() {
	[ -n "${pf:-}" ] && kill "$pf" 2>/dev/null
	[ "${KEEP:-}" = 1 ] || kind delete cluster --name "$cluster" >/dev/null 2>&1
}
trap cleanup EXIT

forward() { # namespace, service, local port: starts a port-forward in $pf
	[ -n "${pf:-}" ] && kill "$pf" 2>/dev/null
	kubectl -n "$1" port-forward "$2" "$3:80" >/dev/null 2>&1 &
	pf=$!
	for _ in $(seq 30); do curl -s -o /dev/null "http://localhost:$3/healthz" && return 0; sleep 1; done
	echo "FAIL port-forward to $2"; failed=1; return 1
}
roundtrip() { # port: sends a secret through the CLI and prints what opening it returns
	local origin=http://localhost:$1 link
	link=$(printf 'helm %s' "$1" | bin/gone send --insecure -s "$origin" --json | sed -n 's/.*"link":"\([^"]*\)".*/\1/p')
	bin/gone get --insecure --allow-origin "$origin" "$link"
}
create() { # port: prints the HTTP status of an unsigned create
	head -c 48 /dev/urandom | curl -s -o /dev/null -w '%{http_code}' -X POST "http://localhost:$1/api/secret" \
		-H 'X-Gone-Version: 1' -H 'X-Gone-Nonce: AAAAAAAAAAAAAAAA' -H 'X-Gone-TTL: 5m' \
		-H 'Content-Type: application/octet-stream' --data-binary @-
}

echo "== cluster ($node)"
docker build -q -f test/helm/Dockerfile -t "$image" bin >/dev/null || exit 1
kind create cluster --name "$cluster" --image "$node" --wait 120s >/dev/null 2>&1 || exit 1
kind load docker-image "$image" --name "$cluster" >/dev/null 2>&1 || exit 1
kubectl create namespace "$ns" >/dev/null
kubectl label namespace "$ns" pod-security.kubernetes.io/enforce=restricted >/dev/null

echo "== install"
if helm install gone charts/gone -n "$ns" "${image_args[@]}" --wait --timeout 3m >/dev/null; then
	echo "PASS installs under Pod Security restricted"
else
	echo "FAIL install"; failed=1
	kubectl -n "$ns" describe pods; kubectl -n "$ns" logs deploy/gone --tail 20
	exit 1
fi
check "helm test" 0 "$(helm test gone -n "$ns" >/dev/null 2>&1; echo $?)"
forward "$ns" service/gone 18080
check "send and open a secret" "helm 18080" "$(roundtrip 18080)"
link=$(printf 'kept' | bin/gone send --insecure -s http://localhost:18080 --json | sed -n 's/.*"link":"\([^"]*\)".*/\1/p')
kubectl -n "$ns" delete pod -l app.kubernetes.io/name=gone --wait >/dev/null
kubectl -n "$ns" rollout status deploy/gone --timeout 120s >/dev/null
forward "$ns" service/gone 18080
check "a secret survives a pod restart" kept "$(bin/gone get --insecure --allow-origin http://localhost:18080 "$link")"

echo "== signed sign-in (oidc)"
# An issuer that can't be reached: Gone starts, and refuses unsigned creates
# because no token can be verified.
helm upgrade gone charts/gone -n "$ns" --reuse-values --set auth.provider=oidc \
	--set auth.issuer=https://idp.invalid --set auth.audience=gone --wait --timeout 3m >/dev/null
forward "$ns" service/gone 18080
check "policy says sign-in is required" true "$(curl -s http://localhost:18080/api/policy | sed -n 's/.*"sign_in_required":\(true\|false\).*/\1/p')"
check "an unsigned create is refused" 401 "$(create 18080)"
helm upgrade gone charts/gone -n "$ns" --reuse-values --set auth.provider=none \
	--set auth.issuer= --set auth.audience= --wait --timeout 3m >/dev/null

if [ "${ENVOY:-1}" = 1 ]; then
	echo "== Envoy Gateway $envoy_version"
	helm install eg oci://docker.io/envoyproxy/gateway-helm --version "$envoy_version" \
		-n envoy-gateway-system --create-namespace --wait --timeout 5m >/dev/null || exit 1
	kubectl apply -f test/helm/gateway.yaml >/dev/null
	kubectl -n envoy-gateway-system wait --for=condition=Available deployment -l gateway.envoyproxy.io/owning-gateway-name=eg --timeout 180s >/dev/null 2>&1 ||
		{ sleep 10; kubectl -n envoy-gateway-system wait --for=condition=Available deployment -l gateway.envoyproxy.io/owning-gateway-name=eg --timeout 180s >/dev/null; }
	pods=$(kubectl get nodes -o jsonpath='{.items[0].spec.podCIDR}')
	helm upgrade gone charts/gone -n "$ns" --reuse-values --set httpRoute.enabled=true \
		--set 'httpRoute.parentRefs[0].name=eg' --set 'httpRoute.parentRefs[0].namespace=envoy-gateway-system' \
		--set "config.trustedProxies={$pods}" --wait --timeout 3m >/dev/null
	check "the route is accepted" True "$(kubectl -n "$ns" get httproute gone -o jsonpath='{.status.parents[0].conditions[?(@.type=="Accepted")].status}')"
	envoy=$(kubectl -n envoy-gateway-system get service -l gateway.envoyproxy.io/owning-gateway-name=eg -o name)
	forward envoy-gateway-system "$envoy" 18081
	check "send and open a secret through Envoy" "helm 18081" "$(roundtrip 18081)"
	# Near GONE_MAX_BYTES (10 MiB), so a proxy body limit would show.
	big=$(mktemp -d)
	head -c 9000000 /dev/urandom >"$big/big.bin"
	link=$(printf 'big' | bin/gone send --insecure -s http://localhost:18081 -f "$big/big.bin" --json | sed -n 's/.*"link":"\([^"]*\)".*/\1/p')
	mkdir "$big/out"
	bin/gone get --insecure --allow-origin http://localhost:18081 -o "$big/out" "$link" >/dev/null 2>&1
	check "a 9 MB file through Envoy" "$(sha256sum <"$big/big.bin")" "$(cat "$big"/out/big.bin 2>/dev/null | sha256sum)"
	rm -rf "$big"
	check "Gone believes Envoy's X-Forwarded-For" 0 "$(kubectl -n "$ns" logs deploy/gone | grep -c 'ignoring X-Forwarded-For')"
	helm upgrade gone charts/gone -n "$ns" --reuse-values --set-json 'config.trustedProxies=[]' --wait --timeout 3m >/dev/null
	forward envoy-gateway-system "$envoy" 18081
	curl -s -o /dev/null http://localhost:18081/api/secret/0123456789abcdef0123456789abcdef
	check "without trusted proxies, Gone ignores it and says so" 1 "$(kubectl -n "$ns" logs deploy/gone | grep -c 'ignoring X-Forwarded-For')"
fi

if [ "$failed" = 0 ]; then
	echo "ALL PASSED"
else
	echo "SOME CHECKS FAILED"
	kubectl -n "$ns" get pods; kubectl -n "$ns" logs deploy/gone --tail 20
fi
exit "$failed"
