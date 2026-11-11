# Developing Anshin

How to build and run both halves on an Arch Linux workstation. The design is in [`design.md`](design.md); the rules
for changes are in [`CONTRIBUTING.md`](../CONTRIBUTING.md).

## Tools

Tool versions are pinned in [`mise.toml`](../mise.toml). With [mise](https://mise.jdx.dev):

```sh
mise install
make setup   # installs the pre-commit identifier scan
make check   # the Done gate; CI runs the same targets
```

Without mise, install the same versions yourself. `go test -race` needs cgo, so a C compiler (`gcc`) must be on
`PATH`. The shipped Agent binaries are built with `CGO_ENABLED=0`.

## Agent

```sh
make check-agent   # gofmt, vet, staticcheck, test -race, linux/amd64 and linux/arm64 builds within the size budget
```

The binaries land in `agent/dist/` (ignored by git).

### Running it on your Machine

The Agent needs a running, logged-in `tailscaled`. It asks tailscaled who this Machine is, binds only the
Machine's tailnet IPv4 and IPv6 addresses, and exits non-zero if tailscaled can't answer. Run it as your own
unprivileged user. It needs no root, because the LocalAPI calls it makes (status and WhoIs) need no privilege.

```sh
cd agent && go run ./cmd/anshin-agent
```

| Variable | Default | Meaning |
|---|---|---|
| `ANSHIN_PORT` | `7247` | TCP port on the tailnet addresses |
| `TAILSCALE_SOCKET` | `/run/tailscale/tailscaled.sock` | tailscaled's LocalAPI socket |

From another of your own untagged devices on the tailnet, or from the Machine itself on its tailnet address:

```sh
curl "http://$(tailscale ip -4):7247/v1/pulse"
# {"schema":1,"kind":"full","hostname":"…","cpu_pct":21}
```

The Agent doesn't answer on loopback. A tagged device, another user's device or an address outside the tailnet
gets `403`. If tailscaled stops answering, requests get `503`.

### Layout

| Package | What it owns |
|---|---|
| `cmd/anshin-agent` | wiring and configuration |
| `internal/recorder` | measuring the Machine; returns `null` for anything unavailable |
| `internal/tailnet` | who this Machine is and who a caller is: the LocalAPI client and a fake |
| `internal/server` | the bind, the WhoIs truth table, the Pulse endpoint |

## Phone

The Android app arrives with the second half of the tracer bullet, along with how to get the Android SDK
command-line tools on Arch.
