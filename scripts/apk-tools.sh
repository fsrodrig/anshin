# shellcheck shell=bash disable=SC2034 # the variables it sets are for the scripts that source it
# Sourced by the apk-*.sh scripts. Finds the Android SDK tools the way scripts/qa does (agent/internal/qa/build.go),
# because a stock SDK install puts none of them on PATH. Sets SDK, BUILD_TOOLS, AAPT2, ZIPALIGN and APKSIGNER;
# apk_tools_profgen sets PROFGEN. Each exits loudly when its tool is missing, so no check is ever skipped.

apk_tools_die() {
  echo "$(basename "$0"): $*" >&2
  exit 1
}

apk_tools_repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SDK=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
if [ -z "$SDK" ] && [ -f "$apk_tools_repo/android/local.properties" ]; then
  SDK=$(sed -n 's/^[[:space:]]*sdk\.dir=//p' "$apk_tools_repo/android/local.properties" | head -n 1)
fi
[ -n "$SDK" ] || apk_tools_die "no Android SDK: set ANDROID_HOME or sdk.dir in android/local.properties"
[ -d "$SDK/build-tools" ] || apk_tools_die "no build-tools in the Android SDK at $SDK"

BUILD_TOOLS=
while IFS= read -r apk_tools_version; do
  apk_tools_dir="$SDK/build-tools/$apk_tools_version"
  if [ -x "$apk_tools_dir/aapt2" ] && [ -x "$apk_tools_dir/zipalign" ] && [ -x "$apk_tools_dir/apksigner" ]; then
    BUILD_TOOLS=$apk_tools_dir
    break
  fi
done < <(find "$SDK/build-tools" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort -rV)
[ -n "$BUILD_TOOLS" ] || apk_tools_die "no build-tools version in $SDK has aapt2, zipalign and apksigner"
AAPT2=$BUILD_TOOLS/aapt2 ZIPALIGN=$BUILD_TOOLS/zipalign APKSIGNER=$BUILD_TOOLS/apksigner

apk_tools_profgen() {
  PROFGEN=$SDK/cmdline-tools/latest/bin/profgen
  [ -x "$PROFGEN" ] || apk_tools_die "no profgen at $PROFGEN: install the SDK package cmdline-tools;latest"
}

# apk_tools_pin <file>: sets PIN to the release certificate's SHA-256 from deploy/android/release-certificate.sha256's
# format, exactly one lowercase hex line besides comments and blank lines. Anything else is refused rather than
# guessed at, since the pin is what keeps a swapped key from shipping.
apk_tools_pin() {
  local lines
  [ -f "$1" ] || apk_tools_die "no certificate pin file at $1"
  lines=$(grep -vE '^[[:space:]]*(#|$)' "$1" || true)
  if [ -z "$lines" ]; then
    apk_tools_die "$1 holds no certificate fingerprint yet: see docs/maintaining.md, The APK signing key"
  fi
  [[ $lines =~ ^[0-9a-f]{64}$ ]] \
    || apk_tools_die "$1 must hold exactly one lowercase hex SHA-256 line besides comments"
  PIN=$lines
}
