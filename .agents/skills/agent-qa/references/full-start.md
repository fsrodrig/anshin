# agent-qa under full-start

The QA stage contract between this skill and a `full-start` thread working on Anshin. The installed full-start profile points here, so this file, versioned with the skill, is the one place these rules change.

## Invocation

Invoke `agent-qa` from the checked-out repository with the thread's provider flag and `--no-post`, before the PR exists. The Router config's task codes apply unless the launch brief named a model or effort for the planner or runner; that choice wins for this whole invocation. No PR yet is a valid input: `scripts/qa init` without `--pr`.

The skill returns its run directory with `result.json`, private `report.md` and public-safe `pr-summary.md`, in every mode, including planning-only and NA. Branch on `result.json`, never on this skill's wording.

## Gates

Read `result`, `reasonCode` and `gates` from `result.json`:

| Outcome | What the thread does |
| --- | --- |
| PASS or NA, `gates.stageClean` true | Clean. Open the PR. Pending outsideQA rows (CI links) and pending manualCriteria go into the PR; they block merge readiness, not the PR. |
| FAIL | Back to implementation with the failed cells. After a material fix, run the stage again on the new head. |
| INCONCLUSIVE `environment` | A stop on the named blocker, with its evidence. Running again on the same unchanged environment rediscovers the same blocker; run again only after the blocker changed. |
| INCONCLUSIVE `missing_observation` | Run again for the missing cells if the reason was transient; otherwise a stop with the cells and reasons. |
| INCONCLUSIVE `no_runner` | Allowed only when `scripts/qa doctor` showed no device run is possible on this host. The PR may open with the summary's unexercised list; it is never evidence. When doctor passes, a device run is expected, and skipping it is not this case. |
| `gates.cleanup` not `ok` or `nothing-owned` | A stop, whatever the result: report each resource `down` left. |

The generic three-run cap limits real repair attempts; it never requires a rerun that cannot change the outcome.

## Proofs outside QA

The thread owns these, not the device run: `make check` at the tested SHA, the named tests' results, CI links once the PR exists, and the issue's manual confirmations (such as a reviewer's accent confirmation). Record them in `<run>/proofs.json` and run `scripts/qa finish` again; a missing proof calls for collecting that proof, not another device run. A pending or rejected manual confirmation stays visible and blocks merge readiness.

## Head pinning

`result.json` names `target.testedSha` and the APK digest. Re-read `HEAD` and the PR head before opening the PR and before posting. A result for another SHA is never presented as current: after app, manifest, R8, network, QA Agent or helper changes, run the stage again. After a docs-only change, the thread may carry the old result forward only by quoting its original tested SHA and the reviewed diff since.

## The PR body

End the PR body with `pr-summary.md` as its `## QA` section, after reading it and every medium you attach. Update that section when outsideQA, manual or head state changes. The private report and run directory never go on the PR.
