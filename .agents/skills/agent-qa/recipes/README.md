# QA recipes

Recipes hold what a previous run already paid to learn: the shortest path to a surface, its labels, the setup, the expected observation, and the traps on the way. A recipe lands only after a run used it successfully.

## Index

- [Pulse](pulse.md): current Pulse states, themes and system bars against the QA Agent.

## Capability gaps

What a run cannot prove today. Plan the affected behavior as required anyway when the spec requires it, name the gap, and accept an INCONCLUSIVE device result, or move the proof outside QA when a test proves it.

- **Real WhoIs and the authorization truth table.** The QA Agent's fake tailnet answers. The `server` tests prove every row.
- **The Agent's bind, `IP_FREEBIND` and the systemd sandbox.** Proven on a real Machine under the shipped unit.
- **MagicDNS, IPv6 and carrier networks.** The emulator reaches the QA Agent at one synthetic IPv4 address.
- **No connection to an outside address, device-wide.** The access log sees only the QA Agent. The refusal is observed in the UI; the connection boundary is `DestinationCheckTest` and `ProxyBypassTest`. A criterion that demands a device-wide capture stays unproven.
- **Live theme switching.** The app has no theme control before #14. Seeds prove restoration after a cold start; `ThemeSwitchTest` proves live switching.
- **Exact font scale 1.3.** Android's text-size presets give 1.24 and 1.35. Unless Android's Settings UI sets and reads back 1.3, the goldens prove 1.3 and the device checks its own preset, labeled with its real value.
- **Contract fixtures.** `contract/fixtures` holds no corpus on this branch. The raw responder replays any file's exact bytes, which is the mechanism for fixtures once they exist; it never proves the production encoder.
- **Budgets.** An emulator number is never an RSS, CPU or startup budget figure.
- **An Obtainium update over a previously signed release.** Each run signs with a fresh throwaway key; updates are tested in the release process.
- **Unverified route:** the in-emulator QA Agent route was not exercised in the first run. `scripts/qa doctor` lists host-dependent setup as `unverified`.

## Writing a recipe

- One file per surface, with a line in the index above saying when to read it.
- Lead with the shortest path that works: labels, bound agent-device commands, `scripts/qa` setup by capability ID, the expected observation. Then the traps, each with its cause.
- Write in capability language: `<run>`, `<bound>`, the placeholder addresses from `references/environment.md`; never a machine path, a hostname or a real address.
