# MachineBook

> **Navigation changed after 7a0eb6f.** The steps below record what a device showed at the commits they name; they were not re-run after this change and need re-verification on a device. Until then, read their labels and routes through this:
>
> - Every screen has a bar. The Dashboard's bar holds the mark and `anshin`, then two icon buttons labelled `Add Machine` (+) and `Settings` (gear); an empty Dashboard also shows an `Add Machine` button. There is no `Add machine` button under the panels.
> - Every other screen's bar starts with a back arrow labelled `Back to <parent>`: `Back to Dashboard`, `Back to <Machine name>`, `Back to Settings`, `Back to Open-source licences`. It does what system Back does. `All Machines` is gone.
> - The Machine screen and the Machine detail are now one Machine screen, opened by pressing a panel. Its bar holds `Back to Dashboard`, the Machine name and an Edit icon labelled `Edit <Machine name>`; the state line (`Live · last Sample <age> ago`) is pinned under the bar; the list holds the address, the Stale or Unreachable notice, the Verdict, CPU, Memory, Storage, Battery, Temperatures, Network, and last a `Machine` section (hostname, uptime, Window, distro, kernel, architecture, Agent version). There is no card, no `Machine details`, no `Back to Machine` and no Overview section, and the top line shows only on the Dashboard.
> - The form's titles are `Add Machine` and `Edit Machine`, its first field is `Machine name` (was `Name`), and its removal button is `Remove Machine` (was `Remove machine`). Edit Machine opened from the Machine screen returns to it; a confirmed removal lands on the Dashboard. Settings has Theme, Machines and About, with `Version` and `Open-source licences` as rows in About; its Machines open Edit Machine and return to Settings.
> - The top line is in sentence case (`warn · laptop`, `All Machines ok`), and the Stale and Unreachable notice ends with `Nothing here is current`, without the age sentence (the age is pinned under the bar).

Verified at 0af9b99 on the installed release build (R8, throwaway key) with the reverse route, in a run that observed all ten scenarios (50 cells) at the default theme (Ink & bone), dark appearance and font scale 1.0, plus 1.35 and the four themes where noted. Follow [environment](../references/environment.md) for setup, ownership and cleanup. Watcher states are in [Pulse](pulse.md) and the separate detail in [Machine detail](detail.md); this file is the surfaces around them: the form, the Dashboard, the Machine screen, edit and removal.

## Routing hazard

Only the synthetic address on port 7247 is routed to the QA Agent. Enter it with a blank Port, with `7247`, or with `07247`: they are the same endpoint. Never enter another tailnet address, another port on the synthetic address, a short MagicDNS name, an ordinary FQDN or a name from a test or golden.

For a second Machine that must stay unreachable, use a name under the reserved `.invalid` top level (`<absent-name>.invalid`), and `<absent-name-2>.invalid` for an address-edit variant. The form accepted both and saved the Machine as Unreachable with `No Pulse yet`, and only the QA Agent's requests were visible in the access log. The name is chosen because `.invalid` never resolves; claim no zero-packet result, since the log cannot see DNS. This is a resolution failure, never a slow socket: the run did not exercise a held socket here.

Only one saved Machine can use the routed endpoint at a time (a second entry with the same address and port is rejected, see the form). To read the other Machine's detail with its own name and values, hand the route over: edit the first Machine's address to an absent `.invalid` name, edit the second Machine's address to the synthetic address, observe, and then restore both in the reverse order. The run saw each Machine's own name, address and CPU after the hand-over, and a cold reopen kept both in their original order.

## Shortest path

Labels, exactly: `Add machine`, `Name`, `Address`, `Port`, `Save`, `Done`, `Cancel`, `All Machines`, `Edit`, `Machine details`, `Back to Machine`, `Remove machine`, `Keep`, `Remove`. No Watch field exists.

Setup, by capability ID. The baseline was `whois-allow`, `cpu-ordinary`, `samples-live`, `machine-minimal`, `battery-none`, `hostname-synthetic`, `verdict-judged` (`scripts/qa up --run <run> --device <AVD name> -- <flags>`, then `scripts/qa agent start --run <run> -- <flags>` to change it; `stop-start` uses `agent stop`). Populated detail replaced `machine-minimal` and `battery-none` with `machine-full`, `battery-discharging` and `window-full`. The other IDs this surface used: `delay`, `raw-file`, `cpu-null`, `cpu-zero`, `battery-unavailable`, `verdict-null`, `verdict-ok`, `verdict-crit`, `whois-deny`, `whois-localapi-error`, `http-status`, `samples-frozen`. `scripts/qa capabilities` lists their flags. Use one value per flag: replace a flag, never append a duplicate.

