# Machine detail

> **MachineBook update:** The steps below were verified before MachineBook, when the card sat under the Watch button and the Machine field. Neither exists now, so every step that types an address or taps `Watch` is superseded until a run re-verifies it.
>
> Current path, **not yet verified by a run**: Add machine → Name, Address, Port → Save → Done → tap the Machine panel, which opens the Machine screen (`All Machines`, the Machine name, its address and `Edit` above the card). The card opens the detail as before. `Back to Machine` and system back return to the Machine screen; `All Machines` and system back from there return to the Dashboard. Where a step says `Watch` or the Machine field identifies the card, read `Edit`. A fresh collection now means closing and reopening the app, or editing the Machine's address.

Verified at 9dba515, the build after #40's Watcher merged, on the installed release build (R8, signed with a throwaway key) on a Pixel 9-profile API 37 Google APIs emulator, with the reverse route. Where a step says what 9e9623c did, that was the build before the merge and is kept only to explain a difference. Follow [environment](../references/environment.md) for setup and ownership, and [Pulse](pulse.md) for the card's states. Ink & bone, font 1.0, dark appearance unless a step says otherwise.

## Card to detail and back

`Live` appears in the detail's header too, so it identifies neither screen. The card shows `Watch` and the Machine field; the detail shows `Back to Machine` and neither of those.

1. Reach a proven card: `<bound> open io.github.fsrodrig.anshin`, type the synthetic address, `press text="Watch"`, then `wait text Live`. Live with a matching GET `/v1/pulse` 200 in the access log proves the route.
2. Take a fresh `snapshot -i`. The whole card is one group whose label lists its values (`Live, <hostname>, Verdict, ok, CPU, 21%, …`). `press @<that group>` opens the detail.
3. Detail: `Back to Machine`, `Live`, `last Sample … ago` and the Overview section are present; `Watch` and the Machine field are not. The header reads `· last Sample <age> ago` (9e9623c read `last Pulse … ago`).
4. `press text="Back to Machine"` returns to the card with its values.
5. The explicit route: `scroll down 0.5` until `Machine details` is visible, then `press text="Machine details"`. It opens the same detail.
6. System back: `press text="Back"` taps the navigation bar's Back (needs three-button navigation, see environment). It returns to the card, not the launcher, with the same Machine selected.

Use a fresh snapshot before every press; refs from an older one go stale when the screen changes.

## Setups

`-machine full` is `machine-full`; `-samples live` (`samples-live`) is now the default and `-samples frozen` (`samples-frozen`) replaced the old `-samples fixed`. Change a setup with `scripts/qa agent start --run <run> -- <flags>`.

- **Populated detail, live Samples:** `-whois allow -machine full -cpu ordinary -hostname synthetic -battery discharging -verdict judged -window full`. Every section has values, a Gap-free Window and eleven charts, and the QA Agent adds a Sample every 2 s, so the detail stays Live. Each new Sample repeats the current Reading, so values hold still while the Window slides; at 9e9623c the branch's own live recorder moved them each Sample. `-window gap` may now be combined with live Samples: the QA Agent evicts the Gap as the production recorder does.
- **Populated detail, frozen Samples:** the same flags with `-samples frozen`. No Sample is ever added, so the detail turns Stale once its data is 10 s old, while the access log keeps showing 200s. Enter the detail within a few seconds of the first frozen Pulse to see the Live-to-Stale transition.
- **Unavailable everywhere:** `-machine minimal -cpu null -hostname null -battery unavailable -verdict null`; `-battery none` is a different state (`no battery`), and `-cpu zero` shows `0%` in the card and `0.0 %` in the detail.
- **A fresh body needs a fresh collection:** go back to the card, `close` the app, change the setup, `open`, type the address and `Watch` again. Restarting the QA Agent alone does not discard what the phone already holds.

## Frame timing

1. Establish the live-Samples setup above, the detail on screen, font 1.0 and dark. Record the visible CPU, then do not restart the Agent, press Home or open an overlay during either interval.
2. `scripts/qa frames --run <run> --reset`, then scroll the detail top to bottom and back with the bound command for at least 60 s, with a recording. Here, `scroll down 0.42 --settle` ten times, then `scroll up 0.42 --settle` ten times, alternating, with a screenshot every fourth gesture and `record start --scope device`, ran 120 s (33 gestures) at 9e9623c, then twelve `scroll up 0.42` returned to the top. At 9dba515 31 gestures (twelve down, twelve up, seven down) ran 81.6 s, with the recording and periodic screenshots and snapshots inside the interval and the repositioning outside it. The screenshots and the recording are overhead inside that interval; say so with the figures.
3. `scripts/qa frames --run <run> --collect <name>` writes `<run>/frames/<name>.txt` and `<name>.json` (frames, janky frames and percent, p50/p90/p95/p99). Measure the elapsed time from the host's monotonic clock around the interval, not from a sleep.
4. Idle: stop scrolling, settle the viewport, `--reset` again, leave the detail untouched for about 60 s, `--collect` again. Take screenshots, snapshots and recordings before the `--reset` and after the `--collect`, never between them (see the traps).

