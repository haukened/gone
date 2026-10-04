#!/usr/bin/env bash
# Builds release archives and SHA256SUMS into dist/.
#
#   scripts/release.sh <tag>
#
# The gone CLI is built for linux, darwin and windows on amd64 and arm64;
# the goned server for linux on amd64 and arm64. Binaries are static
# (CGO_ENABLED=0), stripped and path-trimmed. goned embeds the minified web
# assets, so run `task minify` first. Each archive holds one binary plus
# LICENSE and README.md. dist/RELEASE_NOTES.md holds install instructions
# for the GitHub release body.
set -euo pipefail

tag="${1:-}"
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH[-PRERELEASE]" >&2
  exit 2
fi
if [[ ! -d web/dist ]]; then
  echo "web/dist missing: run 'task minify' first" >&2
  exit 1
fi

root="$(pwd)"
dist="$root/dist"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
rm -rf "$dist"
mkdir -p "$dist"

export CGO_ENABLED=0
ldflags="-s -w -buildid="

# package <name> <goos> <goarch> <pkg> [build flags...] builds one binary
# and archives it with the license and readme.
package() {
  local name="$1" goos="$2" goarch="$3" pkg="$4"
  shift 4
  local base="${name}_${tag}_${goos}_${goarch}" exe="$name"
  [[ "$goos" == windows ]] && exe="$name.exe"
  mkdir -p "$work/$base"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath "$@" -o "$work/$base/$exe" "$pkg"
  cp LICENSE README.md "$work/$base/"
  if [[ "$goos" == windows ]]; then
    (cd "$work" && zip -qrX "$dist/$base.zip" "$base")
  else
    tar -C "$work" -czf "$dist/$base.tar.gz" "$base"
  fi
  echo "built $base"
}

for goos in linux darwin windows; do
  for goarch in amd64 arm64; do
    package gone "$goos" "$goarch" ./cmd/gone -ldflags "$ldflags -X main.version=$tag"
  done
done
for goarch in amd64 arm64; do
  package goned linux "$goarch" ./cmd/goned -tags=prod -ldflags "$ldflags"
done

cd "$dist"
if command -v sha256sum >/dev/null; then
  sha256sum -- * > SHA256SUMS
else
  shasum -a 256 -- * > SHA256SUMS
fi
cat SHA256SUMS

# Release notes are written after SHA256SUMS so they are not listed in it.
cat > RELEASE_NOTES.md <<EOF
## Install the \`gone\` CLI

Download the archive for your platform, verify it, and put \`gone\` on your PATH:

\`\`\`sh
TAG=${tag}
ARCHIVE=gone_\${TAG}_linux_amd64.tar.gz   # or darwin_arm64, windows_amd64.zip, ...
curl -fsSLO "https://github.com/haukened/gone/releases/download/\${TAG}/\${ARCHIVE}"
curl -fsSLO "https://github.com/haukened/gone/releases/download/\${TAG}/SHA256SUMS"
gh attestation verify "\$ARCHIVE" --repo haukened/gone
sha256sum --ignore-missing -c SHA256SUMS   # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS
tar -xzf "\$ARCHIVE" && install -m 0755 "\${ARCHIVE%.tar.gz}/gone" ~/.local/bin/gone
\`\`\`

See [docs/cli.md](https://github.com/haukened/gone/blob/${tag}/docs/cli.md) for usage. The server image is published to \`ghcr.io/haukened/gone:${tag}\`.
EOF
