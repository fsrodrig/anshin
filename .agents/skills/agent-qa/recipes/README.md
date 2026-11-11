# QA recipes

Recipes hold what a previous run already paid to learn: how to reach a surface, set it up, assert on it, and the traps on the way. There are none yet; the first lands with the tracer bullet's add-Machine-to-Pulse scenario.

## Capability gaps

What a run cannot prove. Plan such scenarios as optional and name the gap, or keep them required and accept an INCONCLUSIVE run.

- **Real WhoIs and the authorization truth table.** The QA Agent's fake tailnet adapter decides. The `server` tests prove the table.
- **The Agent's bind, `IP_FREEBIND` and the systemd sandbox.** Proven on a real Machine under the shipped unit.
- **MagicDNS, IPv6 and carrier networks.** The emulator reaches the QA Agent at one IPv4 address.
- **Budgets.** An emulator number is never an RSS, CPU or startup budget figure; those are measured on the Owner's phone and Machines.
- **An Obtainium update over a previously signed release.** Tested in the release process.

## Routing spike (tracer bullet)

The app refuses every address outside the tailnet ranges, correctly, so the emulator must reach the QA Agent at an address inside `100.64.0.0/10`. Never widen the destination check to make this work. Both routes need `adb root`, so a non-Play emulator image. Try in order:

1. An `OUTPUT` DNAT rule in the emulator from a `100.64.0.x` address to `127.0.0.1:<port>`, plus `adb reverse` to the QA Agent on the host's loopback. A plain DNAT to `10.0.2.2` is reported to hit emulator routing quirks.
2. The static QA Agent run inside the emulator, bound to a `100.64.0.x` address added to `lo`. This also keeps the host's hostname and mounts off the screen.

Do not point a `100.64.0.x` address at the host itself: the host is a tailnet node, and Tailscale's firewall drops tailnet-range traffic that doesn't arrive on `tailscale0`.

If neither route works, the fallback is a QA build with a fake `AgentLink`, recorded as a gap: it can't show the HTTP client, the destination check or the access log.

## Writing a recipe

- One file per surface, with a line in an index here saying when to read it.
- Lead with the shortest path that works: the labels, the agent-device commands, the QA Agent setup, the expected observation. Then the traps, each with its cause.
- Write in capability language: `<run>`, `<port>`, `<serial>`, never a machine path, a hostname or a real address.
