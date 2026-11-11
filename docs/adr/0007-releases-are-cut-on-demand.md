# Releases are cut on demand from main; the tag is an output

Development is trunk-based: short-lived branches are squash-merged into `main`, and each pull request title is a Conventional Commit that becomes both the commit on `main` and its release-note line. Releases do not follow every merge. The maintainer starts the `Release` workflow on `main` when a release is worth shipping. It computes the next SemVer version and Keep a Changelog notes with git-cliff, runs the same checks as a pull request on that exact commit, builds the Agent archives, the signed APK and a test-installed AUR package, and publishes a draft GitHub Release with every asset. Publishing that draft is what creates the `vX.Y.Z` tag. The AUR push comes last, because its sources are the published assets, and can be retried on its own. Nobody tags or releases by hand. Before 1.0, `feat` and breaking changes bump the minor version and `fix` bumps the patch.

The workflow is built in the release issues; until then nothing is released.

## Considered Options

- **Release on every merge (as in the maintainer's omarca project)**: right for a web app that deploys every merge anyway, but here every release reaches Obtainium users as an APK update and AUR users as a package bump. Slices often land the Agent half before the phone half, so per-merge releases would ship half-finished features and identical APKs.
- **release-please**: its release PR gives a review moment, but it needs a bot token so CI runs on that PR, it commits version files back to `main`, and its notes differ from git-cliff's. The `release` environment's approval gives the same review moment, with the version and notes in the job summary.
- **A tag pushed by hand, triggering the build**: a tag would exist before the artifacts did, and a failed build would leave a published tag with nothing behind it. A tag made with the workflow's own token also does not start other workflows.
- **A committed `CHANGELOG.md`**: writing back to a protected `main` needs a bypass or a release PR. The notes live on each GitHub Release, which is also what Obtainium shows.
