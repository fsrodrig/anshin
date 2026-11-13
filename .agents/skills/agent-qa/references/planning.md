# Planning a QA run

The coordinator fills the brief below and dispatches it to the planner. The rest of this file is the planner's reference; the review checklist at the end is the coordinator's.

## Planner brief

```text
You are the test planner for one Anshin change. Do this work yourself; dispatch no subagent. Design the plan a
runner will execute on an Android emulator against the installed release app and a synthetic QA Agent; do not
execute it, open a device or build anything.

Repository (read in place, never edit): <checkout>
Run directory (write only here): <run>
Caller: <the user's words, or the pipeline stage and what it needs from the result>
State: <target sha>; PR <number or "none yet">; reports available for this SHA: <make check, named tests, CI links, or "none">
Spec: <issue URL and acceptance criteria, design.md sections, ADRs>
Diff: <run>/focused.patch (code), <run>/changes.stat and <run>/changes.name-status (everything);
      <run>/run.json lists files left out of focused.patch: inspect any that matter by name.
Capabilities: <the JSON from `scripts/qa capabilities`>

Read <checkout>/.agents/skills/agent-qa/references/planning.md (rules and both file formats) and
<checkout>/.agents/skills/agent-qa/recipes/README.md (verified recipes and capability gaps).

Write <run>/plan.md and <run>/plan.json. Reply with the scenario list (id, title, required, cells), the
capability IDs each needs, the outsideQA and manual rows, and every gap and spec disagreement.
```

## Rules

Derive every expected result from the spec (the issue, its acceptance criteria, `docs/design.md`, the ADRs, `GLOSSARY.md`), never from the implementation. Where the code and the spec disagree, plan the spec and record a **spec disagreement**. Use the glossary's words: Machine, Agent, QA Agent, Pulse, Live, Unreachable, Forbidden, Incompatible.

Plan only what the app on this branch has. A screen, state or control the design describes but the code does not have yet (a Dashboard, a theme picker, Stale) is out of scope, not a gap.

### Sort each criterion first

- **Device scenario:** a person could see it in the installed app.
- **outsideQA:** its proof is a test, a CI check, a committed artifact or a document. Name the proof (test class, make target, file). It is never a scenario.
- **manualCriteria:** it needs a person's confirmation, such as a reviewer judging goldens. A passing test never stands in for it.

A criterion can be both: "tests show the theme survives a restart" is an outsideQA row for the test and a device scenario for the restart.

### Map the change

| Change | Surface to plan |
| --- | --- |
| The Pulse screen | its visible text and every state it renders, with exact labels; the accessibility tree from `snapshot -i` |
| Theme tokens, typography, color or font resources | the default theme on every state; each other theme on one representative state (Live) and its null rendering; font scale 1.0 and the device's enlarged preset in the default theme. Colors, contrast and golden equality are outsideQA (JVM tests and Roborazzi); a device adds the real release build, the bundled font and real font scaling, and is compared with the goldens for structure and legibility, never pixels |
| The theme setting | restoration after a cold start, through a seeded theme; live switching is outsideQA (`ThemeSwitchTest`) until the app has a control for it |
| `AgentLink` or the HTTP client | the QA Agent's access log: method, path, status and header names (no `Authorization` or `Cookie`), requests only while the app is visible; an address outside the tailnet shows the refusal in the UI, with `DestinationCheckTest` and `ProxyBypassTest` as outsideQA for the connection boundary |
| A measurement | null renders as unavailable, a real zero as `0%`, never swapped |
| The Pulse decoder, `contract/fixtures`, R8 rules, serialization, the manifest | the minified build decodes each affected body, replayed byte for byte with the raw responder; a body the app must reject shows Incompatible |
| System bars, window insets | each system appearance (light and dark), including after a relaunch |

### Write scenarios

- Number scenarios `S1`…`Sn` and cells `S1.a`…; a cell is one observation: state, theme, font scale, system appearance.
- Each expected result is concrete: visible text, a screen state, access-log lines, or their absence.
- Cite setups by capability ID from the brief's list, with the flags its entry gives. A setup not in the list is a capability gap, named once.
- A change to shared behavior gets a negative check: what must not happen.
- Unspecified behavior (an unknown stored key, a corrupt store) gets no scenario: list it under spec gaps with the test that covers it.
- Stop adding cells when the next one exercises the same code path; say why the rest add nothing.
- A required behavior the environment cannot reach stays required, with its gap. The run is then INCONCLUSIVE, which is the honest result.

