#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
check=$here/brand-check.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

dir=$here/../branding
find "$dir" -type l -print0 > "$work/symlinks"
if [ -s "$work/symlinks" ]; then
  echo "Symlinks are not allowed in $dir:" >&2
  while IFS= read -r -d '' file; do
    printf '  %s\n' "$file" >&2
  done < "$work/symlinks"
  exit 1
fi

failures=0
# expect <pass|fail> <description> <path message must name, or ''> <command run inside copy> [PATH, default $PATH]
# [text output must contain, default '']; nonempty text also requires output to omit 'Symlinks are not allowed'.
expect() {
  local want=$1 what=$2 path=$3 change=$4 check_path=${5:-$PATH} message=${6:-} copy got=pass output
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
  elif [ -n "$message" ] && { ! grep -qF "$message" <<< "$output" || grep -qF 'Symlinks are not allowed' <<< "$output"; }; then
    printf 'FAIL: expected enumeration failure when %s:\n%s\n' "$what" "$output" >&2
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

printf 'hand-written target\n' > "$work/expected.svg"
for target in "$work/external.svg" ../external.svg; do
  cp "$work/expected.svg" "$work/external.svg"
  printf -v change 'rm lockup.svg && ln -s %q lockup.svg' "$target"
  expect fail 'an output is a symlink' lockup.svg "$change"
  if ! cmp -s "$work/expected.svg" "$work/external.svg"; then
    echo 'FAIL: the check overwrote a symlink target outside the copy' >&2
    failures=$((failures + 1))
  fi
done

mkdir -p "$work/suite/scripts" "$work/suite/bin"
cp "$here/brand-check.sh" "$here/brand-check.test.sh" "$work/suite/scripts/"
cp -a "$here/../branding" "$work/suite/branding"
rm "$work/suite/branding/lockup.svg"
cp "$work/expected.svg" "$work/external.svg"
ln -s "$work/external.svg" "$work/suite/branding/lockup.svg"
# Stop before regeneration if the suite's guard regresses, rather than recursively running its own tests.
printf '#!/usr/bin/env bash\nexit 1\n' > "$work/suite/bin/python3"
chmod +x "$work/suite/bin/python3"
got=pass
output=$(PATH="$work/suite/bin:$PATH" "$work/suite/scripts/brand-check.test.sh" 2>&1) || got=fail
if [ "$got" != fail ] || ! grep -qF '/branding/lockup.svg' <<< "$output"; then
  printf 'FAIL: the test suite did not reject a symlinked output:\n%s\n' "$output" >&2
  failures=$((failures + 1))
fi
if ! cmp -s "$work/expected.svg" "$work/external.svg"; then
  echo 'FAIL: the test suite overwrote a symlink target outside its copies' >&2
  failures=$((failures + 1))
fi

real_find=$(command -v find)
for command in find sort; do
  shim=$work/$command
  mkdir "$shim"
  printf '#!/usr/bin/env bash\n' > "$shim/$command"
  if [ "$command" = find ]; then
    printf 'if [[ " $* " != *" -type f "* ]]; then\n  exec %q "$@"\nfi\n' "$real_find" >> "$shim/$command"
  fi
  printf 'echo "forced %s enumeration failure" >&2\nexit 1\n' "$command" >> "$shim/$command"
  chmod +x "$shim/$command"
  expect fail "$command fails during enumeration" '' ':' "$shim:$PATH" "forced $command enumeration failure"
done

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "brand-check: all cases pass"
