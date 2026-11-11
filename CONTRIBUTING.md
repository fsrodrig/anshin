# Contributing

Thanks for helping. Anshin is small on purpose; these notes keep it that way.

## Before you start

- Read the part of [the design](docs/design.md) you're touching, and the [ADRs](docs/adr/). A change that contradicts an ADR needs a new ADR in the same pull request.
- Use the words in the [glossary](GLOSSARY.md) exactly, in code, UI copy, tests and issues.
- For anything bigger than a fix, open an issue first. Work lands as small vertical slices.
- Security problems go through [SECURITY.md](SECURITY.md), never a public issue.

## Setup

Tool versions are pinned in [`mise.toml`](mise.toml). With [mise](https://mise.jdx.dev/):

```sh
mise install
make setup   # from the main checkout: installs the pre-commit identifier scan
```

Without mise, install the same versions yourself.

## Done

`make check` passes. CI runs the same targets, and a pull request merges only when they are green on a branch that is up to date with `main`.

## Pull requests

- Branch from `main` and open a pull request against it. Pull requests are squash-merged.
- The **title** is a [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/): `<type>[(<scope>)][!]: <subject>`. It becomes the commit on `main` and the line in the release notes, so describe the change a user would notice. `scripts/commit-title.sh` lists the allowed types and scopes; CI checks the title.
  - `feat` and `fix` appear in the release notes. `!` marks a breaking change.
  - The scope says which part changed: `agent`, `android`, `contract`, `deploy`, `brand`, `docs`, `ci`, `deps` or `security`.
- Commit messages inside the branch are up to you; only the title lands on `main`.

## Keep the repository free of identifiers

This repository is public. Commit no hostname, MAC address, serial number, tailnet name or address, or username, in file contents or file names.

- Use placeholders: `example.ts.net`, addresses in `100.64.0.0/24` (and `fd7a:115c:a1e0::1`), `00:00:00:00:00:00`, `/home/user`, and `SCRUBBED` for a serial number.
- Never paste raw `/proc` or `/sys` dumps into issues or fixtures. Fixtures are scrubbed captures, and a person reviews each one.
- `scripts/identifier-scan.sh` runs before each commit (after `make setup`) and in CI. It can't recognise every hostname, so also list your own hostnames, tailnet name and Linux username in `.git/info/identifiers` (never committed; the format is at the top of the script).

## Measurements

Every resource number in a pull request or the docs (memory, CPU, binary or APK size, startup time) comes from a measurement you ran, stated with how you ran it.

## License

Contributions are accepted under the repository's [MIT license](LICENSE).
