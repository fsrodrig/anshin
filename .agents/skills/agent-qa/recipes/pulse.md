# Pulse

> **Navigation changed after 7a0eb6f.** The steps below record what a device showed at the commits they name; they were not re-run after this change and need re-verification on a device. Until then, read their labels and routes through this:
>
> - Every screen has a bar. The Dashboard's bar holds the mark and `anshin`, then two icon buttons labelled `Add Machine` (+) and `Settings` (gear); an empty Dashboard also shows an `Add Machine` button. There is no `Add machine` button under the panels.
> - Every other screen's bar starts with a back arrow labelled `Back to <parent>`: `Back to Dashboard`, `Back to <Machine name>`, `Back to Settings`, `Back to Open-source licences`. It does what system Back does. `All Machines` is gone.
> - The Machine screen and the Machine detail are now one Machine screen, opened by pressing a panel. Its bar holds `Back to Dashboard`, the Machine name and an Edit icon labelled `Edit <Machine name>`; the state line (`Live · last Sample <age> ago`) is pinned under the bar; the list holds the address, the Stale or Unreachable notice, the Verdict, CPU, Memory, Storage, Battery, Temperatures, Network, and last a `Machine` section (hostname, uptime, Window, distro, kernel, architecture, Agent version). There is no card, no `Machine details`, no `Back to Machine` and no Overview section, and the top line shows only on the Dashboard.
> - The form's titles are `Add Machine` and `Edit Machine`, its first field is `Machine name` (was `Name`), and its removal button is `Remove Machine` (was `Remove machine`). Edit Machine opened from the Machine screen returns to it; a confirmed removal lands on the Dashboard. Settings has Theme, Machines and About, with `Version` and `Open-source licences` as rows in About; its Machines open Edit Machine and return to Settings.
> - The top line is in sentence case (`warn · laptop`, `All Machines ok`), and the Stale and Unreachable notice ends with `Nothing here is current`, without the age sentence (the age is pinned under the bar).

Verified on the installed release build with the reverse route. Follow [environment](../references/environment.md) for setup and ownership. The entry path and the states on the Dashboard and Machine screen were last observed at 0af9b99 (R8-minified release build, reverse route); rows marked as earlier come from a run on the previous Watch-field build.

## Entry path

Observed at 0af9b99, where Add machine replaced the Watch field. The labels, forms, Dashboard and rejections are in [MachineBook](machinebook.md).

1. `<bound> open io.github.fsrodrig.anshin`. A fresh store shows the empty Dashboard.
2. Press `Add machine`. `fill` `Name` and `Address` (the synthetic address from `up.json`), leave `Port` blank, press `Save`. Quote a selector that has a space: `press 'text="Add machine"'`.
3. Wait for the form's `Live`, then press `Done`: the Dashboard shows the Machine's panel.
4. Press the panel: the Machine screen, with `All Machines`, the Name, the address, `Edit`, the card (one group labelled `Live, <synthetic hostname>, Verdict, ok, CPU, 21%, …`) and `Machine details`. `All Machines` returns to the Dashboard.

Live with CPU 21% and a matching GET `/v1/pulse` 200 prove the route. A failed root-shell `routeProbe` alone does not disprove it.

## Card and detail transition prerequisites

Before each QA Agent setup change, name the starting surface and the next observation in the plan. Take a full snapshot for visible text and a fresh `snapshot -i` before selecting a control. Scroll with the bound command to reveal the needed label; use only a reference from the current interactive snapshot.

