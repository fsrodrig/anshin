#!/usr/bin/env bash
# Checks a built, unsigned release APK before anyone signs it: the size budget, the package and its version, that it
# isn't debuggable, that the app's Baseline Profile made it into the APK, and 16 KB alignment. With no version, the
# APK must carry none, so a version written into source fails here.
set -euo pipefail

if [ $# -lt 3 ] || [ $# -gt 4 ]; then
  echo "usage: $0 <apk> <mapping.txt> <max-bytes> [X.Y.Z]" >&2
  exit 2
fi
apk=$1 mapping=$2 max=$3 version=${4-}
repo="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/apk-tools.sh
. "$repo/scripts/apk-tools.sh"
apk_tools_profgen

fail() {
  echo "apk-check: $apk: $*" >&2
  exit 1
}

[ -f "$apk" ] || fail "no such file"
[ -f "$mapping" ] || fail "no R8 mapping file at $mapping"
[[ $max =~ ^[1-9][0-9]*$ ]] || fail "max-bytes must be a positive integer, got '$max'"

size=$(stat -c %s "$apk")
[ "$size" -le "$max" ] || fail "$size bytes is over the $max-byte budget"

badging=$("$AAPT2" dump badging "$apk") || fail "aapt2 dump badging failed"
package=$(sed -n "1s/^package: name='\([^']*\)'.*/\1/p" <<< "$badging")
code=$(sed -n "1s/.* versionCode='\([^']*\)'.*/\1/p" <<< "$badging")
name=$(sed -n "1s/.* versionName='\([^']*\)'.*/\1/p" <<< "$badging")
[ "$package" = io.github.fsrodrig.anshin ] || fail "package is '$package', not io.github.fsrodrig.anshin"
! grep -q '^application-debuggable' <<< "$badging" || fail "the APK is debuggable"

# The formula is recomputed here rather than trusted from Gradle, so a wrong one in build.gradle.kts fails.
if [ -n "$version" ]; then
  [[ $version =~ ^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})$ ]] && [ "$version" != 0.0.0 ] \
    || fail "expected version '$version' is not X.Y.Z with each part 0 to 999"
  want_code=$((BASH_REMATCH[1] * 1000000 + BASH_REMATCH[2] * 1000 + BASH_REMATCH[3]))
  [ "$name" = "$version" ] || fail "versionName is '$name', expected '$version'"
  [ "$code" = "$want_code" ] || fail "versionCode is '$code', expected '$want_code'"
else
  [ -z "$name" ] && [ -z "$code" ] \
    || fail "a development build must carry no version, got versionName '$name' versionCode '$code'"
fi

entries=$(unzip -Z1 "$apk") || fail "unzip cannot list the APK"
for entry in assets/dexopt/baseline.prof assets/dexopt/baseline.profm; do
  grep -qxF "$entry" <<< "$entries" || fail "no $entry: the APK carries no Baseline Profile"
done

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
unzip -p "$apk" assets/dexopt/baseline.prof > "$work/baseline.prof"
"$PROFGEN" dumpProfile -p "$work/baseline.prof" -a "$apk" -m "$mapping" -o "$work/rules.txt" > "$work/profgen.log" 2>&1 \
  || { cat "$work/profgen.log" >&2; fail "profgen dumpProfile failed"; }
# Library profiles never name MainActivity, so this rule proves android/app/src/main/baseline-prof.txt was merged.
rule='HSPLio/github/fsrodrig/anshin/MainActivity;->onCreate(Landroid/os/Bundle;)V'
grep -qxF "$rule" "$work/rules.txt" || fail "the Baseline Profile has no rule for MainActivity.onCreate"

# profgen validate exits 0 even on a bad line, so its output is the verdict.
source_profile=$repo/android/app/src/main/baseline-prof.txt
problems=$("$PROFGEN" validate "$source_profile" 2>&1) || fail "profgen validate failed on $source_profile"
[ -z "$problems" ] || { printf '%s\n' "$problems" >&2; fail "$source_profile has invalid rules"; }

"$ZIPALIGN" -c -P 16 4 "$apk" > /dev/null || fail "not 16 KB-aligned (zipalign -c -P 16 4)"

echo "apk-check: $apk: $size bytes (budget $max), versionName '${name}' versionCode '${code}', Baseline Profile with MainActivity, 16 KB-aligned"
