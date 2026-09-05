# Android Phase 3: External verifier progress

## Goal

Consume the reviewed main Phase 6 external-verifier contract in the native
student client. A student submits the configured response/artifact, safely waits
through delivery/retry/restart, and sees durable progress and terminal state from
the real external fixture without treating client state as verification truth.

## BDD Success Criteria

#### Scenario: External verification completes natively

- **Given** a paired student with an external-callback occurrence
- **When** the student submits through the native app and the real verifier
  accepts
- **Then** queued/delivered/progress/accepted states replay in order
- **And** the occurrence completes only from the server decision.

#### Scenario: Background/restart preserves waiting state

- **Given** a verifier request awaiting callback
- **When** the app backgrounds, is killed, and later reopens
- **Then** it reloads durable progress/final state with no duplicate submission or
  callback effect.

#### Scenario: Retry/dead-letter/fallback are explicit

- **Given** verifier timeout, 429/5xx, malformed result, or dead letter
- **When** state refreshes
- **Then** the app shows bounded retry/waiting/failed/parent-fallback semantics
  from the server
- **And** it never invents success or exposes raw callback/signature content.

#### Scenario: Revocation and foreign access fail closed

- **Given** active verification and a revoked or foreign device credential
- **When** it subscribes, refreshes, retries, or opens state
- **Then** access is denied without an existence oracle and local pairing/cache is
  handled according to revocation policy.

## Implementation Instructions

1. Consume reviewed external-submission/progress/result types through generated
   Kotlin clients; do not duplicate callback/webhook schemas in app code.
2. Add native submit, waiting/progress, retry/cancel (where authorized), rejected,
   fallback, dead-letter, and completed System C states.
3. Use durable event cursor/replay and bounded reconnect. Backgrounding never
   cancels the external server job; reopen fetches authoritative state.
4. Persist only opaque occurrence/attempt/submission/cursor metadata needed for
   restore. Never store verifier signatures, secret refs, raw callback body, or
   long-lived artifact URLs.
5. Map only safe progress/rationale fields. Raw verifier payload/tool/reasoning
   data and operational endpoint details are never shown/logged.
6. Enforce message/submit idempotency and typed conflict recovery; retries must
   call public server retry semantics rather than create parallel attempts.
7. Add JVM/Compose tests for progress reducer/order/replay, reconnect/restart,
   retry/dead-letter/fallback, malformed/unknown versions, revocation, no secrets,
   and completion authority.

## End-to-End Test Plan

- Pair a fresh emulator against real Tasks; configure/schedule an external task in
  parent web; submit from native app to the separate real fixture.
- Observe multiple signed-delivery progress events and accepted completion.
- Background/kill before callback, restart app and Tasks/verifier at controlled
  points, and verify durable progress/exactly-one result.
- Exercise lost ack/replay, timeout/429/5xx, malformed result, dead letter,
  retry/cancel/fallback, verifier disabled, and schema mismatch.
- Attempt foreign occurrence and revoke mid-wait; inspect app storage/logcat for
  signatures, bodies, endpoints, tokens, or stale state.
- After exploratory PASS, promote waiting/reconnect/terminal/failure cases to
  connected/emulator automation against separate real processes.

Commands:

```bash
cd primer-tasks/android
./gradlew testDebugUnitTest assembleDebug connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Trace native submit through generated public client to Tasks outbox/delivery/
  callback/decision; reject local fixture calls or direct callback invocation.
- Verify background/restart uses public durable state and does not preserve a fake
  local terminal value.
- Search app code/storage/logs/UI for verifier signatures, secrets, raw payloads,
  internal endpoints, and hidden reasoning.
- Confirm unknown result/schema versions fail closed and no client status marks
  completion.
- Confirm E2E fixture is a separate process with no shared DB/internal imports.

## Completion Gate

- [ ] Happy external verification completes through the real separate service.
- [ ] Background/kill/reconnect/restart and exactly-one result pass.
- [ ] Retry/dead-letter/fallback/malformed/disabled/schema negatives pass.
- [ ] Foreign/revoked and secret-retention checks pass.
- [ ] Dedicated exploratory PASS precedes promoted emulator automation.
- [ ] Fresh anti-cheat review approves generated-client and server-authority boundaries.
