#!/usr/bin/env bash
# Verifies a signed Anshin APK: a valid v2 signature, one signer whose certificate is the pinned release certificate,
# and 16 KB alignment. The Release workflow runs it before signing is trusted and again before publishing; anyone can
# run it on a downloaded APK against deploy/android/release-certificate.sha256.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <apk> <pin file>" >&2
  exit 2
fi
apk=$1 pin_file=$2
# shellcheck source=scripts/apk-tools.sh
. "$(cd "$(dirname "$0")" && pwd)/apk-tools.sh"
apk_tools_pin "$pin_file"

fail() {
  echo "apk-verify: $apk: $*" >&2
  exit 1
}

[ -f "$apk" ] || fail "no such file"
# 26 is the app's minSdk, so the verdict covers every Android version that can install it.
out=$("$APKSIGNER" verify --verbose --print-certs --min-sdk-version 26 "$apk" 2>&1) \
  || { printf '%s\n' "$out" >&2; fail "apksigner verify failed"; }
grep -qx 'Verifies' <<< "$out" || fail "apksigner did not report Verifies"
grep -qx 'Verified using v2 scheme (APK Signature Scheme v2): true' <<< "$out" || fail "no v2 signature"
grep -qx 'Number of signers: 1' <<< "$out" || fail "not exactly one signer"

digests=$(sed -n 's/.*certificate SHA-256 digest: \([0-9a-f]*\)$/\1/p' <<< "$out" | sort -u)
[ -n "$digests" ] || fail "apksigner printed no certificate SHA-256"
[ "$digests" = "$PIN" ] || fail "signed by certificate $(paste -sd ' ' <<< "$digests"), not the pinned $PIN"

"$ZIPALIGN" -c -P 16 4 "$apk" > /dev/null || fail "not 16 KB-aligned (zipalign -c -P 16 4)"

echo "apk-verify: $apk: verifies, one signer, certificate SHA-256 $PIN matches the pin, 16 KB-aligned"
