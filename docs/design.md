# Anshin design (MVP)

How Anshin is built. Terms in **bold** are defined in [`GLOSSARY.md`](../GLOSSARY.md); the decisions behind this shape, with the alternatives we rejected, are in [`docs/adr/`](adr/). The original product brief ([`brief.md`](brief.md)) was the starting point; where it disagrees with this document, this document wins.

This design was reviewed over four rounds by a three-model council (Claude Opus, GPT Sol, Gemini) and the owner settled the split decisions.

## Shape

```text
 Pixel (Anshin app, Kotlin + Compose)
   │  HTTP inside WireGuard, only to tailnet addresses
   ├──────────────▶ Machine A: anshin-agent (Go) ──▶ /proc, /sys
   └──────────────▶ Machine B: anshin-agent (Go) ──▶ /proc, /sys
                          │
                          └──▶ tailscaled LocalAPI (WhoIs, Self)
```

- No central server. The phone talks directly to each **Agent**.
- Each **Agent** samples its **Machine** every 2 s whether or not anyone is looking, and keeps a 30-minute **Window** in memory. Charts are full the moment the app opens.
- The phone talks to Agents only while the app is visible. Leaving the app stops all traffic. No background work, no notifications in the MVP.
- The API is read-only. There is no shell, process, file or service control, ever.

## Visual identity

The Owner selected **Interval's paired-frame logo with Level / periwinkle's app layouts and palette**. The source assets, semantic colors and UI rules are in [`branding/README.md`](../branding/README.md), with the combined design in [`branding/board.html`](../branding/board.html). The exploration boards are retained as provenance.

The app defaults to the selected **Ink & bone** palette with periwinkle `#A5A8F5` for identity and navigation. This replaces the original brief's Kanagawa default. The four dark themes are Ink & bone, Tokyo Night, Catppuccin Mocha and Matte Black; the alternates' upstream sources, licenses and substitutions are in [`branding/themes/README.md`](../branding/themes/README.md).

The launcher and app header use the paired-frame mark. Level's vial remains a secondary Verdict glyph with an explicit word beside it. A Live marker never implies an `ok` Verdict. Stale and Unreachable views carry their age or explanation and cannot show a reassuring current Verdict. The compact resource views retain explicit null measurements and honest Gaps.

## Security model

See [ADR 0002](adr/0002-tailnet-identity-no-tokens.md). There are no tokens, passwords or certificates.

### Agent side

- **Bind.** At start the Agent asks tailscaled (`Self`) for its tailnet IPv4 and IPv6 addresses and binds only those, using `IP_FREEBIND` so boot ordering doesn't matter. If tailscaled is unavailable at start, or the addresses or this Machine's identity (its user or tags) later change, the Agent exits non-zero and systemd restarts it, so a newly tagged Machine starts refusing everyone. There is no in-process rebind loop. On a kernel booted with IPv6 disabled, the Agent skips the tailnet IPv6 address with a warning and serves on IPv4 alone.
- **Port.** TCP 7247 by default, which IANA lists as unassigned and which sits below Linux's ephemeral range. `ANSHIN_PORT` overrides it. The address is never configurable.
- **Authorize every request.** The Agent calls LocalAPI WhoIs on the caller's IP (`netip.Addr`, never IP:port, never a header) and caches the answer, keyed by IP: an identity for about 30 s, a "not a tailnet peer" answer for about 5 s, because a restarting tailscaled says that of every address until its netmap loads:

  | Caller same user as this Machine's Owner | Caller tagged | This Machine tagged | Result |
  |---|---|---|---|
  | yes | no | no | **allow** |
  | yes | yes | any | 403 |
  | yes | any | yes | 403 |
  | no | any | any | 403 |
  | WhoIs says "not a tailnet peer" | | | 403 |
  | LocalAPI unreachable or erroring | | | 503 |

  `Node.User` is the user who *created* a node, so for tagged nodes it doesn't mean ownership. That is why tagged rows deny. Allowlists and app-capability grants for tagged Machines are post-MVP.
- **LocalAPI socket.** Use `$TAILSCALE_SOCKET`, else `/run/tailscale/tailscaled.sock`. The socket is world-connectable, and read operations such as WhoIs and status need no privilege, so the Agent runs unprivileged.

### Phone side

