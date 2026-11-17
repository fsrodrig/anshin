#!/usr/bin/env bash
# Runs scripts/apk-sign.sh and scripts/apk-verify.sh on a real unsigned release APK with two throwaway keys made here
# and deleted on exit. Every way the Release workflow's secrets or pin can be wrong must fail and leave no APK.
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <unsigned release apk>" >&2
  exit 2
fi
apk=$1
scripts="$(cd "$(dirname "$0")" && pwd)"

umask 077
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# key <name>: a PKCS12 keystore with alias anshin, its password in $work/<name>.pass, its base64 in $work/<name>.b64
# and its certificate's SHA-256 in $work/<name>.pin. keytool reads the password from the environment, not argv.
key() {
  local pass
  pass=$(head -c 18 /dev/urandom | base64)
  printf '%s' "$pass" > "$work/$1.pass"
  KEY_PASS=$pass keytool -genkeypair -keystore "$work/$1.p12" -storetype PKCS12 -alias anshin -keyalg RSA \
    -keysize 2048 -validity 2 -dname 'CN=anshin-test throwaway' -storepass:env KEY_PASS 2> /dev/null
  base64 -w0 "$work/$1.p12" > "$work/$1.b64"
  KEY_PASS=$pass keytool -exportcert -keystore "$work/$1.p12" -alias anshin -storepass:env KEY_PASS 2> /dev/null \
    | sha256sum | cut -d ' ' -f 1 > "$work/$1.pin"
}
key a
key b
pin_a=$(cat "$work/a.pin")
pin_b=$(cat "$work/b.pin")
[ "$pin_a" != "$pin_b" ] || { echo "FAIL: the two throwaway keys share a certificate" >&2; exit 1; }

failures=0
fail_case() {
  printf 'FAIL: %s\n' "$*" >&2
  cat "$work/log" >&2
  failures=$((failures + 1))
}

# sign <case> <pass|fail> <message on failure> <keystore base64> <password> <pin file> <out>
sign() {
  local name=$1 want=$2 message=$3 got=pass
  ANDROID_KEYSTORE=$4 ANDROID_KEYSTORE_PASSWORD=$5 "$scripts/apk-sign.sh" "$apk" "$7" "$6" > "$work/log" 2>&1 \
    || got=fail
  if [ "$got" != "$want" ]; then
    fail_case "$name: expected $want, got $got"
  elif [ "$want" = fail ] && ! grep -qF "$message" "$work/log"; then
    fail_case "$name: failed without \"$message\""
  fi
}

# verify <case> <pass|fail> <message on failure> <apk> <pin file>
verify() {
  local name=$1 want=$2 message=$3 got=pass
  "$scripts/apk-verify.sh" "$4" "$5" > "$work/log" 2>&1 || got=fail
  if [ "$got" != "$want" ]; then
    fail_case "$name: expected $want, got $got"
  elif [ "$want" = fail ] && ! grep -qF "$message" "$work/log"; then
    fail_case "$name: failed without \"$message\""
  fi
}

# no_output <case> <out>
no_output() {
  if [ -e "$2" ]; then
    : > "$work/log"
    fail_case "$1: left $2 behind"
  fi
}

ks_a=$(cat "$work/a.b64") pass_a=$(cat "$work/a.pass") pass_b=$(cat "$work/b.pass")

sign "key A with pin A" pass "" "$ks_a" "$pass_a" "$work/a.pin" "$work/a.apk"
if [ -f "$work/a.apk" ]; then
  grep -qx "certificate sha256: $pin_a" "$work/log" || fail_case "key A with pin A: the summary names another certificate"
  verify "apk-verify on A's APK with pin A" pass "" "$work/a.apk" "$work/a.pin"
  verify "apk-verify on A's APK with pin B" fail "not the pinned $pin_b" "$work/a.apk" "$work/b.pin"
else
  fail_case "key A with pin A: no signed APK"
fi
verify "apk-verify on the unsigned APK" fail "apksigner verify failed" "$apk" "$work/a.pin"

sign "key A with pin B" fail "not the pinned $pin_b" "$ks_a" "$pass_a" "$work/b.pin" "$work/out.apk"
no_output "key A with pin B" "$work/out.apk"

sign "ANDROID_KEYSTORE empty" fail "ANDROID_KEYSTORE is empty; set it in the apk-signing environment" \
  "" "$pass_a" "$work/a.pin" "$work/out.apk"
no_output "ANDROID_KEYSTORE empty" "$work/out.apk"

sign "password empty" fail "ANDROID_KEYSTORE_PASSWORD is empty" "$ks_a" "" "$work/a.pin" "$work/out.apk"
no_output "password empty" "$work/out.apk"

sign "wrong password" fail "apksigner sign failed" "$ks_a" "$pass_b" "$work/a.pin" "$work/out.apk"
no_output "wrong password" "$work/out.apk"

sign "keystore not base64" fail "ANDROID_KEYSTORE is not valid base64" "not*base64!" "$pass_a" "$work/a.pin" \
  "$work/out.apk"
no_output "keystore not base64" "$work/out.apk"

grep '^#' "$scripts/../deploy/android/release-certificate.sha256" > "$work/comments.pin"
sign "a pin file with only comments" fail "holds no certificate fingerprint yet" "$ks_a" "$pass_a" \
  "$work/comments.pin" "$work/out.apk"
no_output "a pin file with only comments" "$work/out.apk"

tr '[:lower:]' '[:upper:]' < "$work/a.pin" > "$work/upper.pin"
sed 's/../&:/g; s/:$//' "$work/a.pin" > "$work/colons.pin"
cat "$work/a.pin" "$work/b.pin" > "$work/two.pin"
for pin in upper colons two; do
  sign "a pin file in the $pin form" fail "must hold exactly one lowercase hex SHA-256 line" "$ks_a" "$pass_a" \
    "$work/$pin.pin" "$work/out.apk"
  no_output "a pin file in the $pin form" "$work/out.apk"
done

: > "$work/exists.apk"
sign "output already exists" fail "already exists" "$ks_a" "$pass_a" "$work/a.pin" "$work/exists.apk"
[ ! -s "$work/exists.apk" ] || fail_case "output already exists: it was overwritten"

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "apk-sign: all cases pass"
