# The Agent holds the Window; one cursor-based Pulse endpoint

Each Agent samples its Machine every 2 s and keeps the last 30 minutes in a fixed in-memory ring (~60 KB, never written to disk), and the phone keeps no history of its own. The brief put history on the phone, but a foreground-only app then opens to empty charts every time, which breaks the "understand your machines in three seconds" promise. Because the Agent holds the history, the API is a single `GET /v1/pulse?after=<epoch.seq>&wait=<s>`: no cursor (or an evicted, foreign or future one) returns the whole Window as `kind: "full"`, a valid cursor returns only newer Samples as `kind: "delta"`, and `wait` optionally holds the request until the next Sample.

## Considered Options

- **Phone-only in-memory history, three endpoints polled every 5 s (the brief)**: empty charts on open, three round trips to first paint, up to 10 s of staleness from 5 s sampling aliased with 5 s polling.
- **Server-Sent Events**: fine, but needs a streaming client and reconnect handling for no gain at this cadence; the cursor already maps to `Last-Event-ID` if it is ever added.
- **Netdata-style disk tiers or SQLite**: disk writes and a database for a 30-minute need.
