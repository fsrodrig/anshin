# Measurements

Every resource number Anshin claims, with how it was measured. The budgets are in
[design § Budgets](design.md#budgets). Add a row when you measure; never edit a number you didn't re-measure.

## Agent

Measured on 2026-11-11 with the tracer-bullet Agent, which serves the hostname and total CPU % and doesn't yet sample
in the background. Host: Arch Linux, AMD Ryzen 9 4900HS (16 threads), tailscaled 1.104.2, Go 1.27.4.

| What | Measured | Budget | How |
|---|---|---|---|
| Binary, linux/amd64 | 7,258,272 B (6.9 MiB) | ≤ 10 MB | `make check-agent` (`CGO_ENABLED=0 go build -trimpath -ldflags='-s -w'`), then `stat -c %s` |
| Binary, linux/arm64 | 6,750,368 B (6.4 MiB) | ≤ 10 MB | as above, `GOARCH=arm64` |
| RSS at start | 8.4 MB | ≤ 15 MB | `VmRSS` in `/proc/<pid>/status`, 1.5 s after start, run as an unprivileged user |
| RSS, polled every 2 s | 11.4 MB (peak) | ≤ 15 MB | `VmHWM` after 60 `curl` requests to `/v1/pulse` on the tailnet IPv4 address, 2 s apart |
| CPU, idle | 0.008 % of one core | ≤ 0.1 % | `utime+stime` from `/proc/<pid>/stat` over 120 s with no requests (1 tick at `CLK_TCK=100`) |
| CPU, polled every 2 s | 0.058 % of one core | — | as above over 121 s while polled as for RSS (7 ticks) |

That idle CPU figure predates the 2 s sampling loop; the rows below replace it.

Measured on 2026-11-15 with the Agent that keeps the Window and samples every 2 s whether or not anyone is looking.
Same host, kernel and Go. Run as an unprivileged user with `ANSHIN_PORT` set to a spare port, not under the shipped
systemd unit, with no requests sent to it.

| What | Measured | Budget | How |
|---|---|---|---|
| Binary, linux/amd64 | 8,011,936 B (7.6 MiB) | ≤ 10 MB | `make check-agent`, then `stat -c %s` |
| Binary, linux/arm64 | 7,405,728 B (7.1 MiB) | ≤ 10 MB | as above, `GOARCH=arm64` |
| RSS, sampling, idle | `VmRSS` 13,460 kB at 60 s and 14,396 kB at 660 s; `VmHWM` 14,396 kB at 660 s (kB as the kernel prints it, which is KiB: 13.1 and 14.1 MiB) | ≤ 15 MB | `VmRSS` and `VmHWM` in `/proc/<pid>/status`, read at 60 s and 660 s after start |
| CPU, sampling, idle | 0.075 % of one core from 60 s to 660 s; 0.076 % over the first 660 s | ≤ 0.1 % | `utime+stime` from `/proc/<pid>/stat` at `CLK_TCK=100`: 5 ticks at 60 s, 50 ticks at 660 s, so 45 ticks over 600 s (and 50 over 660 s). The host was running other builds at the time |
| Full Pulse, 900-row Window, 16 CPUs | 113,879 B (43,233 B of it per-core Window columns) | — | `go test -v -run TestFullPulseSize ./internal/server`: the desktop Capture's Reading with 900 rows alternating its last two Samples, through the real encoder, uncompressed |

RSS grew between the two readings, and the Window is not full until 30 minutes in. This run lasted 11 minutes and
was not under the shipped systemd unit; re-measure under the shipped unit after at least 30 minutes.

Measured on 2026-11-15 under the shipped systemd unit: the `anshin-agent-bin` package built from snapshot release
archives (goreleaser 2.20.0, the release build flags), installed with `pacman -U` and started with
`systemctl enable --now`. Same host, kernel and Go, tailscaled 1.104.2. No requests were sent during the idle
readings; the filesystem map check before them fetched two Pulses, 2 s and 5 s after start.

| What | Measured | Budget | How |
|---|---|---|---|
| Binary in the release archive, linux/amd64 | 8,065,184 B (7.7 MiB) | ≤ 10 MB | `goreleaser release --snapshot --clean`, then `stat -c %s` on the extracted binary |
| Binary in the release archive, linux/arm64 | 7,405,728 B (7.1 MiB) | ≤ 10 MB | as above |
| RSS under the unit, idle | `VmRSS` 13,440 kB at 1.5 s, 16,892 kB at 60 s, 17,588 kB at 660 s and 16,480 kB at 1,860 s; `VmHWM` 18,928 kB at 1,860 s (kB as the kernel prints it, which is KiB: 16.1 MiB at 1,860 s, peak 18.5 MiB) | ≤ 15 MB | `VmRSS` and `VmHWM` in `/proc/<MainPID>/status` at those times after `systemctl enable --now` returned. At 1,860 s the Window held 900 rows and no Gap |
| Memory of the unit's cgroup | `memory.current` 14,057,472 B, `memory.peak` 17,018,880 B at 1,860 s | — | the unit's `memory.current` and `memory.peak` |
| CPU under the unit, idle | 0.076 % of one core | ≤ 0.1 % | `utime+stime` from `/proc/<MainPID>/stat` at `CLK_TCK=100` from 60 s to 1,860 s: 137 ticks over 1,800 s, no restart |
| CPU under the unit, polled every 2 s | 0.236 % of one core | — | as above over 118.7 s while 60 `curl` requests for the full Pulse, with its 900-row Window, went to the tailnet IPv4 address 2 s apart: 28 ticks |
| RSS under the unit, after polling | `VmRSS` 16,292 kB, `VmHWM` 18,928 kB | ≤ 15 MB | read right after the 60th response |

RSS under the unit is over the 15 MB target; the same binary outside the unit stayed under it for the 180 s it ran.
Run as an unprivileged user with `ANSHIN_PORT` set to a spare port, beside `origin/main`'s binary (built as
`make check-agent` builds it) at the same time and with no requests, it had `VmRSS` 13,292 kB at 60 s and 14,172 kB
at 180 s, against 13,468 kB and 14,260 kB. Two Pulses in its first 5 s left it at 13,616 kB at 60 s, against
13,860 kB for an untouched copy. Outside the unit, this branch's binary and `origin/main`'s were within 200 kB of each
other, and two early Pulses made no difference. No run varied the unit's settings under matched conditions, tested
build or request interactions with the unit, or covered a full Window outside the unit, so the cause of the extra
3 MB under the unit has not been isolated. The Go runtime does not read the unit's `MemoryMax=64M`.

