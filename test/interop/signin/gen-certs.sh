#!/usr/bin/env bash
# Makes a throwaway CA and a server certificate for Dex, for the sign-in
# interop stacks. Written to ./certs, which is ignored by git.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p certs
cd certs
[ -f ca.pem ] && [ -f dex.pem ] && exit 0
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 2 \
  -subj "/CN=gone interop CA" -keyout ca-key.pem -out ca.pem 2>/dev/null
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
  -subj "/CN=dex" -keyout dex-key.pem -out dex.csr 2>/dev/null
printf 'subjectAltName=DNS:dex\nextendedKeyUsage=serverAuth\n' > dex.ext
openssl x509 -req -in dex.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial -days 2 \
  -extfile dex.ext -out dex.pem 2>/dev/null
chmod 0644 ./*.pem
