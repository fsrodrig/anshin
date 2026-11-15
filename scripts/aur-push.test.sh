#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/aur-push.sh"

failures=0
fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

# A typo or a swapped key line in known_hosts would let the push trust the wrong host.
got=$(ssh-keygen -lf "$root/deploy/aur/known_hosts" | awk '{ print $2 }' | LC_ALL=C sort | paste -sd ' ')
want="SHA256:5s5cIyReIfNNVGRFdDbe3hdYiI5OelHGpw2rOUud3Q8 SHA256:RFzBCUItH9LZS0cKB5UE6ceAYhBD5C8GeOBip8Z11+4 SHA256:uTa/0PndEgPZTf76e1DFqXKJEXKsn7m9ivhLQtzGOCI"
[ "$got" = "$want" ] || fail "deploy/aur/known_hosts fingerprints are $got, want the published $want"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git init -q --bare "$tmp/aur.git"
mkdir "$tmp/pkg"
printf 'pkgver=1.2.3\n' > "$tmp/pkg/PKGBUILD"
printf 'pkgver = 1.2.3\n' > "$tmp/pkg/.SRCINFO"
export AUR_SSH_PRIVATE_KEY=not-a-key
commits() { git -C "$tmp/aur.git" log --format=%s master 2>/dev/null | paste -sd '|'; }

"$script" 1.2.3 "$tmp/pkg" "$tmp/aur.git" > /dev/null
[ "$(commits)" = "Update to 1.2.3" ] || fail "first push: master has $(commits)"
[ "$(git -C "$tmp/aur.git" show master:.SRCINFO)" = "pkgver = 1.2.3" ] || fail "first push: .SRCINFO not pushed"

out=$("$script" 1.2.3 "$tmp/pkg" "$tmp/aur.git")
[ "$(commits)" = "Update to 1.2.3" ] || fail "retry: master has $(commits), want no new commit"
[[ $out == *"already has"* ]] || fail "retry: said \"$out\""

printf 'pkgver=1.2.4\n' > "$tmp/pkg/PKGBUILD"
"$script" 1.2.4 "$tmp/pkg" "$tmp/aur.git" > /dev/null
[ "$(commits)" = "Update to 1.2.4|Update to 1.2.3" ] || fail "next release: master has $(commits)"

if AUR_SSH_PRIVATE_KEY='' "$script" 1.2.5 "$tmp/pkg" "$tmp/aur.git" > /dev/null 2>&1; then
  fail "pushed without a key"
fi

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "aur-push: all cases pass"