## Phone

Measured on 2026-11-12 with the tracer-bullet app (one screen showing the hostname and CPU %), built with AGP 9.5.1,
Kotlin 2.4.21 and the Compose BOM 2026.09.00, `compileSdk` 37, on Arch Linux x86_64.

| What | Measured | Budget | How |
|---|---|---|---|
| APK, release | 1,327,640 B (1.27 MiB) | ≤ 5 MB | `./gradlew assembleRelease` (R8 with `isMinifyEnabled` and `isShrinkResources`, no signing config), then `stat -c %s` on `app-release-unsigned.apk` |
| APK, release, signed | 1,343,091 B (1.28 MiB) | ≤ 5 MB | the APK above through `zipalign -p 4` and `apksigner sign` with a throwaway debug key, then `stat -c %s` |
| APK, release, download size | 1,181,627 B (1.13 MiB) | — | `apkanalyzer apk download-size` on the signed APK |
| APK, debug | 12,550,986 B (12.0 MiB) | — | `make check-android`, then `stat -c %s` on `app-debug.apk`; not minified, never shipped |
| Cold start to first Pulse | pending the device step | ≤ 500 ms | — |

Most of the release APK is R8's `classes.dex`: 2,073,916 B, compressed to 1,068,823 B in the APK (`unzip -lv`).

Measured on 2026-11-13 with the theme engine (four themes, the theme setting in Preferences DataStore 1.3.0, JetBrains
Mono NL bundled), same toolchain, Gradle on Temurin 21.0.13, on Arch Linux x86_64.

| What | Measured | Budget | How |
|---|---|---|---|
| APK, release (with the theme engine) | 1,600,214 B (1.53 MiB) | ≤ 5 MB | `./gradlew assembleRelease`, then `stat -c %s` on `app-release-unsigned.apk` |
| APK, release, signed (with the theme engine) | 1,614,673 B (1.54 MiB) | ≤ 5 MB | the APK above through `zipalign -p 4` and `apksigner sign` (build-tools 37.0.0) with a throwaway key, then `stat -c %s` |
| APK, release, download size (with the theme engine) | 1,387,764 B (1.32 MiB) | — | `apkanalyzer apk download-size` on the signed APK |
| APK, debug (with the theme engine) | 13,421,696 B (12.8 MiB) | — | `./gradlew assembleDebug`, then `stat -c %s` on `app-debug.apk`; not minified, never shipped |
| Bundled font in the release APK | 100,331 B compressed (208,576 B) | — | `unzip -lv` on `app-release-unsigned.apk`, `assets/fonts/JetBrainsMonoNL-Regular.ttf`; its `OFL.txt` is 1,940 B compressed |