- **Cleartext.** The app allows cleartext app-wide, because Android's network security config can't match IP literals or address ranges. All requests go through one OkHttp client configured with:
  - `proxy(Proxy.NO_PROXY)`, `followRedirects(false)` and `followSslRedirects(false)`;
  - a custom `SocketFactory` whose sockets override `connect(SocketAddress, Int)` (the one-argument form delegates to it) to throw `IOException` unless the target is a resolved address in `100.64.0.0/10` or `fd7a:115c:a1e0::/48`. An unresolved address counts as outside.

  This covers MagicDNS names and IP literals alike (OkHttp skips `Dns` for literals). A configured system proxy must not be able to bypass it, and a test proves that.
- **Advisory checks.** VPN-transport state is shown only as a hint ("VPN looks off"). It never counts as proof that traffic is on the tailnet.

### Residual risks

These are recorded in `docs/security.md`:
- Identity is per device, not per app, so another app on the Owner's phone could read the metrics. The later fix is device-key pairing.
- On a carrier that uses CGNAT, a `100.64.0.0/10` address doesn't prove the tailnet. This is acceptable only because the phone never sends a secret: the worst case is fake numbers.
- Root on a Machine can falsify anything that Machine reports.

## The Pulse contract

There is one endpoint:

```http
GET /v1/pulse?after=<epoch>.<seq>&wait=<seconds>
```

### Responses

A **Cursor** is `<epoch>.<seq>`: exactly 16 lowercase hex digits, a dot, and a decimal `uint64` with no sign and no leading zero. Every Window row, Sample or Gap, takes the next seq, starting at 1. A Cursor is valid when its epoch is this Agent run's and its seq lies between the oldest retained row's and the newest row's, inclusive.

- **No `after`, or a Cursor that is not valid** → `kind: "full"`. That is a Cursor from another Agent run, one older than the oldest retained row (evicted, seq 0 included), or one in the future. The response contains Machine info, the whole **Window**, the current **Verdict**, and a new Cursor. The phone *replaces* what it had.
- **Valid Cursor** → `kind: "delta"`. The response's Window holds only the rows newer than the Cursor, possibly none. Every Pulse, full or delta, also carries every flat field as the newest Sample's, so the phone needs one decoder and its detail rows stay current.
- **`wait`** is ASCII decimal seconds that fit in 32 bits. The Agent holds a request only when its Cursor is valid and already at the newest row; the hold ends at the next Sample, at `wait`, when the client goes away or when the Agent shuts down, and then answers as without `wait`. More than 30 s is held for 30 s rather than refused, so a newer phone can ask an older Agent for more; 30 s is far below the server's 2-minute idle timeout and common NAT idle timeouts. `0`, or no `wait`, is plain polling.
- **A malformed `after` or `wait`, an empty value, or a repeated parameter** → 400, after authorization, so a refused caller gets 403 whatever it sends. Unknown parameters are ignored. **Anything but GET** → 405.

### Field rules

- JSON, snake_case, with the unit in the name: `_pct`, `_bytes`, `_bps`, `_c`, `_w`, `_s`, `_ms`, `_ts`. `_bps` is bytes per second, never bits. `_ms` is integer milliseconds.
- `null` means "not available on this Machine". Never `0` or `-1`.
- Series are columnar, with explicit timestamps. A **Gap** is a Window row whose `boottime_ms` and every series are `null`, never interpolated.
- `"schema": 1`. Within v1, changes are additive only. A breaking change means `/v2`, served alongside v1.
- Enum-like values (Verdict level, **Reason** codes, battery status) are open strings. The phone keeps an "unknown" fallback. A malformed Verdict field makes only the Verdict unavailable, while a malformed Verdict structure makes the Pulse unreadable; [`contract/README.md`](../contract/README.md#verdict) lists which.
- Cumulative counters that could exceed 2^53 are decimal strings.

### Source of truth

The golden fixtures in `contract/` plus `contract/README.md`, the field glossary (see [ADR 0005](adr/0005-golden-fixtures-are-the-contract.md)). There is no OpenAPI document.

## Sampling and the Window

See [ADR 0003](adr/0003-agent-holds-the-window.md).

### Lanes

- **Fast lane, every 2 s:** CPU (total and per core), load, memory, swap and zram, network and disk I/O counters, temperatures of the CPU sensor's device and of devices without runtime PM, battery.
- **Slow lane, every 30 s:** filesystem capacity, other temperatures on runtime-PM devices, and static Machine info (distro, kernel, arch, agent version).
- Lanes run on each Sample's monotonic time, never the wall clock. A slow-lane value carries its age, from the Sample that launched it.

### The Window

- The sampling loop in `cmd/anshin-agent` reads `CLOCK_MONOTONIC` and `CLOCK_BOOTTIME` and calls `Sample` every 2 s, whether or not anyone is looking. Every measurement, the lanes and the Verdict run once per Sample; a Pulse never measures. `Open` takes Sample 1 at once, with null rates as every counter's baseline, so a Window always has a Sample.
- Rows are Samples and Gaps. A Sample row is stamped with `CLOCK_BOOTTIME` in ms, which counts through suspend, so rows sit at their true distance on a time axis and NTP never steps them. Each series is named after the flat field it records and means exactly what that field means at that Sample: `cpu_pct`, `load_1`, `mem_used_pct`, `swap_used_bytes`, `cpu_temp_c`, `net_rx_bps`, `net_tx_bps`, `disk_read_bps`, `disk_write_bps`, `battery_pct`, `battery_power_w`, plus per-core `cpu_core_pct`.
- **Eviction**, oldest first and leaving no holes: a Sample is kept while it is less than 30 minutes older than the newest and among the newest 900 rows. A Gap is kept while the Sample after it is kept, so a Window may start with a Gap but never ends with one.
- **Storage**: 900 rows in fixed-point columns, with an out-of-range value as `null`, allocated once and never written to disk. That is 64 B per row plus 1 B per CPU: 57,600 B + 900 × CPUs, 72,000 B on a 16-CPU Machine (`TestWindowStorage` pins it). Exact integers keep the Window's last row equal to the flat fields bit for bit, which float32 would not for large byte rates. On the wire, a full Pulse with 900 rows is 113,879 bytes for the 16-CPU desktop Capture, 43,233 of them per-core history (`TestFullPulseSize`, which serves 900 rows alternating the Capture's last two Samples through the real encoder). It is not compressed.
- **Per-core** columns are the CPU ids `/proc/stat` lists at the first Sample that lists any, normally Sample 1, in kernel order. Each value is a whole percent over the same interval as `cpu_pct`: `null` whenever `cpu_pct` is, or when that core is missing or its own counters went backwards or didn't advance. An id that appears later is not recorded until the Agent restarts, so the Window's width never changes within a run.
- A 24 h min/avg/max tier is post-MVP.

