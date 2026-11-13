#!/usr/bin/env bash
# Runs the identifier scan against throwaway repositories. Identifiers are assembled at runtime so this file
# never holds one itself.
set -euo pipefail
unset ANSHIN_IDENTIFIERS
scan=$(cd "$(dirname "$0")" && pwd)/identifier-scan.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

failures=0
# expect <pass|fail> <mode> <path> <content> [denylist entry]
expect() {
  local want=$1 mode=$2 path=$3 text=$4 deny=${5-} repo got=pass
  repo=$(mktemp -d "$work/repo.XXXX")
  git -C "$repo" init -q
  mkdir -p "$repo/$(dirname "$path")"
  printf '%s\n' "$text" > "$repo/$path"
  git -C "$repo" add -A
  if [ -n "$deny" ] && [ -n "${no_newline-}" ]; then
    printf '%s' "$deny" > "$repo/.git/info/identifiers"
  elif [ -n "$deny" ]; then
    printf '%s\n' "$deny" > "$repo/.git/info/identifiers"
  fi
  if [ "$mode" = staged ]; then
    (cd "$repo" && "$scan" --staged >/dev/null 2>&1) || got=fail
  else
    (cd "$repo" && "$scan" >/dev/null 2>&1) || got=fail
  fi
  if [ "$got" != "$want" ]; then
    printf 'FAIL: expected %s (%s) for %s: %s\n' "$want" "$mode" "$path" "$text" >&2
    failures=$((failures + 1))
  fi
}

mac=$(printf '%s:' 3c 7c 3f 1a 2b | sed 's/:$//'):9d
tailnet_v4="100.$((64 + 37)).12.7"
tailnet_v6="fd7a:115c:a1e0:$(printf 'ab%02d' 12):4843::1"
host="box-$((40 + 2))"
user="ann$((4 + 3))"

for mode in worktree staged; do
  expect pass "$mode" docs/a.md 'Placeholders: 00:00:00:00:00:00, 100.64.0.1, 100.64.0.0/10, fd7a:115c:a1e0::/48, example.ts.net, /home/user, serial_number=SCRUBBED'
  expect fail "$mode" docs/a.md "link $mac up"
  expect fail "$mode" contract/uevent "POWER_SUPPLY_SERIAL_NUMBER=$((12345 * 7))"
  expect fail "$mode" contract/serial_number "$((12345 * 7))"
  expect pass "$mode" contract/serial_number SCRUBBED
  expect fail "$mode" docs/a.yaml "product_serial: \"PF$((12345 * 7))\""
  expect fail "$mode" contract/a.json "{\"serial_number\": \"PF$((12345 * 7))\"}"
  expect pass "$mode" contract/a.json '{"serial_number": "SCRUBBED"}'
  expect fail "$mode" "fixtures/$host.tail$((1000 + 234)).ts.net/stat" data
  expect fail "$mode" "fixtures/$tailnet_v4/stat" data
  expect fail "$mode" "fixtures/$tailnet_v6/stat" data
  expect fail "$mode" "fixtures/${mac}/stat" data
  expect fail "$mode" contract/net/dev "  enx${mac//:/}: 1 2"
  expect fail "$mode" "contract/sys/class/net/wlx${mac//:/}/device" data
  expect pass "$mode" contract/net/dev '  enx000000000000: 1 2'
  expect pass "$mode" "contract/sys/class/net/enx000000000000/device" data
  expect fail "$mode" docs/a.md "allow $tailnet_v4/32"
  expect pass "$mode" docs/a.md "fixture peer 100.64.0.7/32"
  expect fail "$mode" docs/a.md "reach it at $host.tail$((1000 + 234)).ts.net"
  expect fail "$mode" docs/a.md "peer $tailnet_v4 answered"
  expect fail "$mode" docs/a.md "peer $tailnet_v6 answered"
  expect fail "$mode" docs/a.md "/home/$user/.config"
  expect fail "$mode" "contract/captures/home/$user/x" data
  expect fail "$mode" "agent/internal/probe/testdata/tree/home/$user/x" data
  expect pass "$mode" android/app/src/main/java/ui/home/HomeScreen.kt data
  expect fail "$mode" docs/a.md "w: file:///home/$user/Repos/x/Foo.kt:10:5 warning"
  expect fail "$mode" docs/a.md "cmd >/home/$user/out.log"
  expect fail "$mode" docs/a.md "[/home/$user/x]"
  expect pass "$mode" docs/a.md 'See file:///home/user/x, ./home/x, ~/home/y and https://example.com/home/page'
  expect pass "$mode" docs/a.md 'Keep it under /home/user. Relative src/home/screen paths are fine.'
  expect pass "$mode" docs/a.md 'Range edges: 100.127.255.255 and fd7a:115c:a1e0:ffff:ffff:ffff:ffff:ffff'
  expect pass "$mode" docs/a.md "Signing certificate SHA-256: $(printf 'a%d:' 1 2 3 4 5 6 7 8 9)$(printf 'b%d:' 1 2 3 4 5 6 7 8 9)c0"
  expect pass "$mode" docs/a.md 'Started at 10:30:15'
  no_newline=1 expect fail "$mode" docs/a.md "on $host today" "$host"
  expect fail "$mode" docs/a.md "on $host today" "$host"
  expect fail "$mode" "fixtures/$host/stat" data "$host"
  expect pass "$mode" docs/a.md "package io.github.$user.anshin" "user:$user"
  expect fail "$mode" docs/a.md "User=$user" "user:$user"
  expect fail "$mode" docs/a.md "ssh $user@machine" "user:$user"
done

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "identifier-scan: all cases pass"