`classes.dex` is now 2,253,184 B, compressed to 1,158,585 B (`unzip -lv`); the rest of the growth is DataStore's
`libdatastore_shared_counter.so`, 30,088 B stored across four ABIs. The 52 screenshot goldens total 1,821,188 B in the
repository (`find android/app/src/test/screenshots -name '*.png' -printf '%s\n'`, summed) and don't ship.

Measured on 2026-11-16 with the Machine detail screen (#15): frame timing while scrolling and while idle, on an
**emulator, not a Pixel**. The build is the release R8 build at 9e9623c, installed on a Pixel 9-profile API 37 Google
APIs emulator with `hw.gpu.enabled=no`, so every frame was rendered in software. Font scale 1.0, dark appearance, theme
Ink & bone. The QA Agent ran `-machine full -samples live`, so it added a Sample every 2 s, and stayed up unrestarted
through both intervals while the app stayed in the foreground on the detail. Frame statistics are `dumpsys gfxinfo`
through `scripts/qa frames`: `--reset` before each interval and `--collect` after it. Durations are from the host's
monotonic clock around each interval.

| What | Measured | Budget | How |
|---|---|---|---|
| Detail, scrolling, emulator | 120.07 s; 1,135 frames; 652 janky (57.44 %); p50 / p90 / p95 / p99 38 / 150 / 150 / 150 ms | — | 33 alternating `scroll down` and `scroll up` gestures (fraction 0.42) through the bound agent-device command, then 12 `scroll up` back to the top. A device screen recording ran throughout and a screenshot was taken every fourth gesture, both inside the interval. 60 consecutive successful app `GET /v1/pulse` polls, each 2.004 to 2.019 s after the previous, spanned 118.39 s, so at least 59 newer Samples arrived; the visible CPU went from 21.6 % to 25.9 % |
| Detail, idle, emulator | 61.00 s; 91 frames (1.49 frames/s); 28 janky (30.77 %); p50 / p90 / p95 / p99 11 / 16 / 16 / 16 ms | — | the detail left untouched after a second `--reset`; 30 successful polls spanned 58.13 s and the visible CPU went from 25.9 % to 25.4 %. Frames divided by seconds is 1.49, far below the display's 60 per second, so nothing redrew continuously |

Both rows are emulator figures. A software-rendered emulator says nothing about a Pixel's GPU, and no jank or percentile
threshold was applied to them, so they support no claim that the detail scrolls smoothly. The frame check on a
physical Pixel at 2 s Sample updates is pending the Owner's run. The raw `gfxinfo` text and the parsed summaries, and
the request-by-request spacing, were kept in the run's evidence directory, outside the repository.

Measured again on 2026-11-16 for the same screen, after the Watcher (#40) merged into the branch: frame timing while
scrolling and while idle, on an **emulator, not a Pixel**. The build is the release R8 build at 9dba515, signed with a
throwaway key and installed on a new Pixel 9-profile API 37 Google APIs emulator with `hw.gpu.enabled=no` and
`hw.gpu.mode=auto`, the same software-rendered profile as above. Font scale 1.0, dark appearance, theme Ink & bone. The
QA Agent ran `-machine full` with `-samples live`, which now repeats the current Reading in every new Sample, so the
visible values hold still and Sample progression is shown by the request cadence and the sliding Window, not by
changing numbers. It stayed up unrestarted through both intervals while the app stayed in the foreground on the detail.
Frame statistics are `dumpsys gfxinfo` through `scripts/qa frames`: `--reset` before each interval and `--collect`
after it. Durations are from the host's monotonic clock around each interval, excluding the `frames` commands.

| What | Measured | Budget | How |
|---|---|---|---|
| Detail, scrolling, emulator | 81.59 s; 766 frames; 475 janky (62.01 %); p50 / p90 / p95 / p99 40 / 150 / 150 / 200 ms | — | 31 `scroll down` and `scroll up` gestures through the bound agent-device command, top to bottom and back, with a device screen recording and periodic screenshots and snapshots inside the interval and the repositioning outside it. 40 consecutive successful app `GET /v1/pulse` polls, each 1.988 to 2.010 s after the previous, spanned 78.00 s; in the same CPU viewport before and after, the wave shifted left and its flat tail of repeated Readings grew |
| Detail, idle, emulator | 60.00 s; 60 frames (1.00 frames/s); 39 janky (65 %); p50 / p90 / p95 / p99 16 / 16 / 16 / 16 ms | — | the detail left untouched after a second `--reset`, with no screenshot, snapshot, recording or overlay inside the interval. 29 consecutive successful polls, each 1.998 to 2.002 s after the previous, spanned 56.00 s; Sample age read 0 s before and 1 s after. 60 frames divided by 60.000 s is 1.00 frames/s, far below the display's 60 per second, so nothing redrew continuously |

A first idle collection took one screenshot just after its `--reset`, so it was not idle; it read 60 frames and 35
janky (58.33 %) and is excluded, with the untouched repeat above in its place.

Both rows are emulator figures. A software-rendered emulator says nothing about a Pixel's GPU, and no jank or percentile
threshold was applied to them, so they support no claim that the detail scrolls smoothly. The frame check on a
physical Pixel at 2 s Sample updates is still pending the Owner's run. The raw `gfxinfo` text and the parsed summaries,
and the request-by-request spacing, were kept in the run's evidence directory, outside the repository.

Measured on 2026-11-17: the heap that decoding a hostile Pulse needs, which sets the phone's 1 MiB response cap. JVM,
not ART: Temurin 21.0.13 with `-XX:+UseSerialGC`, kotlinx-serialization 1.13.0, calling the compiled `decodePulse` from
a throwaway harness that isn't committed. Each body is a Pulse with one field holding a list padded to the size with
one item repeated. Peak is the smallest `-Xmx`, found by bisection in 1 MiB steps, at which the body still decodes;
before is the same with battery decoded through a JSON tree, as it was until this change.

| Field, repeated item | 1 MiB body | 2 MiB body | 2 MiB body, before |
|---|---|---|---|
| `filesystems`, `{}` | 21 MiB | 43 MiB | 43 MiB |
| `cpu_core_pct`, `0` | 19 MiB | 35 MiB | 35 MiB |
| `battery.batteries`, `{}` | 16 MiB | 31 MiB | 105 MiB |
| `battery` unknown key, `{"":0}` | 8 MiB | 8 MiB | 85 MiB |
| `verdict.level`, `{"":0}` | 43 MiB | 85 MiB | 85 MiB |
| `verdict.reasons[].code`, `{"":0}` | 43 MiB | 85 MiB | 85 MiB |

Nesting each item in single-item arrays costs more, because every 2-byte `[]` pair becomes a list in the tree. With
`verdict.level` holding the item wrapped in *n* arrays, 1 MiB body, same method (a cell can differ by about 2 MiB
between runs: the flat `0` row measured 41 MiB once and 43 MiB once):

| Item | n = 0 | n = 2 | n = 5 | n = 20 | n = 150 | n = 1,000 | n = 10,000 |
|---|---|---|---|---|---|---|---|
| `{"":0}` | 43 MiB | 47 MiB | 49 MiB | 51 MiB | 53 MiB | 8 MiB | 8 MiB |
| `{}` | 31 MiB | 43 MiB | 49 MiB | 51 MiB | 53 MiB | 8 MiB | 8 MiB |
| `0` | 43 MiB | 49 MiB | 51 MiB | 53 MiB | 53 MiB | 8 MiB | 8 MiB |
| `[]` | 17 MiB | 37 MiB | 45 MiB | 51 MiB | 53 MiB | 8 MiB | 8 MiB |
| `""` | 13 MiB | 35 MiB | 45 MiB | 51 MiB | 53 MiB | 8 MiB | 8 MiB |
| `null` | 8 MiB | 25 MiB | 37 MiB | 47 MiB | 53 MiB | 8 MiB | 8 MiB |

At 1,000 levels and deeper the decoder refuses the Pulse before building much. 53 MiB is the most any of these 42
bodies needed, so it is the figure the docs use for a 1 MiB body.

A full Window's size per CPU comes from `go test ./internal/server -run TestFullPulseSize -v` on the same day: 113,879 B
for the 16-CPU fixture, 43,233 B of it `window.cpu_core_pct`, so 2.7 kB per CPU at its values. The longest per-core
value is `null,`, 5 B in each of 900 rows, so 4.5 kB per CPU; the 70,646 B left for the rest of that Pulse leaves
(1,048,576 − 70,646) / 4,500 ≈ 217 CPUs under the cap, or about 360 at 2.7 kB.