### Rates

- Rates come from counter deltas divided by actual elapsed time.
- Each direction keeps its own baseline, latest observation and retained delta with its elapsed time. A counter that goes backwards or is malformed resets only its own baseline and yields `null` for that interval. After an unavailable counter becomes valid, its rate stays `null` until a full second of valid observations has elapsed. An appearing or reappearing interface or disk starts with `null` rates and a new baseline; a disappeared row is omitted. A counted interface disappearing makes the network summary `null` for that interval.
- Interface device-link changes reset that interface's baseline and yield `null` rates, including the summary when the interface was or may now be counted. Resets and device changes are detected on every read, even less than a second after the baseline. Unaffected counters keep the one-second baseline minimum and repeat their previous rates on sub-second reads.

### Suspend

- Go's monotonic clock stops during suspend.
- Detect suspend from growth in `CLOCK_BOOTTIME − CLOCK_MONOTONIC` between Samples (read with `unix.ClockGettime`; no cgo). Don't use the wall clock, which NTP can step.
- Growth of 1 s or more is a suspend. That is far above the microseconds between reading the two clocks and below the 2 s cadence, so any suspend that could hide a Sample is caught.
- In the same Sample, before its probes, the recorder appends a Gap row and resets every rate baseline (CPU total and per core, network, disk), so the resume Sample's rates are `null` and become the new baselines, and the next Sample's rates divide by the 2 s since the resume. Level series (load, memory, swap, temperature, battery) keep their values. The Gap and the resume Sample are published together, so the newest row is always a Sample.
- A Gap resets the CPU-temperature debounce, as a `null` temperature does: a Machine `crit` before a suspend reads `ok` for about 8 s after it.
- A sampling stall without a suspend is not a Gap; the chart draws a straight segment across it.

### Restarts

An Agent restart starts a new epoch, a random `uint64` drawn once per run, so a restart never reuses one, even across reboots. History is lost: the phone's next Cursor gets `full` and it replaces its Window, so a short Window after a restart is honest, captioned "last N min". A restart is not a Gap.

## Measurements

Parsed directly from `/proc` and `/sys`, with no gopsutil (see [ADR 0004](adr/0004-read-proc-and-sys-directly.md)). Missing or unreadable inputs make *that* measurement `null`. They never fail the Sample.

### CPU

- `/proc/stat` deltas give total and per-core %.
- `/proc/loadavg` gives load.

### Memory

- `/proc/meminfo`: "used" means `MemTotal − MemAvailable`.
- `/proc/swaps` gives swap.
- zram is reported as swap, labelled.

### Filesystems

