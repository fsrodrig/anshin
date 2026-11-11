# Maintaining Anshin

Settings that live on GitHub rather than in the code, so they can be reviewed here and reapplied. Commands assume `gh` signed in as the Owner. Release steps join this file with the release workflow.

## Repository settings

| Setting | Value | Why |
|---|---|---|
| Merge methods | squash only | One commit per pull request on `main` |
| Squash commit title and message | pull request title, pull request body | The title is the checked Conventional Commit; without this, a one-commit pull request lands with its unchecked commit message |
| Delete branch on merge | on | Trunk-based: branches are short-lived |
| Secret scanning and push protection | on | |
| Private vulnerability reporting | on | The route [SECURITY.md](../SECURITY.md) points to |
| Dependabot alerts and security updates | on | Version updates are configured in `.github/dependabot.yml` |

```sh
gh api -X PATCH repos/fsrodrig/anshin \
  -F allow_squash_merge=true -F allow_merge_commit=false -F allow_rebase_merge=false \
  -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=PR_BODY \
  -F delete_branch_on_merge=true
gh api -X PUT repos/fsrodrig/anshin/private-vulnerability-reporting
gh api -X PUT repos/fsrodrig/anshin/vulnerability-alerts
gh api -X PUT repos/fsrodrig/anshin/automated-security-fixes
```

## The `main` ruleset

[`.github/rulesets/main.json`](../.github/rulesets/main.json) protects `main`: changes arrive only through squash-merged pull requests, after the `CI` and `PR title` checks from GitHub Actions pass on a branch that is up to date with `main`. Nobody can bypass it, the Owner included. It asks for no approving review, because the only maintainer cannot approve their own pull request.

Create it only after the workflows that report those checks are on `main`; a required check that never reports blocks every merge.

```sh
gh api -X POST repos/fsrodrig/anshin/rulesets --input .github/rulesets/main.json
```

To change it, edit the JSON in a pull request and, once merged, apply it with `gh api -X PUT repos/fsrodrig/anshin/rulesets/<id> --input .github/rulesets/main.json` (`gh api repos/fsrodrig/anshin/rulesets` lists the id).

Commits are not required to be signed: unsigned commits on a pull request branch can block a squash merge even though GitHub signs the squash commit.

## Local setup

`mise install` and `make setup` from the main checkout (see [CONTRIBUTING.md](../CONTRIBUTING.md)). Put the Owner's real hostnames, tailnet name and Linux username in `.git/info/identifiers`, so the pre-commit scan catches them too.
