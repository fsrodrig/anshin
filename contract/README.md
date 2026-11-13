# The Pulse contract

This directory is the contract between the Agent and the phone ([ADR 0005](../docs/adr/0005-golden-fixtures-are-the-contract.md)).
There is no OpenAPI document: the golden fixtures in [`fixtures/`](fixtures/) and this file are the source of truth.

- The Agent serves one endpoint, `GET /v1/pulse`, and answers with one JSON object, the **Pulse**.
- Every Pulse carries `"schema": 1`. Within schema 1, changes are additive only. A breaking change means `/v2`,
  served alongside v1.
- The fixtures are generated from [`captures/`](captures/): real Machines' `/proc` and `/sys`, scrubbed, reviewed and
  replayed through the real recorder and server. The phone decodes every one of them.

## Field glossary

| Field | Type | Meaning |
|---|---|---|
| `schema` | integer | Always `1`. The phone treats any other value as **Incompatible**. |
| `kind` | open string | `full`: the phone replaces what it had. `delta` (from #8): newer **Samples** only. The phone decodes any other value, or a missing one, as unknown. |
| `hostname` | string or `null` | The Machine's kernel hostname. `null` when unreadable or empty. |
| `cpu_pct` | number or `null` | Percent of all CPUs' time spent busy since the previous baseline, 0–100, one decimal. `null` on the first reading, after a counter reset or a change in the CPU count, or when `/proc/stat` is unreadable. |

## Rules

- JSON, snake_case, with the unit in the name: `_pct`, `_bytes`, `_bps`, `_c`, `_w`, `_s`, `_ts`.
- `null` means "not available on this Machine". It is never `0` or `-1`.
- Series are columnar, with explicit timestamps. A **Gap** is an explicit run of `null`, never interpolated.
- Cumulative counters that could exceed 2^53 are decimal strings.
- Enum-like values (`kind`, the **Verdict** level, **Reason** codes, battery status) are open strings. The phone
  keeps an "unknown" fallback, as `PulseKind` does in `Pulse.kt`, instead of rejecting the Pulse.

## Cursor rules

Today the Agent ignores `after` and `wait`, and every response is `full`. From #8 these rules bind, as in
[design § Responses](../docs/design.md#responses):

- No `after`, or a cursor that is no longer valid → `kind: "full"`. A cursor is no longer valid when its epoch is
  from another Agent run, it is older than the oldest retained Sample, or it is in the future.
- A valid cursor → `kind: "delta"`. With `wait`, the Agent holds the request until a newer Sample exists, up to
  `wait` seconds.
- A malformed cursor → `400`. Anything but `GET` → `405`.

## Fixtures

- `fixtures/<capture>.json` is generated, one per capture. Never edit one by hand.
- `fixtures/future.json` is the only hand-written fixture. It has a field no Agent sends yet and an unknown Reason
  code, so the phone proves it reads a newer Agent's Pulse.
- `fixtures/kept/<YYYY-MM-DD>/` holds old generated fixtures. Before a pull request changes the wire shape, copy
  the current generated fixtures there. They prove a newer phone still reads older Agents. Nothing regenerates or
  deletes them.

Regenerate after a change to the recorder, the server or a capture, then read the diff before committing:

```sh
cd agent && go test ./internal/server -run '^TestGolden' -update
```

`-update` is defined only in `internal/server`, so `go test ./... -update` fails in every other package with
`flag provided but not defined`. Go's test cache doesn't notice changes under `contract/`, which is outside the Go
module, so after editing a capture or a fixture run the tests with `-count=1`.

`make check-agent` (and so CI) runs the same command and fails if `contract/fixtures` then differs from what is
committed, untracked files included. Commit fixture changes before running it locally. A fixture without a
capture fails `TestGoldenFixturesMatchCaptures`, and `-update` deletes it.

## Captures

A **Capture** is a scrubbed, reviewed copy of the files the Agent reads on a real Machine, taken in two or more
snapshots about 2 s apart. Tests replay it; nothing in a test reads the host it runs on.

### Layout

```text
captures/<name>/
  capture.json     {"format":1,"cpus_online":N,"snapshots":[{"wall":"<RFC 3339 UTC>","boottime_ns":N,"monotonic_ns":N}, …]}
  0/ 1/ 2/         one per snapshot, in order
    index          one line per path, sorted, tab-separated:
                     d <path>                 a directory
                     f <path>                 a file, copied byte for byte at that path
                     l <path> <basename> [dangling]   a symlink, target basename and optional missing-target marker
                     e <path> <ERRNO>         a file or directory read, or traversal, that failed; may coexist with d
    statfs.json    per mount point: the statfs(2) result and mount ID, {"error":"EACCES"}, {"error":"timeout"},
                   or {"skipped":"autofs|network|fuse|hidden/unverified"}
    proc/… sys/… etc/…
```

- `cpus_online` is parsed from `/sys/devices/system/cpu/online` at capture time. That is a kernel source
  independent of `/proc/stat`, so the probe replay test checks the CPU count against it. The capture name is the
  hostname placeholder, so the same test checks the hostname against it.
- Wall time is UTC, so it doesn't reveal the time zone. `boottime_ns − monotonic_ns` grows only while the Machine
  is suspended.
- Links are in the index, never on disk. Replay rebuilds them as real symlinks, so `ReadLink`, `Lstat` and the
  entry types `ReadDir` reports behave as on a Machine. A link's target is recorded by its basename only, because
  the full target is outside the capture, so a probe may use only `path.Base` of what `ReadLink` returns.
- A link with the optional `dangling` marker has a missing target. Without it, replay creates a target.
- A directory can have both `d` and `e` entries: it exists, but listing it failed with the recorded errno.
- `proc/self/mountinfo` is the mount namespace of the shell that ran the capture, not of the Agent's systemd unit.

### What is copied

- `/proc/{stat,loadavg,uptime,meminfo,swaps,diskstats,net/dev,self/mountinfo}`,
  `/proc/sys/kernel/{hostname,osrelease}` and `/etc/os-release`.
- For each `/sys/class/hwmon/*`: `name`, the `device` link and `device/power/runtime_status`; then the
  `temp*_input`, `temp*_label` and `temp*_crit` files, except on these devices:
  - storage sensors (`nvme`, `drivetemp`): every NVMe read sends the drive a command and drivetemp reads can reset
    an HDD's spin-down timer, so only `name` is read;
  - devices backed by a thermal zone (`device` → `thermal_zoneN`): the Agent ignores them, and some (iwlwifi)
    answer with a firmware command;
  - runtime-suspended devices: reading them would wake them.
- For each `/sys/class/power_supply/*`: the battery attributes in
  [design § Battery](../docs/design.md#battery). Never `serial_number` or `model_name`.
- For each `/sys/class/{net,block}/*`: the `device` link only. Nothing is read through it, never `address`.
- `statfs` and the mount ID of each mount point in that snapshot's mountinfo. The topmost entry at a path
  determines its filesystem type. Autofs, network filesystems and FUSE are never statfs'd, because that could
  mount something or block on a server or daemon. Their skip values are `autofs`, `network` and `fuse`.
  A mount below any such unsafe ancestor is also skipped with that ancestor's skip value.
  Before statfs, statx with AT_NO_AUTOMOUNT must return the mount ID expected by this snapshot's mountinfo.
  A failed check or mismatched ID records `hidden/unverified` without calling statfs. A pending result whose
  mount ID differs from the current snapshot is also `hidden/unverified`. Capture requires the MountID adapter.
  Each call is bounded by `-statfs-timeout`; a blocked call remains pending across snapshots.

`/etc/hostname` and `/etc/passwd` are read only to learn what to scrub, and never copied. A path not on the
allowlist (`capture.Allowed`) can't be in a capture: replay rejects it, and the probe replay test fails when a
probe opens one. A probe that starts reading a new file adds it to the allowlist, and the captures are taken again.

### Scrubbing

Machine-derived file contents, paths, link targets, mount-point keys and error strings go through one scrubber.
Capture metadata is encoded from scrubbed values, leaving structural keys and kinds intact. Its rules mirror
[`scripts/identifier-scan.sh`](../scripts/identifier-scan.sh), in this order:

| Rule | What it matches | Replaced with |
|---|---|---|
| serial number | `serial_number`, `product_serial`, `board_serial`, `chassis_serial`, `product_uuid`, `id_serial(_short)` followed by `=` or `:` and a value | `SCRUBBED` |
| MAC address | a run of six or more two-digit hex octets joined by `:` or `-` | `00` octets in the same shape |
| MAC-named interface | `enx`, `wlx` or `wwx` followed by 12 hex digits | `enx000000000000` (and so on) |
| hostname | the kernel hostname and `/etc/hostname`, in full and the first label, anywhere, case-insensitively | the capture's `-name` |
| private denylist | each plain entry in `-identifiers`, anywhere, case-insensitively | `scrubbed` |
| home directory | the path component after `/home/`, `/var/home/`, `/run/media/` or `/media/` | `user` |
| username | the current user, login accounts in `/etc/passwd` and `user:` entries in `-identifiers`, when not adjacent to ASCII letters or digits; a preceding backslash or mountinfo octal escape is also a boundary | `user` |
| tailnet name, IPv4, IPv6 | `*.ts.net`, `100.64.0.0/10` outside the fixture range, `fd7a:115c:a1e0:…` | `example.ts.net`, `100.64.0.1`, `fd7a:115c:a1e0::1` |

- Known usernames are scrubbed as whole tokens in contents and paths, case-insensitively. A pathological short
  username may over-scrub ordinary paths such as `/sys/fs/cgroup`; privacy takes precedence over fidelity.
- The capture refuses to start when `-name` contains a hostname or denylist entry, when an entry is shorter than
  3 characters, or when it hides inside a placeholder.
- Two source paths that scrub to the same path stop the capture. Two Bluetooth peripherals whose battery names
  carry their MACs do this: disconnect one and capture again.
- Patterns can't recognise every identifier. Put the names of your other Machines and your tailnet in the denylist
  before capturing, and review the result.

### Taking a capture of another Machine

Run this in a clone of the repository on that Machine, as your own user. It needs no root and no `tailscaled`, and
writes only into the capture directory.

```sh
mise install && make setup
(cd agent && CGO_ENABLED=0 go build -trimpath -o dist/anshin-agent ./cmd/anshin-agent)
denylist="$(git rev-parse --path-format=absolute --git-common-dir)/info/identifiers"
agent/dist/anshin-agent capture -name <name> -identifiers "$denylist" contract/captures/<name>
```

- `<name>` is the placeholder for the Machine's hostname and names its fixture. It is lowercase letters, digits
  and dashes, and is neither `future` nor `kept`. Pick a word such as `laptop` or `desktop`.
- The report on stdout lists the tree, the replacements by rule and file, and every skipped device and mount with
  the reason. It never prints a raw identifier, so you can paste it into the pull request.

Review it before committing:

1. Read every file in `contract/captures/<name>`.
2. Check that this Machine's identifiers appear nowhere. Each command must print nothing:

   ```sh
   c=contract/captures/<name>; h=$(cat /proc/sys/kernel/hostname); u=$(id -un)
   grep -rniIF -e "$h" -e "${h%%.*}" "$c"; find "$c" | grep -iF -e "$h" -e "${h%%.*}" -e "$u"
   for a in $(sort -u /sys/class/net/*/address | grep -v '^00:00:00:00:00:00$'); do grep -rniF "$a" "$c"; find "$c" | grep -iF "$a"; done
   for s in /sys/class/power_supply/*/serial_number; do [ -r "$s" ] && grep -rnF "$(cat "$s")" "$c"; done
   ```

3. Check that the replacement counts in the report make sense.
4. `git add` the capture and run `scripts/identifier-scan.sh --staged`.
5. Regenerate the fixtures (above), read `contract/fixtures/<name>.json`, and commit both.

## Tests

- **Collector correctness** (are the numbers right): `agent/internal/recorder/probe` replays every capture and
  checks each probe against what the capture recorded; `agent/internal/recorder` runs scripted `/proc/stat`
  sequences through counter resets and CPU-count changes.
- **Wire compatibility** (does the JSON decode): `agent/internal/server` emits the fixtures through loopback HTTP
  with the fake tailnet, and the phone's `ContractFixturesTest` decodes every file under `fixtures/`, `kept/`
  included.
