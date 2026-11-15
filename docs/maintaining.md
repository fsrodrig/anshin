# Maintaining Anshin

Settings that live on GitHub rather than in the code, so they can be reviewed here and reapplied. Commands assume `gh` signed in as the maintainer. § Releases is how a release is cut.

## Repository settings

| Setting | Value | Why |
|---|---|---|
| Merge methods | squash only | One commit per pull request on `main` |
| Auto-merge | on | A pull request can squash-merge automatically once its required checks pass |
| Squash commit title and message | pull request title, pull request body | The title is the checked Conventional Commit; without this, a one-commit pull request lands with its unchecked commit message |
| Delete branch on merge | on | Trunk-based: branches are short-lived |
| Secret scanning and push protection | on | |
| Private vulnerability reporting | on | The route [SECURITY.md](../SECURITY.md) points to |
| Dependabot alerts and security updates | on | Version updates are configured in `.github/dependabot.yml` |

```sh
gh api -X PATCH repos/fsrodrig/anshin \
  -F allow_auto_merge=true -F allow_squash_merge=true -F allow_merge_commit=false -F allow_rebase_merge=false \
  -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=PR_BODY \
  -F delete_branch_on_merge=true
gh api -X PUT repos/fsrodrig/anshin/private-vulnerability-reporting
gh api -X PUT repos/fsrodrig/anshin/vulnerability-alerts
gh api -X PUT repos/fsrodrig/anshin/automated-security-fixes
```

Enable automatic squash merging for an individual pull request with `gh pr merge <number> --auto --squash`. Enabling the repository setting alone does not enroll every pull request. The required checks and up-to-date branch rule still apply.

## The `main` ruleset

[`.github/rulesets/main.json`](../.github/rulesets/main.json) protects `main`: changes arrive only through squash-merged pull requests, after the `CI` and `PR title` checks from GitHub Actions pass on a branch that is up to date with `main`. Nobody can bypass it, the maintainer included. It asks for no approving review, because the only maintainer cannot approve their own pull request.

Create it only after the workflows that report those checks are on `main`; a required check that never reports blocks every merge.

```sh
gh api -X POST repos/fsrodrig/anshin/rulesets --input .github/rulesets/main.json
```

To change it, edit the JSON in a pull request and, once merged, apply it with `gh api -X PUT repos/fsrodrig/anshin/rulesets/<id> --input .github/rulesets/main.json` (`gh api repos/fsrodrig/anshin/rulesets` lists the id).

Commits are not required to be signed: unsigned commits on a pull request branch can block a squash merge even though GitHub signs the squash commit.

## Local setup

`mise install` and `make setup` (see [CONTRIBUTING.md](../CONTRIBUTING.md)); rerun `make setup` after a lefthook version bump, because the hook calls the installed lefthook binary. Put the maintainer's real hostnames, tailnet name and Linux username in `.git/info/identifiers`, so the pre-commit scan catches them too.

## Releases

Releases are cut on demand by the [Release workflow](../.github/workflows/release.yml)
([ADR 0007](adr/0007-releases-are-cut-on-demand.md)). Nobody tags, creates a GitHub Release or commits to the AUR
by hand. Until release signing lands, a release carries the Agent and its AUR package but no APK.

### Environments

| Environment | Deployment branches | Protection | Holds |
|---|---|---|---|
| `release` | `main` only | required reviewer: the maintainer; self-review allowed, because the maintainer both starts and approves a release | nothing; its approval is the review moment, before anything is published |
| `aur` | `main` only | none: the release was already approved | the `AUR_SSH_PRIVATE_KEY` secret |

```sh
id=$(gh api user --jq .id)
gh api -X PUT repos/fsrodrig/anshin/environments/release --input - <<EOF
{"reviewers":[{"type":"User","id":$id}],"prevent_self_review":false,
 "deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
EOF
gh api -X POST repos/fsrodrig/anshin/environments/release/deployment-branch-policies -f name=main -f type=branch
gh api -X PUT repos/fsrodrig/anshin/environments/aur --input - <<'EOF'
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
EOF
gh api -X POST repos/fsrodrig/anshin/environments/aur/deployment-branch-policies -f name=main -f type=branch
```

### The AUR account and key

1. Create an account at https://aur.archlinux.org/register.
2. In an empty temporary directory, make a key only the release uses:
   `ssh-keygen -t ed25519 -N '' -C anshin-release -f aur-release`.
3. Paste `aur-release.pub` under My Account → SSH Public Key on aur.archlinux.org.
4. `gh secret set AUR_SSH_PRIVATE_KEY --env aur < aur-release`, then delete both files. A lost key is replaced by
   repeating these steps.

The first push creates the `anshin-agent-bin` package. The AUR's host keys are pinned in
[`deploy/aur/known_hosts`](../deploy/aur/known_hosts), checked against the fingerprints aur.archlinux.org publishes
on its home page. If the AUR rotates them the push fails: compare the new keys with the published fingerprints, then
update the file in a pull request.

### Cutting a release

1. Start it from `main`: Actions → Release → Run workflow, or `gh workflow run release.yml --ref main`. Tick
   **dry_run** to build, check and test-install everything without publishing.
2. The run computes the version and notes from the pull request titles merged since the last tag, runs the repository, branding, Agent and
   Android checks through the same reusable workflows as pull requests, builds the archives and test-installs the AUR package. Its summary shows the version, the notes,
   the checksums, the rendered PKGBUILD and the `.SRCINFO`. "Nothing to release" means no `feat`, `fix` or breaking
   change has merged since the last tag.
3. Review them, then approve the `release` deployment ("Review deployments" on the run). Rejecting it publishes
   nothing.
4. The `publish` job attests the archives, creates a draft GitHub Release with every asset, and publishes it, which
   creates the `vX.Y.Z` tag at the commit the run started from.
5. The `aur` job pushes the PKGBUILD and `.SRCINFO` to the AUR.

### When a job fails

- Before approval: nothing was published. Fix it on `main` and start a new run.
- `publish`: re-run it. It needs approval again, reuses the draft it left, and never changes a published release.
- `aur`: re-run that job alone ("Re-run failed jobs"). It pushes only what the AUR doesn't have yet. GitHub re-runs
  jobs for 30 days; after that, the next release's push brings the AUR up to date.