**How Sample progression was proven.** The access log's `GET /v1/pulse` 200 lines for the app (not the readiness probe), all in one segment under one uninterrupted setup. 60 consecutive 200s, each spaced 2.004 s to 2.019 s from the previous, spanned 118.39 s; the live clock returns a newer Sample per 2 s, so at least 59 newer Samples arrived. The visible CPU also moved from 21.6% to 25.9%. A log count or a timer alone does not show that Samples progressed. Idle showed 30 successful polls over 58.13 s and CPU 25.9% to 25.4%. That run used 9e9623c's `-samples live`, whose values moved each Sample.

At 9dba515 the QA Agent's `-samples live` repeats the current Reading in every new Sample, so the visible CPU stays at 21%. The oracle is the cadence and the sliding Window: consecutive app `GET /v1/pulse` 200s in one access-log segment under one uninterrupted setup (40 spanning 78.0 s, 1.988 s to 2.010 s apart, during the scroll; 29 spanning 56.0 s, 1.998 s to 2.002 s apart, during the idle), the header's Sample age at 0 s to 1 s in the snapshots taken minutes into the run, and, in the same CPU viewport before and after an interval, the wave shifted left with its flat tail of repeated Readings longer. The request lines carry names only, so they do not show Cursor values.

An idle frame count divided by the elapsed seconds is the oracle for "no continuous redraw": 91 frames in 61.0 s is 1.49 frames/s, far below 60. Age ticks and 2 s Samples are expected. The emulator numbers carry no jank or smoothness threshold; see [Measurements](../../../../docs/measurements.md).

## Oracles

**Stale and Unreachable with last values.** At 9e9623c the detail, still on the interim `follow` loop, showed pinned `Unreachable`, `These are the last values Anshin received, not current ones.` and `No current Verdict` the moment the QA Agent stopped. At 9dba515 it follows the Watcher, and all four steps below were observed:

1. With `Back to Machine` visible, `scripts/qa agent stop --run <run>` (the app stays foreground, nothing else changes). The detail stays `Live` while the data is under 10 s old, then shows pinned `Unreachable` with `· last Sample <age> ago` (read at 10 s and again at 18 s), and first in the list: `No Pulse from <Machine>. The Machine may be off, its Agent stopped, or this phone off the tailnet.`, `Next try in <duration>` or `Trying again`, `VPN looks off` with its advisory text (the emulator has no VPN), and `Last Sample <age> ago. Nothing here is current.` The interactive snapshot shows the cause and the VPN text as `Text view` with a truncated preview; take a full snapshot or screenshot to read them.
2. Observe the age advancing between two observations at least 5 s apart. Scroll to Overview: `Last Verdict` with `ok, <age> ago`, and no bare `ok` and no `Live`. Scroll the seven sections: the earlier values and charts remain, in the unavailable tone with `last` captions, and no new Gap for the stop. Tapping the card after backing out reopens the same aged detail.
3. Recovery: `scripts/qa agent start` with the earlier flags while still in the detail. A new 200, `Live`, age near 0 s, `ok` and the body's values return in the same detail; do not tap `Watch` or leave. `wait text Live` can time out once right after the restart; take a snapshot before concluding it did not recover.
4. Stale: start the QA Agent with `-samples frozen`. The detail turns `Stale` once the data is 10 s old, with `Last Sample <age> ago. Nothing here is current.` first, `Last Verdict` as `ok, <age> ago`, and no cause sentence, no `Next try` and no `VPN looks off`, while the access log keeps showing 200s. Observed at 10 s and again at 22 s. The same Stale card has `No current Verdict` and no `Machine details` button, even scrolled to its end; a tap on it opens the aged detail, and restoring live Samples returns `Live` and the current Verdict in that detail.

**Answers that withhold values.** From a populated detail, change only the QA Agent and keep the detail open. Each answer shows the Machine address, keeps `Back to Machine`, and has no `Watch`, no measurement, no chart and no earlier value or Verdict:

| QA Agent change | Detail shows |
| --- | --- |
| `-whois deny` | `Forbidden` and the Owner/untagged-device explanation |
| `-mode raw -body <schema-2.json>` | `Incompatible` and the update explanation |
| `-mode raw -body <not-a-pulse.txt>` | `Incompatible` and the same explanation |

Those three rows were observed at 9e9623c, and their words are unchanged. The `Incompatible` text is mixed-span; take a full snapshot or screenshot for it, since the interactive snapshot truncates it. Restore the populated setup without navigating; the detail returns to Live with its values, then `Back to Machine` before the next independent cell.

**HTTP failures.** At 9e9623c `-whois localapi-error` showed `Agent can't reach tailscaled` and `-mode raw -status 500` showed `No Pulse` with `HTTP 500`, both without values. Since #40 they are Unreachable. From a fresh populated detail the values stay `Live` at a small age while the failing answers arrive (age 0 s, 3 s and 4 s were read), and once the data is 10 s old the detail shows the Unreachable layout above with the cause `The Agent on <Machine> can't ask tailscaled who this phone is.` (the 503) or `<Machine> answered HTTP 500 instead of a Pulse.`, keeps the last values aged, with `Last Verdict` as `ok, <age> ago`, and recovers to `Live` in the same detail when the QA Agent is restored. Start each from its own fresh populated detail.

