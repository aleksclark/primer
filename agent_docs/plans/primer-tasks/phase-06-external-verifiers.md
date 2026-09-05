# Phase 6: External verifier protocol

## Goal

Prove that verification is an extensible protocol rather than four hard-coded UI
branches. Add the `external_callback` driver: a parent selects an administrator-
configured external verifier, the server delivers a signed idempotent request,
and the verifier returns progress and an accept/reject result through a scoped
callback. The same generic attempt/decision policy updates the checklist.

A real verifier fixture runs as a separate process for E2E. Default host-path
proof starts that process without Compose. Because this phase also adds the
fixture to the opt-in Compose vector, prove that additive service as well. The
fixture is external to the Tasks process and database; no in-process fake is
accepted as boundary proof. Native-client progress handling is owned by a
separate continuation plan.

## BDD Success Criteria

#### Scenario: External verifier accepts a task

- **Given** an occurrence with a snapshotted `external_callback` requirement and
  an allowlisted verifier endpoint
- **When** the student submits the configured web response/artifact
- **Then** a durable job emits one signed request with tenant-safe opaque refs,
  the external service reports progress and an accepted structured result, and
  the generic engine completes the requirement/occurrence
- **And** parent/student SPAs show source, progress, provenance, and safe rationale.

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
- **When** parent/student browsers reconnect with cursors
- **Then** durable safe progress and final state replay in order
- **And** callback handling after restart still resolves the correct attempt.

## Implementation Instructions

1. Add an administrator-configured verifier catalog separate from parent task
   configuration. Records include ID/name/endpoint/active state, supported
   schema versions/capabilities, secret reference/version, timeout/retry, and
   egress policy. Never store plaintext secrets in task revisions or clients.
2. Register `external_callback`. Parent-visible config selects verifier ID/
   capability and schema-validated public options. Occurrences snapshot manifest
   version while delivery resolves auditable secret rotation.
3. Add a durable PostgreSQL outbox/delivery worker with leases/fencing. Request
   envelope includes request/attempt/requirement refs, schema version, expiry,
   idempotency key, callback URL, and allowed opaque handles—not unnecessary
   parent/student identity.
4. Sign method/path/timestamp/body digest/request ID. Verify callback signatures
   in constant time, enforce clock/body bounds, bind verifier+attempt+digest, and
   maintain replay/secret-rotation records.
5. Define typed progress, accepted, rejected, retryable_error, and terminal_error
   results. Only validated accepted/rejected results can ask the generic engine
   to append a decision; callbacks never set occurrence state directly.
6. Use at-least-once delivery, `FOR UPDATE SKIP LOCKED`, finite leases,
   exponential backoff+jitter, max attempts/age, dead-letter state, manual
   retry/cancel, restart recovery, and transactional source events.
7. Prevent SSRF: parents never supply endpoint URLs/headers; admin endpoints are
   HTTPS/allowlist/resolution/redirect/egress validated and credentials never
   follow redirects.
8. Add a separate-process `external-verifier-fixture` that validates production
   signatures/idempotency, supports barrier/failure modes, owns its fixture
   ledger, calls the public callback, and never shares/imports Tasks DB or
   internal packages. Default host/E2E starts that process without Compose.
   Because this phase also adds it to the opt-in Compose vector, include an
   additive Compose service with the same isolation rules.
9. Add parent advanced configuration, verifier health/capability, delivery
   inspector, retry/cancel/fallback, and audit surfaces. Add student web waiting/
   progress/retry/rejected/completed states with System C safe progress.
10. Extend generated REST/WS TypeScript clients and metrics/logs for queue age,
    attempts, latency, status, signature failures, dead letters, and verifier ID;
    redact endpoints where sensitive, signatures, headers, bodies, and object URLs.
11. Emit versioned outbox facts for verification requested/progressed/decided and
    occurrence completed for phase-7 integration readiness.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent selects the real fixture verifier, configures/schedules a task, student
  submits through the SPA, and both observe signed delivery progress/completion.
- Exercise lost ack/callback replay, duplicate/out-of-order callbacks, invalid
  signature/body digest/timestamp, 429/5xx/timeouts, dead letter, retry, cancel,
  fallback, verifier disabled, and schema mismatch.
- Attempt arbitrary endpoint/redirect/private-address configuration and inspect
  logs/metrics for redaction.
- Restart Tasks between request/callback and restart the verifier between
  receives; verify exactly-one decision and durable progress after browser
  reconnect.

### Promoted automation

- After exploratory PASS, add Playwright specs for happy path, progress,
  retry/dead-letter/fallback, signature negatives, capability mismatch, restart,
  and tenant isolation.
- Add process E2E starting Tasks, PostgreSQL/object store as needed, and the
  external fixture as separate processes. Assert real signatures, callbacks,
  persisted delivery, outbox event, and exactly-one decision.
- Add multi-worker lease/callback race tests and planted-red body/signature/
  endpoint/timestamp/secret-log tests.

Commands:

```bash
make tasks-test tasks-cover tasks-clients tasks-web tasks-e2e
make tasks-external-e2e
```

## Anti-Cheating Audit

- Capture real network traffic between separate Tasks/verifier processes; reject
  an in-process function call, shared DB, internal import, or hard-coded result.
- Trace attempt → outbox/delivery → signed request → signed callback → generic
  decision → occurrence with durable product-owned rows at every step.
- Inspect idempotency/replay constraints and concurrent tests for exactly-one
  effects, not merely multiple successful statuses.
- Verify callback auth binds body/path/time/verifier/attempt and supports audited
  secret rotation; reject unsigned progress.
- Inspect SSRF handling, redirects, DNS resolution, retry/dead-letter limits,
  and lease fencing.
- Ensure logs/UI/WS contain no signatures, secrets, raw bodies, long-lived URLs,
  or hidden reasoning.
- Unknown schema/capability versions never auto-pass.

## Completion Gate

- [ ] All external protocol BDD scenarios pass across separate real processes.
- [ ] Dedicated browser exploratory PASS precedes Playwright promotion.
- [ ] Playwright external-progress suite is green.
- [ ] Signature/idempotency/restart/lease/concurrency/SSRF planted-red tests pass.
- [ ] Exactly one durable decision and completion event result from retries/races.
- [ ] Generated clients, Go/web/coverage/build/diff gates pass.
- [ ] Anti-cheating audit finds no in-process verifier substitute, unsigned callback, arbitrary egress, or fake durability.
