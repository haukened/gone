#!/usr/bin/env bash
# Sign-in interop checks against real proxies (docs/sign-in.md). Needs
# Docker, curl, jq and a static bin/goned (task build-prod). Not run in CI.
#
#   oidc stack:     Dex -> oauth2-proxy -> Gone (GONE_AUTH=oidc)
#   authelia stack: Authelia + Caddy forward_auth -> Gone (GONE_AUTH=authelia)
#
# Pass KEEP=1 to leave the stacks running afterwards.
set -uo pipefail
cd "$(dirname "$0")"
./gen-certs.sh
[ -x ../../../bin/goned ] || { echo "build bin/goned first: task build-prod"; exit 1; }

compose() { docker compose -f compose.yaml "$@"; }
cleanup() { [ "${KEEP:-}" = 1 ] || compose down -v --remove-orphans >/dev/null 2>&1; }
trap cleanup EXIT
compose up -d --quiet-pull >/dev/null || exit 1

failed=0
check() { # name, want, got
	if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want $2, got $3"; failed=1; fi
}
create() { # url, extra curl args...: prints the HTTP status of a create
	local url=$1; shift
	head -c 48 /dev/urandom | curl -s -o /dev/null -w '%{http_code}' -X POST "$url/api/secret" \
		-H 'X-Gone-Version: 1' -H 'X-Gone-Nonce: AAAAAAAAAAAAAAAA' -H 'X-Gone-TTL: 5m' \
		-H 'Content-Type: application/octet-stream' --data-binary @- "$@"
}
wait_for() { # url, curl args...
	for _ in $(seq 60); do curl -sf -o /dev/null "$@" && return 0; sleep 1; done
	echo "FAIL timed out waiting for $1"; failed=1; return 1
}

OIDC=http://127.0.0.1:14180
OIDC_DIRECT=http://127.0.0.1:18081
DEX=(--resolve dex:15556:127.0.0.1 --cacert certs/ca.pem)
CADDY=(--resolve gone.example.test:18443:127.0.0.1 --resolve auth.example.test:18443:127.0.0.1 -k)
GONE=https://gone.example.test:18443
AUTH=https://auth.example.test:18443
AUTHELIA_DIRECT=http://127.0.0.1:18082

echo "== oidc: Dex + oauth2-proxy"
wait_for "$OIDC/api/policy"
check "policy says sign-in is required" true "$(curl -s "$OIDC/api/policy" | jq -r .sign_in_required)"
check "create without a token is stopped" 0 "$([ "$(create "$OIDC")" = 201 ] && echo 1 || echo 0)"
token=$(curl -s "${DEX[@]}" https://dex:15556/dex/token -u gone:interop-only-client-secret \
	-d grant_type=password -d username=alice@example.com -d password=password -d 'scope=openid email profile' | jq -r .id_token)
check "Dex issued an ID token" 0 "$([ -z "$token" ] || [ "$token" = null ] && echo 1 || echo 0)"
for _ in $(seq 30); do [ "$(create "$OIDC_DIRECT" -H "Authorization: Bearer $token")" = 201 ] && break; sleep 1; done
check "create through oauth2-proxy with the ID token" 201 "$(create "$OIDC" -H "Authorization: Bearer $token")"
check "create straight to Gone with the ID token (signed, so the network doesn't matter)" 201 "$(create "$OIDC_DIRECT" -H "Authorization: Bearer $token")"
check "create straight to Gone with a forged token" 401 "$(create "$OIDC_DIRECT" -H "Authorization: Bearer ${token%.*}.AAAA")"
check "create straight to Gone with no token" 401 "$(create "$OIDC_DIRECT")"

echo "== authelia: Authelia + Caddy forward_auth"
wait_for "$GONE/api/policy" "${CADDY[@]}"
check "an open path needs no sign-in" true "$(curl -s "${CADDY[@]}" "$GONE/api/policy" | jq -r .sign_in_required)"
check "create without a session is stopped" 0 "$([ "$(create "$GONE" "${CADDY[@]}")" = 201 ] && echo 1 || echo 0)"
check "create without a session but a forged Remote-User is stopped" 0 "$([ "$(create "$GONE" "${CADDY[@]}" -H 'Remote-User: mallory')" = 201 ] && echo 1 || echo 0)"
jar=$(mktemp)
login=$(curl -s "${CADDY[@]}" -c "$jar" -o /dev/null -w '%{http_code}' "$AUTH/api/firstfactor" \
	-H 'Content-Type: application/json' -d '{"username":"alice","password":"password","keepMeLoggedIn":false}')
check "Authelia login" 200 "$login"
check "create through Caddy with a session" 201 "$(create "$GONE" "${CADDY[@]}" -b "$jar")"
check "create straight to Gone with a forged Remote-User (untrusted peer)" 401 "$(create "$AUTHELIA_DIRECT" -H 'Remote-User: mallory')"
rm -f "$jar"

[ "$failed" = 0 ] && echo "ALL PASSED" || { echo "SOME CHECKS FAILED"; compose logs --tail 20; }
exit "$failed"
