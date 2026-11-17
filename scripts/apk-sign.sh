#!/usr/bin/env bash
# Signs a checked, unsigned release APK with the release key and refuses to leave any output unless the result
# verifies against the pinned certificate and holds exactly the input's entries. The Release workflow's `sign` job
# runs it with ANDROID_KEYSTORE (a base64 PKCS12 file, key alias `anshin`) and ANDROID_KEYSTORE_PASSWORD from the
# apk-signing environment. Prints the signed APK's identity as `key: value` lines.
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "usage: ANDROID_KEYSTORE=… ANDROID_KEYSTORE_PASSWORD=… $0 <unsigned.apk> <out.apk> <pin file>" >&2
  exit 2
fi
in=$1 out=$2 pin_file=$3
scripts="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/apk-tools.sh
. "$scripts/apk-tools.sh"

fail() {
  echo "apk-sign: $*" >&2
  exit 1
}

secrets_help="set it in the apk-signing environment: see docs/maintaining.md, The APK signing key"
[ -n "${ANDROID_KEYSTORE-}" ] || fail "ANDROID_KEYSTORE is empty; $secrets_help"
[ -n "${ANDROID_KEYSTORE_PASSWORD-}" ] || fail "ANDROID_KEYSTORE_PASSWORD is empty; $secrets_help"
apk_tools_pin "$pin_file"
[ -f "$in" ] || fail "no unsigned APK at $in"
[ ! -e "$out" ] || fail "$out already exists"

# Only the final mv writes $out, so a failure at any step leaves no partial or unverified APK behind.
umask 077
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
printf '%s' "$ANDROID_KEYSTORE" | base64 -d > "$work/release.p12" 2> /dev/null \
  || fail "ANDROID_KEYSTORE is not valid base64; $secrets_help"

# -P 16 keeps the native libraries 16 KB-aligned for 16 KB-page devices.
"$ZIPALIGN" -P 16 -f 4 "$in" "$work/aligned.apk" > /dev/null || fail "zipalign failed"
# The password reaches apksigner through the environment, never through argv. minSdk 26 needs no v1 (JAR)
# signature, and leaving it out keeps the signed APK's entries equal to the checked ones.
"$APKSIGNER" sign --ks "$work/release.p12" --ks-type PKCS12 --ks-key-alias anshin \
  --ks-pass env:ANDROID_KEYSTORE_PASSWORD --v1-signing-enabled false --v2-signing-enabled true \
  --v3-signing-enabled true --out "$work/signed.apk" "$work/aligned.apk" \
  || fail "apksigner sign failed: check ANDROID_KEYSTORE_PASSWORD and that the key alias is anshin"
rm -f "$work/release.p12"

"$scripts/apk-verify.sh" "$work/signed.apk" "$pin_file" > /dev/null

# apk-check.sh checked the unsigned APK; equal names and CRC-32s mean everything it proved holds for the signed one.
entries() { unzip -v "$1" | awk '$7 ~ /^[0-9a-f]+$/ && length($7) == 8 { $1 = $2 = $3 = $4 = $5 = $6 = ""; print }'; }
if ! diff <(entries "$in") <(entries "$work/signed.apk") > "$work/entries.diff"; then
  cat "$work/entries.diff" >&2
  fail "the signed APK's entries differ from the unsigned APK's"
fi

badging=$("$AAPT2" dump badging "$work/signed.apk")
mv "$work/signed.apk" "$out"
echo "name: $(basename "$out")"
echo "size: $(stat -c %s "$out") bytes"
echo "sha256: $(sha256sum "$out" | cut -d ' ' -f 1)"
echo "certificate sha256: $PIN"
echo "versionName: $(sed -n "1s/.* versionName='\([^']*\)'.*/\1/p" <<< "$badging")"
echo "versionCode: $(sed -n "1s/.* versionCode='\([^']*\)'.*/\1/p" <<< "$badging")"
echo "build-tools: $(basename "$BUILD_TOOLS")"
