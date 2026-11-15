#!/usr/bin/env bash
# Fails when a HuJSON file (every tracked *.hujson, or the paths given) doesn't parse with Tailscale's reference
# parser, github.com/tailscale/hujson, or isn't in its hujsonfmt format. The parser is pinned as a tool in
# agent/go.mod, because the tailnet policy file it checks is read by Tailscale, not by any Anshin code.
set -euo pipefail

files=()
for f in "$@"; do files+=("$(realpath "$f")"); done
cd "$(git rev-parse --show-toplevel)"
if [ ${#files[@]} -eq 0 ]; then
  while IFS= read -r f; do files+=("$PWD/$f"); done < <(git ls-files '*.hujson')
fi
if [ ${#files[@]} -eq 0 ]; then
  echo "no HuJSON files to check" >&2
  exit 1
fi

status=0
for f in "${files[@]}"; do
  if ! out=$(go -C agent tool hujsonfmt -l "$f" 2>&1); then
    printf '%s: not valid HuJSON:\n%s\n' "$f" "$out" >&2
    status=1
  elif [ -n "$out" ]; then
    echo "$f: not in hujsonfmt's format; run: go -C agent tool hujsonfmt -w $f" >&2
    status=1
  fi
done
exit "$status"
