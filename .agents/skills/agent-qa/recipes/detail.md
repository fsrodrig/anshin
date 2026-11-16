# Machine detail

Verified at 9e9623c on the installed release build (R8, signed with a throwaway key) on a Pixel 9-profile API 37 Google APIs emulator, with the reverse route. Merging #40's Watcher changed the detail in Stale and Unreachable and the QA Agent's `-samples` flag; every step marked **needs a fresh device run** describes the merged build and has not been observed yet. Follow [environment](../references/environment.md) for setup and ownership, and [Pulse](pulse.md) for the card's states. Ink & bone, font 1.0, dark appearance unless a step says otherwise.

## Card to detail and back

`Live` appears in the detail's header too, so it identifies neither screen. The card shows `Watch` and the Machine field; the detail shows `Back to Machine` and neither of those.

1. Reach a proven card: `<bound> open io.github.fsrodrig.anshin`, type the synthetic address, `press text="Watch"`, then `wait text Live`. Live with a matching GET `/v1/pulse` 200 in the access log proves the route.
2. Take a fresh `snapshot -i`. The whole card is one group whose label lists its values (`Live, <hostname>, Verdict, ok, CPU, 21%, …`). `press @<that group>` opens the detail.
3. Detail: `Back to Machine`, `Live`, `last Sample … ago` and the Overview section are present; `Watch` and the Machine field are not. At 9e9623c the header read `last Pulse … ago`; the wording is the merged build's (**needs a fresh device run**).
4. `press text="Back to Machine"` returns to the card with its values.
5. The explicit route: `scroll down 0.5` until `Machine details` is visible, then `press text="Machine details"`. It opens the same detail.
6. System back: `press text="Back"` taps the navigation bar's Back (needs three-button navigation, see environment). It returns to the card, not the launcher, with the same Machine selected.

Use a fresh snapshot before every press; refs from an older one go stale when the screen changes.

## Setups

`-machine full` is `machine-full`; `-samples live` (`samples-live`) is now the default and `-samples frozen` (`samples-frozen`) replaced the old `-samples fixed`. Change a setup with `scripts/qa agent start --run <run> -- <flags>`.

- **Populated detail, live Samples:** `-whois allow -machine full -cpu ordinary -hostname synthetic -battery discharging -verdict judged -window full`. Every section has values, a Gap-free Window and eleven charts, and the QA Agent adds a Sample every 2 s, so the detail stays Live. Each new Sample repeats the current Reading, so values hold still while the Window slides; at 9e9623c the branch's own live recorder moved them each Sample. `-window gap` may now be combined with live Samples: the QA Agent evicts the Gap as the production recorder does.
- **Populated detail, frozen Samples:** the same flags with `-samples frozen`. No Sample is ever added, so the detail turns Stale once its data is 10 s old (**needs a fresh device run**).
- **Unavailable everywhere:** `-machine minimal -cpu null -hostname null -battery unavailable -verdict null`; `-battery none` is a different state (`no battery`), and `-cpu zero` shows `0%` in the card and `0.0 %` in the detail.
- **A fresh body needs a fresh collection:** go back to the card, `close` the app, change the setup, `open`, type the address and `Watch` again. Restarting the QA Agent alone does not discard what the phone already holds.

## Frame timing

1. Establish the live-Samples setup above, the detail on screen, font 1.0 and dark. Record the visible CPU, then do not restart the Agent, press Home or open an overlay during either interval.
2. `scripts/qa frames --run <run> --reset`, then scroll the detail top to bottom and back with the bound command for at least 60 s, with a recording. Here, `scroll down 0.42 --settle` ten times, then `scroll up 0.42 --settle` ten times, alternating, with a screenshot every fourth gesture and `record start --scope device`, ran 120 s (33 gestures), then twelve `scroll up 0.42` returned to the top. The screenshots and the recording are overhead inside that interval; say so with the figures.
3. `scripts/qa frames --run <run> --collect <name>` writes `<run>/frames/<name>.txt` and `<name>.json` (frames, janky frames and percent, p50/p90/p95/p99). Measure the elapsed time from the host's monotonic clock around the interval, not from a sleep.
4. Idle: stop scrolling, settle the viewport, `--reset` again, leave the detail untouched for about 60 s, `--collect` again. Take screenshots outside the interval.

**How Sample progression was proven.** The access log's `GET /v1/pulse` 200 lines for the app (not the readiness probe), all in one segment under one uninterrupted setup. 60 consecutive 200s, each spaced 2.004 s to 2.019 s from the previous, spanned 118.39 s; the live clock returns a newer Sample per 2 s, so at least 59 newer Samples arrived. The visible CPU also moved from 21.6% to 25.9%. A log count or a timer alone does not show that Samples progressed. Idle showed 30 successful polls over 58.13 s and CPU 25.9% to 25.4%. That run used 9e9623c's `-samples live`, whose values moved each Sample. The merged QA Agent repeats the Reading, so the visible CPU no longer moves; the age resetting with each Sample and the access log's 2 s cadence are the oracle instead (**needs a fresh device run**).

An idle frame count divided by the elapsed seconds is the oracle for "no continuous redraw": 91 frames in 61.0 s is 1.49 frames/s, far below 60. Age ticks and 2 s Samples are expected. The emulator numbers carry no jank or smoothness threshold; see [Measurements](../../../../docs/measurements.md).

