# Phase 11: Eliminate Android pairing flake

## Goal

Remove the known timing/order dependence in `TvViewModelTest` pairing behavior
and make repeated deterministic checks part of the Android PR gate. This phase
may proceed after Phase 09 in parallel with coverage work, but not in the same
workspace at the same time.

## BDD Success Criteria

### Scenario: Successful pairing is stable under concurrent home refresh

- **Given** a real OkHttp client, MockWebServer dispatcher keyed by request path,
  and an initially unpaired view model
- **When** pairing succeeds and catalog plus `/now` refresh concurrently under
  varied response order/delay
- **Then** credentials are stored once, destination remains catalog/home, and
  both refresh states settle without parse errors.

### Scenario: Pairing rejection is stable and preserves safe input

- **Given** rejected/used/expired codes and delayed network responses
- **When** pairing is submitted repeatedly or the scope is cancelled
- **Then** no token is stored, the refusal is surfaced, submitting clears, and
  only the non-secret server address is retained.

### Scenario: Revocation and unpair cannot race back to paired state

- **Given** a paired device, concurrent catalog refresh, revocation, and unpair
- **When** operations interleave
- **Then** credentials/grants are cleared durably, the pairing destination wins,
  and late responses cannot restore authenticated state.

### Scenario: Repeated CI reveals any regression

- **Given** the focused pairing/view-model suite
- **When** it is forced to execute repeatedly with Gradle caches unable to skip
  tests
- **Then** all configured repetitions pass with bounded timeouts
- **And** any single failure fails the job and retains test reports.

## Implementation Instructions

- Reproduce/stress before modifying. A 5/5 passing sample is not proof because a
  prior reviewed sample failed 1/5.
- Inspect concurrent `refreshHome` requests and FIFO MockWebServer responses.
  Replace response-order assumptions with a path-aware dispatcher or otherwise
  deterministic protocol fixture. Use coroutine test dispatchers/barriers where
  appropriate, while retaining a real OkHttp socket boundary for these tests.
- Make view-model state transitions robust to cancellation and stale late
  responses; do not merely lengthen the existing 5-second polling timeout.
- Ensure test scopes/jobs are fully cancelled/joined before server shutdown and
  assertions wait on the actual terminal state, not an intermediate token write.
- Add a repository script/Gradle/CI repeated target using `--rerun-tasks` (or
  equivalent) and focused filters. Preserve the ordinary full unit/APK job.
- Do not conflate the existing TV Android app with the separate incomplete
  Primer Tasks Android continuation.

## End-to-End Test Plan

- Before and after the fix, run at least 20 forced repetitions of
  `./gradlew :app:testDebugUnitTest --tests
  'com.aleksclark.primer.tv.app.ui.TvViewModelTest' --rerun-tasks --no-daemon`
  (or an equivalent single-build repeated JUnit parameter/stress suite that
  truly re-executes each test).
- Add deliberate catalog/`now` response inversions, delays, rejection, scope
  cancellation, revocation, and unpair interleavings using a path-keyed real
  MockWebServer socket boundary.
- Run full `cd android && ./gradlew test assembleDebug --no-daemon` and the
  Android workflow locally where feasible.
- Plant the old FIFO/order-sensitive fixture or stale-state defect in an isolated
  worktree and prove the repeated gate catches it; revert and rerun green.
- Connected emulator/physical TV proof is not required for this JVM race unless
  production behavior changes; if claimed, it must actually be run and recorded.

## Anti-Cheating Audit

- Check that repetitions really execute (`--rerun-tasks`, unique reports/counts)
  rather than Gradle reporting cached/up-to-date success.
- Reject increased sleeps/timeouts/retries as the sole fix.
- Verify path-aware MockWebServer responses match production endpoints and real
  JSON parsing; mocked repository callbacks cannot replace the socket race.
- Inspect view-model cancellation/stale-response guards, not just tests.
- Check teardown for leaked jobs/requests and CI for swallowed per-iteration
  exit codes.
- Check reports do not claim a physical device or Tasks Android continuation.

## Completion Gate

- [ ] Deterministic pairing/rejection/revocation/unpair scenarios pass.
- [ ] At least 20 forced focused repetitions pass at exact head.
- [ ] Full Android unit tests and debug APK assembly pass.
- [ ] Planted old-race red proof fails the repeated gate.
- [ ] No timeout inflation, retries, caching, or mocks hide the defect.
- [ ] CI uploads failure reports and fails on any repetition.
- [ ] Independent review passes and the Android fix PR is merged.
