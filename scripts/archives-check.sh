#!/usr/bin/env bash
# Check the built artifacts as well as the config, so packaging and version wiring cannot drift.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <version> <dist dir>" >&2
  exit 2
fi
version=$1 dist=$2
root=$(cd "$(dirname "$0")/.." && pwd)

fail() {
  echo "archives-check: $1" >&2
  exit 1
}

archives="anshin-agent_${version}_linux_amd64.tar.gz anshin-agent_${version}_linux_arm64.tar.gz"
listed=$(awk '{ print $2 }' "$dist/checksums.txt" | LC_ALL=C sort | paste -sd ' ')
[ "$listed" = "$archives" ] || fail "checksums.txt lists $listed, want $archives"
(cd "$dist" && sha256sum --quiet -c checksums.txt) || fail "an archive does not match checksums.txt"

case $(uname -m) in
  x86_64) host=amd64 ;;
  aarch64) host=arm64 ;;
  *) host="" ;;
esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for arch in amd64 arm64; do
  archive="$dist/anshin-agent_${version}_linux_$arch.tar.gz"
  files=$(tar -tzf "$archive" | LC_ALL=C sort | paste -sd ' ')
  [ "$files" = "LICENSE anshin-agent anshin-agent.service" ] || fail "$archive holds $files"
  mkdir "$work/$arch"
  tar -xzf "$archive" -C "$work/$arch"
  cmp -s "$work/$arch/anshin-agent.service" "$root/deploy/systemd/anshin-agent.service" || fail "$archive holds another unit"
  cmp -s "$work/$arch/LICENSE" "$root/LICENSE" || fail "$archive holds another LICENSE"
  if [ "$arch" = "$host" ]; then
    said=$("$work/$arch/anshin-agent" version)
    [ "$said" = "anshin-agent $version" ] || fail "the $arch binary says \"$said\", want \"anshin-agent $version\""
  fi
done

"$root/scripts/aur-render.sh" "$version" "$dist/checksums.txt" > "$work/PKGBUILD"
# shellcheck disable=SC2016 # expanded by the inner shell, which sources the PKGBUILD
sources=$(bash -c 'source "$1" && printf "%s\n" "${source_x86_64[@]##*/}" "${source_aarch64[@]##*/}"' _ "$work/PKGBUILD" |
  LC_ALL=C sort | paste -sd ' ')
[ "$sources" = "$archives" ] || fail "the PKGBUILD downloads $sources, want $archives"
