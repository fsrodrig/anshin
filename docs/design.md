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

The app defaults to the selected **Ink & bone** palette with periwinkle `#A5A8F5` for identity and navigation. This replaces the original brief's Kanagawa default. The four dark themes are Ink & bone, Tokyo Night, Catppuccin Mocha and Matte Black; alternate palette values and licenses need verification during implementation.

The launcher and app header use the paired-frame mark. Level's vial remains a secondary Verdict glyph with an explicit word beside it. A Live marker never implies an `ok` Verdict. Stale and Unreachable views carry their age or explanation and cannot show a reassuring current Verdict. The compact resource views retain explicit null measurements and honest Gaps.

## Security model

See [ADR 0002](adr/0002-tailnet-identity-no-tokens.md). There are no tokens, passwords or certificates.

### Agent side

- **Bind.** At start the Agent asks tailscaled (`Self`) for its tailnet IPv4 and IPv6 addresses and binds only those, using `IP_FREEBIND` so boot ordering doesn't matter. If tailscaled is unavailable at start, or the addresses later change, the Agent exits non-zero and systemd restarts it. There is no in-process rebind loop.
- **Authorize every request.** The Agent calls LocalAPI WhoIs on the caller's IP (`netip.Addr`, never IP:port, never a header) and caches the result for about 30 s, keyed by IP:

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

- **No `after`, or a cursor that is no longer valid** → `kind: "full"`. A cursor is no longer valid when its epoch is from another Agent run, it is older than the oldest retained **Sample** (evicted), or it is in the future. The response contains Machine info, the whole **Window**, the current **Verdict**, and a new cursor. The phone *replaces* what it had.
- **Valid cursor** → `kind: "delta"`. The response contains only newer Samples plus the current Verdict. With `wait`, the Agent holds the request until a newer Sample exists, up to `wait` seconds. `wait` is optional; without it the endpoint is plain polling.
- **Malformed cursor** → 400. **Anything but GET** → 405.

### Field rules

- JSON, snake_case, with the unit in the name: `_pct`, `_bytes`, `_bps`, `_c`, `_w`, `_s`, `_ts`.
- `null` means "not available on this Machine". Never `0` or `-1`.
- Series are columnar, with explicit timestamps. A **Gap** is an explicit null run, never interpolated.
- `"schema": 1`. Within v1, changes are additive only. A breaking change means `/v2`, served alongside v1.
- Enum-like values (Verdict level, **Reason** codes, battery status) are open strings. The phone keeps an "unknown" fallback.
- Cumulative counters that could exceed 2^53 are decimal strings.

### Source of truth

The golden fixtures in `contract/` plus `contract/README.md`, the field glossary (see [ADR 0005](adr/0005-golden-fixtures-are-the-contract.md)). There is no OpenAPI document.

## Sampling and the Window

See [ADR 0003](adr/0003-agent-holds-the-window.md).

### Lanes

- **Fast lane, every 2 s:** CPU (total and per core), load, memory, swap and zram, network and disk I/O counters, temperatures, battery.
- **Slow lane, every 30 s:** filesystem capacity and static Machine info (distro, kernel, arch, agent version).

### The Window

- 30 minutes at 2 s is 900 rows of about 12 summary series, plus per-core CPU stored as uint8. That is roughly 60 KB.
- It is allocated once and never written to disk.
- A 24 h min/avg/max tier is post-MVP.

### Rates

- Rates come from counter deltas divided by actual elapsed time.
- A counter that goes backwards, or a device that appears or disappears, resets the baseline and yields `null` for that interval.

### Suspend

- Go's monotonic clock stops during suspend.
- Detect suspend from growth in `CLOCK_BOOTTIME − CLOCK_MONOTONIC` between Samples (read with `unix.ClockGettime`; no cgo). Don't use the wall clock, which NTP can step.
- A suspend produces a Gap and null rates.

### Restarts

An Agent restart starts a new epoch. History is lost, and the app says so honestly.

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
- **Check the mount is really visible before `statfs`.** `statx(mountpoint, STATX_MNT_ID).stx_mnt_id` must equal that mountinfo entry's mount ID (field 1), because systemd sandboxing (or any over-mount) can hide a mount point while mountinfo still lists it. If it doesn't match, try the next-shortest mount point with the same `MAJ:MIN`. If none is visible, leave the filesystem out rather than report the numbers of whatever is mounted on top.
- `statfs` runs at most once in flight per mount. A blocked mount is never replaced by a second probe; it reports "unavailable" with its age.
- The dashboard's primary disk is the filesystem holding `/`.

### Temperatures

