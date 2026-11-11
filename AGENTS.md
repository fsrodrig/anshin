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

## Git and releases

- Never push to `main`. Work on a branch, in a worktree under `worktrees/` (gitignored), and open a pull request. It is squash-merged once `CI` and `PR title` pass on a branch that is up to date with `main`.
- The PR title is a Conventional Commit with an optional scope from `scripts/commit-title.sh`. It becomes the commit on `main` and the release-note line, so name the change a user would notice. Commits inside the branch are free-form.
- Releases are cut on demand by the `Release` workflow ([ADR 0007](docs/adr/0007-releases-are-cut-on-demand.md)). Never create a tag, a GitHub Release or an AUR commit by hand. Never write a version number into source; the release sets it.
- Workflows pin every action by full commit SHA, default to no permissions, and never use `pull_request_target`. Release secrets live only in GitHub environments restricted to `main`; never reference them from a job that runs on pull requests.
- Repository settings and rulesets are documented in [`docs/maintaining.md`](docs/maintaining.md). Change them only when the user asks.

## Done

- Done means `make check` passes. It grows a target for each component as it lands; CI runs the same targets.
- Never make a check pass by weakening what it protects: no wider address range in a debug build, no flag in `anshin-agent` that swaps the tailnet adapter or the bind, no deleted truth-table row, no golden regenerated without reading its diff, no identifier-scan exception for a real identifier.

## QA

- When the user asks for QA or evidence of a change on a device, use the `agent-qa` skill. Until the tracer bullet lands its runner, it only plans.

## Agent assets

- Skills live in `.agents/skills/`; `.claude/skills` is a symlink to it, so add files under `.agents/`. Commit no machine path in a skill.
