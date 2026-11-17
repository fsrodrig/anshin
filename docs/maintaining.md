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
by hand. Every release carries the Agent archives, the signed APK and the AUR package. Until the owner has made the
signing key and set it up (§ The APK signing key), every run fails at **Sign the APK** and publishes nothing, which is
deliberate: no release ships without its APK.

### Environments

| Environment | Deployment branches | Protection | Holds |
|---|---|---|---|
| `release` | `main` only | required reviewer: the maintainer; self-review allowed, because the maintainer both starts and approves a release | nothing; its approval is the review moment, before anything is published |
| `apk-signing` | `main` only | none: signing publishes nothing, and the approval comes later, on `release` | the `ANDROID_KEYSTORE` (the keystore file, base64) and `ANDROID_KEYSTORE_PASSWORD` secrets |
| `aur` | `main` only | none: the release was already approved | the `AUR_SSH_PRIVATE_KEY` secret |

```sh
id=$(gh api user --jq .id)
gh api -X PUT repos/fsrodrig/anshin/environments/release --input - <<EOF
{"reviewers":[{"type":"User","id":$id}],"prevent_self_review":false,
 "deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
EOF
gh api -X POST repos/fsrodrig/anshin/environments/release/deployment-branch-policies -f name=main -f type=branch
gh api -X PUT repos/fsrodrig/anshin/environments/apk-signing --input - <<'EOF'
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
EOF
gh api -X POST repos/fsrodrig/anshin/environments/apk-signing/deployment-branch-policies -f name=main -f type=branch
gh api -X PUT repos/fsrodrig/anshin/environments/aur --input - <<'EOF'
{"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
EOF
gh api -X POST repos/fsrodrig/anshin/environments/aur/deployment-branch-policies -f name=main -f type=branch
```

Create `apk-signing` before any Release run. The first time a job names an environment that doesn't exist, GitHub
creates it with no protection at all, and the `sign` job names `apk-signing`. An environment made that way would let a
workflow on any branch read the keystore once it is set.

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

### The APK signing key

Every release APK is signed with one key. Android installs an update only over an APK signed with the same
certificate, so the key can never change without every user uninstalling the app and losing their saved Machines.
Make it once, offline, and do these in order:

1. Create the `apk-signing` environment (§ Environments).
2. In an empty temporary directory on a machine you trust, make the key. The password is generated, never typed on a
   command line, so it stays out of the process list and the shell history. Save it in a password manager before the
   next step:

   ```sh
   umask 077
   export KSPASS=$(head -c 33 /dev/urandom | base64 | tr -d '\n')
   keytool -genkeypair -keystore anshin-release.p12 -storetype PKCS12 -alias anshin -keyalg RSA -keysize 4096 \
     -validity 10000 -dname CN=Anshin -storepass:env KSPASS -keypass:env KSPASS
   ```

   The alias must be `anshin`: the signing script uses it. The certificate ships in every APK, so its subject names no
   person.
3. Get the certificate's fingerprint, the value apksigner prints as `certificate SHA-256 digest`:

   ```sh
   keytool -exportcert -keystore anshin-release.p12 -alias anshin -storepass:env KSPASS | sha256sum | cut -d ' ' -f 1
   ```

4. Back the key up before anything else uses it: two copies of `anshin-release.p12` on separate offline media, kept in
   different places, with the password stored apart from both. Test a restore from each copy: listing it with
   `keytool -list -keystore <copy> -storepass:env KSPASS` must show the `anshin` entry, and the step 3 command run on
   the copy must print the same fingerprint.
5. Open a pull request that puts the fingerprint, alone on one line, below the comments in
   [`deploy/android/release-certificate.sha256`](../deploy/android/release-certificate.sha256). The value is public.
   Once it merges, the Release workflow refuses any APK signed by another certificate.
6. Set the secrets, then delete the working copy of the key and end the shell that held `KSPASS`:

   ```sh
   base64 -w0 anshin-release.p12 | gh secret set ANDROID_KEYSTORE --env apk-signing
   printf '%s' "$KSPASS" | gh secret set ANDROID_KEYSTORE_PASSWORD --env apk-signing
   ```

7. Start the Release workflow with **dry_run**. It signs the APK and keeps no signed copy. In its summary, the
   `certificate sha256` must equal the pinned one, and `versionName` and `versionCode` must be the version you expect.

A lost key means no more updates: every user must uninstall Anshin, losing their Machines, and install an APK signed
with a new key. Rotating the key while keeping updates needs APK Signature Scheme v3.1 and the old key. A leaked key
can't be revoked, and whoever holds it can sign an APK Android accepts as an update. Replace it only through a reviewed
pull request that changes the pin, and say so in the release notes.

### Cutting a release

1. Start it from `main`: Actions → Release → Run workflow, or `gh workflow run release.yml --ref main`. Tick
   **dry_run** to build, check and test-install everything without publishing.
2. The run computes the version and notes from the pull request titles merged since the last tag, runs the repository, branding, Agent and
   Android checks through the same reusable workflows as pull requests, builds the APK at that version and signs it
   (a dry run signs too, but keeps no signed copy), builds the archives and test-installs the AUR package. Its summary
   shows the version, the notes, the signed APK's SHA-256 and certificate, the checksums, the rendered PKGBUILD and the
   `.SRCINFO`. "Nothing to release" means no `feat`, `fix` or breaking
   change has merged since the last tag.
3. Review them, then approve the `release` deployment ("Review deployments" on the run). Rejecting it publishes
   nothing.
4. The `publish` job checks the APK against the pinned certificate again, adds it to `checksums.txt`, attests the
   archives and the APK, creates a draft GitHub Release with every asset, and publishes it, which creates the
   `vX.Y.Z` tag at the commit the run started from.
5. The `aur` job pushes the PKGBUILD and `.SRCINFO` to the AUR.

### When a job fails

- Before approval: nothing was published. Fix it on `main` and start a new run.
- **Sign the APK**: the secrets are missing or wrong, or the certificate isn't the pinned one. Nothing was published.
  The job's error names what failed. Fix the `apk-signing` secrets, never the pin unless the key itself changed in a
  reviewed pull request, and start a new run. A run before § The APK signing key is done fails here by design.
- `publish`: re-run it. It needs approval again, reuses the draft it left, and never changes a published release.
- `aur`: it runs after `publish`, so its failure leaves the GitHub Release, the APK and the attestations as they were.
  Re-run that job alone ("Re-run failed jobs"). It pushes only what the AUR doesn't have yet. GitHub re-runs
  jobs for 30 days; after that, the next release's push brings the AUR up to date.
