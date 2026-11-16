#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/aur-render.sh"
template="$root/deploy/aur/PKGBUILD"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failures=0
fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

a=$(printf 'a%.0s' {1..64}) b=$(printf 'b%.0s' {1..64})
printf '%s  anshin-agent_1.2.3_linux_amd64.tar.gz\n%s  anshin-agent_1.2.3_linux_arm64.tar.gz\n' "$a" "$b" > "$tmp/checksums.txt"

# makepkg rejects a pkgver with a hyphen, which is what keeps the unrendered template from building.
[[ $(sed -n 's/^pkgver=//p' "$template") == *-* ]] || fail "the template's pkgver has no hyphen, so makepkg would accept it"

"$script" 1.2.3 "$tmp/checksums.txt" > "$tmp/PKGBUILD" || fail "rendering failed"
[ "$(head -n 1 "$tmp/PKGBUILD")" = "pkgname=anshin-agent-bin" ] || fail "the rendered PKGBUILD keeps the template's comment"
! grep -q unrendered "$tmp/PKGBUILD" || fail "a placeholder survived rendering"
# shellcheck disable=SC2016 # expanded by the inner shell, which sources the PKGBUILD
got=$(bash -c 'source "$1" && printf "%s\n" "$pkgver" "${source_x86_64[@]}" "${sha256sums_x86_64[@]}" "${source_aarch64[@]}" "${sha256sums_aarch64[@]}"' _ "$tmp/PKGBUILD" | paste -sd ' ')
want="1.2.3 https://github.com/fsrodrig/anshin/releases/download/v1.2.3/anshin-agent_1.2.3_linux_amd64.tar.gz $a https://github.com/fsrodrig/anshin/releases/download/v1.2.3/anshin-agent_1.2.3_linux_arm64.tar.gz $b"
[ "$got" = "$want" ] || fail "rendered sources and sums are \"$got\", want \"$want\""

sed 's/1\.2\.3/0.0.0.snapshot/' "$tmp/checksums.txt" > "$tmp/snapshot.txt"
"$script" 0.0.0.snapshot "$tmp/snapshot.txt" > /dev/null || fail "a snapshot did not render"

expect_refused() {
  if "$script" "$@" > /dev/null 2>&1; then fail "rendered with $*"; fi
}
expect_refused v1.2.3 "$tmp/checksums.txt"
expect_refused 1.2 "$tmp/checksums.txt"
expect_refused 1.2.3-rc1 "$tmp/checksums.txt"
expect_refused 1.2.4 "$tmp/checksums.txt"
head -n 1 "$tmp/checksums.txt" > "$tmp/one-arch.txt"
cat "$tmp/checksums.txt" "$tmp/checksums.txt" > "$tmp/twice.txt"
sed "s/$a/${a:1}/" "$tmp/checksums.txt" > "$tmp/short-sum.txt"
expect_refused 1.2.3 "$tmp/one-arch.txt"
expect_refused 1.2.3 "$tmp/twice.txt"
expect_refused 1.2.3 "$tmp/short-sum.txt"
expect_refused 1.2.3

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "aur-render: all cases pass"
