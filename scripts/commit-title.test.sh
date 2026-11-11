#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

failures=0
expect() {
  local want=$1 title=$2 got=pass
  ./commit-title.sh "$title" 2>/dev/null || got=fail
  if [ "$got" != "$want" ]; then
    printf 'FAIL: expected %s for "%s"\n' "$want" "$title" >&2
    failures=$((failures + 1))
  fi
}

expect pass 'feat(agent): sample battery charge'
expect pass 'fix: keep null temperatures null'
expect pass 'feat(android)!: drop schema 1 support'
expect pass 'build(deps): bump the actions group with 2 updates'
expect pass 'fix(security): refuse addresses outside the tailnet'
expect pass 'chore: add repository rules'
expect pass 'docs(contract): describe the Gap field'
expect pass 'perf(agent): read GPU temperature without waking it'

expect fail 'Add battery support'
expect fail 'feat add battery'
expect fail 'feature(agent): sample battery'
expect fail 'feat(phone): sample battery'
expect fail 'feat(agent):sample battery'
expect fail 'feat(agent):  sample battery'
expect fail 'feat(agent): Sample battery'
expect fail 'feat(agent): sample battery.'
expect fail 'feat(Agent): sample battery'
expect fail "feat: $(printf 'x%.0s' {1..100})"

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "commit-title: all cases pass"