- Read hwmon devices.
- **Ignore hwmon devices backed by a thermal zone** (`device` → `thermal_zoneN`, e.g. `acpitz`, `iwlwifi`).
- **Skip devices whose `device/power/runtime_status` is `suspended`**, so a sleeping dGPU is never woken.
- The CPU sensor is coretemp "Package id 0", else k10temp/zenpower `Tctl`, else the first other recognised CPU driver.
- Configurable override: `ANSHIN_CPU_SENSOR`.
- Every other sensor is display-only, and labelled.

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

### Network

- `/proc/net/dev` deltas give rx/tx rates.
- Loopback and tailscale/virtual interfaces are excluded from the summary. Every interface is listed on the detail screen.

### Disk I/O

- `/proc/diskstats` deltas, per *physical* disk (partitions and dm devices rolled up to their parent). Not per filesystem.

## Verdict

The Agent computes it (`ok` / `warn` / `crit` plus **Reasons**) from fixed defaults. Inputs that are unavailable are excluded, never treated as healthy, and never given an invented threshold.

| Input | warn | crit |
|---|---|---|
| Any listed filesystem used | ≥ 90 % | ≥ 95 % |
| Memory available | < 10 % | < 5 % |
| CPU temperature (CPU sensor only) | ≥ crit − 5 °C | its `tempN_crit`, else 95 °C |
| Battery, while discharging | < 15 % | < 7 % |

A temperature condition must hold for 5 consecutive Samples (10 s) before it changes the Verdict, so a short boost spike doesn't flap it. Zen CPUs expose no `crit` through k10temp and can sit near 95 °C under sustained load, so check the defaults against both Machines under a long build before the MVP closes.

The dashboard's top line renders the worst Verdict across Machines. There is no notification in the MVP.

## Agent modules

```text
agent/
├─ cmd/anshin-agent/   wiring, sampling loop, `capture` and `doctor` subcommands
└─ internal/
   ├─ recorder/        DEEP: Open, Run(ctx), Read(cursor) Batch, Wait(ctx, cursor) (Batch, error)
   │  └─ probe/        parsers over Host{FS fs.FS; Statfs func(path) (FsStat, error)}
   ├─ tailnet/         PORT: Self(ctx), WhoIs(ctx, netip.Addr); adapters: LocalAPI client, fake
   └─ server/          DEEP: bind, authorize (truth table), long-poll, encode
```

### `recorder`

The one module that knows about lanes, rates, the Window, cursors, epochs, Gaps and the Verdict. `server` only ever calls `Read` and `Wait`.

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
  - Signature: `pulse(endpoint, after, wait) → Ok(Batch) | Unreachable | Forbidden | Incompatible | Failed`.
- **`Watcher`** is DEEP.
  - Signature: `watch(machine) → Flow<MachineView>`, where MachineView is Connecting | **Live** | **Stale**(since) | **Unreachable**(last?, retryAt, vpnHint) | **Forbidden** | **Incompatible**.
  - It owns: long-poll looping, full/delta merging, staleness, jittered capped backoff, and stopping when not visible (collected with `collectAsStateWithLifecycle`).
  - Tested with `runTest` virtual time and the fake link.
- **`MachineBook`** is DEEP.
  - It lists, saves, edits and removes Machines. It accepts MagicDNS short names, FQDNs and tailnet IP literals, plus a port and a display name. It rejects duplicates and non-tailnet literals, and runs a test connection on save.
  - Tested against a real DataStore in a temp file.
- **`ui`**:
  - semantic theme tokens (four themes);
  - JetBrains Mono with tabular numerals for numbers;
  - Sparkline;
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

- `probe`: replayed fixture trees.
- `recorder`: scripted readings covering counter resets, suspend Gaps, eviction and cursor rules.
- `server`: the authorization truth table (every row), 400/405/503 cases, long-poll and its cancellation.
- `go test -race`.

### Fixture capture

`anshin-agent capture` does the following:
- copies only an **allowlist** of the files `probe` reads;
- records `Statfs` results and errors per mount;
- takes ≥ 2 snapshots about 2 s apart, with timestamps;
- **scrubs** hostnames, MACs, serial numbers and usernames from file contents *and* from file and directory names;
- prints the tree for human review.

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
- `AgentLink` against MockWebServer serving the fixtures, plus the destination check: an outside address is refused, including with a system proxy set.
- Roborazzi screenshots.

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
contract/   fixtures/*.json, README.md (field glossary)
deploy/     systemd/, aur/PKGBUILD, tailscale/policy.example.hujson
branding/   SVG sources: symbol, wordmark, icons; theme palettes
docs/       design.md, security.md, install.md, maintaining.md, adr/, brief.md
scripts/    repository checks (identifier scan, PR title)
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
- SMART, fans, processes, containers, logs;
- discovery;
- tablet and landscape layouts;
- a light or system-following theme;
- iOS;
- packages other than AUR (.deb, .rpm, install script).
