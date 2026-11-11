# Anshin

Go agent (`agent/`) + native Kotlin/Compose Android app (`android/`) for watching Linux Machines over a tailnet. Monorepo; issues are vertical slices, tracer bullet first.

## Before changing anything

- Read [`docs/design.md`](docs/design.md) for the section you're touching. It is the source of truth for module shapes, the Pulse contract, measurement semantics and the security model.
- Use the words in [`GLOSSARY.md`](GLOSSARY.md) exactly, in code, UI copy, tests and issues (Machine, Agent, Sample, Window, Gap, Verdict, Pulse…). When a new domain term settles, add it there.
- Check [`docs/adr/`](docs/adr/) before proposing a different stack, auth model, history location, metrics library or contract format; those were deliberate. A change that contradicts an ADR needs a new ADR.
- Talk about architecture in the deep-module vocabulary: module, interface, depth, seam, adapter, leverage, locality. A seam earns its place only with two real adapters.

## Rules that are easy to break

- Unavailable measurements are `null`, end to end. A missing sensor, battery or mount never becomes `0`, `-1` or a guess.
- The API stays read-only. The Agent reads; it never executes, kills, writes or configures anything on the Machine.
- Authorization lives in one place (the server's WhoIs truth table) and every row is tested. The phone sends no credentials.
- The phone talks to an address only through the one OkHttp client with the tailnet destination check.
- Fixtures are scrubbed, reviewed captures. Commit no hostname, MAC, serial number, tailnet name or username, in file contents or file names.
- Every resource number in docs or PRs (RSS, CPU, APK size, startup) comes from a measurement you ran, with how you ran it.
