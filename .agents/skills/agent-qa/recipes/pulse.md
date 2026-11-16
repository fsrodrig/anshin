# Pulse

> **MachineBook update:** The entry path and the Fresh store, Malformed address and Documentation address outside the tailnet rows below predate MachineBook and are superseded until a run re-verifies them.
>
> Current entry path, **not yet verified by a run**: Add machine → Name, Address, Port → Save → Done → tap the Machine panel. See [the phone flow in docs/develop.md](../../../../docs/develop.md#installing-the-debug-apk-on-your-phone).
>
> Wherever this recipe names the Watch button or the Machine field as a label, the Machine screen's `Edit` and `All Machines` stand in for them, also not yet verified by a run. Back from the detail returns to the Machine screen, and Back from there to the Dashboard.

Verified on the installed release build with the reverse route. Follow [environment](../references/environment.md) for setup and ownership. Open the app with `<bound> open io.github.fsrodrig.anshin`, enter the synthetic target from `up.json`, and tap Watch. Live with CPU 21% and a matching GET `/v1/pulse` 200 prove the route. A failed root-shell `routeProbe` alone does not disprove it.

## Card and detail transition prerequisites

Before each QA Agent setup change, name the starting surface and the next observation in the plan. Take a full snapshot for visible text and a fresh `snapshot -i` before selecting a control. Scroll with the bound command to reveal the needed label; use only a reference from the current interactive snapshot.

The card/detail transition changed in #15: the detail is its own screen. `Live` also appears in its header, so it no longer identifies the card. `Machine details` or `Watch` identify the card; `Back to Machine` identifies the detail. The verified transition is in [Machine detail](detail.md).

On #40's build, before #15 merged, `PulseView` kept detail mode for the same Machine across new Pulses and error responses, and only the card contained the `Live` label. A previous run observed detail navigation successfully, then stopped when valid-body recovery returned to detail and the runner waited for card-only `Live` twice. The 2026-11-16 run observed this recovery after Unreachable, Forbidden and Incompatible: the retained detail returned with the new values, and `Back to Machine` showed the card's `Live`. That build hid detail outside Live.

Since #15 the detail stays open in every state: Stale and Unreachable with data show the last values aged under the state word, and Connecting, Forbidden, Incompatible and Unreachable before any Pulse show the card's message under `Back to Machine`. A run at 9dba515 observed every one of these on the release build, and the steps below follow it.

1. For a card observation, if detail is open, press the visible `Back to Machine` button using a fresh reference, then wait for `Watch` and observe the planned card values. A fresh valid card can wait for `Live` directly.
2. For a detail observation, enter through the card's `Machine details` button (Live) or a tap on the card (Live, Stale, or Unreachable with history, all three observed) only when the card is observed. If already in detail, keep it open. Wait for `Back to Machine` and the planned body-specific detail values, with scrolling as needed. The heading alone does not prove a new Pulse arrived.
3. Before each malformed-body transition, restore the plan's populated body and observe its exact detail values. From a card, open detail first; from a preceding error inside the detail, wait for the detail oracle directly. Only then select the malformed body, wait for `Incompatible`, and record the explanation and absence of old values. Repeat the populated-detail prerequisite independently for every malformed body.

Use text oracles and matching access-log requests for network changes, rather than fixed sleeps. A `Live` label left from the preceding body is insufficient. Keep the device error, advised retry and stop rules in the environment reference; this prerequisite is applied before a wait, not as another recovery attempt after its retry fails.

## Observed states

The 2026-11-16 run, on the release build at the default theme and actual font scales 1.0 and 1.35, observed these states and readable labels. The heading is the first line under the Watch button. Where the Machine has no current Verdict (every state but Live with a Verdict), the top line reads `No current Verdict from <Machine>`.

| Setup capability | Expected screen |
| --- | --- |
| Fresh store | **Superseded:** previously Idle, Machine field and disabled Watch. Current expectation, not yet verified by a run: empty Dashboard, "No Machines yet" and Add machine. |
| Malformed address | **Superseded:** previously Not a Machine address in the Watch flow. Current expectation, not yet verified by a run: rejected in the form with no request. |
| `delay` 3s on the first request | Connecting, "Asking <Machine> for its Pulse.", before the first answer |
| `whois-allow`, `cpu-ordinary`, `hostname-synthetic` | Live, CPU 21% |
| `cpu-null`, `hostname-null`, `verdict-null` | Live, CPU, Machine label and Verdict unavailable |
| `samples-frozen` | Live first, then Stale once the data is 10 s old: "Last Sample <age> ago. Nothing here is current.", "Last Verdict" with "ok, <age> ago", and a CPU chart that says "last", never "now". The age keeps growing through empty 200 answers. No `Machine details` button, even scrolled to the card's end, and `No current Verdict`; since #15 a tap on the card opens the detail, Stale with its age |
| `stop-start` stop | Unreachable once the data is 10 s old, with the old values kept as history. Cause: "No Pulse from <Machine>. The Machine may be off, its Agent stopped, or this phone off the tailnet." Then "Next try in <duration>" or "Trying again" |
| `stop-start` stop from a fresh watch | The same Unreachable screen with no history |
| `whois-deny` | Forbidden, "This Agent only answers its Owner's untagged devices." Old values and detail are gone. Asked again about once every 30 to 60 s, without rapid polls (37 s in this run) |
| `raw-file` schema-2 fixture | Incompatible, "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent." Asked again slowly (56 s in this run); the next request has no `after` |
| `whois-localapi-error` | Unreachable, "The Agent on <Machine> can't ask tailscaled who this phone is." |
| `http-status` 500 | Unreachable, "<Machine> answered HTTP 500 instead of a Pulse." |
| `delay` 6s on the first request | Connecting, then Unreachable at the 5 s read timeout with the "No Pulse from <Machine>" cause, never Incompatible |
| Documentation address outside the tailnet | **Superseded:** previously Unreachable, "<address> isn't a tailnet address, so Anshin won't connect to it." Nothing reached the QA Agent. Current expectation, not yet verified by a run: rejected in the form with no request. |

Every Unreachable screen showed "VPN looks off" with "This phone shows no active VPN. If Tailscale is off, turn it on; Anshin keeps trying either way." because the emulator has no VPN. The hint disappears when Live returns, and the traffic never waited for it.

Recovery needs no Watch. Removing a failure setup returned Live on the next request for Unreachable, and on the next slow retry for Forbidden and Incompatible (47 s and 52 s from the setup change here). Allow up to 75 s for those two.

`cpu-zero` showed 0%, distinct from unavailable. The other three themes each showed ordinary and unavailable values and survived a cold relaunch after seeding; seeded default survived too. All four themes retained legible system bars in light and dark appearance. See the environment reference for seed and settings commands. Full snapshots and screenshots are needed for mixed-span result text. Device appearance does not prove exact token values, font identity or contrast; use the corresponding tests. The refusal screen alone does not prove a device-wide connection boundary. The device route fallback was not exercised in this run.
