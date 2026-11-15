#!/usr/bin/env bash
# Runs release-version.sh, with the repository's cliff.toml, in a scratch repository of Conventional Commits.
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/release-version.sh"

repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false
git config tag.gpgsign false

failures=0
fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}
commit() { git commit -q --allow-empty -m "$1"; }
# expect <want output or "refused"> <case>
expect() {
  local want=$1 name=$2 got
  if got=$("$script" notes.md 2> /dev/null | paste -sd ' '); then :; else got=refused; fi
  [ "$got" = "$want" ] || fail "$name: expected \"$want\", got \"$got\""
}

commit 'feat(agent): sample battery charge'
expect "version=0.1.0 tag=v0.1.0" "first release"
grep -q '^## \[0.1.0\] - ' notes.md || fail "first release: notes lack the version heading"
grep -q '^- \*\*agent:\*\* Sample battery charge$' notes.md || fail "first release: notes lack the feat line"

git tag v0.1.0
expect refused "nothing since the last tag"

commit 'docs: describe the Gap field'
commit 'chore: tidy'
expect refused "only changes that never reach the notes"

commit 'fix: keep null temperatures null'
expect "version=0.1.1 tag=v0.1.1" "a fix bumps the patch"

commit 'feat(android): show the Verdict'
expect "version=0.2.0 tag=v0.2.0" "a feat bumps the minor before 1.0"

git tag v0.2.0
commit 'feat!: drop schema 1 support'
expect "version=0.3.0 tag=v0.3.0" "a breaking change bumps the minor before 1.0"

if "$script" > /dev/null 2>&1; then fail "no notes file accepted"; fi

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "release-version: all cases pass"
