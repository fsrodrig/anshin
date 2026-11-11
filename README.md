# anshin

**Your systems. At peace.**

Anshin (安心, "peace of mind") lets you watch the health of your own Linux machines from your Android phone: CPU, memory, disks, battery and temperatures, privately, over your Tailscale tailnet. No accounts, no cloud, no telemetry, no tokens.

- A tiny Go **agent** runs on each Linux machine, samples it every 2 s, and keeps the last 30 minutes in memory.
- The native Android **app** connects straight to each agent over the tailnet, only while you're looking at it.
- Access is decided by your tailnet identity: only your own devices get in.

> **Status: pre-MVP.** The branding direction is selected; app and Agent implementation have not started. Work is tracked in [issues](../../issues), starting with the tracer bullet.

## Read next

- [Design](docs/design.md): how it's built
- [Branding](branding/README.md): selected logo, palette and app layouts
- [Glossary](GLOSSARY.md): the words we use
- [Decisions](docs/adr/): why it's built this way
- [Original brief](docs/brief.md): where it started

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Report security problems privately, as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
