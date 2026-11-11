# Tailnet identity instead of tokens; direct HTTP inside WireGuard

There are no bearer tokens, passwords or client certificates. The agent binds only its own Tailscale addresses and asks the local tailscaled who is calling (LocalAPI WhoIs on the caller's IP) on every request, allowing the caller only when it belongs to the same tailnet user and neither the caller nor the agent's machine is tagged; anything else, including WhoIs "not found" or LocalAPI errors, is denied. WireGuard already authenticates every peer, so a token would be a second secret to generate, copy, store and rotate that protects against almost nothing, and if the bind were ever wrong a token check would fail open to the LAN where WhoIs fails closed.

## Considered Options

- **Bearer token per agent (the original brief)**: rejected as above.
- **Tailscale Serve HTTPS in front of a private Unix socket**: real TLS and no cleartext on Android, but needs tailnet HTTPS enabled, root `tailscale serve` on every machine, Tailscale ≥ 1.104.4, and publishes machine names in public Certificate Transparency logs. Kept as a possible later second exposure mode.
- **tsnet (agent as its own tailnet node)**: duplicates the node, its keys and a network stack on hosts that already run tailscaled.

## Consequences

- The phone speaks plain HTTP. Cleartext is allowed app-wide, and the app's single HTTP client refuses any connection whose address is outside `100.64.0.0/10` or `fd7a:115c:a1e0::/48`, and never follows redirects. Because the phone sends no credential, reaching an impostor at a tailnet-range address leaks nothing.
- Identity is per device, not per app: another app on the owner's phone could read the metrics. Accepted for read-only data; device-key pairing is the later upgrade.
- Tagged machines are denied in the MVP. Grants via app capabilities and an explicit allowlist come later.