Keep `plan.md` under 150 lines. A gap is one line: its ID, what it blocks, and the consequence; scenarios cite gap IDs instead of repeating it.

## plan.json

```json
{
  "schemaVersion": 1,
  "scope": "Theme engine: four themes, persisted choice",
  "scenarios": [
    {
      "id": "S1",
      "title": "Live Pulse in the default theme",
      "required": true,
      "capabilities": ["whois-allow", "cpu-ordinary"],
      "cells": [{"id": "S1.a", "state": "Live", "theme": "ink-and-bone", "fontScale": "1.0", "appearance": "dark"}],
      "gaps": [],
      "expected": "Live, qa-machine, CPU 21%, in Ink & bone's colors as the matching golden shows."
    }
  ],
  "outsideQA": [{"id": "P1", "criterion": "No color literal outside the theme tokens", "proof": "ColorLiteralTest"}],
  "manualCriteria": [{"id": "M1", "criterion": "A reviewer confirms each accent is distinct", "confirmation": "reviewer comment on the PR"}],
  "capabilityGaps": [{"id": "G1", "summary": "No exact 1.3 font preset on the device", "blocks": ["S4"], "consequence": "1.3 rests on the goldens; the device checks its enlarged preset"}],
  "specGaps": [{"id": "U1", "summary": "Unknown stored theme key", "coverage": "ThemeSettingTest"}],
  "specDisagreements": [{"id": "D1", "spec": "what the issue says", "implementation": "what the code does"}]
}
```

`outsideQA[].phase` defaults to `local`: a required local proof must PASS or be NA before stageClean. Use `future-ci` only for CI that runs after the PR opens; it may remain PENDING. Manual confirmations keep their separate gate. Unknown phases block stageClean.

`required` defaults to true. Plan text reaches the public summary, so write no address, path or identifier in titles, criteria or gap summaries.

## plan.md

```markdown
# QA plan: <scope>

<One paragraph: what the change does on the phone and which surfaces it reaches.>

| Item | Value |
| --- | --- |
| Target | `<branch>` at `<sha>`, PR <n or none yet> |
| Reports for this SHA | <what the brief listed> |
| Capability gaps | <IDs, or none> |

## Acceptance coverage

| Criterion | Scenario, outsideQA or manual row |
| --- | --- |

## Gaps

- G1 <blocks> <consequence>

## S1: <title>

Cells: <ids>. Setup: <capability IDs and flags>.

1. <step, with exact labels>
2. Expect <observable result>.
```

## proofs.json

The coordinator writes this in step 4, for the target SHA only. Unlisted rows stay PENDING. PASS needs a named command with exit 0, or an exact-SHA CI URL with `ciConclusion: "success"`. A nonzero exit contradicts PASS and becomes FAIL even with a CI URL.

Observed theme, `fontMultiplier` and appearance must equal each specified planned theme, `fontScale` and appearance. Missing or mismatching setup stays INCONCLUSIVE. Device PASS also requires the verified release APK record, its actual digest and the matching `up.json`. Finish records both initialization cleanliness and live cleanliness; a dirty or unreadable checkout blocks stageClean.

```json
{
  "outsideQA": [{"id": "P1", "state": "PASS", "sha": "<target sha>", "command": "make check", "exit": 0, "ciUrl": ""}],
  "manualCriteria": [{"id": "M1", "state": "PENDING", "evidence": ""}]
}
```

## Review checklist (coordinator)

- Every acceptance criterion and every runtime change is in Acceptance coverage.
- Expected results are observable: no "works correctly" or "renders fine".
- Each setup is a capability ID from `scripts/qa capabilities`; each missing one is a named gap.
- No scenario asks for a weakened app or Agent, or for a control the app does not have.
- Theme and font cells follow the map's budget; no cross product.
- `plan.json` matches `plan.md` scenario for scenario and cell for cell.