- Read `/proc/self/mountinfo`.
- Drop pseudo filesystems, network filesystems, `PrivateTmp` mounts and read-only image mounts *before* any `statfs`.
- **Dedupe by mountinfo `MAJ:MIN`** and label each filesystem with its shortest mount point. btrfs mixes the subvolume into `f_fsid`, so 5 subvolumes are 5 fsids but 1 `MAJ:MIN`.
- **Check the mount is really visible before `statfs`.** `statx(fd, "", AT_EMPTY_PATH, STATX_MNT_ID).stx_mnt_id` on an opened `O_PATH` descriptor must equal that mountinfo entry's mount ID (field 1), because systemd sandboxing (or any over-mount) can hide a mount point while mountinfo still lists it. Capacity is read with `fstatfs` on that same descriptor, so a path replacement cannot supply another filesystem's numbers. If it doesn't match, try the next-shortest mount point with the same `MAJ:MIN`. If none is visible, leave the filesystem out rather than report the numbers of whatever is mounted on top.
- `statfs` runs at most once in flight per mount. A blocked mount is never replaced by a second probe; it reports "unavailable", aged from its last successful `statfs` or, with none, its first attempt.
- A successful `statfs` is stamped with the time of the Sample that launched it, which is passed into the recorder like every other time, so its age can only over-report, by the probe's duration.
- The dashboard's primary disk is the filesystem holding `/`.

### Temperatures

- Hwmon driver reads run outside the recorder mutex, with one shared 50 ms wait bound per Sample and at most one outstanding operation per device. Concurrent Samples share the in-flight Sample's result.
- Enumeration checks the ordered guards once per device per Sample, with directory identity captured before the guards and verified afterward; a changed identity is omitted for that Sample. Workers read runtime status, then check directory identity immediately before each sensor attribute open, and check again after the last open; unchanged identities reuse settled indices and labels.
- Transient label errors leave discovery unknown and omit that device's sensors for the Sample; discovery retries on the device's next read: the next Sample on the fast lane, the next slow read on the slow lane. Absent or empty labels settle as `tempN`.
- Only the CPU sensor chosen in this Sample has its static `tempN_crit` read, within the same deadline and device ownership. The threshold is cached per device identity and sensor index and discarded when identity changes. A threshold that misses the deadline is null for that Sample.
- Launching a refresh discards retained temperatures. Pending, failed or timed-out refreshes keep the last completed sensor IDs with null temperatures and null ages until a timely read succeeds. Late completions can refresh IDs for the same identity, but never supply current values; failed discovery is retried.
- CPU selection stays unavailable while IDs are unknown for a possible CPU device: the override's hwmon name when set, or a recognised CPU driver otherwise. Unrelated discovery does not suppress the CPU sensor. Devices without runtime PM are always on the fast lane; possible CPU devices are also fast while discovery is unknown for any possible CPU device.
- Between slow reads, retain only the last timely values with `age_s` from their launch. Skipped or absent devices drop retained rows but keep their identity and last launch time until the 30 s interval elapses; the same identity resumes that cadence, and a replacement starts fresh.

- Read hwmon devices in hwmon order. For each device, read the guards in order: `name`, the `device` link, then
  `device/power/runtime_status`. Stop at the first guard that rules the device out or can't be read. `capture`
  applies the same rule (`probe.HwmonSkip`), so a Capture holds every file the probe reads.
- **Ignore hwmon devices backed by a thermal zone** (`device` → `thermal_zoneN`, e.g. `acpitz`, `iwlwifi`).
- **Skip devices whose `device/power/runtime_status` is `suspended`**, so a sleeping dGPU is never woken.
- **Read awake runtime-PM devices on the slow lane** unless they hold the CPU sensor (§ Lanes). On recent kernels each amdgpu hwmon temperature read calls `pm_runtime_mark_last_busy` + `pm_runtime_put_autosuspend`, and amdgpu's autosuspend delay is 5 s, so reading an awake dGPU every 2 s would keep it from ever suspending again. An `ANSHIN_CPU_SENSOR` on a runtime-PM device keeps that device on the fast lane: the Owner's explicit choice.
- **Don't read storage-device sensors** (hwmon `name` `nvme` or `drivetemp`). Every NVMe temperature read (`tempN_input`) sends the drive a SMART log command, and the kernel's drivetemp docs warn that reads can reset an HDD's spin-down timer. On the dev machine (kernel 7.2, 100 unbuffered reads 50 ms apart), an NVMe `temp1_input` read took a median 6.5 ms and up to 493 ms, against 0.14 ms for k10temp. Drive temperatures come back with SMART, post-MVP.
- The CPU sensor is coretemp "Package id 0", else k10temp/zenpower `Tdie` if present, else `Tctl`, else the first sensor of another recognised CPU driver (`coretemp`, `k10temp`, `zenpower`, `k8temp`, `via_cputemp`). The choice is by identity, not readability: an unreadable chosen sensor makes the CPU temperature `null`, never a fallback to another sensor. Only the CPU sensor's `tempN_crit` is read. k10temp exposes `Tdie` only on the CPUs whose `Tctl` carries a fan-control offset (Ryzen 1600X/1700X/1800X +20 °C, 2700X +10 °C, Threadripper 19xx/29xx +27 °C). On those CPUs, reading `Tctl` would trip false Verdicts.
- Configurable override: `ANSHIN_CPU_SENSOR=<hwmon name>/<label>`, where the label is `tempN_label`, else `tempN`. It
  has the syntax of the Pulse's `cpu_sensor`. One that matches no listed sensor leaves the CPU temperature `null`; it
  never falls back.
