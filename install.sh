#!/bin/sh
# Installs the gone CLI from a GitHub release, for Linux and macOS:
#
#   curl -fsSL https://raw.githubusercontent.com/haukened/gone/main/install.sh | sh
#
# It downloads the archive for this machine, checks it against the
# release's SHA256SUMS, checks its signed build provenance when the GitHub
# CLI (gh) is signed in, and installs gone into ~/.local/bin. It never uses
# sudo. Read it first if you like: everything happens in main() at the end.
#
#   --version vX.Y.Z   install that release instead of the latest (GONE_VERSION)
#   --dir DIR          install into DIR (GONE_INSTALL_DIR; default ~/.local/bin)
#   --help             show this help
#
# GONE_RELEASES_URL replaces https://github.com/haukened/gone/releases, for
# mirrors and tests.

set -eu

REPO=haukened/gone

say() { printf 'gone install: %s\n' "$*" >&2; }
die() { say "$*"; exit 1; }

usage() {
	cat <<'EOF'
Installs the gone CLI from a GitHub release (Linux and macOS).

usage: install.sh [--version vX.Y.Z] [--dir DIR]

  --version vX.Y.Z   install that release instead of the latest (GONE_VERSION)
  --dir DIR          install into DIR (GONE_INSTALL_DIR; default ~/.local/bin)
  --help             show this help

Piped from curl:  curl -fsSL https://raw.githubusercontent.com/haukened/gone/main/install.sh | sh -s -- --dir DIR
EOF
}

# platform prints <os>_<arch> as the release names them.
platform() {
	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	MINGW* | MSYS* | CYGWIN* | Windows*) die "on Windows, download gone_<version>_windows_amd64.zip from https://github.com/$REPO/releases" ;;
	*) die "no release for $os; build from source: https://github.com/$REPO/blob/main/docs/cli.md" ;;
	esac
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "no release for $arch; build from source: https://github.com/$REPO/blob/main/docs/cli.md" ;;
	esac
	printf '%s_%s\n' "$os" "$arch"
}

# valid_tag reports whether $1 is a release tag (scripts/check-release-ref.sh).
valid_tag() {
	printf '%s\n' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'
}

# latest_tag prints the newest release's tag. It follows the redirect from
# /releases/latest instead of calling GitHub's API, which is rate limited.
latest_tag() {
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$releases/latest") ||
		die "couldn't reach $releases/latest"
	printf '%s\n' "${url##*/}"
}

# sha256 prints the SHA-256 of file $1.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

main() {
	version=${GONE_VERSION:-}
	dir=${GONE_INSTALL_DIR:-$HOME/.local/bin}
	releases=${GONE_RELEASES_URL:-https://github.com/$REPO/releases}
	while [ $# -gt 0 ]; do
		case "$1" in
		--version) [ $# -ge 2 ] || die "--version needs a value"; version=$2; shift 2 ;;
		--version=*) version=${1#*=}; shift ;;
		--dir) [ $# -ge 2 ] || die "--dir needs a value"; dir=$2; shift 2 ;;
		--dir=*) dir=${1#*=}; shift ;;
		-h | --help) usage; exit 0 ;;
		*) die "unknown option $1 (try --help)" ;;
		esac
	done

	command -v curl >/dev/null 2>&1 || die "needs curl"
	command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 ||
		die "needs sha256sum or shasum"
	target=$(platform)

	if [ -z "$version" ]; then
		version=$(latest_tag)
	fi
	case "$version" in v*) ;; *) version=v$version ;; esac
	valid_tag "$version" || die "not a release version: $version"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	name=gone_${version}_${target}
	archive=$name.tar.gz
	say "downloading gone $version for $target"
	curl -fsSL -o "$tmp/$archive" "$releases/download/$version/$archive" ||
		die "couldn't download $archive; is $version a release?"
	curl -fsSL -o "$tmp/SHA256SUMS" "$releases/download/$version/SHA256SUMS" ||
		die "couldn't download SHA256SUMS for $version"

	want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
	[ -n "$want" ] || die "SHA256SUMS has no entry for $archive"
	[ "$(sha256 "$tmp/$archive")" = "$want" ] ||
		die "$archive doesn't match SHA256SUMS; nothing was installed"
	say "checksum matches SHA256SUMS"

	if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
		gh attestation verify "$tmp/$archive" --repo "$REPO" >/dev/null 2>&1 ||
			die "$archive failed its provenance check (gh attestation verify); nothing was installed"
		say "provenance verified: built by $REPO's release workflow"
	else
		say "provenance check skipped (needs the GitHub CLI, signed in); to check by hand:"
		say "  gh attestation verify $archive --repo $REPO"
	fi

	tar -xzf "$tmp/$archive" -C "$tmp"
	[ -f "$tmp/$name/gone" ] || die "$archive doesn't contain gone"
	old=""
	if [ -x "$dir/gone" ]; then
		old=$("$dir/gone" version 2>/dev/null | awk '{print $2}') || old=""
	fi
	mkdir -p "$dir"
	cp "$tmp/$name/gone" "$dir/.gone.new.$$"
	chmod 0755 "$dir/.gone.new.$$"
	mv -f "$dir/.gone.new.$$" "$dir/gone"
	if [ -n "$old" ] && [ "$old" != "$version" ]; then
		say "updated $dir/gone from $old to $version"
	else
		say "installed $dir/gone ($version)"
	fi

	case ":$PATH:" in
	*":$dir:"*) say "run: gone help" ;;
	*) say "$dir isn't on your PATH; add this to your shell profile:"
	   say "  export PATH=\"$dir:\$PATH\"" ;;
	esac
}

main "$@"
