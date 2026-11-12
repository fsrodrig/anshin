# The Agent holds the Window; one cursor-based Pulse endpoint

Each Agent samples its Machine every 2 s and keeps the last 30 minutes in a fixed in-memory ring (~60 KB, never written to disk), and the phone keeps no history of its own. The brief put history on the phone, but a foreground-only app then opens to empty charts every time, which breaks the "understand your machines in three seconds" promise. Because the Agent holds the history, the API is a single `GET /v1/pulse?after=<epoch.seq>&wait=<s>`: no cursor (or an evicted, foreign or future one) returns the whole Window as `kind: "full"`, a valid cursor returns only newer Samples as `kind: "delta"`, and `wait` optionally holds the request until the next Sample.

## Considered Options

- **Phone-only in-memory history, three endpoints polled every 5 s (the brief)**: empty charts on open, three round trips to first paint, up to 10 s of staleness from 5 s sampling aliased with 5 s polling.
- **Server-Sent Events**: fine, but needs a streaming client and reconnect handling for no gain at this cadence; the cursor already maps to `Last-Event-ID` if it is ever added.
- **Netdata-style disk tiers or SQLite**: disk writes and a database for a 30-minute need.
- **Agentless over SSH (as omarchy-argus does)**: the phone streams a sampler script to `bash -s`, so nothing is installed on the Machine. Rejected. A foreground-only phone samples nothing while it isn't looking, so charts open empty, as in the brief's design. The phone would also hold a credential that can run commands on every Machine (an SSH key, or Tailscale SSH access), which breaks [ADR 0002](0002-tailnet-identity-no-tokens.md) and the read-only API. Restricting SSH to a forced sampler command needs setup on each Machine, which is an install anyway.
