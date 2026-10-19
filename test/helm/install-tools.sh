#!/usr/bin/env bash
# Installs the pinned helm, kind, kubectl and kubeconform the chart tests
# use (linux/amd64), each checked against its SHA-256 before it runs. The
# digests are from each project's published checksum file.
#
#   test/helm/install-tools.sh DIR   installs into DIR (add it to PATH)
set -euo pipefail
dir=${1:?usage: install-tools.sh DIR}
mkdir -p "$dir"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

HELM_VERSION=v4.3.1
HELM_SHA256=0403a88ae20913d11b10a2467b50087721fea7581736da6b9d3992f5357fe54b
KIND_VERSION=v0.33.0
KIND_SHA256=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
KUBECTL_VERSION=v1.37.1
KUBECTL_SHA256=65691ff77eb6fa44c908b77a1082c9f092c3b9733b5cefabec0d1104890e21a8
KUBECONFORM_VERSION=v0.8.0
KUBECONFORM_SHA256=9bc2bffbf71f261128533edaf912153948b7ff238f9a531ae6d34466ec287883

fetch() { # url, file, sha256
	curl -fsSLo "$2" "$1"
	echo "$3  $2" | sha256sum -c - >/dev/null
}
fetch "https://get.helm.sh/helm-$HELM_VERSION-linux-amd64.tar.gz" helm.tgz "$HELM_SHA256"
tar -xzf helm.tgz linux-amd64/helm
install -m 0755 linux-amd64/helm "$dir/helm"
fetch "https://kind.sigs.k8s.io/dl/$KIND_VERSION/kind-linux-amd64" kind "$KIND_SHA256"
install -m 0755 kind "$dir/kind"
fetch "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl" kubectl "$KUBECTL_SHA256"
install -m 0755 kubectl "$dir/kubectl"
fetch "https://github.com/yannh/kubeconform/releases/download/$KUBECONFORM_VERSION/kubeconform-linux-amd64.tar.gz" kubeconform.tgz "$KUBECONFORM_SHA256"
tar -xzf kubeconform.tgz kubeconform
install -m 0755 kubeconform "$dir/kubeconform"
echo "installed helm $HELM_VERSION, kind $KIND_VERSION, kubectl $KUBECTL_VERSION, kubeconform $KUBECONFORM_VERSION into $dir"