- Every other sensor is display-only, and labelled.
- A sensor whose input can't be read stays listed, with a `null` temperature. Values are °C to one decimal; a value
  below absolute zero is `null`.

### Battery

- Read `/sys/class/power_supply/*` with `type=Battery`, skipping `scope=Device` (peripherals).
- Handle every battery present; don't assume `BAT0`.
- Energy (µWh) and charge (µAh) units stay distinct.
- Reported:
  - percent;
  - status (an open string);
  - watts, from `power_now`, else `current_now × voltage_now`;
  - time remaining, while discharging;
  - health (`*_full / *_full_design`).
- A desktop shows "no battery", not 0 %.
- Classify before reading anything else: a supply's `type`, then its `scope`, and nothing more until it is a system battery. A peripheral's name can carry a Bluetooth MAC or a USB serial, and reading a HID battery's `capacity` can send the device a request.
- Percent prefers the driver's `capacity`, which is what the desktop shows, over now ÷ full.
- Watts are a magnitude, because drivers disagree on the sign of `power_now` and `current_now`; the direction comes from status.
- Time left exists only while discharging, from the battery family's own rate. Any battery with a missing rate, or a summed rate of zero, makes time left `null`, never infinity. A battery reporting a measured zero rate still contributes its remaining energy or charge when another battery is discharging at a positive rate.
- Health is not clamped: a new cell can exceed its design capacity.
- Several batteries get a Machine-level summary for the card and the Verdict, plus one row each for the detail view. Batteries in different families make the summary percent and time left `null` rather than guess a voltage.
- `battery: null` means no system battery. When the Agent can't tell (a `type` or `scope` it can't read), it sends an object whose summary is all `null`, and the phone shows unavailable.
- The rules, field by field, are in [`contract/README.md`](../contract/README.md#battery).

### Network

- `/proc/net/dev` deltas give rx/tx rates.
- The summary counts only interfaces backed by a device, meaning `/sys/class/net/<if>/device` exists. That leaves out loopback, tailscale, WireGuard, bridges, docker and veth, so tunnelled traffic isn't counted twice. Only the link's existence is read, never `address` (a MAC). With no such interface, the summary is `null`. If a link's existence can't be determined (an error other than not-exist), that interface's `device_backed` is `null` and so is the summary, rather than a guess either way. Every interface is listed on the detail screen.

### Disk I/O

- `/proc/diskstats` deltas per disk backed by a device, meaning `/sys/class/block/<dev>/device` exists. Partitions and dm devices are left out because the kernel already counts their I/O on the parent disk, so adding them would count it twice. zram and loop devices have no `device` link and are left out too, as is a disk whose link can't be determined. Bytes are sectors × 512, always. Not per filesystem.

## Verdict

The Agent computes it (`ok` / `warn` / `crit` plus **Reasons**) from fixed defaults. Inputs that are unavailable are excluded, never treated as healthy, and never given an invented threshold.

| Input | warn | crit |
|---|---|---|
| Any listed filesystem used | ≥ 90 % | ≥ 95 % |
| Memory available | < 10 % | < 5 % |
| CPU temperature (CPU sensor only) | ≥ crit − 5 °C | its `tempN_crit`, else 95 °C |
| Battery, while discharging | < 15 % | < 7 % |

Each input that crosses a threshold adds a **Reason** with its own level, an open code (`filesystem_full` per filesystem, `memory_low`, `cpu_hot`, `battery_low`) and a plain-language message. The Verdict is the worst Reason's level, `ok` with none, and its Reasons are worst first, then in input order (filesystems, memory, CPU temperature, battery). When no input can be judged at all, the Verdict is `null`: an `ok` from nothing would treat unavailable as healthy.

The CPU temperature has two conditions, at or above warn and at or above crit, each checked against that Sample's thresholds. Each turns on only after holding for 5 consecutive Samples and off only after not holding for 5, so a short boost spike doesn't flap the Verdict in either direction. The first and last of those Samples must be at least 8 s apart on the Samples' monotonic time, in either direction; at the 2 s cadence that is the fifth Sample, 8 s after the first. A `null` temperature or a change of CPU sensor resets both to off. The other inputs are judged from each Sample alone. Messages, the memory formula and the exact availability rules are in [`contract/README.md`](../contract/README.md#verdict). The defaults were checked against both Machines under sustained load. Both are AMD Zen with k10temp `Tctl` and no `tempN_crit`, so the 95 °C fallback applies and warn starts at 90 °C. The Agent polled every 2 s while a loop of `go build -a -p $(nproc) std cmd` kept every CPU busy for 10 minutes, with 60 s idle before and 2 min of cooldown after. A Ryzen 7 6800H peaked at 88.8 °C (median 83.3 °C under load) and a Ryzen 9 4900HS laptop on AC peaked at 87.8 °C (median 77.9 °C). Neither crossed 90 °C, the Verdict stayed `ok` throughout, and the defaults stand. The margin is under 2.5 °C, so a hotter Machine or season may reach warn under sustained load, which is the intent of warn.

The dashboard's top line renders the worst known Verdict among Live Machines. Unknown Verdict words are listed separately by Machine name with their first Reason message, using the neutral text token; they prevent ALL MACHINES OK and never receive a severity rank. With no known current Verdict there is no ranked headline. There is no notification in the MVP.

## Agent modules

```text
agent/
├─ cmd/anshin-agent/   wiring, the sampling loop (reads both clocks, calls Sample every 2 s), `capture` and `doctor` subcommands
├─ cmd/anshin-qa/      test-only QA Agent: `server` behind the fake tailnet, a synthetic Machine; never shipped
├─ cmd/anshin-qa-helper/ the agent-qa skill's device helper, run as `scripts/qa`
└─ internal/
   ├─ recorder/        DEEP: Open(host, Instant, cfg), Sample(Instant), Read(Cursor) Batch, Wait(ctx, Cursor) (Batch, error)
   │  └─ probe/        parsers over Host{FS fs.FS; OpenFilesystem func(path) (FilesystemHandle, error)}
   ├─ capture/         `anshin-agent capture`: allowlist, scrubber, on-disk format
   │  └─ replay/       test-only: a committed capture becomes probe.Hosts
   ├─ tailnet/         PORT: Self(ctx), WhoIs(ctx, netip.Addr); adapters: LocalAPI client, fake
   ├─ server/          DEEP: bind, authorize (truth table), long-poll, encode
   └─ qa/              test-only: QA Agent setups and the helper; `make check-agent` keeps it out of anshin-agent
```

### `recorder`

The one module that knows about lanes, rates, the Window, Cursors, epochs, Gaps and the Verdict. `server` only ever calls `Read` and `Wait`, which copy from the Window and never measure. The caller reads the clocks and passes an `Instant` (monotonic and boottime) into `Open` and `Sample`, so the recorder has no Clock port and never reads the host's time. A `Batch` is the recorder's unencoded Pulse.

Inside it, `probe` has its own test surface: replayed fixture trees. The rate, Window and Verdict logic has another: scripted readings with timestamps.

### `tailnet`

A real seam with two adapters:
- a hand-rolled LocalAPI client of about 80 lines (the official client pulls in about 18 tailscale.com packages);
- a fake.

### `server`

Tested through loopback HTTP against the fake tailnet. These tests are also what emits the golden fixtures.

## Phone modules

Package `io.github.fsrodrig.anshin`. One `:app` Gradle module, with OkHttp, kotlinx.serialization (`ignoreUnknownKeys = true`), DataStore and Compose Canvas. No DI framework and no Retrofit (see [ADR 0001](adr/0001-native-kotlin-compose-phone-app.md)).

- **`AgentLink`** is a PORT with two adapters: OkHttp (with the destination check) and a fake.
  - Signature: `pulse(endpoint, after, wait) → Ok(Pulse) | Unreachable | Forbidden | Incompatible | Failed`.
- **`Watcher`** is DEEP.
  - Signature: `watch(machine) → Flow<MachineView>`, where MachineView is Connecting | **Live** | **Stale**(since) | **Unreachable**(last?, retryAt, vpnHint) | **Forbidden** | **Incompatible**.
  - It owns: long-poll looping, full/delta merging, staleness, jittered capped backoff, and stopping when not visible (collected with `collectAsStateWithLifecycle`).
  - Tested with `runTest` virtual time and the fake link.
- **`MachineBook`** is DEEP.
  - It lists, saves, edits and removes Machines. It accepts MagicDNS short names, FQDNs and tailnet IP literals, plus a port and a display name. It rejects duplicates and non-tailnet literals, and runs a test connection on save.
  - Tested against a real DataStore in a temp file.
- **`ui`**:
  - semantic theme tokens (four themes; the choice is kept in the `settings` Preferences DataStore and applies without a restart);
  - JetBrains Mono NL, bundled with its OFL license, with tabular numerals for numbers, units, paths, hostnames, addresses, timestamps and Verdict words;
  - Sparkline: Compose Canvas with a time-based x axis over the Window's span; the line breaks at every `null` and Gap, a Gap is hatched at its true width, and nothing is ever joined across one;
  - screens: Dashboard, Machine detail, Add/Edit machine, Settings (theme, machines, about, version, licences).
  - Screenshot tests with Roborazzi, per state × theme.

### Toolchain

The host has only JDK 27. Pin the Gradle daemon JVM to 21 (committed `gradle/gradle-daemon-jvm.properties`, wrapper ≥ 9.8) *and* the compile/test toolchain to JDK 21 (foojay resolver).

## Seams we deliberately don't have

Each would have one adapter, which makes it a hypothetical seam. Add one only when a second adapter is real:

- `MetricsBackend` (non-Linux)
- `HistoryStore` (a database)
- a `Clock` port in `recorder` (time is passed in instead)
- `Notifier`
- a TLS abstraction
- `PollingStrategy`
- `ChartEngine`
- on the phone: Repository/UseCase layers, DTO↔domain mappers of the same shape, multiple Gradle modules, a theme plugin system

## Testing

### Agent

- `probe`: replayed fixture trees, plus labelled synthetic trees for hardware no Capture has yet.
- `recorder`: scripted readings covering counter resets, suspend Gaps (the 1 s threshold, null rates and fresh baselines after one, the debounce reset), eviction after 30 minutes and of a leading Gap, every Cursor rule and an epoch change, per-core CPU and the disk summary, the Window's storage size, the Window's last row equal to the flat fields, one Verdict per Sample, and the Verdict at, just below and just above every threshold, through its debounce and resets. `Wait` returns on the next Sample, at once for a Cursor it doesn't hold, and leaves no goroutine behind when cancelled.
- `server`: the authorization truth table (every row), 400/405/503 cases (every malformed `after` and `wait`), long-poll returning as soon as the next Sample lands, a `wait` past 30 s held for 30 s, a client's cancel ending the hold, and shutdown ending held requests at once rather than waiting them out.
- `go test -race`.

### Fixture capture

`anshin-agent capture` does the following:
- copies only an **allowlist** of the files `probe` reads;
- classifies power supplies just as the probe does, never visiting non-batteries or `scope=Device` peripherals, and refuses a Capture when `type` or `scope` is unreadable, so a peripheral's name never reaches a Capture;
- records `Statfs` results and errors per mount;
- records every link it passes through as data in a per-snapshot index, not as a symlink, because targets are outside the capture. A link keeps its target's basename, so `probe` can still recognise `thermal_zoneN`. Replay rebuilds the links, so `/sys/class/{net,block}/*/device` exists exactly where it did;
- takes ≥ 2 snapshots about 2 s apart, with timestamps;
- **scrubs** hostnames, MACs, serial numbers and usernames from file contents *and* from file and directory names;
- prints the tree for human review.

The format, the allowlist, the scrub rules and the review procedure are in [`contract/README.md`](../contract/README.md#captures).

### CI

- Rejects any MAC or serial pattern in `git ls-files` paths and contents.
- Replays committed inputs with a fixed clock and epoch, never sampling its own host.
- Regenerates the golden wire fixtures and runs `git diff --exit-code`.

### Contract

- The phone decodes every golden fixture, including old ones.
- A hand-written "future" fixture with an unknown field and an unknown Reason must decode.
- Collector-correctness tests (are the numbers right) are kept separate from wire-compatibility tests (does the JSON decode).

### Phone

- `Watcher` under virtual time.
- `MachineBook` against a real DataStore.
- `AgentLink` against MockWebServer serving the fixtures, plus the destination check: an outside address is refused, including with a system proxy set. A body cut off before its declared length or final chunk is `Failed`, not `Incompatible`.
- Roborazzi screenshots of every screen state in every theme, verified against committed goldens in CI.
- The theme tokens against their JSON in `branding/`, their WCAG contrast and accent distinctness, and a guard against color literals in production code.

## Packaging and release

The public GitHub repo is MIT-licensed. Agent and app share one version tag (`vX.Y.Z`); the Pulse carries its own `schema`. Releases are cut on demand from `main`, and the version and release notes come from the Conventional Commit titles of the pull requests merged since the last tag ([ADR 0007](adr/0007-releases-are-cut-on-demand.md)).

### Agent

- goreleaser builds static linux/amd64 and linux/arm64 binaries (`CGO_ENABLED=0`), with checksums and GitHub artifact attestations.
- AUR package `anshin-agent-bin`. It is the only install path in the MVP, because both Machines run Arch.
- Hardened systemd unit:
  - `[Unit]`: `After=tailscaled.service`, `Wants=tailscaled.service`, `StartLimitIntervalSec=0`.
  - `[Service]`: `DynamicUser=yes`, `NoNewPrivileges=yes`, `CapabilityBoundingSet=`, `ProtectSystem=strict`, `ProtectHome=read-only`, **no `ProcSubset`**, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, `MemoryMax=64M`, `Restart=always`, `RestartSec=5`.
- Acceptance check: compare `MAJ:MIN → total bytes` maps, not sets.
  - Agent side: the agent's filesystem list, with the agent running under the shipped unit.
  - Host side: `df -B1 --output=target,fstype,size`, keeping the rows that pass the same fstype filter, each mapped with `findmnt -no MAJ:MIN -M <target>`. Never `stat`: it returns a different device ID per btrfs subvolume.
- `anshin-agent doctor` explains the bind, LocalAPI, sensors and battery in plain language.

### App

- CI signs a release APK (R8, Baseline Profile) and attaches it to the GitHub Release.
- Obtainium tracks the repo.
- The signing keystore is backed up offline, because losing it breaks updates.

### CI (GitHub Actions)

| Workflow | Runs |
|---|---|
| `ci.yml` | repository checks (identifier scan, workflow and shell lint, offline link check), then `agent.yml` and `android.yml` as their paths change, behind one `CI` gate job that is the required check |
| `agent.yml` | vet, staticcheck, `test -race`, cross-build, binary-size budget, fixture diff |
| `android.yml` | lint, unit tests, Roborazzi verify, `assembleRelease`, APK-size budget |
| `pr-title.yml` | the PR title is a Conventional Commit (`scripts/commit-title.sh`) |
| `release.yml` | on demand from `main` ([ADR 0007](adr/0007-releases-are-cut-on-demand.md)); reuses `agent.yml` and `android.yml` |

`agent.yml` and `android.yml` are reusable workflows, so a release runs the same checks as a pull request. `make check` runs the same targets locally and is what "done" means. Signing secrets live only in GitHub environments restricted to `main`, so pull-request jobs can never reach them. Repository settings and rulesets are in [`maintaining.md`](maintaining.md).

### Budgets

Measured from the tracer bullet onward, never claimed unmeasured. Targets:

| What | Target |
|---|---|
| Agent RSS | ≤ 15 MB |
| Agent idle CPU | ≤ 0.1 % of one core |
| Agent binary | ≤ 10 MB |
| APK | ≤ 5 MB |
| Cold start to first Pulse on the phone | ≤ 500 ms |

## Repository layout

```text
agent/      Go module (see Agent modules)
android/    Android Studio project, one :app module
contract/   captures/ (scrubbed Machine captures), fixtures/*.json (golden Pulses), README.md (field glossary)
deploy/     systemd/, aur/PKGBUILD, tailscale/policy.example.hujson
branding/   SVG sources: symbol, wordmark, icons; theme palettes
docs/       design.md, security.md, install.md, maintaining.md, adr/, brief.md
scripts/    repository checks (identifier scan, PR title) and `qa`, the device QA helper
.github/    workflows/, rulesets/, issue and pull request templates
.agents/    agent skills (`.claude/skills` links here)
```

## Not in the MVP

These are deliberate, not forgotten:
- the 24 h Window tier;
- alerts (Android WorkManager check with local notifications, and/or agent → self-hosted ntfy);
- HTTPS mode via Tailscale Serve;
- device-key pairing;
- allowlists and app-capability grants for tagged Machines;
- GPU utilisation (hwmon and NVML reads can wake a suspended dGPU);
- SMART and drive temperatures, fans, processes, containers, logs;
- discovery;
- tablet and landscape layouts;
- a light or system-following theme;
- iOS;
- packages other than AUR (.deb, .rpm, install script).
