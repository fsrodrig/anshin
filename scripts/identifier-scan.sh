#!/usr/bin/env bash
# Fails when a tracked (or, with --staged, staged) path or text file holds something that identifies a real
# Machine or person: a MAC, a serial number, a tailnet name or address, a home directory, or an entry from the
# maintainer's private denylist. Patterns cannot recognise every real hostname, so fixtures still get a human review.
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

grep_args=(-I -z -n -o --no-color -E)
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
    printf '%s: %s\n' "$rule" "${line//$'\t'/:}"
    found=1
  done
}

# Prints file<TAB>line<TAB>match for each match of an extended regex in the scanned text files. Matches can
# contain colons (MACs, IPv6), so the tab is what lets a filter anchor on the start of the match.
content() { { git grep "${grep_args[@]}" "$@" -- . || true; } | tr '\0' '\t'; }
# grep that treats "no lines" as success, for filtering inside pipelines.
keep() { grep "$@" || true; }

mapfile -t paths < <(list_paths)

# scan <rule> <case: -i or ''> <regex> <allowed match regex>: reports every match in contents and in file
# names, except matches that are documented placeholders.
scan() {
  local rule=$1 regex=$3 allowed=$4 flags=()
  [ -z "$2" ] || flags=("$2")
  {
    content "${flags[@]}" "$regex"
    printf '%s\n' "${paths[@]}" | keep -n -o "${flags[@]}" -E "$regex" | while IFS=: read -r n match; do
      printf '%s\t%s\n' "${paths[n - 1]}" "$match"
    done
  } | keep -v "${flags[@]}" -E $'\t'"($allowed)\$" | report "$rule"
}

hex='[0-9A-Fa-f]'
# The match takes the whole colon- or dash-separated run, so only a run of exactly six octets is a MAC; a longer
# one is a certificate fingerprint.
scan "MAC address" -i "\\b($hex{2}:)+$hex{2}\\b|\\b($hex{2}-)+$hex{2}\\b" "(00[:-]){5}00|($hex{2}[:-]){0,4}$hex{2}|($hex{2}[:-]){6,}$hex{2}"

quote="[\"']"
scan "serial number" -i \
  "(serial_number|product_serial|board_serial|chassis_serial|product_uuid|id_serial(_short)?)$quote?[[:space:]]*[=:][[:space:]]*$quote?[A-Za-z0-9][^\"'[:space:]]*" \
  "[^[:space:]=:]+$quote?[[:space:]]*[=:][[:space:]]*$quote?SCRUBBED"

scan "tailnet name" -i '[a-z0-9-]+\.ts\.net\b' 'example\.ts\.net'

# 100.64.0.0/10 is the whole tailnet range and 100.64.0.0/24 is reserved for fixtures; its last address (the
# destination check's upper bound) and Tailscale's fixed 100.100.100.100 service address identify nobody. Any
# other address, with or without a prefix length, does.
scan "tailnet IPv4 address" '' \
  '\b100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.[0-9]{1,3}\.[0-9]{1,3}(/[0-9]+)?' \
  '100\.64\.0\.0/10|100\.64\.0\.[0-9]{1,3}(/[0-9]+)?|100\.127\.255\.255|100\.100\.100\.100'

scan "tailnet IPv6 address" -i '\bfd7a:115c:a1e0:[0-9a-f:]+(/[0-9]+)?' 'fd7a:115c:a1e0::([0-9a-f]{1,4})?(/[0-9]+)?|fd7a:115c:a1e0(:ffff){5}'

# Only an absolute path counts in contents (a trailing period ends the sentence, not the name). In file names
# only a captured tree under contract/ can hold a home directory; elsewhere `home` is an ordinary package name.
scan "home directory" '' "(^|[[:space:]\"'\`(=:])/home/[A-Za-z0-9_-]+([.][A-Za-z0-9_-]+)*" ".?/home/(user|owner|example)"
printf '%s\n' "${paths[@]}" | keep -E '^contract/(.*/)?home/[^/]+/' | keep -vE '/home/(user|owner|example)/' \
  | report "home directory in path"

printf '%s\n' "${paths[@]}" | keep -E '(^|/)(serial_number|product_serial|board_serial|chassis_serial|product_uuid)$' \
  | while IFS= read -r path_file; do
      if [ "$mode" = staged ]; then value=$(git show ":$path_file"); else value=$(cat "$path_file"); fi
      [ "$value" = SCRUBBED ] || echo "$path_file"
    done | report "serial number file"

denylist=${ANSHIN_IDENTIFIERS:-$(git rev-parse --path-format=absolute --git-common-dir)/info/identifiers}
if [ -f "$denylist" ]; then
  while IFS= read -r entry || [ -n "$entry" ]; do
    entry=${entry%%#*}
    entry=$(printf '%s' "$entry" | tr -d '[:space:]')
    [ -n "$entry" ] || continue
    if [[ $entry == user:* ]]; then
      name=${entry#user:}
      content -i "/home/$name\\b|User=$name\\b|\\b$name@" | report "private denylist (username)"
      printf '%s\n' "${paths[@]}" | keep -iE "(^|/)home/$name(/|$)" | report "private denylist (username in path)"
    else
      content -i -F "$entry" | report "private denylist"
      printf '%s\n' "${paths[@]}" | keep -iF "$entry" | report "private denylist (path)"
    fi
  done < "$denylist"
fi

if [ "$found" -ne 0 ]; then
  echo "Identifier scan failed: scrub the matches above, or use a placeholder (see CONTRIBUTING.md)." >&2
  exit 1
fi
