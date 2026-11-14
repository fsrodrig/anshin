# Anshin

Anshin lets one person watch the health of their own Linux computers from their phone, privately, over their tailnet.

## Language

### Who and what

**Machine**:
A Linux computer the Owner watches with Anshin.
_Avoid_: server, node, host, device

**Agent**:
The Anshin program running on a Machine that measures it and answers the phone.
_Avoid_: daemon, exporter, collector, service

**Owner**:
The tailnet user whose own untagged devices may watch a Machine.
_Avoid_: admin, account, user (alone)

**Tailnet node**:
Any device on the Owner's tailnet, including the phone. Every Machine is a tailnet node; not every tailnet node is a Machine.
_Avoid_: node (alone)

### What the Agent knows

**Sample**:
One timestamped set of measurements of a Machine.
_Avoid_: snapshot, point, metric, reading

**Window**:
The most recent Samples an Agent remembers, so charts are full the moment the phone looks.
_Avoid_: buffer, history, cache

**Gap**:
A stretch of the Window with no Samples, because the Machine was asleep or the Agent restarted. Shown as a gap, never filled in.
_Avoid_: outage, missing data

**Verdict**:
The Agent's one-word judgement of its Machine's health (ok, warn or crit), with the Reasons behind it.
_Avoid_: status, alert, health, alarm

**Reason**:
One plain-language cause behind a Verdict, such as a nearly full disk or a hot CPU.
_Avoid_: issue, problem, finding

**Pulse**:
What an Agent tells the phone about its Machine: the Window (or what's new since last time) and the current Verdict.
_Avoid_: payload, response, metrics, snapshot

### What the phone shows

**Live**:
The phone is receiving fresh Pulses from a Machine.

**Stale**:
The phone has data for a Machine, but none recent enough to trust as current; shown with its age, never as current.
_Avoid_: old, cached

**Unreachable**:
The phone cannot get a Pulse from a Machine. Anshin does not claim to know why (Machine off, Agent stopped, tailnet down).
_Avoid_: offline, down, dead

**Forbidden**:
The Agent answered but refused this phone, because it is not one of the Owner's untagged devices.
_Avoid_: unauthorized, auth failed

**Incompatible**:
The Agent answered in a version of the Pulse the phone cannot understand.
_Avoid_: outdated, unsupported

### Testing

**Capture**:
A scrubbed, reviewed copy of the files the Agent reads on a real Machine, taken in two or more snapshots about 2 s apart, that tests replay. Its snapshots are not Samples.
_Avoid_: dump, fixture (a fixture is a golden Pulse generated from a Capture)

**QA Agent**:
The test-only stand-in for an Agent that the `agent-qa` skill runs (`anshin-qa`): the Agent's own server behind a fake tailnet, answering for a synthetic Machine. Never shipped.
_Avoid_: mock agent, fake server, test server
