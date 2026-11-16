# Lifecycle evidence for foreground polling

Use this reference when the plan requires foreground-only requests. The 2026-11-16 run observed this procedure on the release build: the same process and Activity, a `wm_on_stop_called` event, clock correlation, a 12 s hidden interval with no new polls, a resumed request carrying `after` and `wait`, and Stale on return. It adds no app instrumentation.

## Boundary and evidence

`PulseScreen` collects `Watcher.watch` with `collectAsStateWithLifecycle`, whose default minimum state is STARTED. Below STARTED, collection is canceled. The cold Flow has no separate polling job; its held long-poll and any backoff belong to the collector, and canceling the call closes its connection. The screen keeps the Flow, so collecting it again resumes with the held Cursor: the first request back carries `after` (the access log's `query_names`), and a hide longer than 10 s shows Stale with its age until the first answer. `WatcherTest`, `WatcherHttpTest` and `PulseScreenLifecycleTest` cover this in unit tests, not a device lifecycle transition.

Home command return, launcher visibility, `wm_stop_activity` and an onPause event do not prove that onStop ran. Android's `wm_on_stop_called` event is written after the Activity's onStop callback returns. Use that event for the current app process and Activity token as the conservative completed-stop boundary. Keep the matching start/resume events and read-only Activity state dump too. On API levels with different event names, inspect the device's event tag definitions and framework source before substituting one. A missing boundary leaves the required cell INCONCLUSIVE.

The runner uses only its bound command for Home, foregrounding and screenshots. `adb` below reads logs, clocks and state only:

```sh
adb -s <serial> logcat -b events -d -v epoch -v usec 'wm_on_stop_called:I' 'wm_on_start_called:I' 'wm_on_resume_called:I' '*:S'
adb -s <serial> shell dumpsys activity activities
adb -s <serial> shell cat /system/etc/event-log-tags
```

Save output inside the run before Home, after stop and after foregrounding. Identify the Activity by current process, component and token, not another app's stop or a stale event. Record the build/API and app PID. The dump corroborates the stopped state; the event supplies its timestamp. Preserve full private logs and quote only the relevant lines in the observation.

## Clock correlation

With the reverse route, access-log `time` is host UTC at server request arrival. Event-log epoch time is device wall time. With the device route both are device wall time. `duration_ms` describes completion; the access line is written only when the handler returns. Read every access segment after pending handlers have finished or canceled, including segments before an Agent restart.

For reverse-route correlation, immediately before Home and after the hidden interval, take several samples of `adb -s <serial> shell date +%s.%N`. Bracket each invocation with host `time.time_ns()` and `time.monotonic_ns()`, retaining raw output. Confirm the device prints numeric fractional epoch seconds. For device reading D and host invocation bounds H0 and H1, host-minus-device offset lies in [H0-D, H1-D], expanded by the device clock's printed precision. Intersect the samples' bounds only when they agree and no wall-clock step is detected against monotonic elapsed time. Preserve the bounds and calculation. Disjoint bounds, a clock step or insufficient precision make a boundary-adjacent request ambiguous, not a definite defect or PASS.

Map the stop event to a host-time interval using those bounds and event timestamp precision. For a device-route Agent use the same device clock, still checking for clock steps. Never subtract device event time directly from host request time or use command duration as a clock offset.

## Required observation

1. Establish Live and its matching request. Record the current access segment, PID, clock samples and pre-Home events. Retain the plan's in-flight setup if it has one.
2. Send Home with the bound command. Capture the matching completed onStop event and stopped-state dump. Preserve command times for context only.
3. Stay hidden for at least twelve seconds after the completed-stop boundary, covering several two-second poll opportunities. If the plan restarts the QA Agent while hidden, preserve both log segments and observe both sides of that restart. Take the second clock samples before foregrounding.
4. Foreground through the same bound command, capture matching start/resume events, and observe a fresh request plus the plan's exact body-specific values. Capture the screen with `device_screenshot`.
5. Correlate every request with the boundary intervals. A request that began before stop may finish or cancel afterward. A server arrival alone cannot distinguish a packet already in flight from a new post-stop poll. Report a boundary-adjacent arrival as unresolved unless independent evidence establishes its origin; never invent a grace period that permits new polling. Repeated new requests during the stopped interval contradict foreground-only polling. A PASS needs a demonstrated stopped interval with no new polling and resumed requests on return; unresolved arrivals or missing events keep the cell INCONCLUSIVE.

The access log's `canceled` field stayed false for the held request cancelled at stop, because the QA Agent's handler still completes with 200 after the client leaves. Do not use that field as cancellation proof. The request's early `duration_ms` (2976 ms against the 10 s hold, ending at the stop boundary) is the cancellation evidence.

Keep the strict assertion: no new polling after lifecycle stop. A Home animation may still be STARTED, so a request between Home return and the proven stop boundary does not by itself violate that assertion. This evidence does not retroactively change an old result.
