#!/usr/bin/env bash
# Runs scripts/apk-check.sh on copies of a real release APK: it passes as built, and each broken property fails it
# with its own message, so a check can't quietly stop checking.
set -euo pipefail

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
  echo "usage: $0 <unsigned release apk> <mapping.txt> [the APK's X.Y.Z]" >&2
  exit 2
fi
apk=$1 mapping=$2 version=${3-}
check="$(cd "$(dirname "$0")" && pwd)/apk-check.sh"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$apk" "$work/app.apk"

failures=0
# expect <pass|fail> <case> <message on failure> <apk-check arguments…>
expect() {
  local want=$1 name=$2 message=$3 got=pass
  shift 3
  "$check" "$@" > "$work/out" 2>&1 || got=fail
  if [ "$got" != "$want" ]; then
    printf 'FAIL: %s: expected %s, got %s\n' "$name" "$want" "$got" >&2
    cat "$work/out" >&2
    failures=$((failures + 1))
  elif [ "$want" = fail ] && ! grep -qF "$message" "$work/out"; then
    printf 'FAIL: %s: failed without "%s"\n' "$name" "$message" >&2
    cat "$work/out" >&2
    failures=$((failures + 1))
  fi
}

expect pass "the APK as built" "" "$work/app.apk" "$mapping" 5000000 ${version:+"$version"}
if [ -n "$version" ]; then
  expect fail "a versioned APK checked as a development build" "must carry no version" "$work/app.apk" "$mapping" 5000000
else
  expect fail "a development build checked for a version" "versionName is ''" "$work/app.apk" "$mapping" 5000000 1.2.3
fi
expect fail "over the size budget" "over the 1000-byte budget" "$work/app.apk" "$mapping" 1000 ${version:+"$version"}
expect fail "no mapping file" "no R8 mapping file" "$work/app.apk" "$work/missing.txt" 5000000 ${version:+"$version"}

cp "$work/app.apk" "$work/no-profile.apk"
zip -q -d "$work/no-profile.apk" 'assets/dexopt/*'
expect fail "no Baseline Profile" "carries no Baseline Profile" "$work/no-profile.apk" "$mapping" 5000000 ${version:+"$version"}

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "apk-check: all cases pass"
