#!/usr/bin/env bash
# Prints deploy/aur/PKGBUILD rendered for one release: pkgver and both sha256sums come from goreleaser's
# checksums.txt, so the AUR package installs only the archives that release built.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <version> <checksums.txt>" >&2
  exit 2
fi
version=$1 checksums=$2
template="$(cd "$(dirname "$0")/.." && pwd)/deploy/aur/PKGBUILD"

fail() {
  echo "aur-render: $1" >&2
  exit 1
}

# make check-deploy renders goreleaser's snapshot, whose version is 0.0.0.snapshot.
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(\.snapshot)?$ ]] || fail "\"$version\" is not X.Y.Z"

sum() {
  local name="anshin-agent_${version}_linux_$1.tar.gz" sums
  sums=$(awk -v name="$name" '$2 == name { print $1 }' "$checksums")
  [[ $sums =~ ^[0-9a-f]{64}$ ]] || fail "$checksums must list $name once, with its SHA-256"
  printf '%s' "$sums"
}
amd64=$(sum amd64)
arm64=$(sum arm64)

for placeholder in '^pkgver=unrendered-pkgver$' "'unrendered-sha256-x86_64'" "'unrendered-sha256-aarch64'"; do
  [ "$(grep -c -- "$placeholder" "$template")" -eq 1 ] || fail "$template must hold $placeholder exactly once"
done

# The template's first paragraph is about the template, not the package.
awk 'body { print; next } /^$/ { body = 1 }' "$template" | sed \
  -e "s/^pkgver=unrendered-pkgver\$/pkgver=$version/" \
  -e "s/'unrendered-sha256-x86_64'/'$amd64'/" \
  -e "s/'unrendered-sha256-aarch64'/'$arm64'/"
