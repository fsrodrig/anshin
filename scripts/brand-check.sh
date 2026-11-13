#!/usr/bin/env bash
# Fails when the generated files in the branding directory differ from what its build.py produces from the committed
# sources, or when build.py would add a file that isn't there. The generator runs on a copy, so the working tree is
# never touched, and files build.py doesn't write (README, explorations, themes) can never show up as a difference.
set -euo pipefail

dir=${1:-branding}
[ -f "$dir/build.py" ] || { echo "usage: $0 [<branding-dir>]: no build.py in $dir" >&2; exit 2; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp -a "$dir/." "$work/copy/"

# -B keeps bytecode out of the copy, where it would read as a file the generator added.
python3 -B "$work/copy/build.py" > /dev/null

find "$work/copy" -type f -print0 | sort -z > "$work/files"

out_of_date=()
while IFS= read -r -d '' file; do
  rel=${file#"$work/copy"/}
  cmp -s "$file" "$dir/$rel" || out_of_date+=("$dir/$rel")
done < "$work/files"

if [ "${#out_of_date[@]}" -gt 0 ]; then
  echo "These files differ from what $dir/build.py generates, or are missing:" >&2
  printf '  %s\n' "${out_of_date[@]}" >&2
  echo "Run \`python3 $dir/build.py\` and commit its outputs." >&2
  exit 1
fi
echo "brand-check: $dir matches what build.py generates"