**Before any Pulse.** The detail also stays open when the Watcher has no Pulse yet, with `Back to Machine` and no `Watch`, value, chart, age or `Last Verdict`:

1. `Connecting` with `Asking <Machine> for its Pulse.`, while the first request is slow (`-delay 6s`).
2. At the 5 s read timeout, `Unreachable` with the `No Pulse from <Machine>` cause, `Next try in <duration>`, and `VPN looks off`; never `Incompatible`. Both appeared first on the card (fresh `Watch` against `-delay 6s`), and then inside the detail.
3. Restoring the setup without `-delay` returns `Live` and the values in place.

A detail with no Pulse is reached by recreating the Activity, since a fresh Watcher has no data: set `settings text-size` to the 1.35 preset, establish a populated detail, press Home and prove the Activity stopped as in [lifecycle](../references/lifecycle.md), start the QA Agent with `-delay 6s` while hidden, set `settings text-size` back to its 1.0 preset (read `platformValue`), then `open` the app. The first request has neither `after` nor `wait`, a new `performCreate` follows the stop although the ActivityRecord token is the same, and the Machine and detail selection survived. Process death was not exercised.

**Resuming.** `press text="Home"` (agent-device may report COMMAND_FAILED because the launcher took the foreground, which is the outcome) and prove the Activity stopped as in [lifecycle](../references/lifecycle.md). At 9e9623c each return started a new collection: with `-delay 4s` the detail showed `Connecting` with `Back to Machine` and none of the earlier values (the snapshot had to be prompt, it lasted about 4 s), and with the Agent stopped it showed `Unreachable` with its message and no earlier age, values or Verdict; restarting the Agent restored `Live` in the same detail. The Watcher keeps the Window and Cursor while hidden, so at 9dba515 a return after more than 10 s showed `Stale` with the last values and their age first (27 s after a hidden stop of at least 23.7 s), then `Live` once the first resumed request was answered (after 4 s); with the Agent stopped while hidden, it showed `Unreachable` at 24 s with the last values, cause, `Next try` and `VPN looks off`, never `Connecting`, and restarting the Agent restored `Live` with no navigation. Hidden, the app made no request. `Connecting` appears only for a Machine with no Pulse yet (see Before any Pulse).

## Traps

- **An unowned install on the default AVD.** `up` refuses it, because uninstalling would delete data the run does not own. Create a uniquely named AVD for the run with `avdmanager create avd` from an installed non-Play image, `-d pixel_9`, no `--force`, and check `hw.mainKeys=no` in its generated config; a generic profile has `mainKeys=yes` and hides the navigation bar. Open it through `device_open`, and delete only that AVD after owned cleanup and shutdown.
- **Replaying `desktop.delta.json` right after `desktop.json`.** This is an inconsistent conversation, not a fixture of an update. The full already ends at Cursor `…1.3`, while the delta is an answer to Cursor `…1.2` (contract § Fixtures), and § Window invariant 4 forbids keeping or joining history in that case. The phone rightly drops the Window: Live with the delta's flat values (CPU 70.2%, Memory 55.8%), the Overview Window and every history chart say they are unavailable, and `Back to Machine` stays. The raw responder keeps serving the delta even when the phone asks for a full Window, so history never comes back. Proof of a valid full/delta merge belongs to the unit tests, not this replay.
- **First `sys.boot_completed` read empty.** Read it again until it is `1` before `up` or any UI step; an online adb device is not a booted one.
- **`open --settle` is unsupported.** `help open` lists the options; use plain `open`.
- **`scroll up` reports `Already at top` while a lower section (Network) is still on screen.** The 0.7 fraction did not move that viewport; repeated 0.42 gestures did. Confirm the top with a fresh snapshot, not the tool's message.
- **A screenshot between `--reset` and `--collect` makes the idle interval not idle.** The first idle collection had one device screenshot just after its `--reset`. The screenshot is a device action the app did not ask for, so the interval no longer shows what the untouched detail draws; its statistics (60 frames, 35 janky, 58.33 %) came out close to the untouched repeat's (60 frames, 39 janky, 65 %), which only shows the two cannot be told apart from the numbers. Keep the collection under another name, exclude it from the result, and repeat the idle interval with no screenshot, snapshot, recording or overlay inside it.
- **The age text can read 9 s when the state flips at 10 s.** After an HTTP 503 the first snapshot read `9 s` and the screenshot just after it read `10 s`: the displayed seconds and the Watcher's own clock are not read at the same instant. Do not pass or fail the 10 s boundary from one reading; the threshold belongs to the Watcher's tests.
- **A capture subprocess can exit 143 without output** after a few frames of a long scroll. Take the remaining frames through the same bound session and keep the overlap.
