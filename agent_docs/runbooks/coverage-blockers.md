# primer-agents coverage — current gate and historical blockers

## Current gate

`make agents-cover` enforces **85%** over `primer-agents/internal/...`, through
`AGENTS_COVER_MIN` in the [root Makefile](../../Makefile) and the
[module coverage helper](../../scripts/enforce-module-cover.sh). The Agents CI
workflow also enforces 85%. Do not lower the floor or change the package set to
match an older measurement.

The coverage-unblock changes are reachable through PR #74 (`da783895`); the
Makefile records an 85.0% result at that checkpoint. This is **not a measurement
of the current head**. Re-run the gate and record the exact SHA, command, and
result before making a new acceptance claim.

## Historical Phase 8 report — not current instructions

The original report below is retained as history. Its 78% gate and proposed
78→82→85 escalation are superseded, not an approved present-day policy. The old
blocker diagnoses and estimates have not been revalidated; they must not be used
to justify skipping tests or requiring a live provider. Go statement coverage is
execution-counted, not sampled; the original sampling explanation below is not
an instrumentation guarantee.

### Historical measurement

**78.7%** (`go test ./internal/... -coverpkg=./internal/...` — all packages pass)

**Original target statement (superseded):** 85%, with a claimed 78% intermediate
gate to be raised as blockers resolved. The current gate above supersedes it.

## Historical blocker diagnoses

### 1. Worker goroutine paths (worker/execute, heartbeat, cancel poller)

`worker.go:execute` spawns two long-lived goroutines (heartbeat and cancel poller) that run concurrently with the MAF runner. The cancellation and heartbeat-failure branches within those goroutines require:
- A real MAF provider that returns in-flight updates (not a scripted provider that completes immediately)
- Lease expiry simulation with real wall-clock delay or DB manipulation

These paths are exercised by `TestWorkerCancellationReachesRuntime` (passes) but the goroutine branches that observe cancellation mid-stream, extend the lease under load, or fire the heartbeat-failure cancel are not consistently covered because the timing-sensitive paths complete before coverage instrumentation can sample them.

**Resolution**: Add a deterministic barrier-controlled provider in `worker_test.go` that holds a channel open while lease extension and cancel-poller branches are hit deliberately. Estimated effort: 2–4 hours; not hollow tests.

### 2. SSE LISTEN/NOTIFY branches (sse/stream.go)

`Stream` is 61.5% covered. The LISTEN/NOTIFY subscription goroutine (`SubscribeRunEvents`), the notification-channel-closed fallback, and the heartbeat tick branch are not exercised in CI because:
- The LISTEN/NOTIFY PostgreSQL feature requires the driver to hold a dedicated connection open long enough for the notification to arrive
- The testcontainer Postgres instance in CI does not fire the trigger reliably fast enough in short-lived tests

**Resolution**: Add a test that explicitly calls `pg_notify` from a second connection after appending an event and asserts the SSE stream delivers it without polling. Estimated effort: 3–5 hours.

### 3. Adapter methods — GetSchedule (app/app.go)

`GetSchedule` adapter at 0%: the integration test `TestRunServesAuthenticatedAPI` creates a schedule (covered) but does not call `GET /agents/v1/schedules/{id}` — that route is not yet wired to the app integration test because the schedule ID is not available after the non-blocking create response in the test flow.

**Resolution**: Capture the schedule ID from the create response and add a GET call. 30-minute fix.

### 4. `repo/session_turns.go:WaitForNewEvents`

`WaitForNewEvents` at 0%: this utility is called only by `sse/stream.go:Stream` in the non-LISTEN/NOTIFY fallback path, which requires live LISTEN/NOTIFY (blocker 2).

### 5. `appservice/service.go:withTx` error branches

Certain error paths inside `withTx` (Begin failure) are only reachable by injecting a broken pool — not supported without a mock or fault-injection shim, which would be hollow (it would never fire in production).

**Resolution**: Not worth adding; these are infrastructure error paths.

## Historical non-blocker notes

- `runtime/` is excluded from `./internal/...` coverage (it has its own wave-1 test suite at full coverage)
- `testutil/dsn_test.go` helper paths are infrastructure; already tested in their own package
- `logging/attrsToAny` and `WithGroup` — now covered by `TestRedactingHandlerWithAttrsAndWithGroup`

## Superseded gate proposal

The original report claimed that `make agents-cover` enforced **78%** and proposed
the following escalation. This is retained for provenance only; the current floor
is 85%, not any intermediate value in this table:

| Phase | Gate | Blocker resolved |
|-------|------|-----------------|
| Historical Phase 8 proposal | 78% | — |
| After blocker 1 | 82% | Worker barrier-provider tests |
| After blocker 2 | 85% | SSE LISTEN/NOTIFY tests |
