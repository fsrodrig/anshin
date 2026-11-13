# Developing Anshin

How to build and run both halves on an Arch Linux workstation. The design is in [`design.md`](design.md); the rules
for changes are in [`CONTRIBUTING.md`](../CONTRIBUTING.md).

## Tools

Tool versions are pinned in [`mise.toml`](../mise.toml). With [mise](https://mise.jdx.dev):

```sh
mise install
make setup   # installs the pre-commit identifier scan
make check   # the Done gate; CI runs the same targets
```

Without mise, install the same versions yourself. `go test -race` needs cgo, so a C compiler (`gcc`) must be on
`PATH`. The shipped Agent binaries are built with `CGO_ENABLED=0`.

## Branding

```sh
make check-brand   # the generated files in branding/ match what branding/build.py produces
```

See [`branding/README.md`](../branding/README.md#sources-and-regeneration).

## Agent

```sh
make check-agent   # gofmt, vet, staticcheck, test -race, linux/amd64 and linux/arm64 builds within the size budget
```

The binaries land in `agent/dist/` (ignored by git).

`make check-agent` also regenerates `contract/fixtures` from `contract/captures` and fails if they differ from what
is committed, so commit a fixture change before running it. [`contract/README.md`](../contract/README.md) has the
regenerate command and how to capture a Machine.

### Running it on your Machine

The Agent needs a running, logged-in `tailscaled`. It asks tailscaled who this Machine is, binds only the
Machine's tailnet IPv4 and IPv6 addresses, and exits non-zero if tailscaled can't answer. Run it as your own
unprivileged user. It needs no root, because the LocalAPI calls it makes (status and WhoIs) need no privilege.

```sh
cd agent && go run ./cmd/anshin-agent
```

| Variable | Default | Meaning |
|---|---|---|
| `ANSHIN_PORT` | `7247` | TCP port on the tailnet addresses |
| `TAILSCALE_SOCKET` | `/run/tailscale/tailscaled.sock` | tailscaled's LocalAPI socket |

From another of your own untagged devices on the tailnet, or from the Machine itself on its tailnet address:

```sh
curl "http://$(tailscale ip -4):7247/v1/pulse"
# {"schema":1,"kind":"full","hostname":"…","cpu_pct":21}
```

The Agent doesn't answer on loopback. A tagged device, another user's device or an address outside the tailnet
gets `403`. If tailscaled stops answering, requests get `503`.

### Layout

| Package | What it owns |
|---|---|
| `cmd/anshin-agent` | wiring and configuration |
| `internal/recorder` | measuring the Machine; returns `null` for anything unavailable |
| `internal/recorder/probe` | parsers for `/proc` and `/sys`, reading only through a `Host` |
| `internal/capture` | `anshin-agent capture`: copies and scrubs what `probe` reads into `contract/captures` |
| `internal/capture/replay` | test-only: replays a committed capture as a `Host` |
| `internal/tailnet` | who this Machine is and who a caller is: the LocalAPI client and a fake |
| `internal/server` | the bind, the WhoIs truth table, the Pulse endpoint |

## Phone

The app is one Gradle module, `android/app`, in Kotlin with Compose. Gradle, the Android SDK and the JDK aren't in
`mise.toml`: the Gradle wrapper is committed, and the SDK is installed per user as below.

### The JDK

The Gradle daemon, the Kotlin compiler and the unit tests all run on JDK 21, whatever JDK starts the wrapper.
`android/gradle/gradle-daemon-jvm.properties` pins the daemon, `jvmToolchain(21)` pins compilation and tests, and if
no JDK 21 is installed Gradle downloads one into `~/.gradle/jdks` through the foojay resolver. You only need some JDK
on `PATH` (or in `JAVA_HOME`) to start the wrapper and the SDK tools; Arch's `jdk-openjdk` is fine. To keep it out of
the system, a user-local one works too:

```sh
mise install java@temurin-21
export JAVA_HOME="$(mise where java@temurin-21)"
```

### The Android SDK on Arch, without root

Install Google's command-line tools under your home directory. Take the current `commandlinetools-linux-*_latest.zip`
from Google's [repository manifest](https://dl.google.com/android/repository/repository2-3.xml) (the
`cmdline-tools;latest` package; the manifest has its SHA-1):

```sh
mkdir -p ~/Android/Sdk/cmdline-tools
cd "$(mktemp -d)"
curl -fLO https://dl.google.com/android/repository/commandlinetools-linux-<build>_latest.zip
echo "<sha1>  commandlinetools-linux-<build>_latest.zip" | sha1sum -c
unzip -q commandlinetools-linux-<build>_latest.zip
mv cmdline-tools ~/Android/Sdk/cmdline-tools/latest
```

Then accept the licenses and install the packages the build and the emulator need:

```sh
sdk=~/Android/Sdk/cmdline-tools/latest/bin
yes | $sdk/sdkmanager --licenses
$sdk/sdkmanager "platform-tools" "platforms;android-37.0" "build-tools;37.0.0" "emulator" \
  "system-images;android-37.0;google_apis;x86_64"
echo no | $sdk/avdmanager create avd -n anshin-api37 -k "system-images;android-37.0;google_apis;x86_64" -d pixel_9
```

Newer command-line tools print that `sdkmanager` is deprecated in favour of `android sdk`, and that `--licenses`
is no longer needed; both commands above still work.

The emulator needs KVM. `ls -l /dev/kvm` must show it readable and writable by you (Arch's default is mode `0666`;
otherwise add yourself to the `kvm` group), and `~/Android/Sdk/emulator/emulator -accel-check` should say
"KVM … is installed and usable".

Add these to your shell's profile:

```sh
export ANDROID_HOME=~/Android/Sdk
export PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$ANDROID_HOME/cmdline-tools/latest/bin:$PATH"
```

Gradle finds the SDK through `ANDROID_HOME`, or through `android/local.properties` (ignored by git), which needs an
absolute path:

```sh
printf 'sdk.dir=%s/Android/Sdk\n' "$HOME" > android/local.properties
```

### Building and testing

```sh
make check-android   # lint, JVM unit tests and the debug APK, with Gradle deprecations as errors
```

From `android/`, the same tasks one by one: `./gradlew lintDebug`, `./gradlew testDebugUnitTest` and
`./gradlew assembleDebug`. The debug APK lands in `android/app/build/outputs/apk/debug/app-debug.apk`.

`./gradlew assembleRelease` builds the R8-minified release APK, unsigned until release signing lands (#18), at
`android/app/build/outputs/apk/release/app-release-unsigned.apk`.

### Installing the debug APK

Turn on USB debugging on the phone (or start the emulator with `emulator -avd anshin-api37`), then:

```sh
adb devices          # the phone or emulator must be listed as "device"
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n io.github.fsrodrig.anshin/.MainActivity
```

Type the Machine's MagicDNS name or tailnet IP and tap **Watch**. The phone must be on the tailnet as one of the
Owner's untagged devices, or the Agent answers Forbidden. An emulator isn't on the tailnet, so on an emulator the
screen can only show Unreachable.

### Layout

| File | What it owns |
|---|---|
| `TailnetHttp.kt` | the app's only HTTP client: no proxy, no redirects, and a socket that refuses any address outside `100.64.0.0/10` and `fd7a:115c:a1e0::/48` |
| `AgentLink.kt` | asking an Agent for its Pulse, and turning the answer into Ok, Unreachable, Forbidden, Incompatible or Failed |
| `Pulse.kt` | decoding the Pulse; unknown fields are ignored and missing measurements stay `null` |
| `MachineAddress.kt` | turning what the Owner typed into the Pulse URL, port 7247 unless they gave one |
| `PulseScreen.kt` | the one screen, polling every 2 s only while it is visible |