`PulseView` keeps detail mode for the same Machine across new Pulses and error responses. On the Machine screen only the card contains the `Live` label (the detail's header has one too); a Dashboard panel's label carries its state as well. A previous run observed detail navigation successfully, then stopped when valid-body recovery returned to detail and the runner waited for card-only `Live` twice. The earlier 2026-11-16 run observed the recovery sequence below after Unreachable, Forbidden and Incompatible: the retained detail returned with the new values, and `Back to Machine` showed the card's `Live`. The 0af9b99 run observed it again with the populated `machine-full` setup: from a detail opened independently before each failure, Unreachable (aged values, cause, `Next try`, `VPN looks off`), Forbidden and Incompatible each hid the values and charts, and the same detail returned with its values, charts and a fresh age when the setup was restored, with no Save and no navigation. Detail is hidden outside Live: Stale and Unreachable keep the aged values under their own banner, Forbidden and Incompatible show only the message, and detail returns to Live when Live does.

1. For a card observation, if detail is open, press the visible `Back to Machine` button using a fresh reference, then wait for `Live` and observe the planned card values. If an error hides detail, restore the plan's valid body first, wait for `Back to Machine`, then return to the card. A fresh valid card can wait for `Live` directly. For a Dashboard observation, leave the detail with `Back to Machine`, then the Machine screen with `All Machines`; never press `All Machines` from the detail, and note that leaving the Machine screen discards retained detail.
2. For a detail observation, enter through the card or its `Machine details` button only when the card is observed. If already in detail, keep it open. Wait for `Back to Machine` and the planned body-specific detail values, with scrolling as needed. The heading alone does not prove a new Pulse arrived.
3. Before each malformed-body transition, restore the plan's populated body and observe its exact detail values. From a card, open detail first; from a preceding error with retained detail mode, wait for the detail oracle directly. Only then select the malformed body, wait for `Incompatible`, and record the explanation and absence of old values. Repeat the populated-detail prerequisite independently for every malformed body.

Use text oracles and matching access-log requests for network changes, rather than fixed sleeps.

## Observed states

The earlier 2026-11-16 run, on the Watch-field release build at the default theme and actual font scales 1.0 and 1.35, observed these states and readable labels. In the 0af9b99 build the top lines sit above the panels, and on the Machine screen above `All Machines`, with one line per Machine. Where a Machine has no current Verdict (every state but Live with a Verdict), its line reads `No current Verdict from <Machine>`; the 0af9b99 run observed that line for Connecting, Stale, Unreachable, Forbidden and Incompatible.

| Setup capability | Expected screen |
| --- | --- |
| Fresh store | 0af9b99: the empty Dashboard, "No Machines yet", "Add a Machine running the Anshin Agent by its MagicDNS name or tailnet IP." and `Add machine`, with no top line and no request. (Earlier Watch-field build: Idle, Machine field and disabled Watch.) |
| Malformed address | 0af9b99: `[::1` in `Address` is rejected in the Add machine form with "Not a Machine address. Enter a MagicDNS name or a tailnet IP, without http:// or a path." under the field. Nothing is saved and no request is made; Cancel and a cold reopen leave the Dashboard empty. (Earlier: Not a Machine address in the Watch flow.) |
| `delay` 3s on the first request | Connecting. Panel: `Connecting, <address>, Asking for its first Pulse`; detail: "Asking <Machine> for its Pulse." (0af9b99; the second also after the Activity was recreated, see [Machine detail](detail.md)) |
| `whois-allow`, `cpu-ordinary`, `hostname-synthetic` | Live, CPU 21% |
| `cpu-null`, `hostname-null`, `verdict-null` | Live, CPU and Verdict unavailable (0af9b99, with `hostname-synthetic`). Machine label unavailable: earlier run |
| `samples-frozen` | Live first, then Stale once the data is 10 s old. 0af9b99: panel `Stale, <address>, Last Sample 11 s ago. Not current.` with the last values; detail `Stale`, "Last Sample <age> ago. Nothing here is current." (14 s, then 21 s), "Last Verdict" with "ok, <age> ago", and a CPU chart that says "last", never "now". No cause sentence, no `Next try`, no `VPN looks off`, no current Verdict. The age keeps growing through empty 200 answers |
| `stop-start` stop | Unreachable once the data is 10 s old, with the old values kept as history. 0af9b99: panel `Unreachable, <address>, Last Sample 21 s ago. Not current.`; detail `Unreachable` with "No Pulse from <Machine>. The Machine may be off…", "Next try in <n> s" (3 s, then 11 s), "VPN looks off", "Last Sample <age> ago. Nothing here is current.", "Last Verdict" with "ok, <age> ago", and a chart that says "last" |
| `stop-start` stop from a fresh watch | The same Unreachable screen with no history (earlier run; at 0af9b99 an address edit to an absent name gave Unreachable with no earlier values) |
| `whois-deny` | Forbidden, "This Agent only answers its Owner's untagged devices." Old values and detail are gone; the panel reads `Forbidden, <address>, Only answers its Owner's untagged devices`. The retry cadence was timed in the earlier run only: about once every 30 to 60 s, without rapid polls (37 s) |
| `raw-file` schema-2 fixture | Incompatible, "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent." The panel reads `Incompatible, <address>, Update Anshin and the Agent`; old values are gone. Asked again slowly (56 s in the earlier run); the next request has no `after` |
| `whois-localapi-error` | Unreachable, "The Agent on <Machine> can't ask tailscaled who this phone is." (0af9b99 observed the Test connection form, "Saved, but …"; the Machine screen wording is from the earlier run) |
| `http-status` 500 | Unreachable, "<Machine> answered HTTP 500 instead of a Pulse." (the same split) |
| `delay` 6s on the first request | Connecting, then Unreachable at the 5 s read timeout with the "No Pulse from <Machine>" cause, never Incompatible (earlier run) |
| Documentation address outside the tailnet | 0af9b99: the Add machine form rejects `192.0.2.1` with "192.0.2.1 isn't a tailnet address. Tailnet IPs are in 100.64.0.0/10 or fd7a:115c:a1e0::/48." under `Address`. Nothing is saved and no request is made. (Earlier Watch-field build: Unreachable, "<address> isn't a tailnet address, so Anshin won't connect to it.") |

The 0af9b99 run did not re-observe the `stop-start` stop from a fresh watch, `delay` 6s, `hostname-null`, the retry intervals for Forbidden and Incompatible, or the Machine screen wording for `whois-localapi-error` and `http-status` 500, so those rows keep the earlier run's values. The non-JSON `raw-file` body was observed there only through Test connection (Incompatible, "Saved, but the Agent answered in a version of…").

Every Unreachable screen showed "VPN looks off" with "This phone shows no active VPN. If Tailscale is off, turn it on; Anshin keeps trying either way." because the emulator has no VPN. The hint disappears when Live returns, and the traffic never waited for it.

Recovery needs no Save or new entry: the 0af9b99 run restored every failure through the Watcher's normal retry, in the same detail where one was open. Removing a failure setup returned Live on the next request for Unreachable, and on the next slow retry for Forbidden and Incompatible (47 s and 52 s from the setup change in the earlier run). Allow up to 75 s for those two.

`cpu-zero` showed 0%, distinct from unavailable. All four themes showed ordinary and unavailable values and survived a cold relaunch after seeding; at 0af9b99 that covered the Dashboard, Machine screen, Add machine form and detail, with light appearance for the populated Machine, dark for `cpu-null`, legible system bars, and both saved Machines surviving two opens. See the environment reference for seed and settings commands. Full snapshots and screenshots are needed for mixed-span result text. Device appearance does not prove exact token values, font identity or contrast; use the corresponding tests. The refusal screen alone does not prove a device-wide connection boundary. The device route fallback was exercised in neither run.
