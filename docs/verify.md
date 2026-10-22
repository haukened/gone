# Verify a release

Every Gone release is built by this repository's release workflow on GitHub Actions, from a version tag whose commit is already on `main`. The workflow signs a [build-provenance attestation](https://docs.github.com/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds) for everything it publishes, so you can check that a download was built from this repository's source, by its workflow, and wasn't changed afterwards.

The checks below use the [GitHub CLI](https://cli.github.com/) (`gh`), signed in to any GitHub account.

## What is signed

| Download | Where | Check |
| --- | --- | --- |
| CLI and server archives (`.tar.gz`, `.zip`) | [Releases](https://github.com/haukened/gone/releases) | [A CLI archive](#a-cli-archive) |
| Linux packages (`.deb`, `.rpm`) | [Releases](https://github.com/haukened/gone/releases) | [A package](#a-package) |
| Container image | `ghcr.io/haukened/gone` | [The container image](#the-container-image) |
| Helm chart | `oci://ghcr.io/haukened/charts/gone` | [The Helm chart](#the-helm-chart) |

Each release also has a `SHA256SUMS` file covering every file in it, and the provenance in two offline forms: `gone_<tag>.sigstore.json` (a Sigstore bundle, for `gh attestation verify ... --bundle`) and `gone_<tag>.intoto.jsonl` (the same statement as an in-toto DSSE envelope).

The [install script](cli.md#install-script-linux-and-macos) does these checks for you: it always checks the checksum, and checks the provenance whenever `gh` is installed and signed in.

## A CLI archive

```sh
TAG=v3.12.0
ARCHIVE=gone_${TAG}_linux_amd64.tar.gz

curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/${ARCHIVE}"
curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/SHA256SUMS"

# 1. The archive was built by this repository's release workflow.
gh attestation verify "$ARCHIVE" --repo haukened/gone

# 2. The archive matches the published checksum.
sha256sum --ignore-missing -c SHA256SUMS   # macOS: shasum -a 256 --ignore-missing -c SHA256SUMS

tar -xzf "$ARCHIVE"
install -m 0755 "gone_${TAG}_linux_amd64/gone" ~/.local/bin/gone
gone version
```

On macOS, an archive downloaded in a browser is quarantined, and Gatekeeper refuses to run the unsigned `gone`. After verifying it, clear the flag with `xattr -d com.apple.quarantine ~/.local/bin/gone`. The install script and `curl` downloads aren't quarantined.

## A package

```sh
TAG=v3.13.0
PKG=gone-server_${TAG#v}_amd64.deb      # or gone_… for the CLI, or the .rpm
curl -fsSLO "https://github.com/haukened/gone/releases/download/${TAG}/${PKG}"
gh attestation verify "$PKG" --repo haukened/gone
```

## The container image

Every image is built with an SBOM and SLSA provenance, and GitHub signs an attestation for its digest:

```sh
gh attestation verify oci://ghcr.io/haukened/gone:v3.12.0 --repo haukened/gone \
  --signer-workflow haukened/gone/.github/workflows/release.yaml \
  --source-ref refs/tags/v3.12.0
```

Then deploy by that tag or by digest, rather than following `latest`.

## The Helm chart

```sh
gh attestation verify oci://ghcr.io/haukened/charts/gone:3.13.0 --repo haukened/gone
```

## What a passing check tells you

- The file was built by `.github/workflows/release.yaml` in `haukened/gone`, on GitHub's runners, and hasn't changed since.
- The build ran from a version tag (`vMAJOR.MINOR.PATCH`, optionally with a `-prerelease` suffix) whose commit is on `main`. Any other tag fails the release workflow before anything is built.
- The full test suite, including the browser-crypto interop tests, passed before the build. Release builds use no shared CI cache.

It doesn't tell you the source is free of bugs: for what Gone defends against, see the [Security model](security.md). To report a problem, see [Reporting a vulnerability](../SECURITY.md).