1. `<bound> open io.github.fsrodrig.anshin`. A fresh store shows the empty Dashboard.
2. `<bound> snapshot -i`, then `<bound> press 'text="Add machine"'` (or `press @<ref>`).
3. `<bound> snapshot -i`, then `<bound> fill @<ref> "<text>"` for `Name` and for `Address` (the synthetic address). Leave `Port` blank. `press 'text="Save"'`.
4. `<bound> snapshot --force-full` until the form says `Live`. Press `Done`: the Dashboard shows the Machine's panel.
5. Press the panel (its `@<ref>` from a fresh snapshot): the Machine screen. `Edit` opens the Edit machine form. `All Machines` returns to the Dashboard.

Take a fresh `snapshot -i` before every `press` and `fill`, and a `snapshot --force-full` for result text. Expected: the form's `Live` with "<address> answered with a Pulse. <Name> is on your dashboard.", then a panel whose label reads `<Name>, Live, <address>, Updated <n> s ago, CPU, 21%, Memory, — unavailable, Primary disk, CPU temperature`, with a matching `GET /v1/pulse` 200 in the access log.

## The form

Add machine has `Name`, `Address`, `Port` (hint "Leave empty for the Agent's port, 7247"), `Save` and `Cancel`. While an accepted Save is being tested the form shows "Saved. Asking <address> for a Pulse…" with `Save` disabled and `Done` offered; afterwards `Save` stays and `Cancel` has become `Done`. Edit machine has the same fields filled, `Save`, `Done` and `Remove machine`. From Add, `Done` returns to the Dashboard; from Edit it returns to the Machine screen.

Rejections, each with no Machine saved and no request. The message appears under the field named. The interactive snapshot shows long messages as a truncated `preview:`; a full snapshot or the execution output gives the whole text.

| Entry | Field and message |
| --- | --- |
| Blank Name | Name: "Give this Machine a name." |
| 41 ASCII letters as Name | Name: "Use at most 40 characters." |
| Blank Address | Address: "Enter a MagicDNS name or a tailnet IP." |
| `192.0.2.1` | Address: "192.0.2.1 isn't a tailnet address. Tailnet IPs are in 100.64.0.0/10 or fd7a:115c:a1e0::/48." |
| `[::1` | Address: "Not a Machine address. Enter a MagicDNS name or a tailnet IP, without http:// or a path." |
| The synthetic address with `:7247` typed into Address | Address: "Put the port in the Port field." |
| Port `0` | Port: "Enter a port from 1 to 65535." |
| The synthetic address with Port `7247`, a new Name, when that endpoint is already saved | Address: "<Machine> already uses this address and port." |
| An existing Machine's Name in another case with a space on each side, and its address in upper case with a trailing dot | Name: "Another Machine is already called <Name>." and Address: "<Machine> already uses this address and port.", together |

A blank Name, `192.0.2.1` and Port `0` in one Save showed all three messages at once. After every rejection in an empty store the run pressed `Cancel`, saw a fresh empty form reopen, closed and opened the app cold, and saw an empty Dashboard with no QA Agent request in a four-second observation. With a saved Machine the Watcher's `after`/`wait` requests keep the access log busy, so a rejection is checked against that stream: no extra plain request and no new panel. The duplicate rejections left exactly two panels, which survived a cold reopen.

Test connection runs after an accepted Save and is a plain request (no `after`, no `wait`), separate from the Watcher's stream. The Machine is saved before it starts, so no result rolls it back. Outcomes, by setup (the status line, then the message start). The `whois-allow` and `.invalid` rows come from the Add form; the others were each produced in the Edit form by renaming a Machine and pressing `Save`:

| Setup | Status line, then message start |
| --- | --- |
| `whois-allow` | `Live`, "<address> answered with a Pulse. <Name> is on your dashboard." |
| A `.invalid` name | `Unreachable`, "Saved, but no Pulse came from <name>…" |
| `stop-start` stop | `Unreachable`, "Saved, but no Pulse came from <address>. The…" |
| `whois-localapi-error` | `Unreachable`, "Saved, but the Agent on <address> can't ask tailscaled who this phone is." |
| `http-status` 500 | `Unreachable`, "Saved, but <address> answered HTTP 500 instead of a Pulse." |
| `whois-deny` | `Forbidden`, "Saved, but this Agent only answers its Owner'…" |
| `raw-file` with the schema-2 file, or with the non-JSON file | `Incompatible`, "Saved, but the Agent answered in a version of…" |

Each failure left `Done` usable. In each case the Name was changed first (a different suffix each time), and after `Done` and a cold reopen the Dashboard showed that new Name and the same two entries: the persisted rename is the proof that the failure did not roll the Save back. The baseline was restored before the next case.

## The Dashboard

