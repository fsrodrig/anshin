# The device environment

The coordinator's reference for step 3. Two tools share the device and never swap roles:

- **The bound agent-device command** drives everything a person would do: open and close the app, tap, type, settings, screenshots, recordings. It is the command `device_open` returns, with its session and target arguments; a globally installed `agent-device` is a different session and is never used instead.
- **`scripts/qa`** does the plumbing a person couldn't: build and sign the APK, root the emulator, install, route the synthetic address to the QA Agent, seed a theme, and remove all of it. It never drives the UI.

`scripts/qa help` lists the commands; `scripts/qa capabilities` lists every QA Agent setup by ID. Neither needs a device.

## Open the device

1. `scripts/qa doctor` exits 0. Its `unverified` list is what only this run can show.
2. `device_list`. Choose a stopped, non-Play Android emulator (the AVD from `docs/develop.md` is a Google APIs image). Record whether it was booted and whether any session had it open. A device that is booted or open elsewhere is someone else's: stop with that conflict unless the user handed it to this run.
3. `device_open` with its `hostId` and `id`. Save the returned bound command and session in `<run>/device.json`; use that command unchanged for every UI action. Each thread must use its own bound session. If a handoff loses its command, call `device_open` for the same run-owned device and use the newly returned command. On a device-tool error, quote it and follow its advice once; then report it and go to cleanup.

4. Before screenshots or `up`, allow at most two minutes for Android to finish booting. Identify the emulator serial for the selected AVD with read-only `adb devices` and `adb -s <serial> emu avd name`; require the exact selected AVD name and `adb -s <serial> shell getprop sys.boot_completed` returning `1`. If identity or readiness cannot be established within the bound, stop and report an environment failure. An online adb device alone is not ready.

If the selected AVD has an unowned app install, keep the helper's install refusal. Create a uniquely named dedicated AVD from an already installed non-Play system image using `avdmanager create avd`, without `--force` or copied userdata. Record ownership, open it through `device_open`, and remove only that run-created AVD after owned cleanup and shutdown. Never wipe or delete the original.

## Start the run's plumbing

```sh
scripts/qa up --run <run> --device <AVD name from device_list> -- <QA Agent flags, e.g. -whois allow>
```

`--serial <emulator serial>` selects by serial instead of `--device`; do not pass both. `up` refuses a dirty tree or a checkout that moved off the target SHA, then builds `assembleRelease`, aligns and signs a copy with a throwaway key, verifies the signature, the package and `debuggable=false`, and requires the R8 mapping. It generates the four theme seeds through the real DataStore, roots the emulator, claims it with an owner marker, records the font scale and night mode, installs the APK (refusing an existing install, whose data it must not touch), starts the QA Agent and adds the route. Every resource enters `<run>/ledger.json` before it exists; every command is in `<run>/commands.log`. `<run>/up.json` holds the serial, the APK record, the seed records (or `seedError`) and `routeProbe`.

If `up` fails, read its error, keep what it learned, and go to cleanup: the ledger already names everything to remove.

### Routes

1. **`--route reverse`** (default). The QA Agent listens on the host's loopback; `adb reverse` maps the emulator's `127.0.0.1:7247` to it, and an `OUTPUT` DNAT rule in the emulator sends the synthetic address (default `100.64.0.7`) there. Nothing tailnet-shaped is bound or routed on the host, which is itself a tailnet node.
2. **`--route device`**, when the first gives no Live Pulse. Run `scripts/qa down --run <run>`, then `up` again on the same run with this route: the QA Agent is cross-compiled for the emulator's ABI and runs inside it on the synthetic address added to its loopback.

The route is proven only when the app shows Live for `qa-machine` **and** the QA Agent's access log has that request. `routeProbe` from the emulator's root shell is a hint. If both routes fail, the run is INCONCLUSIVE with reasonCode `environment`; there is no fake-`AgentLink` fallback.

## Setups the runner asks for

The runner changes the QA Agent with `scripts/qa agent start --run <run> -- <flags>` (a restart on the same route) and `scripts/qa agent stop --run <run>` (the route stays, so the app sees an Agent that went away). Raw-mode inputs that exist in the repository:

- `agent/internal/qa/testdata/schema-2.json`: a schema 2 Pulse, so the app shows Incompatible.
- `agent/internal/qa/testdata/not-a-pulse.txt`: a 200 that is not JSON, also Incompatible.

Addresses the runner types into the app, all placeholders: the synthetic address for every Agent scenario; `192.0.2.1` (a documentation address outside the tailnet) for the refusal; a malformed entry such as `[::1` for "Not a Machine address".

## Themes, fonts and appearance

**Seed a theme** (the app has no picker yet):

1. Close the app with the bound command: `<bound> close io.github.fsrodrig.anshin`.
2. `scripts/qa seed --run <run> --theme <key>`. It refuses a running app or an install the run doesn't own, writes the seed atomically with the app's owner, mode and SELinux context, and verifies its digest on the device.
3. Open the app with the bound command and observe; close and open again to see it survive a relaunch.

Keys: `ink-and-bone`, `tokyo-night`, `catppuccin-mocha`, `matte-black`. A fresh install has an empty store, which is the default-theme cell; `<bound> settings clear-app-state io.github.fsrodrig.anshin` returns the run's own install to that state. A seed proves cold-start restoration and the installed appearance, not live switching: that is `ThemeSwitchTest` until the app has a control. A failed seed leaves its cells INCONCLUSIVE with the error.

**Font scale.** `<bound> settings text-size <preset>` reports the real multiplier as `platformValue`; record that number, then relaunch the app. Android's presets give 1.0 (`large`), 1.24 and 1.35, not 1.3. A device cell at 1.35 is labeled 1.35. An exact 1.3 cell needs Android's own Settings UI through the bound command and a read-back of 1.3; otherwise it is a capability gap and the goldens' 1.3 case is the proof.

**Appearance.** `<bound> settings appearance light|dark`, then relaunch to check the system bars. Use a documented state action; a bare appearance getter is unsupported.

`down` restores the font scale and night mode recorded at `up`.

## Evidence

Save full `snapshot` output for result text; interactive-only snapshots can omit it. Inspect the full screenshot when mixed-span text is abbreviated. Screenshots and recordings go under `<run>/media/` through the bound command (`screenshot <run>/media/<cell>.png`, `record start <run>/media/<scenario>.mp4` and `record stop`). The access logs are `<run>/logs/access-<n>.jsonl`: time, method, path, status, header names (never values), duration, cancellation. They cover only requests that reached the QA Agent, so they never show that the app made no connection elsewhere; the refusal's connection boundary is `DestinationCheckTest` and `ProxyBypassTest`. agent-device's `network dump` reads the app's log, not the network.

A screenshot that shows the typed address, a path or a serial stays local unless it is cropped (`screenshot --crop-on <selector>`) and labeled as cropped.

## Cleanup

Run this whatever happened before, in this order, and keep every error:

1. Stop any recording with the bound command.
2. `<bound> close io.github.fsrodrig.anshin`.
3. `scripts/qa down --run <run>`. It removes, newest first, only what the ledger owns: the QA Agent, the route, the install the run made, the settings it recorded, its owner marker and root. It is safe to repeat and works after a partial `up`.
4. `device_close`, with `shutdown` only if this run booted the emulator.

A non-`ok` `down` lists each resource left and why. That is a stop: report it, and never clear it with a wipe, a reboot or another session's tools.
