#!/usr/bin/env bash
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/changed-components.sh"

repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false

mkdir -p agent/cmd docs
echo 'package main' > agent/cmd/main.go
echo '# docs' > docs/readme.md
git add -A
git commit -qm base
base=$(git rev-parse HEAD)

failures=0
# expect <want> <case>: commits whatever the case staged on top of base, compares, then resets to base.
expect() {
  local want=$1 name=$2 got
  git commit -qm "$name" --allow-empty
  got=$("$script" "$base")
  if [ "$got" != "$want" ]; then
    printf 'FAIL: %s: expected %s, got %s\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
  git reset -q --hard "$base"
}

echo '// change' >> agent/cmd/main.go && git add -A
expect agent=true "edit inside agent/"

echo 'more' >> docs/readme.md && git add -A
expect agent=false "docs only"

git mv agent/cmd/main.go docs/main.go
expect agent=true "move a file out of agent/ unchanged"

git mv docs/readme.md agent/readme.md
expect agent=true "move a file into agent/ unchanged"

echo 'x:' > Makefile && git add -A
expect agent=true "Makefile"

expect agent=false "no change"

got=$("$script" 0000000000000000000000000000000000000000)
if [ "$got" != agent=true ]; then
  printf 'FAIL: unknown base: expected agent=true, got %s\n' "$got" >&2
  failures=$((failures + 1))
fi

got=$("$script" "")
if [ "$got" != agent=true ]; then
  printf 'FAIL: empty base (a push to main): expected agent=true, got %s\n' "$got" >&2
  failures=$((failures + 1))
fi

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "changed-components: all cases pass"
