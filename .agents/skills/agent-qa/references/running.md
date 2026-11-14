# Running a QA plan

The coordinator fills the brief below after planning and doctor succeeded and dispatches it to the one runner. The rest of this file is the runner's reference.

## Runner brief

```text
You are the QA runner for one Anshin change. Do this work yourself; dispatch no subagent. Execute the plan's
device setup, scenarios and owned cleanup, observe, and record what you saw. Fixing anything
is not your job.

Repository (read in place, never edit): <checkout>
Run directory (write only here): <run>
Plan: <run>/plan.md and <run>/plan.json
Device: <hostId and device id selected from device_list, prior boot/open state and owned AVD record>
Call device_open yourself. Save its returned bound command in <run>/device.json; you are the sole UI session owner.
Helper: <checkout>/scripts/qa, for `up`, `agent start|stop`, `seed` and `down`, always with --run <run>
Setup to perform: <route and QA Agent capability flags>; up writes <run>/up.json.

Read <checkout>/.agents/skills/agent-qa/references/running.md and
<checkout>/.agents/skills/agent-qa/references/environment.md, and any recipe in recipes/README.md that names your surface.

Write one <run>/observations/<scenario id>.json per scenario you reach. Reply with each scenario's result,
every cell you could not observe and why, any recording still running, and anything you changed on the
device that `scripts/qa down` would not undo.
```

## How to run

- Run every required scenario before any optional one; within a scenario, run every planned cell.
- Start the app with `<bound> open io.github.fsrodrig.anshin`. Use `--settle` on actions and `wait text "<expected>"` for anything the network delivers; the app polls every 2 s only while it is visible.
- Change the QA Agent only with the helper, by the capability flags the plan names. A setup the plan doesn't name is out of scope: record the cell INCONCLUSIVE with the reason.
- Record the actual theme, font multiplier (`platformValue`) and appearance of each cell, not the planned ones.
- A device-tool error: quote it, follow its advice once, then mark the affected cells INCONCLUSIVE and continue with the rest. Use no other route to the UI (no `adb shell input`, no other session).
- Complete the environment reference's owned cleanup before returning, even after a failed cell. Report cleanup errors and remaining resources; the coordinator does not take over your UI session.

A cell is **PASS** when you observed its expected result, **FAIL** when you observed something that contradicts it, **INCONCLUSIVE** when you could not observe it reliably, and **NOT_RUN** when you never reached it. A PASS or FAIL cites its evidence: a screenshot, a recording, an access-log file, or a saved `snapshot` output, each a path relative to `<run>`.

## Observation file

```json
{
  "id": "S2",
  "result": "PASS",
  "cells": [
    {
      "id": "S2.a",
      "result": "PASS",
      "theme": "tokyo-night",
      "fontMultiplier": "1.0",
      "appearance": "dark",
      "evidence": ["media/S2.a-live.png", "logs/access-3.jsonl"],
      "note": "Seeded tokyo-night; Live qa-machine 21% after close and reopen"
    }
  ],
  "note": "",
  "gaps": []
}
```

`scripts/qa finish` trusts the cells over the headline: a planned cell without a PASS keeps the scenario from passing, and evidence must exist inside the run.
