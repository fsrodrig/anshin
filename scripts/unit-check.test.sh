#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/unit-check.sh"
unit="$root/deploy/systemd/anshin-agent.service"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failures=0
# expect <want> <case> <sed program applied to the shipped unit>
expect() {
  local want=$1 name=$2 got=pass
  sed -e "$3" "$unit" > "$tmp/unit"
  "$script" "$tmp/unit" > /dev/null 2>&1 || got=fail
  if [ "$got" != "$want" ]; then
    printf 'FAIL: %s: expected %s, got %s\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
}

expect pass "the shipped unit" ''
expect pass "comments and blank lines" '/^\[Service\]/a # a comment\n\n; another'
expect pass "another order within a section" '/^NoNewPrivileges=yes$/d; /^MemoryMax=64M$/a NoNewPrivileges=yes'
expect fail "ProcSubset added" '/^MemoryMax=64M$/a ProcSubset=pid'
expect fail "ProtectProc added" '/^MemoryMax=64M$/a ProtectProc=invisible'
expect fail "a directive dropped" '/^NoNewPrivileges=yes$/d'
expect fail "a value changed" 's/^MemoryMax=64M$/MemoryMax=128M/'
expect fail "a directive moved to another section" '/^StartLimitIntervalSec=0$/d; /^RestartSec=5$/a StartLimitIntervalSec=0'
expect fail "a directive repeated" '/^Restart=always$/a Restart=always'

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "unit-check: all cases pass"
