#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/archives-check.sh"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# dist <name> <version the binaries print> [<extra file in the archives>]: a dist directory shaped like goreleaser's,
# with a stand-in binary.
dist() {
  local d="$tmp/$1" says=$2 extra=${3-}
  mkdir -p "$d/files"
  printf '#!/bin/sh\necho "anshin-agent %s"\n' "$says" > "$d/files/anshin-agent"
  chmod +x "$d/files/anshin-agent"
  cp "$root/LICENSE" "$root/deploy/systemd/anshin-agent.service" "$d/files/"
  local members=(LICENSE anshin-agent anshin-agent.service)
  if [ -n "$extra" ]; then
    touch "$d/files/$extra"
    members+=("$extra")
  fi
  for arch in amd64 arm64; do
    tar -czf "$d/anshin-agent_1.2.3_linux_$arch.tar.gz" -C "$d/files" "${members[@]}"
  done
  (cd "$d" && sha256sum anshin-agent_1.2.3_linux_*.tar.gz > checksums.txt)
  echo "$d"
}

failures=0
expect() {
  local want=$1 name=$2 dir=$3 got=pass
  "$script" 1.2.3 "$dir" > /dev/null 2>&1 || got=fail
  if [ "$got" != "$want" ]; then
    printf 'FAIL: %s: expected %s, got %s\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
}

expect pass "a release as goreleaser builds it" "$(dist good 1.2.3)"
expect fail "a binary without the release's version" "$(dist devel '(devel)')"
expect fail "an extra file in the archives" "$(dist extra 1.2.3 README.md)"

d=$(dist tampered 1.2.3)
echo x >> "$d/anshin-agent_1.2.3_linux_arm64.tar.gz"
expect fail "an archive that doesn't match its checksum" "$d"

d=$(dist listed 1.2.3)
echo "$(printf '0%.0s' {1..64})  anshin-agent_1.2.3_windows_amd64.zip" >> "$d/checksums.txt"
expect fail "checksums.txt listing a third file" "$d"

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "archives-check: all cases pass"