Empty: the title `anshin`, "No Machines yet", "Add a Machine running the Anshin Agent by its MagicDNS name or tailnet IP." and `Add machine`; no top line.

With Machines, the top lines come first, then one full-width panel per Machine in saved order, then `Add machine`. A panel is one button whose label is `<Name>, <State>, <address>, <detail>`. Two panels fit together without scrolling at 1.0. A panel never shows a Verdict.

| Machine | Panel |
| --- | --- |
| Live, baseline | `First, Live, <address>, Updated 1 s ago, CPU, 21%, Memory, — unavailable, Primary disk, CPU temperature`. No Battery row (`battery-none`) |
| Live, the populated `raw-file` body | CPU 21%, Memory 25%, Primary disk 41%, CPU temperature 52 °C, Battery 64% discharging (read from the screenshot; the snapshot repeated the previous label) |
| Live, `machine-full` and `battery-discharging` | `Primary, Live, <address>, Updated 1 s ago, CPU, 21%, Memory, 63%, Primary disk, 25%, CPU temperature, 62 °C, Battery, 64% discharging` |
| Live, `cpu-null`, `battery-unavailable`, `verdict-null` | CPU, Memory, Primary disk, CPU temperature and Battery each `— unavailable`, never `0%` or `0 °C`. A Battery row shows here, unlike `battery-none`; the top line is "No current Verdict from <Name>" |
| Live, `cpu-zero` | `CPU, 0%`, still `Live` |
| Absent `.invalid` name | `Second, Unreachable, <absent-name>.invalid, No Pulse yet` |
| First request delayed (`delay` 3s), cold open | `Primary, Connecting, <address>, Asking for its first Pulse`, no values |
| `samples-frozen` | `Primary, Stale, <address>, Last Sample 11 s ago. Not current.` with the last values |
| `stop-start` stop | `Primary, Unreachable, <address>, Last Sample 21 s ago. Not current.` with the last values |
| `whois-deny` | `Primary, Forbidden, <address>, Only answers its Owner's untagged devices`, no values |
| `raw-file` schema 2 | `Primary, Incompatible, <address>, Update Anshin and the Agent`, no values |

The populated body is a synthetic full schema-1 Pulse the runner writes into `<run>` and serves with `raw-file` at status 200 (`-mode raw -body <run>/populated.json -status 200`). It lists two filesystems, the one at 90% first and the primary disk at 41.3%, memory 25%, a CPU temperature of 52.1 °C, a discharging battery at 64% and an ok Verdict. The panel took the primary disk (41%), never the first filesystem (90%). The body has no cursor or Window, so it is re-read without long-polling; do not infer polling continuity from it.

Top lines sit above the panels and, on the Machine screen, above `All Machines`. There is one line per Machine: "OK · <Name>" or "CRIT · <Name>" for a Live Machine with a Verdict (with `verdict-crit`, the worst Reason follows: "CPU at 97.0 °C (crit at 95.0 °C)"), otherwise "No current Verdict from <Name>". A single Live Machine with the populated body's ok Verdict gave "ALL MACHINES OK". With an unreachable second Machine the lines were "OK · First" and "No current Verdict from Second"; "ALL MACHINES OK" did not appear. The line uses the saved Name, never the reported hostname.

Independence: with the second Machine unreachable, four observations across more than 20 s showed the first Machine fresh (`Updated 0 s ago` to `1 s ago`) while its `after`/`wait` requests continued; after `cpu-zero` it showed `CPU, 0%` and stayed `Live`. The second Machine kept `No Pulse yet`.

## The Machine screen

Press a panel. The screen shows the top lines, `All Machines`, the Name, the address, `Edit`, and the card: one group whose label starts `Live, <synthetic hostname>, Verdict, ok, CPU, 21%, CPU, last 30 min: …`, followed by `Machine details` further down (scroll). Unreachable with no earlier Pulse shows no card: "Unreachable", "No Pulse from <Name>. The Machine may be off…", "Next try in <n> s" and "VPN looks off".

