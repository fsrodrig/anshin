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
| `kind` | open string | `full`: `window` holds every retained row and replaces what the phone has. `delta`: `window` holds only the rows newer than the request's **Cursor**, possibly none. The phone decodes any other value, or a missing one, as unknown. |
| `cursor` | string | The **Cursor** of the newest row, `<epoch>.<seq>` ([§ Cursor rules](#cursor-rules)). The phone sends it back as `after`. Absent from Agents before the Window. |
| `window` | object | The rows the Cursor asked for ([§ Window](#window)). Absent from Agents before the Window, and the phone treats absent or `null` as no Window. |
| `hostname` | string or `null` | The Machine's kernel hostname. `null` when unreadable or empty. |
| `filesystems` | array or `null` | Local filesystems grouped by mountinfo MAJ:MIN, labeled by their shortest mount point. Null when none are visible or discovery is unavailable. Each row has `maj_min`, `mount_point`, `fs_type`, independently nullable `total_bytes`, `used_bytes`, `available_bytes`, `used_pct`, and `age_s`. |
| `primary_disk` | string or `null` | The MAJ:MIN reference to the listed filesystem whose visible mount holds `/`. Never inferred from list order. |
| `cpu_pct` | number or `null` | Percent of all CPUs' time spent busy since the previous baseline, 0–100, one decimal. `null` on the first Sample, the first Sample after a Gap, after a counter reset or a change in the CPU count, or when `/proc/stat` is unreadable. |
| `cpu_core_pct` | array or `null` | Whole-percent busy per CPU, 0–100, over the same interval as `cpu_pct`, one entry per CPU listed when the Agent first read `/proc/stat` (normally at start), in kernel order. An entry is `null` whenever `cpu_pct` is, or when that CPU is missing from `/proc/stat` or its own counters went backwards or didn't advance. A CPU id that appears later is not listed until the Agent restarts. `null` while no per-CPU line has been readable; every `cpuN` line must parse, or none counts. |
| `mem_total_bytes`, `mem_available_bytes` | integer or `null` | MemTotal and MemAvailable from meminfo, converted from KiB to bytes. Each field is independently nullable. |
| `mem_used_bytes` | integer or `null` | Total minus available. Null if either input is unavailable or available exceeds total. |
| `mem_used_pct` | number or `null` | Used / total × 100, rounded to one decimal. Null when used is unavailable or total is zero. A measured zero remains zero. |
| `swap_total_bytes`, `swap_used_bytes` | integer or `null` | Sum of sizes and usage respectively in swaps. Null with no swap entries, unreadable input, malformed rows or overflow; a bad size or usage invalidates only its aggregate. |
| `swap_entries` | array or `null` | One row per parseable swap entry; null when none are available. Each row has `name` from the kernel, `zram` true only for `/dev/zram` followed by digits, and independently nullable `total_bytes` and `used_bytes`. Used greater than total is invalid. |
| `load_1`, `load_5`, `load_15` | number or `null` | Nonnegative load averages over 1, 5 and 15 minutes. Each is independently nullable. |
| `uptime_s` | number or `null` | Nonnegative seconds since boot, including suspend, from uptime. |
| `cpu_temp_c` | number or `null` | The **CPU sensor**'s temperature, equal to its `temp_c` in `temperatures`. Null when no CPU sensor is chosen (none recognised, or `ANSHIN_CPU_SENSOR` matches no listed sensor), or when the chosen sensor's input is unreadable, not an integer, or below absolute zero. A measured zero remains zero. |
| `cpu_sensor` | string or `null` | The CPU sensor's ID, in the syntax `ANSHIN_CPU_SENSOR` takes. It always names a row of `temperatures`. Null when no CPU sensor is chosen. |
| `temperatures` | array or `null` | Every listed hwmon temperature, in device order, then index order, including the CPU sensor. Each row is `{"sensor": string, "temp_c": number or null, "age_s": integer or null}`; `temp_c` is null when the input is unreadable, not an integer, or below absolute zero. `age_s` is whole seconds since the Sample that launched the read which produced `temp_c`: `0` when read in this Sample, null when `temp_c` is null. Null when no sensor is listed. Storage sensors, thermal-zone-backed devices and runtime-suspended devices are never listed. |
| `net_rx_bps`, `net_tx_bps` | integer or `null` | Bytes per second received and sent since the previous baseline, summed over the interfaces backed by a device (`/sys/class/net/<if>/device` exists), so loopback, tunnels, bridges and veths don't count traffic twice. Each is independently nullable: null with no such interface, when `/proc/net/dev` is unreadable, when any counted interface's rate is null or it appeared or disappeared in the interval, or when any interface's device link can't be determined. |
| `net_interfaces` | array or `null` | Every interface in `/proc/net/dev`, in kernel order; null when it is unreadable or lists none. Each row has `name`, `device_backed` (true or false from the existence of the `device` link alone, never `address`; null when that can't be determined) and independently nullable `rx_bps` and `tx_bps`. |
| `disks` | array or `null` | Every disk in `/proc/diskstats` backed by a device (`/sys/class/block/<dev>/device` exists; a `/` in a diskstats name is `!` in sysfs), in kernel order; null when it is unreadable or lists none. Partitions and dm devices (LUKS, LVM) are left out because the kernel already counts their I/O on the parent disk; zram and loop devices have no `device` link. A disk whose link can't be determined is left out. Each row has `name` from the kernel and independently nullable `read_bps` and `write_bps`: sectors × 512 bytes, always, per the kernel's iostats documentation. |
| `disk_read_bps`, `disk_write_bps` | integer or `null` | The sum over `disks` rows, added before rounding. Each is independently `null` when `/proc/diskstats` is unreadable or lists no device-backed disk, when any listed disk's rate in that direction is `null`, or when a listed disk appeared or disappeared in the interval. |
| `battery` | object or `null` | The Machine's system batteries ([§ Battery](#battery)). `null` means the Machine has none: no `/sys/class/power_supply`, or no supply there with `type` Battery and a `scope` other than Device. An object has a Machine-level summary, `pct`, `status` (the kernel's string, open), `power_w` (a magnitude) and `time_left_s` (integer, only while Discharging), and `batteries`: one row per system battery in name order with `name`, `pct`, `status`, `power_w` and `health_pct`. Every value is independently nullable; `batteries` is `null`, never `[]`, when no battery could be read. An Agent from before batteries omits the key, which the phone shows as unavailable, never as "no battery". |
| `verdict` | object or `null` | The Agent's **Verdict** on this **Sample** ([§ Verdict](#verdict)): `{"level": string, "reasons": array}`. `level` is `ok`, `warn` or `crit` (open). Each Reason is `{"code": string, "level": string, "message": string}`: an open code, that Reason's own level, and a plain-language message. `reasons` is `[]`, never `null`, when the level is `ok`. `null` when no input could be judged at all. An Agent from before the Verdict omits the key, which the phone decodes as `null`. |
| `distro`, `kernel`, `arch`, `agent_version` | string or `null` | Slow lane, cached for 30 seconds. Distro is os-release PRETTY_NAME, falling back to NAME; kernel is osrelease; arch is uname's machine field, never the Agent binary's target architecture. Agent version is injected at build time with `-ldflags=-X=main.version=<release>` or read from Go build info, which reports `(devel)` for local builds. Unavailable values remain null. |

Byte measurements and their aggregates are bounded by signed 64-bit integers for phone compatibility. Conversion or sum overflow yields null. Load and uptime reject negative and non-finite values. Missing sensors never fail the rest of the reading.

A rate is a counter delta divided by the actual elapsed time on the monotonic clock, rounded to a whole number. Each direction keeps its own baseline, latest observation and retained delta with its elapsed time. A counter that goes backwards or is malformed resets only its own baseline and yields `null` for that interval. After an unavailable counter becomes valid, its rate stays `null` until a full second of valid observations has elapsed. A disappeared interface or disk is omitted from the detail rows; a counted interface disappearing also makes the network summary `null` for that interval. An appearing or reappearing row starts with `null` rates and a new baseline. A change in an interface's device link also resets its baseline and yields `null` rates, including the summary when it was or may now be counted. The first Sample, taken when the Agent starts, has null rates and is every counter's baseline. A suspend resets every baseline the same way ([§ Window](#window)): the first Sample after a Gap has null rates, so a counter that kept running through the suspend never becomes a spike.

Measurements are taken once per Sample, every 2 s, whether or not anyone is looking; a Pulse never takes one, so every Pulse between two Samples is the same. CPU, network and disk keep a one-second baseline minimum, which applies only to Samples closer than that: such a Sample repeats the previous rates and keeps the baseline for unaffected counters. Resets, appearances, disappearances and device-link changes are detected on every Sample. The slow-lane cache belongs to the recorder, under its lock, and refreshes on the first Sample at or after 30 seconds.

## Rules

- JSON, snake_case, with the unit in the name: `_pct`, `_bytes`, `_bps`, `_c`, `_w`, `_s`, `_ms`, `_ts`. `_bps` is
  **bytes** per second, never bits. `_ms` is integer milliseconds.
- `null` means "not available on this Machine". It is never `0` or `-1`.
- Series are columnar, with explicit timestamps. A **Gap** is a Window row whose `boottime_ms` and every series are
  `null`, never interpolated.
- Cumulative counters that could exceed 2^53 are decimal strings.
- Enum-like values (`kind`, the **Verdict** level, **Reason** codes, battery status) are open strings. The phone
  keeps an "unknown" fallback, as `PulseKind` does in `Pulse.kt`, instead of rejecting the Pulse.

## Window

`window` is columnar: `span_s` (integer, 1800), `boottime_ms`, one array per series below, all the length of
`boottime_ms`, and `cpu_core_pct`. Each series is named after the flat field it records and means exactly what that
field means at that row. Unknown keys inside `window` are for a newer Agent; the phone ignores them.

| Series | Flat field | Precision |
|---|---|---|
| `boottime_ms` | (none) | Integer ms on `CLOCK_BOOTTIME`, which counts through suspend; `null` marks a Gap row |
| `cpu_pct` | `cpu_pct` | One decimal |
| `load_1` | `load_1` | Two decimals |
| `mem_used_pct` | `mem_used_pct` | One decimal |
| `swap_used_bytes` | `swap_used_bytes` | Integer |
| `cpu_temp_c` | `cpu_temp_c` | One decimal |
| `net_rx_bps`, `net_tx_bps` | `net_rx_bps`, `net_tx_bps` | Integer |
| `disk_read_bps`, `disk_write_bps` | `disk_read_bps`, `disk_write_bps` | Integer |
| `battery_pct` | `battery.pct` | One decimal |
| `battery_power_w` | `battery.power_w` | Two decimals |
| `cpu_core_pct` | `cpu_core_pct` | An array per CPU, as the flat field, of arrays the length of `boottime_ms` with integer 0–100 or `null` entries; `null` when the flat field is |

Rows are Samples and Gaps, oldest first. A Sample row is kept while it is less than 30 minutes older than the
newest and among the newest 900 rows; a Gap row is kept while the Sample after it is kept.
At the 900-row limit, evicting a leading Gap also evicts the Sample after it. A Gap row sits between
the last Sample before a suspend and the first after it, so its true width runs from the previous row's time to
the next row's. A Window may start with a Gap, when the Machine slept before its first retained Sample. A
`boottime_ms` of `null` tells a Gap apart from a Sample whose values are merely unavailable, which breaks a chart's
line but isn't hatched.

A suspend is growth of 1 s or more in `CLOCK_BOOTTIME − CLOCK_MONOTONIC` between two Samples. The Sample that sees
it is written right after a Gap row, with fresh baselines, and also resets the CPU temperature's debounce. An
Agent restart is not a Gap: it starts a new epoch, and the phone's next request gets `full`.

The Agent guarantees, and both sides test:

1. Every array in `window` has the length of `boottime_ms`, and so does every inner `cpu_core_pct` array. A delta
   with no new rows has `[]` everywhere (`cpu_core_pct` is one `[]` per CPU), never `null`.
2. A non-empty full Window's last row is a Sample. A Gap row is never last and never next to another Gap row.
3. In every Pulse, the flat fields are the newest Sample's. When `window` is non-empty, `window.X[last] == X` for
   every series.
4. The rows' seqs are contiguous and end at `cursor`'s seq. A delta answering Cursor `s` with new Cursor `s′` has
   exactly `s′ − s` rows.
5. The first Sample of a run and the first Sample after a Gap have `null` `cpu_pct`, per-core, network and disk
   rates. Level series (load, memory, swap, temperature, battery) keep their values.
6. The non-null `boottime_ms` entries strictly increase.

## Cursor rules

As in [design § Responses](../docs/design.md#responses):

```http
GET /v1/pulse                          → full
GET /v1/pulse?after=9f3c2a71d04e5b68.5 → delta if valid, else full
GET /v1/pulse?after=9f3c2a71d04e5b68.5&wait=25
```

- A **Cursor** is `<epoch>.<seq>`, matching `^[0-9a-f]{16}\.(0|[1-9][0-9]*)$` with seq ≤ 18446744073709551615. The
  epoch is a random 64-bit number drawn once per Agent run. Every row, Sample or Gap, takes the next seq, starting
  at 1. The phone sends back the `cursor` string it was given.
- A Cursor is valid when its epoch is this run's and its seq is between the oldest retained row's and the newest
  row's, inclusive. A valid Cursor gets `kind: "delta"` with the rows whose seq is greater. Anything else gets
  `kind: "full"`: no `after` (the zero Cursor, always older than the Window), another run's epoch, a seq the Window
  has evicted (seq 0 included) or a seq in the future.
- `wait` must match `^[0-9]+$` and fit in 32 bits. The Agent holds a request only when its Cursor is valid and at the
  newest row. The hold ends at the next Sample, at `wait` seconds, when the client cancels or when the Agent shuts
  down, and then the request is answered as without `wait`: an empty delta if nothing is newer. A `wait` over 30 is
  held for 30 s; `0` means don't hold.
- A malformed `after` or `wait`, an empty value (`after=`, `wait=`), a repeated parameter or a query that isn't
  valid URL encoding → `400`. Authorization comes first, so a refused caller gets `403` whatever the query. Unknown
  parameters are ignored. Anything but `GET` → `405`.

## Filesystem capacity

Discovery reads `/proc/self/mountinfo`, filtering pseudo, network, FUSE, PrivateTmp and image mounts before
any capacity call. Descendants of an excluded mount are excluded too. Local read-only filesystems and
major-zero btrfs mounts remain eligible. Before statfs, statx must return the entry's mount ID. A hidden
mount falls back to the next-shortest eligible mount in the same MAJ:MIN group; a group with none visible
is omitted. The label remains the shortest mount point, even when capacity comes through a fallback.

Capacity refreshes every 30 s. The first Sample starts discovery and later Samples schedule refreshes when due,
matching Machine info. Slow calls run outside the recorder
lock. One pending worker per MAJ:MIN group and ownership of each path survive disappearance and
replacement. Blocked calls are never replaced; their row has null capacity and an age. Changed mount
identities invalidate late results. A removed group is omitted; an unchanged group that reappears while
its call is pending retains its unavailable row. Previously returned rows never change underneath a caller.

`age_s` is whole seconds since the launch of the last successful capacity call, or the initial attempt if there
has been no success. A call is stamped with the time of the Sample that launched it, not when it returns, so the
recorder never consults a clock of its own and `age_s` can only over-report, by the call's duration. A refresh clears all previous capacity values immediately, so blocked or failed work cannot
present old values as current. Pending visibility checks have no row until verified.

The fragment size is the capacity unit, falling back to block size when unavailable. Total is blocks × unit;
used is blocks minus free blocks × unit; available is available blocks × unit. Products exceeding signed
64-bit range and invalid counters become null independently. Percent is used / (used + available) × 100,
rounded to one decimal; an unavailable input, overflowing sum or zero denominator makes it null.
A measured zero remains zero. Older Agents omit both filesystem fields; the phone decodes them as null.

## Temperatures

- Hwmon driver reads run outside the recorder mutex, with one shared 50 ms wait bound per Sample and at most one outstanding operation per device. Concurrent Samples share the in-flight Sample's result.
- Enumeration checks the ordered guards once per device per Sample, with directory identity captured before the guards and verified afterward; a changed identity is omitted for that Sample. Workers read runtime status, then check directory identity immediately before each sensor attribute open, and check again after the last open; unchanged identities reuse settled indices and labels.
- Transient label errors leave discovery unknown and omit that device's sensors for the Sample; discovery retries on the device's next read: the next Sample on the fast lane, the next slow read on the slow lane. Absent or empty labels settle as `tempN`.
- Only the CPU sensor chosen in this Sample has its static `tempN_crit` read, within the same deadline and device ownership. The threshold is cached per device identity and sensor index and discarded when identity changes. A threshold that misses the deadline is null for that Sample.
- Launching a refresh discards retained temperatures. Pending, failed or timed-out refreshes keep the last completed sensor IDs with null temperatures and null ages until a timely read succeeds. Late completions can refresh IDs for the same identity, but never supply current values; failed discovery is retried.
- CPU selection stays unavailable while IDs are unknown for a possible CPU device: the override's hwmon name when set, or a recognised CPU driver otherwise. Unrelated discovery does not suppress the CPU sensor. Devices without runtime PM are always on the fast lane; possible CPU devices are also fast while discovery is unknown for any possible CPU device.
- Each device is on a lane, decided every Sample after the guards. The fast lane, read every Sample, holds the CPU sensor's device (including one `ANSHIN_CPU_SENSOR` names) and devices without runtime PM: no `device` link, `runtime_status` `unsupported`, or `device/power` without `runtime_status`. Every other device is on the slow lane: read on the first Sample that sees it, then on the first Sample at least 30 s after its last read was launched, on the Sample's monotonic time. A lane follows the CPU choice on the Sample it moves.
- Between slow reads, a slow-lane device's rows keep the values of its last read that completed within its Sample's bound, with `age_s` counted from that Sample, like a filesystem's capacity age. A device with an outstanding read shows its IDs with null temperatures. A replaced or vanished device drops its retained values; a suspended one is not listed. Skipped or absent devices keep their identity and last launch time until the 30 s interval elapses; the same identity resumes that cadence with no retained rows, and a replacement starts fresh. `cpu_temp_c` always comes from this Sample.

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

## Verdict

How the phone reads a malformed `verdict`:

- A missing or `null` `verdict`, `reasons` or Reason field is accepted.
- A missing, `null` or non-string level, or a non-string Reason field, makes only the Verdict unavailable; the
  rest of the Pulse is kept.
- A `verdict` that isn't an object, `reasons` that isn't an array, a Reason that is `null` or isn't an object, or a
  value in the level or a Reason field nested beyond the JSON reader's depth limit makes the whole Pulse unreadable.
- Unknown keys inside the Verdict or a Reason are skipped at any depth.

The Agent judges every Sample from fixed defaults; nothing is configurable. A Pulse carries the newest Sample's
Verdict, so reading the Agent more often never advances the debounce.

| Input | Reason code | warn | crit | Message |
|---|---|---|---|---|
| Each listed filesystem's `used_pct`, the rounded value on the wire | `filesystem_full`, one per filesystem | ≥ 90 | ≥ 95 | `/ is 91.2 % full`, with the row's `mount_point` |
| Memory available: `mem_available_bytes` ÷ `mem_total_bytes` × 100, rounded to one decimal | `memory_low` | < 10 | < 5 | `Only 8.4 % of memory is available` |
| `cpu_temp_c`, the CPU sensor only | `cpu_hot` | ≥ crit − 5 | the sensor's `tempN_crit`, else 95 °C | `CPU at 92.0 °C (crit at 95.0 °C)` |
| `battery.pct`, only while `battery.status` is exactly `Discharging` | `battery_low` | < 15 | < 7 | `Battery at 12.0 % and discharging` |

- Numbers in messages have one decimal, like the wire.
- An unavailable input adds no Reason and is never given an invented threshold: a filesystem with a `null`
  `used_pct`, memory with either value `null`, a total of 0 or available above total, no CPU sensor or a `null`
  `cpu_temp_c`, no battery, or a battery with a `null` `pct` or `status`. A battery with a known `pct` and any other
  status is judged and adds no Reason.
- `verdict` is `null` when every input is unavailable, because an `ok` judged from nothing would treat unavailable
  as healthy.
- The level is the worst Reason's level, `ok` with none. Reasons are ordered worst first (`crit` before `warn`), then
  by input: filesystems in list order, memory, CPU temperature, battery.
- The CPU temperature is debounced; the other inputs are not. It has two conditions, at or above the warn threshold
  and at or above the crit threshold, each checked against that Sample's thresholds. A condition turns on after it
  has held for 5 consecutive Samples and off after it has not held for 5 consecutive Samples; in either direction, the first and last must be at least 8 s apart on the Samples' monotonic time. The input is `crit`
  while the crit condition is on, else `warn` while the warn condition is on, else `ok`. A Sample with a `null`
  `cpu_temp_c` resets both conditions to off, as do a Gap and a change of `cpu_sensor`, which counts that Sample as
  the first for the new sensor.
- Codes and levels are open strings: a newer Agent may send codes or levels this list doesn't have. The phone shows unknown Verdict words neutrally by Machine name, outside the top line's ranking of known current Verdicts, and they prevent ALL MACHINES OK.

## Battery

- **Classification first.** Each `/sys/class/power_supply` entry has its `type` read, then its `scope`, and nothing
  else until it is known to be a system battery: `type` Battery with no `scope` (ACPI batteries have none) or a
  `scope` other than Device. A peripheral's directory name can carry a Bluetooth MAC or a USB serial, and reading
  a HID battery's `capacity` can send the device a request. UPS, Mains, USB and Wireless supplies are not batteries.
- **Can't tell.** When the class directory can't be listed, or a supply's `type` or `scope` read fails with
  anything but "no such file", that supply is left out of `batteries` and every summary field is `null`, because it
  could be a system battery. The phone shows unavailable, not "no battery".
- **Percent** is the driver's `capacity` when it is an integer in 0–100, else now ÷ full in the battery's family
  when full > 0. Now > full is `null`, as swap used > total is. One decimal.
- **Families.** `energy_*` (µWh, µW) is preferred over `charge_*` (µAh, µA), and the two are never mixed: converting
  would need a voltage guess. A battery's family is energy when `energy_now` is readable, else charge.
- **Power** is `|power_now|` ÷ 10⁶, else `|current_now|` × `voltage_now` ÷ 10¹², in watts, two decimals. It is a
  magnitude because drivers disagree on sign; the direction comes from `status`. A `voltage_now` of 0 is a driver
  placeholder and gives `null`. A measured 0 W stays 0.
- **Health** is full ÷ full design × 100 in one family, one decimal, and is not clamped: a new cell can exceed its
  design capacity.
- **Status** is the kernel's string, trimmed (`Charging`, `Discharging`, `Not charging`, `Full`, `Unknown` or
  anything newer); empty or unreadable is `null`.
- **Several batteries.** One battery's summary is its row. With more, `pct` is Σnow ÷ Σfull when every battery has
  now and full in the same family; `status` is Discharging if any discharges and none charges, Charging the other
  way round, `null` when both happen, else the shared status or `null`; `power_w` is the sum of the rows' watts,
  `null` if any is `null` or when one charges while another discharges.
- **Time left** is Machine-level only, and only while the summary status is Discharging: Σnow ÷ Σrate × 3600 in
  one family, where the rate is the family's own (µW from `power_now`, else current × voltage; µA from
  `current_now`, else power ÷ voltage), from measurements of the same instant. Mixed families, any battery with a missing
  rate, a summed rate of zero, or a result outside signed 64-bit range gives `null`. A measured zero rate on
  one battery still counts its remaining energy or charge when another battery is discharging at a positive rate.
- Every derived number is finite before it is set, so one battery can never make the Pulse fail to encode.

## Fixtures

- Three fixtures are generated per capture. Never edit one by hand.
  - `fixtures/<capture>.json`: the full Pulse after one Sample per snapshot, at the capture's own clocks.
  - `fixtures/<capture>.delta.json`: the delta that answers the second-to-last Sample's Cursor, with one row.
  - `fixtures/<capture>.suspend.json`: the full Pulse of the same replay with a 300 s suspend scripted before the
    last snapshot, so its Window is two Samples, a Gap and the resume Sample. Only the clocks handed to the recorder
    are scripted (`CLOCK_BOOTTIME` runs 300 s ahead from the last snapshot on); the capture's files are not, so its
    `uptime_s` is the capture's and disagrees with its `boottime_ms`.
  - Golden replays use epoch 1, so every Cursor in them starts `0000000000000001.`.
- `fixtures/future.json` is the only hand-written fixture. It has a field no Agent sends yet and an unknown Reason
  code, so the phone proves it reads a newer Agent's Pulse.
- `fixtures/kept/<YYYY-MM-DD>/` holds old generated fixtures. Before a pull request changes the wire shape, copy
  the current generated fixtures there. They prove a newer phone still reads older Agents. Nothing regenerates,
  overwrites or deletes them. If that day's directory already exists, use `<YYYY-MM-DD>-2`, then `-3`, and so on.
  One older set predates that suffix and sits in a named subdirectory instead: `kept/2026-11-14/filesystems/` holds
  the fixtures from before filesystems were added, `kept/2026-11-14-2/` the ones from before network and disk
  rates, `kept/2026-11-14-3/` the ones from before battery and temperatures, `kept/2026-11-14-4/` the ones from
  before temperatures, `kept/2026-11-14-5/` the ones from before the Verdict, and `kept/2026-11-15/` the ones from before the Window. The phone's `ContractFixturesTest` reads every `.json` file under `fixtures/`, at any depth.

Regenerate after a change to the recorder, the server or a capture, then read the diff before committing:

```sh
cd agent && go test ./internal/server -run '^TestGolden' -update
```

`-update` is defined only in `internal/server`, so `go test ./... -update` fails in every other package with
`flag provided but not defined`. Go's test cache doesn't notice changes under `contract/`, which is outside the Go
module, so after editing a capture or a fixture run the tests with `-count=1`.

`make check-agent` (and so CI) runs the same command and fails if `contract/fixtures` then differs from what is
committed, untracked files included. Commit fixture changes before running it locally. A fixture that isn't one of a
capture's three fails `TestGoldenFixturesMatchCaptures`, and `-update` deletes it.

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
  [design § Battery](../docs/design.md#battery). Never `serial_number` or `model_name`. Classification reads
  `type`, then `scope`, outside the recorded tree, just as the probe does. Non-batteries and peripherals with
  `scope` Device are never visited: a peripheral's name can carry a Bluetooth MAC or a USB serial that no scrub
  pattern recognises, and reading a HID battery's `capacity` can query the device. The report counts skipped
  supplies without naming them. If `type` or `scope` fails with an error other than "no such file", the Capture
  refuses with an error that names no supply, because omitting it would replay "no battery" instead of unavailable.
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
- Two source paths that scrub to the same path stop the capture.
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
  that the left-out partitions and LUKS device add nothing the disk doesn't count, and that every battery row is a
  system battery; it also runs labelled synthetic sysfs trees for batteries no capture has yet (laptops with
  `energy_*` and `charge_*`, two batteries, peripherals), until #30 adds a real laptop capture; `agent/internal/recorder` runs
  scripted `/proc/stat`, `/proc/net/dev` and `/proc/diskstats` sequences with explicit time values through counter
  resets, CPU-count changes, interfaces and disks appearing and disappearing, and division by elapsed time, and
  scripted mount tables and capacity calls with explicit time values through blocked, failed, hidden and replaced
  mounts. The Verdict runs on scripted Readings: every threshold at, just below and just above it, unavailable
  inputs, ordering, and the CPU temperature's debounce and resets, judged once per Sample however often it is read.
- **Window** (`agent/internal/recorder`): scripted Samples with both clocks explicit cover eviction after 30
  minutes and of a leading Gap, every Cursor rule, an epoch change, the Cursor grammar, a scripted suspend becoming
  a Gap at the 1 s threshold with null rates after it and rates over the 2 s since the resume, a counter reset
  becoming `null` in the Window, per-core CPU, the disk summary, the Window's fixed storage, and invariant 3 over a
  run. `Wait` returns on the next Sample and leaves nothing behind when cancelled.
- **Long-poll** (`agent/internal/server`): over loopback HTTP, a held request answers within a second of the next
  Sample, an expired `wait` answers an empty delta, a client's cancel ends the hold, shutdown ends held requests
  at once, and every malformed `after` and `wait` is `400` (`403` for a refused caller).
- **Wire compatibility** (does the JSON decode): `agent/internal/server` emits the fixtures through loopback HTTP
  with the fake tailnet, and the phone's `ContractFixturesTest` decodes every file under `fixtures/`, `kept/`
  included.
