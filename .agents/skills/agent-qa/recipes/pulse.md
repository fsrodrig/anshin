# Pulse

Verified on the installed release build with the reverse route. Follow [environment](../references/environment.md) for setup and ownership. Open the app with `<bound> open io.github.fsrodrig.anshin`, enter the synthetic target from `up.json`, and tap Watch. Live with CPU 21% and a matching GET `/v1/pulse` 200 prove the route. A failed root-shell `routeProbe` alone does not disprove it.

## Card and detail transition prerequisites

Before each QA Agent setup change, name the starting surface and the next observation in the plan. Take a full snapshot for visible text and a fresh `snapshot -i` before selecting a control. Scroll with the bound command to reveal the needed label; use only a reference from the current interactive snapshot.

`PulseView` keeps detail mode for the same Machine across new Pulses and error responses. Only the card contains the `Live` label. A previous run observed detail navigation successfully, then stopped when valid-body recovery returned to detail and the runner waited for card-only `Live` twice. The recovery sequence below follows that state handling in source; it still needs a fresh device run.

1. For a card observation, if detail is open, press the visible `Back to Machine` button using a fresh reference, then wait for `Live` and observe the planned card values. If an error hides detail, restore the plan's valid body first, wait for `Back to Machine`, then return to the card. A fresh valid card can wait for `Live` directly.
2. For a detail observation, enter through the card's `Machine details` button only when the card is observed. If already in detail, keep it open. Wait for `Back to Machine` and the planned body-specific detail values, with scrolling as needed. The heading alone does not prove a new Pulse arrived.
3. Before each malformed-body transition, restore the plan's populated body and observe its exact detail values. From a card, open detail first; from a preceding error with retained detail mode, wait for the detail oracle directly. Only then select the malformed body, wait for `Incompatible`, and record the explanation and absence of old values. Repeat the populated-detail prerequisite independently for every malformed body.

Use text oracles and matching access-log requests for network changes, rather than fixed sleeps. A `Live` label left from the preceding body is insufficient. Keep the device error, advised retry and stop rules in the environment reference; this prerequisite is applied before a wait, not as another recovery attempt after its retry fails.

## Observed states

At default theme and actual font scales 1.0 and 1.35, the run observed these states and readable labels:

| Setup capability | Expected screen |
| --- | --- |
| Fresh store | Idle, Machine field and disabled Watch |
| Malformed address | Not a Machine address |
| `delay` | Connecting before response |
| `whois-allow`, `cpu-ordinary`, `hostname-synthetic` | Live, CPU 21% |
| `cpu-null`, `hostname-null` | Live, both values unavailable |
| `whois-deny` | Forbidden |
| `raw-file` schema-2 fixture | Incompatible |
| `stop-start` stop | Unreachable |
| Documentation address outside the tailnet | Unreachable, refusal explanation |
| `whois-localapi-error` | Agent can't reach tailscaled |
| `http-status` 500 | No Pulse, HTTP 500 |

`cpu-zero` showed 0%, distinct from unavailable. The other three themes each showed ordinary and unavailable values and survived a cold relaunch after seeding; seeded default survived too. All four themes retained legible system bars in light and dark appearance. See the environment reference for seed and settings commands. Full snapshots and screenshots are needed for mixed-span result text. Device appearance does not prove exact token values, font identity or contrast; use the corresponding tests. The refusal screen alone does not prove a device-wide connection boundary. The device route fallback was not exercised in this run.
