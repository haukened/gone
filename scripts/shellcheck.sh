#!/usr/bin/env bash
# Runs shellcheck v0.11.0, pinned by digest, on every shell script, the
# same way locally and in CI. install.sh and the package scripts are POSIX
# sh; the rest are bash. Needs Docker.
set -euo pipefail
cd "$(dirname "$0")/.." || exit 1
image=koalaman/shellcheck:v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d
run() { docker run --rm -v "$PWD:/mnt:ro" -w /mnt "$image" "$@"; }
run -s sh install.sh packaging/scripts/*.sh
run scripts/*.sh test/helm/*.sh test/packages/run.sh test/install/run.sh test/interop/signin/*.sh
echo "shellcheck: clean"
