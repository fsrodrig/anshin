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

The idle CPU figure isn't yet representative: the 2 s sampling loop arrives with the Window (#8), so re-measure
idle CPU and RSS then.

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
| APK, release (with the theme engine) | 1,583,830 B (1.51 MiB) | ≤ 5 MB | `./gradlew assembleRelease`, then `stat -c %s` on `app-release-unsigned.apk` |
| APK, release, signed (with the theme engine) | 1,598,275 B (1.52 MiB) | ≤ 5 MB | the APK above through `zipalign -p 4` and `apksigner sign` (build-tools 37.0.0) with a throwaway key, then `stat -c %s` |
| APK, release, download size (with the theme engine) | 1,369,860 B (1.31 MiB) | — | `apkanalyzer apk download-size` on the signed APK |
| APK, debug (with the theme engine) | 13,421,696 B (12.8 MiB) | — | `./gradlew assembleDebug`, then `stat -c %s` on `app-debug.apk`; not minified, never shipped |
| Bundled font in the release APK | 100,331 B compressed (208,576 B) | — | `unzip -lv` on `app-release-unsigned.apk`, `assets/fonts/JetBrainsMonoNL-Regular.ttf`; its `OFL.txt` is 1,940 B compressed |

`classes.dex` is now 2,219,692 B, compressed to 1,140,685 B (`unzip -lv`); the rest of the growth is DataStore's
`libdatastore_shared_counter.so`, 30,088 B stored across four ABIs. The 48 screenshot goldens total 1,658,205 B in the
repository (`find android/app/src/test/screenshots -name '*.png' -printf '%s\n'`, summed) and don't ship.
