# Phase 6: External verifier protocol

## Goal

Prove that verification is an extensible protocol rather than four hard-coded UI
branches. Add the `external_callback` driver: a parent selects an administrator-
configured external verifier, the server delivers a signed idempotent request,
and the verifier returns progress and an accept/reject result through a scoped
callback. The same generic attempt/decision policy updates the checklist.

A real verifier fixture runs as another Compose service for E2E. It is external
to the Tasks process and database; no in-process fake is accepted as boundary
proof.

## BDD Success Criteria

#### Scenario: External verifier accepts a task

- **Given** an occurrence with a snapshotted `external_callback` requirement and
  an allowlisted verifier endpoint
- **When** the student submits the configured response/artifact
- **Then** a durable job emits one signed request with tenant-safe opaque refs,
  the external service reports progress and an accepted structured result, and
  the generic engine completes the requirement/occurrence
- **And** parent/student UIs show source, progress, provenance, and safe rationale.

#### Scenario: At-least-once delivery remains exactly-once in effect

- **Given** the verifier receives a request but its response/ack is lost
- **When** Tasks retries delivery and the verifier/callback replay
- **Then** request and callback idempotency return the same attempt/result,
  exactly one immutable decision exists, and no duplicate completion/event is
  emitted.

#### Scenario: Forged, stale, or mismatched callback fails closed

- **Given** a missing/invalid signature, expired timestamp, replay outside the
  window, wrong verifier/attempt/tenant binding, or changed body digest
- **When** a callback is submitted
- **Then** it is rejected without changing verification state
- **And** a bounded security/audit event identifies the failure class without
  logging secrets or response bodies.

#### Scenario: External outage is visible and bounded

- **Given** DNS/connection timeout, 429/5xx, malformed response, or a verifier
  that never calls back
- **When** delivery/reconciliation runs
- **Then** finite exponential backoff with jitter and max age/attempts applies,
  progress shows waiting/retrying/failed, and the task never auto-completes
- **And** a parent can retry, cancel, or route to a configured human-review path.

#### Scenario: Configuration cannot create SSRF or leak secrets

- **Given** a parent task author and a verifier catalog managed by product admins
- **When** the parent configures a requirement
- **Then** the task stores only an allowlisted verifier ID and public config
  schema fields, not arbitrary URLs/headers/secrets
- **And** loopback, link-local, cloud metadata, private-network, redirect, and DNS
  rebinding targets are blocked according to deployment policy.

#### Scenario: Capability/schema mismatch blocks publication

- **Given** a verifier manifest with supported input/output/config schema versions
- **When** a task asks for unsupported media, version, or required capability
- **Then** publication is rejected with a useful parent error
- **And** disabling/removing a verifier leaves existing occurrences visibly
  blocked/reviewable rather than silently accepted.

#### Scenario: External progress survives reconnect/restart

- **Given** a verifier reports multiple progress steps and Tasks restarts
- **When** parent/student reconnect with cursors
- **Then** durable safe progress and final state replay in order
- **And** callback handling after restart still resolves the correct attempt.

## Implementation Instructions

1. Add an administrator-configured verifier catalog separate from parent task
   configuration. Catalog records include ID, name, endpoint, active state,
   supported manifest/config/submission/result schema versions, capabilities,
   secret reference/version, timeout, retry policy, and egress policy. Do not
   store plaintext secrets in task revisions or return them to clients.
2. Register `external_callback` in the verification registry. Its parent-visible
   config selects verifier ID/capability and schema-validated public options.
   Occurrences snapshot the manifest/version but resolve secret rotation at
   delivery time with an auditable key ID.
3. Add a durable outbox/delivery worker using real PostgreSQL leases/fencing.
   Request envelope includes event/request ID, attempt/requirement refs, schema
   version, created/expiry time, idempotency key, callback URL, allowed response
   metadata/artifact handles, and no parent/student names unless explicitly
   required and approved.
4. Sign exact method/path/timestamp/body digest/request ID with HMAC (or a
   documented asymmetric alternative). Verify callback signatures in constant
   time, enforce clock window and body limits, bind verifier + attempt + result
   digest, and keep a replay ledger. Support secret rotation with bounded overlap.
5. Make callback result a typed discriminated union: progress, accepted,
   rejected, retryable_error, terminal_error. Validate against the registered
   result schema and legal attempt state. Only accepted/rejected terminal results
   can ask the generic engine to commit a decision; they cannot directly update
   occurrence state.
