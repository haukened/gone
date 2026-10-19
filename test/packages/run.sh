#!/usr/bin/env bash
# Checks the .deb and .rpm packages in dist/ (scripts/release.sh): lintian
# and rpmlint, then on Debian 13, Ubuntu 24.04 and Fedora 43 containers that
# boot systemd: install both packages, start the server and reach it, check
# it runs as an unprivileged dynamic user with a private data directory,
# reinstall (the upgrade path) keeping edited settings, and remove. Needs
# Docker and permission to run privileged containers. CI runs it
# (build.yaml); locally: task packages.
#
#   KEEP=1  leave the containers running afterwards
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
shopt -s nullglob
debs=(dist/*_amd64.deb)
rpms=(dist/*.x86_64.rpm)
[ "${#debs[@]}" = 2 ] && [ "${#rpms[@]}" = 2 ] || { echo "build the packages first: scripts/release.sh v0.0.0-dev"; exit 1; }
# The release's tag, from its CLI archive's name.
tag=$(sed -n 's/.*gone_\(v[^_]*\)_linux_amd64\.tar\.gz$/\1/p' dist/SHA256SUMS)

failed=0
containers=()
check() { # name, want, got
	if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want $2, got $3"; failed=1; fi
}
# shellcheck disable=SC2329 # called by the EXIT trap below
cleanup() {
	[ "${KEEP:-}" = 1 ] && return
	for c in "${containers[@]}"; do docker rm -f "$c" >/dev/null 2>&1; done
}
trap cleanup EXIT

echo "== lint"
if out=$(docker run --rm -v "$PWD/dist:/dist:ro" debian:13 sh -c \
	'apt-get update -qq >/dev/null && apt-get install -y -qq lintian >/dev/null 2>&1 && lintian --fail-on error --tag-display-limit 0 /dist/*.deb' 2>&1); then
	echo "PASS lintian"
else
	echo "FAIL lintian"; failed=1
fi
grep -E '^[EW]: ' <<<"$out" | sed 's/^/  /'
if out=$(docker run --rm -v "$PWD/dist:/dist:ro" -v "$PWD/test/packages/rpmlintrc:/rpmlintrc:ro" fedora:43 sh -c \
	'dnf -q -y install rpmlint >/dev/null 2>&1 && rpmlint -r /rpmlintrc /dist/*.rpm' 2>&1); then
	echo "PASS rpmlint"
else
	echo "FAIL rpmlint"; failed=1
fi
grep -E ': [EW]: ' <<<"$out" | sed 's/^/  /'

# boot <name> <image> <install command>: starts a container running systemd
# with curl, ready for the packages.
boot() {
	local name=$1 image=$2 setup=$3
	docker build -q -t "gone-pkg-$name" - >/dev/null <<DOCKERFILE || return 1
FROM $image
RUN $setup
CMD ["/sbin/init"]
DOCKERFILE
	docker rm -f "gone-pkg-$name" >/dev/null 2>&1
	docker run -d --name "gone-pkg-$name" --privileged --cgroupns=private --tmpfs /run --tmpfs /tmp \
		-v "$PWD/dist:/dist:ro" "gone-pkg-$name" >/dev/null || return 1
	containers+=("gone-pkg-$name")
	for _ in $(seq 60); do
		case $(docker exec "gone-pkg-$name" systemctl is-system-running 2>/dev/null) in
		running | degraded) return 0 ;;
		esac
		sleep 1
	done
	echo "FAIL $name: systemd didn't start"; failed=1; return 1
}

# on <command>: runs a shell command in the current container ($ctr).
on() { docker exec "$ctr" sh -c "$1"; }

# exercise <name> <install> <reinstall> <remove-server>: the same checks on
# every distribution.
exercise() {
	ctr="gone-pkg-$1"
	local install=$2 reinstall=$3 remove=$4
	echo "== $1"
	local msg
	msg=$(on "$install" 2>&1)
	check "$1: installs" 0 "$?"
	check "$1: gone version" "$tag" "$(on 'gone version' | awk '{print $2}')"
	check "$1: install says how to start it" yes "$(grep -q 'systemctl enable --now gone' <<<"$msg" && echo yes || echo no)"
	check "$1: not started on install" inactive "$(on 'systemctl is-active gone' 2>/dev/null)"
	on 'systemd-analyze verify /usr/lib/systemd/system/gone.service' >/dev/null 2>&1
	check "$1: systemd-analyze verify" 0 "$?"
	on 'echo GONE_RATE_BURST=7 >>/etc/default/gone && systemctl enable --now gone' >/dev/null 2>&1
	local ready=no
	for _ in $(seq 30); do on 'curl -sf http://127.0.0.1:8080/readyz' >/dev/null 2>&1 && { ready=yes; break; }; sleep 1; done
	check "$1: serves /readyz on 127.0.0.1:8080" yes "$ready"
	check "$1: page shows the release" yes "$(grep -q "$tag" <<<"$(on 'curl -s http://127.0.0.1:8080/')" && echo yes || echo no)"
	check "$1: not listening beyond localhost" refused "$(on "curl -s -o /dev/null --max-time 3 http://\$(hostname -i | awk '{print \$1}'):8080/healthz && echo open || echo refused")"
	local pid user
	pid=$(on 'systemctl show -p MainPID --value gone')
	user=$(on "stat -c %U /proc/$pid")
	check "$1: runs as a dynamic user, not root" yes "$([ -n "$user" ] && [ "$user" != root ] && echo yes || echo no)"
	check "$1: data directory is private" 700 "$(on 'stat -L -c %a /var/lib/gone')"
	on "$reinstall" >/dev/null 2>&1
	check "$1: reinstall (upgrade path)" 0 "$?"
	check "$1: still running after the upgrade" active "$(on 'systemctl is-active gone')"
	check "$1: edited settings kept" yes "$(on 'grep -q GONE_RATE_BURST=7 /etc/default/gone' && echo yes || echo no)"
	on "$remove" >/dev/null 2>&1
	check "$1: removes" 0 "$?"
	check "$1: stopped on removal" no "$(on 'systemctl is-active --quiet gone' && echo yes || echo no)"
	check "$1: data kept on removal" yes "$(on 'test -d /var/lib/private/gone' && echo yes || echo no)"
}

apt_setup='apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv curl >/dev/null && rm -rf /var/lib/apt/lists/*'
apt_install='apt-get install -y -qq /dist/gone_*_amd64.deb /dist/gone-server_*_amd64.deb'
apt_reinstall='apt-get install -y -qq --reinstall /dist/gone-server_*_amd64.deb'
apt_remove='apt-get remove -y -qq gone-server'
for distro in debian:13 ubuntu:24.04; do
	name=${distro//[:.]/-}
	boot "$name" "$distro" "$apt_setup" && exercise "$name" "$apt_install" "$apt_reinstall" "$apt_remove"
done
if boot fedora-43 fedora:43 'dnf -q -y install systemd curl procps-ng hostname && dnf clean all'; then
	exercise fedora-43 'dnf -y install /dist/gone-*.x86_64.rpm' \
		'dnf -y -q reinstall /dist/gone-server-*.x86_64.rpm' 'dnf -y -q remove gone-server'
fi

if [ "$failed" = 0 ]; then echo "ALL PASSED"; else echo "SOME CHECKS FAILED"; fi
exit "$failed"
