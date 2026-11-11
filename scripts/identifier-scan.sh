#!/usr/bin/env bash
# Fails when a tracked (or, with --staged, staged) path or text file holds something that identifies a real
# Machine or person: a MAC, a serial number, a tailnet name or address, a home directory, or an entry from the
# Owner's private denylist. Patterns cannot recognise every real hostname, so fixtures still get a human review.
#
# The private denylist lives in .git/info/identifiers (shared by every worktree, never tracked), or in the file
# named by ANSHIN_IDENTIFIERS. One entry per line, `#` starts a comment:
#   <word>        matched case-insensitively anywhere (hostnames, the tailnet name)
#   user:<name>   matched only as a home directory, a `User=` value or before `@`, because a Linux username
#                 can equal the GitHub handle that is legitimately in the package name and repository URL.
set -euo pipefail
# report() sets found=1, so the last command of each pipeline must run in this shell.
shopt -s lastpipe

mode=worktree
case "${1-}" in
  --staged) mode=staged ;;
  "") ;;
  *) echo "usage: $0 [--staged]" >&2; exit 2 ;;
esac

cd "$(git rev-parse --show-toplevel)"

grep_args=(-I -n -o --no-color -E)
if [ "$mode" = staged ]; then
  grep_args+=(--cached)
  list_paths() { git diff --cached --name-only --diff-filter=ACMR; }
else
  list_paths() { git ls-files; }
fi

found=0
report() {
  local rule=$1
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    printf '%s: %s\n' "$rule" "$line"
    found=1
  done
}

# Prints file:line:match for each match of an extended regex in the scanned text files.
content() { git grep "${grep_args[@]}" "$@" -- . || true; }
# grep that treats "no lines" as success, for filtering inside pipelines.
keep() { grep "$@" || true; }

hex='[0-9A-Fa-f]'
content "\\b($hex{2}:){5}$hex{2}\\b|\\b($hex{2}-){5}$hex{2}\\b" \
  | keep -viE ':(00[:-]){5}00$' | report "MAC address"

quote="[\"']"
content -i "(serial_number|product_serial|board_serial|chassis_serial|product_uuid|id_serial(_short)?)[[:space:]]*[=:][[:space:]]*$quote?[A-Za-z0-9][^\"'[:space:]]*" \
  | keep -vE "[=:][[:space:]]*$quote?SCRUBBED\$" | report "serial number"

content -i '[a-z0-9-]+\.ts\.net\b' \
  | keep -viE ':example\.ts\.net$' | report "tailnet name"

# 100.64.0.0/10 is the tailnet range. CIDR notation, 100.64.0.x (reserved for fixtures) and Tailscale's fixed
# 100.100.100.100 service address identify nobody.
content '\b100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}(/[0-9]+)?' \
  | keep -vE ':(100\.64\.0\.[0-9]{1,3}|100\.100\.100\.100|[0-9.]+/[0-9]+)$' | report "tailnet IPv4 address"

content -i '\bfd7a:115c:a1e0:[0-9a-f:]+(/[0-9]+)?' \
  | keep -viE ':fd7a:115c:a1e0::([0-9a-f]{1,4})?(/[0-9]+)?$' | report "tailnet IPv6 address"

content '/home/[A-Za-z0-9_.-]+' \
  | keep -vE ':/home/(user|owner|example)$' | report "home directory"

list_paths | keep -E '(^|/)(serial_number|product_serial|board_serial|chassis_serial|product_uuid)$' \
  | while IFS= read -r path_file; do
      if [ "$mode" = staged ]; then value=$(git show ":$path_file"); else value=$(cat "$path_file"); fi
      [ "$value" = SCRUBBED ] || echo "$path_file"
    done | report "serial number file"

list_paths | keep -E "($hex{2}[:-]){5}$hex{2}|(^|/)home/[A-Za-z0-9_.-]+(/|$)" | keep -vE '(^|/)home/(user|owner|example)(/|$)' \
  | report "identifier in path"

denylist=${ANSHIN_IDENTIFIERS:-$(git rev-parse --path-format=absolute --git-common-dir)/info/identifiers}
if [ -f "$denylist" ]; then
  while IFS= read -r entry; do
    entry=${entry%%#*}
    entry=$(printf '%s' "$entry" | tr -d '[:space:]')
    [ -n "$entry" ] || continue
    if [[ $entry == user:* ]]; then
      name=${entry#user:}
      content -i "/home/$name\\b|User=$name\\b|\\b$name@" | report "private denylist (username)"
      list_paths | keep -iE "(^|/)home/$name(/|$)" | report "private denylist (username in path)"
    else
      content -i -F "$entry" | report "private denylist"
      list_paths | keep -iF "$entry" | report "private denylist (path)"
    fi
  done < "$denylist"
fi

if [ "$found" -ne 0 ]; then
  echo "Identifier scan failed: scrub the matches above, or use a placeholder (see CONTRIBUTING.md)." >&2
  exit 1
fi
