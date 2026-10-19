#!/usr/bin/env bash
# Checks install.sh against the release in dist/ (scripts/release.sh),
# served locally the way GitHub serves releases (server.py): a fresh install,
# a pinned version, an update, a tampered archive, a cut-off script, an
# unknown architecture, and a release that doesn't exist. Nothing may be
# installed when a check fails. Needs bash, curl and python3. CI runs it on
# Linux and macOS (build.yaml); locally: task install-test.
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
[ -f dist/SHA256SUMS ] || { echo "build a release first: scripts/release.sh v0.0.0-dev"; exit 1; }
# The release's tag, from its CLI archive's name.
tag=$(sed -n 's/.*gone_\(v[^_]*\)_linux_amd64\.tar\.gz$/\1/p' dist/SHA256SUMS)

port=18765
work=$(mktemp -d)
pids=()
# shellcheck disable=SC2329 # called by the EXIT trap below
cleanup() {
	for p in "${pids[@]}"; do kill "$p" 2>/dev/null; done
	rm -rf "$work"
}
trap cleanup EXIT

failed=0
check() { # name, want, got
	if [ "$2" = "$3" ]; then echo "PASS $1"; else echo "FAIL $1: want $2, got $3"; failed=1; fi
}
serve() { # dir, port: serves dir as a release and waits until it answers
	python3 -I test/install/server.py "$1" "$tag" "$2" &
	pids+=($!)
	for _ in $(seq 50); do curl -s -o /dev/null "http://127.0.0.1:$2/releases/latest" && return 0; sleep 0.1; done
	echo "FAIL server on $2 didn't start"; failed=1
}
# install <dir> <releases url> [args...]: runs install.sh as curl | sh would,
# with no GitHub CLI sign-in, and prints its exit status.
install_to() {
	local dir=$1 url=$2; shift 2
	env -u GH_TOKEN -u GITHUB_TOKEN GH_CONFIG_DIR="$work/gh" GONE_RELEASES_URL="$url" \
		sh -s -- --dir "$dir" "$@" <install.sh >"$work/out" 2>&1
	echo $?
}
installed() { # dir: prints the installed version, or "none"
	if [ -x "$1/gone" ]; then "$1/gone" version | awk '{print $2}'; else echo none; fi
}

serve dist "$port"
url=http://127.0.0.1:$port/releases

check "installs the latest release" 0 "$(install_to "$work/a" "$url")"
check "the installed gone runs" "$tag" "$(installed "$work/a")"
check "says the provenance check was skipped" yes "$(grep -q 'provenance check skipped' "$work/out" && echo yes || echo no)"
check "says the directory isn't on PATH" yes "$(grep -q "isn't on your PATH" "$work/out" && echo yes || echo no)"
check "installs a pinned version" 0 "$(install_to "$work/b" "$url" --version "$tag")"
check "accepts a version without the v" 0 "$(install_to "$work/b" "$url" --version "${tag#v}")"
check "a release that doesn't exist fails" 1 "$(install_to "$work/c" "$url" --version v9.9.9)"
check "  and installs nothing" none "$(installed "$work/c")"

# An older gone already installed is replaced and reported.
mkdir -p "$work/d"
printf '#!/bin/sh\necho "gone v0.0.1"\n' >"$work/d/gone"
chmod 0755 "$work/d/gone"
install_to "$work/d" "$url" >/dev/null
check "replaces an older gone" "$tag" "$(installed "$work/d")"
check "  and reports the update" yes "$(grep -q "from v0.0.1 to $tag" "$work/out" && echo yes || echo no)"

# A tampered archive fails the checksum and installs nothing.
mkdir -p "$work/tampered"
cp dist/* "$work/tampered/"
for f in "$work"/tampered/gone_"$tag"_*.tar.gz; do printf 'x' >>"$f"; done
serve "$work/tampered" $((port + 1))
check "a tampered archive fails" 1 "$(install_to "$work/e" "http://127.0.0.1:$((port + 1))/releases")"
check "  and installs nothing" none "$(installed "$work/e")"
check "  and says why" yes "$(grep -q "doesn't match SHA256SUMS" "$work/out" && echo yes || echo no)"

# A download cut off part-way runs nothing: main() is called on the last line.
head -c "$(($(wc -c <install.sh) - 20))" install.sh >"$work/cut.sh"
env GONE_RELEASES_URL="$url" sh -s -- --dir "$work/f" <"$work/cut.sh" >/dev/null 2>&1
check "a cut-off script installs nothing" none "$(installed "$work/f")"
head -c "$(($(wc -c <install.sh) / 2))" install.sh >"$work/half.sh"
env GONE_RELEASES_URL="$url" sh -s -- --dir "$work/f" <"$work/half.sh" >/dev/null 2>&1
check "a half-downloaded script installs nothing" none "$(installed "$work/f")"
check "--help works when piped" yes "$(sh -s -- --help <install.sh 2>&1 | grep -q 'usage: install.sh' && echo yes || echo no)"

# An unknown architecture fails with a message.
mkdir -p "$work/bin"
# shellcheck disable=SC2016 # $1 belongs to the stub uname
printf '#!/bin/sh\ncase "$1" in -m) echo riscv64 ;; *) echo Linux ;; esac\n' >"$work/bin/uname"
chmod 0755 "$work/bin/uname"
status=$(PATH="$work/bin:$PATH" install_to "$work/g" "$url")
check "an unknown architecture fails" 1 "$status"
check "  and says so" yes "$(grep -q 'no release for riscv64' "$work/out" && echo yes || echo no)"

if [ "$failed" = 0 ]; then echo "ALL PASSED"; else echo "SOME CHECKS FAILED"; cat "$work/out"; fi
exit "$failed"