The card, or `Machine details` after `scroll down 0.42`, opens the separate detail, which has `Back to Machine` and no `All Machines` or `Edit`. See [Machine detail](detail.md). Navigation is one level at a time: panel → card or `Machine details` → detail → `Back to Machine` (or the navigation bar's `Back`) → Machine screen → `All Machines` (or `Back`) → Dashboard. Across those transitions the Watcher's `after`/`wait` stream continued without an extra plain request, an early cancellation, a lost Cursor or a second stream: in a 71 s interval at font scale 1.35, and in held slices that completed normally at about 10 s each.

## Edit, rename and address change

From the Machine screen press `Edit`, `fill` the field, `Save`, `Done`.

- Rename the Machine: the Save's plain Test connection fell inside the unchanged held Watcher request; the panel kept its position and the top line used the new Name.
- Change Address to another absent `.invalid` name: `Unreachable`, "No Pulse from <Name>…", "Next try in <n> s", with none of the earlier measurements and no way into the detail. The other Machine stayed saved.
- Change Address back to the synthetic address and set Port to `7247`: `Live` with fresh values (CPU 21.0% in the detail), no duplicate. After a cold close and open the Dashboard held both Machines in their original order with their names and addresses.

## Removal

`Edit`, then `Remove machine`. The form shows inline "Remove <Name>?" and "Anshin stops watching it. Nothing changes on the Machine." with `Remove` and `Keep`.

- `Keep` dismisses the question, `Remove machine` returns, and `Done` and a cold reopen show both Machines.
- `Remove` on one of two: the Dashboard shows only the other, still `Live` and updating through its existing `after`/`wait` requests, with no extra plain request and no management call.
- `Remove` on the last: the empty Dashboard, still empty after a cold reopen. A 15.9 s observation after the pending handler finished showed no new request.

## Foreground lifecycle of a Test connection

With `delay` 30s set, `Save` shows "Saved. Asking <address> for a Pulse…". Pressing Home then, with the Activity stop proven as in [lifecycle](../references/lifecycle.md): the plain Test connection ended after 2.1 s, before its five-second timeout and near the stop (the Watcher's held request after 1.8 s), no request arrived for 14.2 s hidden, and the Machine stayed saved. On return the form showed `Save` enabled, no result and no "canceled" message; another Save edited the same Machine, leaving exactly two panels. Pressing `Done` while Asking returned to the Machine screen and kept the Machine; the Test ended at 1.6 s while the Watcher's held request ran on to its read timeout. A Watcher resumed after 14.3 s hidden showed `Stale` ("Last Sample 18 s ago. Not current.") until the first answer, then `Live`. Home reports COMMAND_FAILED when the launcher takes the foreground; that is the expected outcome, and the lifecycle events, not the command, prove the stop.

## Enlarged text, themes and system bars

At platformValue 1.35 (reported by the bound `settings text-size` preset) both panels, the form's three rejections with `Save` and `Cancel`, the removal confirmation with `Keep` and `Remove`, and the Machine screen and detail were readable with no overlap once scrolled. Restore 1.0 afterwards and read it back.

For each theme: `<bound> close io.github.fsrodrig.anshin`, `scripts/qa seed --run <run> --theme <key>`, `<bound> settings appearance light|dark`, open twice. Light appearance with the populated Machine and dark appearance with `cpu-null`, `battery-unavailable` and `verdict-null` showed a readable Dashboard, form, Machine screen and detail, legible Back, Home and Recents with the keyboard hidden, and both Machines surviving two opens. Appearance does not prove exact token values; the goldens do.

## Traps

- **Expired references.** The interactive snapshot's `@e<n>` references expire. Cause: the screen changes between the snapshot and the action. Fix: a fresh `snapshot -i` before every `press` and `fill`.
- **An unquoted selector.** `press text=Add machine` returned INVALID_ARGS. Cause: a value with a space needs quotes. Fix: `press 'text="Add machine"'`, as its hint says.
- **Truncated messages.** Long form messages arrive as `Text view` with a `preview:` prefix and `[truncated]`. Cause: the interactive snapshot carries only a preview of long text. Assert the prefix, and read the rest from a full snapshot or a full screenshot that stays private.
- **An unowned install on the chosen AVD.** `up` refused it. Cause: the helper never touches data it did not install. Fix: the dedicated Pixel 9 AVD in [environment](../references/environment.md); the original stayed untouched.
- **A rejection is not an empty log.** Cause: ongoing Watchers keep requesting. Compare with their continued `after`/`wait` stream and look for an extra plain request.
- **Per-case renames.** The failure cases rename the Machine each time. Cause: the persisted Name is the proof that the failure did not roll back the Save. Restore the Name before the next case.
- **Only one Machine on the routed endpoint.** Cause: a second entry with that address and port is a duplicate. Hand the route over as in Routing hazard, and restore it before moving on.
- **Removal empties the store.** Cause: removal is the point. Run it last, after every scenario that needs the saved Machines (the run did).
- **A host command died with exit 143** after a long capture or at a final transition. The UI session was unaffected. Continue from the screen as it stands; navigating or relaunching would discard retained detail.
- **Settling on a nearly-empty tree.** `Cancel` and cold opens reported "the app may still be loading". Fix: wait for a specific text before interacting.
- **The first `sys.boot_completed` read on a new AVD was empty.** Read it again until it is `1` before `up`.
- **The route probe is a hint.** Live in the app with the request in the access log is the proof; see [environment](../references/environment.md).
