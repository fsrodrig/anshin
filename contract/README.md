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
| `filesystems` | array or `null` | Local filesystems grouped by mountinfo MAJ:MIN, labeled by their shortest mount point. Null when none are visible or discovery is unavailable. Each row has `maj_min`, `mount_point`, `fs_type`, independently nullable `total_bytes`, `used_bytes`, `available_bytes`, `used_pct`, and `age_s`. |
| `primary_disk` | string or `null` | The MAJ:MIN reference to the listed filesystem whose visible mount holds `/`. Never inferred from list order. |
| `cpu_pct` | number or `null` | Percent of all CPUs' time spent busy since the previous baseline, 0–100, one decimal. `null` on the first reading, after a counter reset or a change in the CPU count, or when `/proc/stat` is unreadable. |
| `mem_total_bytes`, `mem_available_bytes` | integer or `null` | MemTotal and MemAvailable from meminfo, converted from KiB to bytes. Each field is independently nullable. |
| `mem_used_bytes` | integer or `null` | Total minus available. Null if either input is unavailable or available exceeds total. |
| `mem_used_pct` | number or `null` | Used / total × 100, rounded to one decimal. Null when used is unavailable or total is zero. A measured zero remains zero. |
| `swap_total_bytes`, `swap_used_bytes` | integer or `null` | Sum of sizes and usage respectively in swaps. Null with no swap entries, unreadable input, malformed rows or overflow; a bad size or usage invalidates only its aggregate. |
| `swap_entries` | array or `null` | One row per parseable swap entry; null when none are available. Each row has `name` from the kernel, `zram` true only for `/dev/zram` followed by digits, and independently nullable `total_bytes` and `used_bytes`. Used greater than total is invalid. |
| `load_1`, `load_5`, `load_15` | number or `null` | Nonnegative load averages over 1, 5 and 15 minutes. Each is independently nullable. |
| `uptime_s` | number or `null` | Nonnegative seconds since boot, including suspend, from uptime. |
| `cpu_temp_c` | number or `null` | The **CPU sensor**'s temperature, equal to its `temp_c` in `temperatures`. Null when no CPU sensor is chosen (none recognised, or `ANSHIN_CPU_SENSOR` matches no listed sensor), or when the chosen sensor's input is unreadable, not an integer, or below absolute zero. A measured zero remains zero. |
| `cpu_sensor` | string or `null` | The CPU sensor's ID, in the syntax `ANSHIN_CPU_SENSOR` takes. It always names a row of `temperatures`. Null when no CPU sensor is chosen. |
| `temperatures` | array or `null` | Every listed hwmon temperature, in device order, then index order, including the CPU sensor. Each row is `{"sensor": string, "temp_c": number or null}`; `temp_c` is null when the input is unreadable, not an integer, or below absolute zero. Null when no sensor is listed. Storage sensors, thermal-zone-backed devices and runtime-suspended devices are never listed. |
| `net_rx_bps`, `net_tx_bps` | integer or `null` | Bytes per second received and sent since the previous baseline, summed over the interfaces backed by a device (`/sys/class/net/<if>/device` exists), so loopback, tunnels, bridges and veths don't count traffic twice. Each is independently nullable: null with no such interface, when `/proc/net/dev` is unreadable, when any counted interface's rate is null or it appeared or disappeared in the interval, or when any interface's device link can't be determined. |
| `net_interfaces` | array or `null` | Every interface in `/proc/net/dev`, in kernel order; null when it is unreadable or lists none. Each row has `name`, `device_backed` (true or false from the existence of the `device` link alone, never `address`; null when that can't be determined) and independently nullable `rx_bps` and `tx_bps`. |
| `disks` | array or `null` | Every disk in `/proc/diskstats` backed by a device (`/sys/class/block/<dev>/device` exists; a `/` in a diskstats name is `!` in sysfs), in kernel order; null when it is unreadable or lists none. Partitions and dm devices (LUKS, LVM) are left out because the kernel already counts their I/O on the parent disk; zram and loop devices have no `device` link. A disk whose link can't be determined is left out. Each row has `name` from the kernel and independently nullable `read_bps` and `write_bps`: sectors × 512 bytes, always, per the kernel's iostats documentation. |
| `distro`, `kernel`, `arch`, `agent_version` | string or `null` | Slow lane, cached for 30 seconds. Distro is os-release PRETTY_NAME, falling back to NAME; kernel is osrelease; arch is uname's machine field, never the Agent binary's target architecture. Agent version is injected at build time with `-ldflags=-X=main.version=<release>` or read from Go build info, which reports `(devel)` for local builds. Unavailable values remain null. |

Byte measurements and their aggregates are bounded by signed 64-bit integers for phone compatibility. Conversion or sum overflow yields null. Load and uptime reject negative and non-finite values. Missing sensors never fail the rest of the reading.

A rate is a counter delta divided by the actual elapsed time on the monotonic clock, rounded to a whole number. Each direction keeps its own baseline, latest observation and retained delta with its elapsed time. A counter that goes backwards or is malformed resets only its own baseline and yields `null` for that interval. After an unavailable counter becomes valid, its rate stays `null` until a full second of valid observations has elapsed. A disappeared interface or disk is omitted from the detail rows; a counted interface disappearing also makes the network summary `null` for that interval. An appearing or reappearing row starts with `null` rates and a new baseline. A change in an interface's device link also resets its baseline and yields `null` rates, including the summary when it was or may now be counted. The first reading's rates are measured from baselines taken when the Agent starts.

Until #8 introduces the sampling loop and Window, fast measurements refresh on each read. CPU, network and disk keep a one-second baseline minimum: a read less than a second after the baseline repeats the previous rates and keeps the baseline for unaffected counters. Resets, appearances, disappearances and device-link changes are detected on every read, including sub-second reads. Suspend detection comes with #8. The slow-lane cache belongs to the recorder, under its lock, and refreshes on the first read at or after 30 seconds.

## Rules

- JSON, snake_case, with the unit in the name: `_pct`, `_bytes`, `_bps`, `_c`, `_w`, `_s`, `_ts`. `_bps` is
  **bytes** per second, never bits.
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

## Filesystem capacity

Discovery reads `/proc/self/mountinfo`, filtering pseudo, network, FUSE, PrivateTmp and image mounts before
any capacity call. Descendants of an excluded mount are excluded too. Local read-only filesystems and
major-zero btrfs mounts remain eligible. Before statfs, statx must return the entry's mount ID. A hidden
mount falls back to the next-shortest eligible mount in the same MAJ:MIN group; a group with none visible
is omitted. The label remains the shortest mount point, even when capacity comes through a fallback.

Capacity refreshes every 30 s. Until #8 owns continuous sampling, the first Read starts discovery and
subsequent Reads schedule refreshes when due, matching Machine info. Slow calls run outside the recorder
lock. One pending worker per MAJ:MIN group and ownership of each path survive disappearance and
replacement. Blocked calls are never replaced; their row has null capacity and an age. Changed mount
identities invalidate late results. A removed group is omitted; an unchanged group that reappears while
its call is pending retains its unavailable row. Previously returned rows never change underneath a caller.

`age_s` is whole seconds since the launch of the last successful capacity call, or the initial attempt if there
has been no success. A call is stamped with the time of the read that launched it, not when it returns, so the
recorder never consults a clock of its own and `age_s` can only over-report, by the call's duration. A refresh clears all previous capacity values immediately, so blocked or failed work cannot
present old values as current. Pending visibility checks have no row until verified.

The fragment size is the capacity unit, falling back to block size when unavailable. Total is blocks × unit;
used is blocks minus free blocks × unit; available is available blocks × unit. Products exceeding signed
64-bit range and invalid counters become null independently. Percent is used / (used + available) × 100,
rounded to one decimal; an unavailable input, overflowing sum or zero denominator makes it null.
A measured zero remains zero. Older Agents omit both filesystem fields; the phone decodes them as null.

## Temperatures

- Hwmon driver reads run outside the recorder mutex, with one shared 50 ms wait bound per Read and at most one outstanding operation per device. Concurrent Reads share the in-flight Read's result.
- Enumeration checks the ordered guards once per device per Read. Workers read runtime status, then check directory identity immediately before each sensor attribute open, and check again after the last open; unchanged identities reuse settled indices and labels. Transient label errors leave discovery unknown and omit that device’s sensors for the Read; discovery retries on the next Read. Absent or empty labels settle as `tempN`.
- Only the CPU sensor chosen in this Read has its static `tempN_crit` read, within the same deadline and device ownership. The threshold is cached per device identity and sensor index and discarded when identity changes. A threshold that misses the deadline is null for that Read.
- Pending devices retain their last completed sensor IDs with null temperatures. Late completions can refresh IDs for the same identity, but never supply current values; failed discovery is retried.
- CPU selection stays unavailable while IDs are unknown for a possible CPU device: the override's hwmon name when set, or a recognised CPU driver otherwise. Unrelated discovery does not suppress the CPU sensor.

The Agent lists `/sys/class/hwmon/*` in hwmon order: by name length, then name, so `hwmon2` comes before `hwmon10`.
For each device it reads the guards in order and stops at the first that rules the device out:

1. `name`. Unreadable or empty: skip. A storage sensor (`nvme`, `drivetemp`): skip.
2. The `device` link. Absent: read the device, with no status check. Any other error, including a `device` that is
   not a link: skip. A link to `thermal_zoneN`: skip.
3. `device/power/runtime_status`. Unreadable: skip. Missing, with no `device/power` either: skip. Missing, with
   `device/power` present: read. `suspended`: skip.

Then it lists `temp*_input` in index order and reads each one's `tempN_label`. A sensor's ID is
`<hwmon name>/<label>`, with the label trimmed, or `tempN` when it is missing or empty. Values are
millidegrees, converted to °C and rounded to one decimal. Unreadable, non-integer or below-absolute-zero input
gives `null`.

The CPU sensor is `ANSHIN_CPU_SENSOR` when set: the first listed sensor with that ID, else none. Otherwise it is the
first listed match of coretemp `Package id 0`, then k10temp/zenpower `Tdie`, then `Tctl`, then any sensor of
`coretemp`, `k10temp`, `zenpower`, `k8temp` or `via_cputemp`. The choice is by identity: an unreadable chosen
sensor gives `cpu_temp_c: null`, never another sensor's value. Only the CPU sensor's `tempN_crit` is read; the
Verdict uses it, and it isn't on the wire. Older Agents omit the three temperature fields; the phone decodes them as
null.

## Fixtures

- `fixtures/<capture>.json` is generated, one per capture. Never edit one by hand.
- `fixtures/future.json` is the only hand-written fixture. It has a field no Agent sends yet and an unknown Reason
  code, so the phone proves it reads a newer Agent's Pulse.
- `fixtures/kept/<YYYY-MM-DD>/` holds old generated fixtures. Before a pull request changes the wire shape, copy
  the current generated fixtures there. They prove a newer phone still reads older Agents. Nothing regenerates,
  overwrites or deletes them. If that day's directory already exists, use `<YYYY-MM-DD>-2`, then `-3`, and so on.
  One older set predates that suffix and sits in a named subdirectory instead: `kept/2026-11-14/filesystems/` holds
  the fixtures from before filesystems were added, `kept/2026-11-14-2/` the ones from before network and disk
  rates, and `kept/2026-11-14-3/` the ones from before temperatures. The phone's `ContractFixturesTest` reads every
  `.json` file under `fixtures/`, at any depth.

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
  capture.json     {"format":1,"arch":"<uname machine, optional>","cpus_online":N,"snapshots":[{"wall":"<RFC 3339 UTC>","boottime_ns":N,"monotonic_ns":N}, …]}
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

- `arch` is optional metadata recorded from the live uname machine field and scrubbed. Old Captures without it replay architecture as null, never the test runner's architecture. Agent version is not replayed from the runner.
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
- For each `/sys/class/hwmon/*`, in order: `name`; then, except on storage sensors, the `device` link; then, except
  on thermal-zone devices, `device/power/runtime_status`; then, except on runtime-suspended devices, the
  `temp*_input`, `temp*_label` and `temp*_crit` files. The rule is `probe.HwmonSkip`, shared with the probe, so a
  Capture holds every file the probe reads. The devices are:
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
| username | the current user, login accounts in `/etc/passwd` and `user:` entries in `-identifiers`, when not adjacent to ASCII letters or digits; a preceding backslash is a boundary, and a preceding mountinfo octal escape or percent escape is a boundary only when its decoded byte is not an ASCII letter or digit | `user` |
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
- Later index and skip sections compare with snapshot 0. `+` adds an entry or skip reason absent from snapshot 0;
  `-` removes one present there. Empty later skip sections say `unchanged`; the initial section says `none`
  only when nothing was skipped. Displayed links include `dangling` when their target is missing.

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
  checks each probe against what the capture recorded, including which interfaces and disks are device-backed and
  that the left-out partitions and LUKS device add nothing the disk doesn't count; `agent/internal/recorder` runs
  scripted `/proc/stat`, `/proc/net/dev` and `/proc/diskstats` sequences with explicit time values through counter
  resets, CPU-count changes, interfaces and disks appearing and disappearing, and division by elapsed time, and
  scripted mount tables and capacity calls with explicit time values through blocked, failed, hidden and replaced
  mounts.
- **Wire compatibility** (does the JSON decode): `agent/internal/server` emits the fixtures through loopback HTTP
  with the fake tailnet, and the phone's `ContractFixturesTest` decodes every file under `fixtures/`, `kept/`
  included.