6. Use at-least-once delivery and idempotent receiver semantics. Claim with
   `FOR UPDATE SKIP LOCKED`, finite leases, exponential backoff+jitter, max
   attempts/age, dead-letter state, manual retry/cancel, and restart recovery.
   Events follow a transactional outbox with source-of-truth state.
7. Add SSRF controls: parents never enter endpoint URLs; admin configuration is
   validated against HTTPS/allowlists, resolved addresses, redirects, and
   deployment egress policy. Re-resolve safely at connect time and prevent
   credential forwarding across redirects.
8. Add a standalone `external-verifier-fixture` Compose service that validates
   production signatures/idempotency, supports barrier/failure modes, stores its
   own in-memory fixture ledger only as the external test service, and calls the
   real public callback. It may not share Tasks DB or import internal packages.
9. Add parent advanced requirement configuration, verifier health/capability
   view, delivery inspector, retry/cancel/fallback actions, and audit. Add student
   waiting/progress/retry/rejected/completed states. Preserve System C and safe
   progress/no raw payload display.
10. Extend REST/WS generated clients, operational metrics, and logs for queue age,
    attempts, latency, status classes, signature failures, dead letters, and
    verifier ID; redact URLs where sensitive, signatures, headers, bodies, and
    artifact URLs.
11. Emit domain outbox events for verification requested/progressed/decided and
    occurrence completed with globally unique event IDs and schema versions.
    These events are product-owned facts and form part of phase-7 Primer readiness.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent selects the real fixture verifier, configures/schedules a task, student
  submits, and both observe signed delivery progress then completion.
- Exercise lost ack/callback replay, duplicate/out-of-order callbacks, invalid
  signature/body digest/timestamp, 429/5xx/timeouts, dead letter, manual retry,
  cancel, fallback to parent review, verifier disabled, and schema mismatch.
- Attempt arbitrary endpoint/redirect/private address configuration and inspect
  logs/metrics for redaction.
- Restart Tasks between request and callback and restart the verifier between
  receives; verify exactly-one decision and durable progress.

### Android emulator acceptance

- From a paired emulator, submit a response/artifact to an external requirement,
  background/kill the app while waiting, reopen, and observe replayed progress
  and terminal state from the real fixture service.

### Promoted automation

- After exploratory PASS, add Playwright external-verifier specs for happy path,
  progress, retry/dead-letter/fallback, signature negatives, capability mismatch,
  restart, and tenant isolation.
- Add emulator-backed waiting/reconnect/terminal state test.
- Add process E2E starting Tasks, PostgreSQL/object store as needed, and the
  external fixture as separate processes/containers. Assert real wire signature,
  callback, persisted delivery, outbox event, and exactly-one decision.
- Add concurrency/lease tests with multiple Tasks workers and callback races;
  run under race detector where in-process concurrency applies.
- Add planted-red tests for body mutation, signature mismatch, raw private URL,
  stale timestamp, and tracked secret/log leakage.

Commands:

```bash
make tasks-test tasks-cover tasks-clients tasks-web tasks-android tasks-e2e
make tasks-external-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Capture real network traffic between separate Tasks and verifier processes;
  reject an in-process function call, shared DB, fixture import of internal code,
  or handler hard-coded result.
- Trace source attempt → outbox/delivery → signed request → signed callback →
  generic decision → occurrence. Require durable rows at each product-owned step.
- Inspect idempotency/replay constraints and concurrent tests for exactly-one
  decision/event effects, not merely multiple 200 responses.
- Verify callback auth is server-side, constant-time where applicable, bounded,
  and binds body/path/time/verifier/attempt; reject a shared global secret with no
  rotation/audit or unsigned progress.
- Inspect SSRF handling and redirects/DNS resolution. Parent config must never
  become arbitrary URL/headers.
- Verify retry/dead-letter limits and lease fencing. Reject swallowed errors,
  infinite retries, or status marked complete on delivery alone.
- Ensure logs/UI/WS do not contain signatures, secrets, raw callback bodies,
  long-lived object URLs, or raw hidden model reasoning.
- Confirm schema/capability mismatch blocks task publication or execution visibly;
  unknown versions never auto-pass.

## Completion Gate

- [ ] All external protocol BDD scenarios pass across separate real processes.
- [ ] Dedicated exploratory browser/Android agents PASS before promotion.
- [ ] Playwright and emulator external-progress suites are green.
- [ ] Signature/idempotency/restart/lease/concurrency/SSRF planted-red tests pass.
- [ ] Exactly one durable decision and completion event result from retries/races.
- [ ] Generated clients, Go/web/Android/coverage/build/diff gates pass.
- [ ] Anti-cheating audit finds no in-process verifier substitute, unsigned callback, arbitrary egress, or fake durability.