## Oracles

**Stale and Unreachable with last values (needs a fresh device run).** At 9e9623c the detail, still on the interim `follow` loop, showed pinned `Unreachable`, `These are the last values Anshin received, not current ones.` and `No current Verdict` the moment the QA Agent stopped, and kept the earlier values and charts in the unavailable tone with no new Gap. The merged build follows the Watcher:

1. With `Back to Machine` visible, `scripts/qa agent stop --run <run>` (the app stays foreground, nothing else changes). The detail stays `Live` while the data is under 10 s old, then shows pinned `Unreachable` with `· last Sample <age> ago`, and first in the list: `No Pulse from <Machine>. The Machine may be off, its Agent stopped, or this phone off the tailnet.`, `Next try in <duration>` or `Trying again`, `VPN looks off` (the emulator has no VPN), and `Last Sample <age> ago. Nothing here is current.`
2. Observe the age advancing between two observations at least 5 s apart. Scroll to Overview: `Last Verdict` with `ok, <age> ago`, and no bare `ok` and no `Live`. Scroll the seven sections: the earlier values and charts remain, in the unavailable tone, with no new Gap for the stop.
3. Recovery: `scripts/qa agent start` with the earlier flags while still in the detail. A new 200, `Live`, age near 0 s, `ok` and the body's values return in the same detail; do not tap `Watch` or leave.
4. Stale: start the QA Agent with `-samples frozen`. The detail turns `Stale` once the data is 10 s old, with `Last Sample <age> ago. Nothing here is current.` first and no cause sentence, while the access log keeps showing 200s.

**Answers that withhold values.** From a populated detail, change only the QA Agent and keep the detail open. Each answer shows the Machine address, keeps `Back to Machine`, and has no `Watch`, no measurement, no chart and no earlier value or Verdict:

| QA Agent change | Detail shows |
| --- | --- |
| `-whois deny` | `Forbidden` and the Owner/untagged-device explanation |
| `-mode raw -body <schema-2.json>` | `Incompatible` and the update explanation |
| `-mode raw -body <not-a-pulse.txt>` | `Incompatible` and the same explanation |

Those three rows were observed at 9e9623c, and their words are unchanged. The `Incompatible` text is mixed-span; take a full snapshot or screenshot for it, since the interactive snapshot truncates it. Restore the populated setup without navigating; the detail returns to Live with its values, then `Back to Machine` before the next independent cell.

**HTTP failures (needs a fresh device run).** At 9e9623c `-whois localapi-error` showed `Agent can't reach tailscaled` and `-mode raw -status 500` showed `No Pulse` with `HTTP 500`, both without values. Since #40 they are Unreachable: once the data is 10 s old, the detail shows the Unreachable layout above with the cause `The Agent on <Machine> can't ask tailscaled who this phone is.` or `<Machine> answered HTTP 500 instead of a Pulse.`, and keeps the last values aged.

**Resuming.** `press text="Home"` (agent-device may report COMMAND_FAILED because the launcher took the foreground, which is the outcome) and prove the Activity stopped as in [lifecycle](../references/lifecycle.md). At 9e9623c each return started a new collection: with `-delay 4s` the detail showed `Connecting` with `Back to Machine` and none of the earlier values (the snapshot had to be prompt, it lasted about 4 s), and with the Agent stopped it showed `Unreachable` with its message and no earlier age, values or Verdict; restarting the Agent restored `Live` in the same detail. The Watcher keeps the Window and Cursor while hidden, so on the merged build a return after more than 10 s shows `Stale` with the last values and their age first, then `Live` on the first answer, or `Unreachable` with those values if the Agent is stopped (**needs a fresh device run**). `Connecting` now appears only for a Machine with no Pulse yet, such as a detail restored after process death.

## Traps

- **An unowned install on the default AVD.** `up` refuses it, because uninstalling would delete data the run does not own. Create a uniquely named AVD for the run with `avdmanager create avd` from an installed non-Play image, `-d pixel_9`, no `--force`, and check `hw.mainKeys=no` in its generated config; a generic profile has `mainKeys=yes` and hides the navigation bar. Open it through `device_open`, and delete only that AVD after owned cleanup and shutdown.
- **Replaying `desktop.delta.json` right after `desktop.json`.** This is an inconsistent conversation, not a fixture of an update. The full already ends at Cursor `…1.3`, while the delta is an answer to Cursor `…1.2` (contract § Fixtures), and § Window invariant 4 forbids keeping or joining history in that case. The phone rightly drops the Window: Live with the delta's flat values (CPU 70.2%, Memory 55.8%), the Overview Window and every history chart say they are unavailable, and `Back to Machine` stays. The raw responder keeps serving the delta even when the phone asks for a full Window, so history never comes back. Proof of a valid full/delta merge belongs to the unit tests, not this replay.
- **First `sys.boot_completed` read empty.** Read it again until it is `1` before `up` or any UI step; an online adb device is not a booted one.
- **`open --settle` is unsupported.** `help open` lists the options; use plain `open`.
- **`scroll up` reports `Already at top` while a lower section (Network) is still on screen.** The 0.7 fraction did not move that viewport; repeated 0.42 gestures did. Confirm the top with a fresh snapshot, not the tool's message.
- **A capture subprocess can exit 143 without output** after a few frames of a long scroll. Take the remaining frames through the same bound session and keep the overlap.
