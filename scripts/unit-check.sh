#!/usr/bin/env bash
# Pin the hardening without hiding measurements: ProcSubset=pid would hide required /proc files.
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <unit file>" >&2
  exit 2
fi

pinned='[Unit] Description=Anshin Agent
[Unit] Documentation=https://github.com/fsrodrig/anshin
[Unit] After=tailscaled.service
[Unit] Wants=tailscaled.service
[Unit] StartLimitIntervalSec=0
[Service] ExecStart=/usr/bin/anshin-agent
[Service] DynamicUser=yes
[Service] NoNewPrivileges=yes
[Service] CapabilityBoundingSet=
[Service] ProtectSystem=strict
[Service] ProtectHome=read-only
[Service] RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
[Service] MemoryMax=64M
[Service] Restart=always
[Service] RestartSec=5
[Install] WantedBy=multi-user.target'

got=$(awk '/^[[:space:]]*([#;]|$)/ { next } /^\[/ { section = $0; next } { print section " " $0 }' "$1" | LC_ALL=C sort)
want=$(LC_ALL=C sort <<< "$pinned")
if [ "$got" != "$want" ]; then
  diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2 || true
  echo "unit-check: $1 differs from the pinned unit (< pinned, > $1)" >&2
  exit 1
fi
