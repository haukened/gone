#!/usr/bin/env bash
# Builds release archives and SHA256SUMS into dist/.
#
#   scripts/release.sh <tag>
#
# The gone CLI is built for linux, darwin and windows on amd64 and arm64;
# the goned server for linux on amd64 and arm64. Binaries are static
# (CGO_ENABLED=0), stripped and path-trimmed. goned embeds the minified web
# assets, so run `task minify` first. Each archive holds one binary plus
# LICENSE and README.md. The Linux binaries are also packaged as .deb and
# .rpm (gone and gone-server, with packaging/ and the nfpm pinned in
# tools/nfpm). dist/RELEASE_NOTES.md holds install instructions for the
# GitHub release body.
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
  package goned linux "$goarch" ./cmd/goned -tags=prod -ldflags "$ldflags -X main.version=$tag"
done

# changelog prints a one-entry nfpm changelog (chglog) pointing at the
# release, dated by the tagged commit so the packages are reproducible.
changelog() {
  cat <<EOF
- semver: ${tag#v}
  date: $(git log -1 --format=%aI)
  packager: David Haukeness <david@hauken.us>
  changes:
    - note: "Release $tag: https://github.com/haukened/gone/releases/tag/$tag"
EOF
}

# debian_changelog <package> prints the same entry as a Debian changelog.
debian_changelog() {
  local v="${tag#v}"
  printf '%s (%s) stable; urgency=medium\n\n  * Release %s: https://github.com/haukened/gone/releases/tag/%s\n\n -- David Haukeness <david@hauken.us>  %s\n' \
    "$1" "${v/-/\~}" "$tag" "$tag" "$(git log -1 --format=%aD)"
}

# nfpm_packages builds the .deb and .rpm packages for the CLI (gone) and the
# server (gone-server) from the binaries the archives hold, so the packages
# and archives are the same build. Configs are in packaging/.
nfpm_packages() {
  local version="${tag#v}" goarch name bin
  for goarch in amd64 arm64; do
    for name in gone gone-server; do
      bin="$work/gone_${tag}_linux_${goarch}/gone"
      [[ "$name" == gone-server ]] && bin="$work/goned_${tag}_linux_${goarch}/goned"
      # nfpm expands PKG_VERSION and PKG_ARCH itself but not file paths, so
      # the binary's path is filled in here. (GOARCH would also change the
      # architecture go tool builds nfpm for.)
      debian_changelog "$name" | gzip -9n >"$work/$name.changelog.gz"
      for packager in deb rpm; do
        sed -e "s|\${BIN}|$bin|" -e "s|\${CHANGELOG}|$work/changelog.yml|" \
          -e "s|\${DEB_CHANGELOG}|$work/$name.changelog.gz|" "packaging/$name.yaml" >"$work/$name.yaml"
        [[ "$packager" == deb ]] && sed -i '/^changelog:/d' "$work/$name.yaml"
        PKG_VERSION="$version" PKG_ARCH="$goarch" \
          go tool -modfile=tools/nfpm/go.mod nfpm package \
          --config "$work/$name.yaml" --packager "$packager" --target "$dist/" >/dev/null
      done
      echo "packaged $name $goarch"
    done
  done
}
changelog >"$work/changelog.yml"
nfpm_packages

cd "$dist"
if command -v sha256sum >/dev/null; then
  sha256sum -- * > SHA256SUMS
else
  shasum -a 256 -- * > SHA256SUMS
fi
cat SHA256SUMS

# Release notes are written after SHA256SUMS so they are not listed in it.
# Package versions put a pre-release after a tilde (3.13.0~rc.1).
version_deb=${tag#v}
version_deb=${version_deb/-/\~}
cat > RELEASE_NOTES.md <<EOF
## Install the \`gone\` CLI

On Linux or macOS, the install script downloads the right archive, checks it against \`SHA256SUMS\` (and its signed provenance when the GitHub CLI is signed in), and puts \`gone\` in \`~/.local/bin\`, without sudo:

\`\`\`sh
curl -fsSL https://getgone.dev/install.sh | sh -s -- --version ${tag}
\`\`\`

Or install a package (\`arm64\` / \`aarch64\` builds are attached too):

\`\`\`sh
sudo apt install ./gone_${version_deb}_amd64.deb      # Debian, Ubuntu
sudo dnf install ./gone-${version_deb}-1.x86_64.rpm   # Fedora, RHEL
\`\`\`

Or download an archive, verify it, and put \`gone\` on your PATH:

\`\`\`sh
TAG=${tag}
ARCHIVE=gone_\${TAG}_linux_amd64.tar.gz   # or darwin_arm64, windows_amd64.zip, ...
curl -fsSLO "https://github.com/haukened/gone/releases/download/\${TAG}/\${ARCHIVE}"
curl -fsSLO "https://github.com/haukened/gone/releases/download/\${TAG}/SHA256SUMS"
gh attestation verify "\$ARCHIVE" --repo haukened/gone
sha256sum --ignore-missing -c SHA256SUMS   # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS
tar -xzf "\$ARCHIVE" && install -m 0755 "\${ARCHIVE%.tar.gz}/gone" ~/.local/bin/gone
\`\`\`

On macOS, an archive downloaded in a browser is quarantined, and Gatekeeper stops the unsigned \`gone\` from running. Clear it with \`xattr -d com.apple.quarantine ~/.local/bin/gone\`, or use the install script, whose download isn't quarantined.

The signed build provenance covers every archive and package. It is also attached as \`gone_${tag}.sigstore.json\`, for offline checks with \`gh attestation verify "\$ARCHIVE" --repo haukened/gone --bundle gone_${tag}.sigstore.json\`. The same signed provenance statement is in \`gone_${tag}.intoto.jsonl\` (an in-toto DSSE envelope).

See [docs/cli.md](https://github.com/haukened/gone/blob/${tag}/docs/cli.md) for usage.

## Run the server

- **Docker:** \`ghcr.io/haukened/gone:${tag}\` ([guide](https://github.com/haukened/gone/blob/${tag}/docs/deploy/docker.md)).
- **Kubernetes:** \`helm install gone oci://ghcr.io/haukened/charts/gone --version ${tag#v}\` ([guide](https://github.com/haukened/gone/blob/${tag}/docs/deploy/kubernetes.md)).
- **A Linux package with a systemd service:** \`gone-server_${version_deb}_amd64.deb\` or \`gone-server-${version_deb}-1.x86_64.rpm\` ([guide](https://github.com/haukened/gone/blob/${tag}/docs/deploy/packages.md)).
EOF
