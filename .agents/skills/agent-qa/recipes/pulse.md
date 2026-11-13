# Pulse

Verified on the installed release build with the reverse route. Follow [environment](../references/environment.md) for setup and ownership. Open the app with `<bound> open io.github.fsrodrig.anshin`, enter the synthetic target from `up.json`, and tap Watch. Live with CPU 21% and a matching GET `/v1/pulse` 200 prove the route. A failed root-shell `routeProbe` alone does not disprove it.

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
