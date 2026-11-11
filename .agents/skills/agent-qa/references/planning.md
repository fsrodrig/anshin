# Planning a QA run

The coordinator fills the brief below and dispatches it to the planner. The rest of this file is the planner's reference; the review checklist at the end is the coordinator's.

## Planner brief

```text
You are the test planner for one Anshin change. Design the test plan that a QA runner will execute on an
Android emulator with agent-device, against the installed app and a QA Agent; do not execute it.

Repository (read-only): <worktree>
Run directory (write here only): <run>
Change: <run>/changes.patch, the diff from the merge base.
Spec: <issue URL and acceptance criteria, design.md sections, ADRs, the user's request>

Read <worktree>/.agents/skills/agent-qa/references/planning.md (the rules and both file formats) and
<worktree>/.agents/skills/agent-qa/recipes/README.md (capability gaps). Read the changed code as needed.

Write <run>/plan.md and <run>/plan.json. Reply with: the scenario list (id, title, required), the QA Agent
setup each scenario needs, and every requirement you could not cover, with the reason.
```

## Rules

Derive every expected result from the spec (the issue, its acceptance criteria, `docs/design.md`, the ADRs, `GLOSSARY.md`), never from the implementation. Where the code and the spec disagree, plan the spec and flag the disagreement in `plan.md`. Use the glossary's words: Machine, Agent, Pulse, Window, Gap, Verdict, Reason, Live, Stale, Unreachable, Forbidden, Incompatible.

### Map the change

| Change | Surface to plan |
| --- | --- |
| A screen (Dashboard, Machine detail, Add/Edit machine, Settings) | its visible text and states, the four themes, font scale 1.0 and 1.3, the accessibility tree from `snapshot -i` |
| `Watcher` | the MachineView it leads to: Connecting, Live, Stale (pause the QA Agent), Unreachable (stop it, or airplane mode), Forbidden (WhoIs answer), Incompatible (schema), and history after an Agent restart (new epoch) |
| `MachineBook` | adding, editing and removing a Machine; duplicates and non-tailnet addresses refused; the test connection on save |
| `AgentLink` or the HTTP client | the QA Agent's access log: requests only while the app is visible, a delta with a cursor on return, no `Authorization` or `Cookie` header; an address outside the tailnet refused with zero requests in the log |
| A measurement or the Verdict | how null renders (a desktop has "no battery", never 0 %), Gaps drawn as gaps, a Verdict word beside its glyph |
| R8 rules, serialization, the manifest | the minified build decodes every Pulse it is shown |

### Write scenarios

- One scenario per behavior change and per acceptance criterion, numbered `S1`…`Sn`.
- Each has a concrete expected result: visible text, a MachineView, access-log lines (method, path, status, count), or their absence.
- A change to shared behavior gets a negative check: what must not happen.
- A UI change is checked in all four themes and at font scale 1.3.
- A scenario the environment cannot run (see the capability gaps) stays in the plan with `"required": false` and the gap named in `expected`, unless it is an acceptance criterion: then it stays required and the run is INCONCLUSIVE.
- Stop adding scenarios when the next one would no longer test this change.

## plan.json

```json
{
  "scope": "Dashboard: show Stale with its age",
  "scenarios": [
    {
      "id": "S1",
      "title": "A paused Agent turns Live into Stale with its age",
      "required": true,
      "setup": "QA Agent running, WhoIs allow; one Machine saved",
      "expected": "Dashboard shows the Machine Live; after `anshin-qa` is paused for 30 s it shows Stale with an age, and no current Verdict."
    }
  ]
}
```

`required` defaults to true.

## plan.md

```markdown
# QA plan: <scope>

<One paragraph: what the change does and which surfaces it reaches.>

## Preconditions

| Item | Value |
| --- | --- |
| Branch | `<branch>` at `<sha>` |
| QA Agent | <WhoIs answer, schema, pauses or restarts the scenarios need> |
| Capability gaps | <what this environment cannot exercise, or "none"> |

## Surface map

| Changed file | Surface | Scenarios |
| --- | --- | --- |

## S1: <title>

Covers: <requirement or file>

1. <step, with exact labels>
2. Expect <observable result>.
```

## Review checklist (coordinator)

- Every acceptance criterion and every behavior change in `changes.patch` maps to a scenario, or to a named gap.
- Expected results are observable: no "works correctly" or "renders fine".
- UI changes cover the four themes and font scale 1.3; traffic changes cite access-log lines.
- No scenario asks for a weakened app or Agent.
- `plan.json` matches `plan.md` scenario for scenario.
