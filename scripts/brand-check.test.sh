#!/usr/bin/env bash
# Runs the brand check against throwaway copies of branding/.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
check=$here/brand-check.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

failures=0
# expect <pass|fail> <description> <path the message must name, or ''> <command run inside the copy>
expect() {
  local want=$1 what=$2 path=$3 change=$4 check_path=${5:-$PATH} copy got=pass output
  copy=$(mktemp -d "$work/branding.XXXX")
  cp -a "$here/../branding/." "$copy/"
  # Regenerating first makes the baseline consistent whatever state the working tree is in, so drift there is
  # reported by the check itself rather than by this test.
  python3 -B "$copy/build.py" > /dev/null
  (cd "$copy" && eval "$change")
  output=$(PATH="$check_path" "$check" "$copy" 2>&1) || got=fail
  if [ "$got" != "$want" ]; then
    printf 'FAIL: expected %s when %s:\n%s\n' "$want" "$what" "$output" >&2
    failures=$((failures + 1))
  elif [ -n "$path" ] && ! grep -qF "$copy/$path" <<< "$output"; then
    printf 'FAIL: message does not name %s when %s:\n%s\n' "$path" "$what" "$output" >&2
    failures=$((failures + 1))
  fi
  if [ -n "$(cd "$copy" && find . -name __pycache__)" ]; then
    printf 'FAIL: the check wrote bytecode into the checked directory when %s\n' "$what" >&2
    failures=$((failures + 1))
  fi
}

expect pass 'nothing changed' '' ':'
expect fail 'lockup.svg is hand-edited' lockup.svg 'printf x >> lockup.svg'
expect fail 'an Android resource is hand-edited' android/res/values/anshin_colors.xml \
  'sed -i "s/anshin_accent/anshin_tint/" android/res/values/anshin_colors.xml'
expect fail 'palette.json changes without regenerating' android/res/values/anshin_colors.xml \
  'python3 -c "import json; p = json.load(open(\"palette.json\")); p[\"colors\"][\"accent\"] = \"#123456\"; json.dump(p, open(\"palette.json\", \"w\"))"'
expect fail 'symbol.svg changes without regenerating' monochrome.svg 'sed -i "s/stroke-width=\"[^\"]*\"/stroke-width=\"3\"/" symbol.svg'
expect fail 'a generated output is deleted' android/res/mipmap-anydpi-v33/ic_launcher.xml 'rm android/res/mipmap-anydpi-v33/ic_launcher.xml'
expect pass 'a source changes and the outputs are regenerated' '' \
  'sed -i "s/stroke-width=\"[^\"]*\"/stroke-width=\"3\"/" symbol.svg && python3 -B build.py > /dev/null'
expect pass 'a hand-written file sits beside the outputs' '' \
  'mkdir -p themes && printf "{}\n" > themes/example.json && printf "notes\n" > themes/README.md && printf "x\n" >> README.md'

for command in find sort; do
  shim=$work/$command
  mkdir "$shim"
  printf '#!/usr/bin/env bash\necho "forced enumeration failure" >&2\nexit 1\n' > "$shim/$command"
  chmod +x "$shim/$command"
  expect fail "$command fails during enumeration" '' 'printf x >> lockup.svg' "$shim:$PATH"
done

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "brand-check: all cases pass"
