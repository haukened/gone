#!/usr/bin/env bash
# Runs the Codacy CLI v2, downloading it on first use.
#
# The release is pinned and its archive is verified against the release's
# published checksums file (sha256sum -c) before it is extracted. To upgrade,
# change CODACY_CLI_V2_VERSION to a new tag from
# https://github.com/codacy/codacy-cli-v2/releases and review the diff.
# Usage: .codacy/cli.sh <codacy-cli-v2 arguments>   (or "download" to only fetch)

set -euo pipefail

CODACY_CLI_V2_VERSION="1.0.0-main.382.sha.473b61c"
release_url="https://github.com/codacy/codacy-cli-v2/releases/download/${CODACY_CLI_V2_VERSION}"

bin_name="codacy-cli-v2"

fatal() {
    echo "$*" >&2
    exit 1
}

os_name=$(uname | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
"x86_64") arch="amd64" ;;
"aarch64" | "arm64") arch="arm64" ;;
esac

case "$os_name" in
linux | darwin) ;;
*) fatal "Unsupported OS: $os_name" ;;
esac

if [ -z "${CODACY_CLI_V2_TMP_FOLDER:-}" ]; then
    if [ "$os_name" = "linux" ]; then
        CODACY_CLI_V2_TMP_FOLDER="$HOME/.cache/codacy/codacy-cli-v2"
    else
        CODACY_CLI_V2_TMP_FOLDER="$HOME/Library/Caches/Codacy/codacy-cli-v2"
    fi
fi

bin_folder="${CODACY_CLI_V2_TMP_FOLDER}/${CODACY_CLI_V2_VERSION}"
bin_path="${bin_folder}/${bin_name}"

fetch() {
    local url="$1"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -O "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url"
    else
        fatal "Error: Could not find curl or wget, please install one."
    fi
}

sha256_check() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c -
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 -c -
    else
        fatal "Error: Could not find sha256sum or shasum."
    fi
}

download_cli() {
    local archive="codacy-cli-v2_${CODACY_CLI_V2_VERSION}_${os_name}_${arch}.tar.gz"
    local checksums="codacy-cli-v2_${CODACY_CLI_V2_VERSION}_checksums.txt"
    local work
    work=$(mktemp -d)
    trap 'rm -rf "$work"' RETURN

    echo "Downloading Codacy CLI ${CODACY_CLI_V2_VERSION} (${os_name}/${arch})..."
    (
        cd "$work"
        fetch "${release_url}/${archive}"
        fetch "${release_url}/${checksums}"
        # Verify only the archive we downloaded; the file lists every platform.
        grep -F "  ${archive}" "$checksums" | sha256_check
    ) || fatal "Error: could not download or verify ${archive}."

    mkdir -p "$bin_folder"
    tar xzf "${work}/${archive}" -C "$bin_folder"
}

if [ ! -x "$bin_path" ]; then
    download_cli
    chmod +x "$bin_path"
fi

if [ "$#" -eq 1 ] && [ "$1" = "download" ]; then
    echo "Codacy cli v2 download succeeded"
else
    exec "$bin_path" "$@"
fi
