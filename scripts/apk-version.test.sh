#!/usr/bin/env bash
# Every malformed anshin.version must stop the build at configuration with the property's own message, so a build
# that fails for another reason (no network, a broken wrapper) can't pass for a refused version.
set -euo pipefail
android="$(cd "$(dirname "$0")/../android" && pwd)"

failures=0
for value in 1.2 1.2.3.4 01.2.3 1.2.3-rc.1 v1.2.3 1000.0.0 1.1000.0 0.0.0 ' 1.2.3' ''; do
  if out=$(cd "$android" && ./gradlew --warning-mode=fail -q help "-Panshin.version=$value" 2>&1); then
    printf "FAIL: anshin.version='%s' was accepted\n" "$value" >&2
    failures=$((failures + 1))
  elif ! grep -qF "anshin.version must be X.Y.Z" <<< "$out"; then
    printf "FAIL: anshin.version='%s' failed without the anshin.version message:\n%s\n" "$value" "$out" >&2
    failures=$((failures + 1))
  fi
done

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "apk-version: every malformed anshin.version is refused"
