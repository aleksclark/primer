# Phase 7: Multi-user operations and Primer-ready release

## Goal

Turn the completed standalone web/server feature set into an operable,
integration-ready product without embedding it into Primer yet. Add tenant-admin
parent invitations (all parents still have the single `admin` role),
comprehensive audit/retention, backup/restore drills, observability and limits,
and authenticated service/event seams that a later Primer adapter can consume.
Re-prove the default host Make/non-Docker path and, because this phase changes
the opt-in Compose/Stacklane vector, re-prove that additive isolation/hot-reload
matrix. Run the complete browser acceptance matrix against the default host path.

The phase ends with a standalone release candidate and explicit future
integration contract, not a cross-database shortcut or premature LMS sync.
Native-client release hardening and device-replacement regression live in a
separate continuation plan.

## BDD Success Criteria

#### Scenario: Admin invites a second parent

- **Given** a tenant with one parent admin
- **When** the admin creates a short-lived invitation and a second Primer Identity
  principal accepts through the BFF flow
- **Then** a second tenant membership with role `admin` is created exactly once
- **And** both parents can manage the same students/tasks while another tenant
  remains isolated
- **And** expired, reused, wrong-subject, or foreign-tenant invites fail closed.

#### Scenario: Last-admin and membership changes are safe

- **Given** multiple parent admins
- **When** one membership is revoked
- **Then** its BFF sessions and pending confirmation handles are revoked promptly,
  audit records the actor/target, and at least one active admin remains
- **And** the revoked parent cannot continue through REST, WebSocket replay, agent
  tools, artifact URLs, or invitation endpoints.

#### Scenario: Audit reconstructs task completion

- **Given** manual, dialogue, rubric, and external completed occurrences
- **When** a parent opens/exports the audit timeline
- **Then** it links task/revision/schedule/occurrence/attempt/submissions/decisions,
  actor/session/agent/provider/verifier provenance, overrides, and domain events
  without exposing secrets or hidden reasoning
- **And** audit rows are append-only and tenant-scoped.

#### Scenario: Retention deletes bytes without falsifying decisions

- **Given** expired chats, reasoning-free run events, artifacts, derivatives, and
  durable completion evidence under configured policies
- **When** retention jobs run
- **Then** eligible sensitive bytes/content are deleted or tombstoned, required
  digests/provenance/decision facts remain, object and DB state reconcile, and
  legal holds/admin policy are respected
- **And** retries/restarts are idempotent.

#### Scenario: Backup and restore preserve authority

- **Given** a populated standalone stack
- **When** PostgreSQL and object-store backup are taken, restored to a fresh
  instance, and workers resume
- **Then** tenants, memberships, students, tasks, schedules, occurrences,
  decisions, audit, outbox, and referenced artifacts are consistent
- **And** no terminal task duplicates, stale lease, or cross-instance credential
  is accepted inadvertently.

#### Scenario: Student browser session replacement does not duplicate completion

- **Given** a revoked/lost student browser session and a fresh re-pair to the same
  student
- **When** the new session loads work and retries any server-known terminal state
- **Then** it observes existing completion, cannot replay the old credential, and
  creates no duplicate decisions/submissions.

#### Scenario: Primer-facing service read is authenticated and tenant-scoped

- **Given** a registered service principal/audience in the Tasks integration
  adapter and tenant grant
- **When** it lists occurrence/completion facts or subscribes/polls domain events
- **Then** it receives versioned generated-client DTOs/events for granted tenants
  only, with idempotent cursor/ack behavior
- **And** wrong audience/scope/client, raw Stytch token, missing grant, or foreign
  tenant is denied.

#### Scenario: Standalone remains authoritative and decoupled

- **Given** Primer/LMS is absent or unavailable
- **When** browser users complete every verification type
- **Then** Primer Tasks continues normally and queues outbound integration events
  durably
- **And** no query, migration, FK, view, or fallback touches a Primer database.

#### Scenario: Operational failures are diagnosable

- **Given** stale scheduler/job/outbox leases, provider failures, external
  dead-letters, object inconsistencies, pairing attacks, slow WebSocket clients,
  or unsupported browser protocol versions
- **When** they occur
- **Then** health/readiness, metrics, structured logs, and parent diagnostics show
  bounded actionable state without secrets/student media/chat bodies
- **And** readiness fails only for documented critical dependencies.

#### Scenario: Default host path and opt-in Compose matrix remain green

- **Given** two release-candidate worktree instances on the default host
  Make/non-Docker path
- **When** full browser suites and stop-one isolation run
- **Then** each instance remains isolated and all phase 1–6 browser flows pass
- **And** clean contract/client generation leaves the worktree clean.
- **Given** this phase also changes the additive opt-in Compose/Stacklane vector
- **When** Compose check, hot-reload probes, two-instance, and stop-one isolation run
- **Then** each opt-in instance remains isolated without becoming the default
  host command.

## Implementation Instructions

1. Add invitation records and accept/revoke membership domain services. Bind
   tenant, non-authoritative intended-subject/email hint, inviter, expiry,
   one-use digest, and actual authenticated Identity subject. Never email-merge.
2. Add last-admin protection, session/confirmation revocation, membership audit,
   and two-parent concurrency tests. Recheck membership on every sensitive
   request/tool/WS replay rather than trusting long-lived role snapshots.
3. Consolidate append-only audit events and parent **Inspect/Monitor** UI across
   verification kinds. Define redaction/visibility for student/parent text,
   safe rationale, artifact metadata, provider usage, external data, and logs.
4. Implement configurable leased/fenced retention jobs for conversations,
   transient events, uploads/partials, originals/derivatives, external payloads,
   audit, and outbox. Preserve immutable decision/provenance digests and explicit
   tombstones; add dry-run, report, hold, and exclusion controls.
5. Add backup/restore scripts/runbook for PostgreSQL and object storage with
   ordering, encryption, checksums, manifests, distinct-instance restore, lease
   reconciliation, consistency scan, and session/credential handling.
6. Add OpenTelemetry/Prometheus and redacted structured logs for HTTP/WS,
   pairing, schedule/job/outbox lag, agent usage/failure, verification outcomes,
   upload/scan/retention, external delivery, and client protocol versions.
7. Define authenticated future Primer integration surfaces: read-only completion
   projections with stable refs; durable versioned event feed or signed webhooks;
   tenant grants/scopes; audience/client validation. Generate a Go client for
   future Primer consumption without adding LMS code/shared DB.
8. Add replay-safe integration source refs, event versioning/retention/backfill,
   and consumer-outage behavior. Tasks never waits synchronously for Primer to
   mark its own occurrence complete.
9. Harden request/body and rate limits, CSP/CORS/origin, secret rotation, object
   URL TTL, dependency/vulnerability scans, WS connection/subscription bounds,
   inference quotas, and production fail-fast config.
10. Finish standalone deployment artifacts: production multi-stage image,
    migration command/job, non-root/read-only runtime, graceful HTTP/WS/worker
    drain, documented rollback; preserve dev hot reload.
11. Add parent diagnostics for browser sessions, sync/last-seen, job/outbox lag,
    failed verification, retention, provider/verifier availability, and protocol
    compatibility.
12. Run contract compatibility/generated-client policy gates. Publish normalized
    contract/digest as CI/release artifacts rather than tracked schema/generated
    source; compare against immutable release baseline.
13. Re-run default host Make gates. Because this phase changes the opt-in
    Compose vector, also re-run Stacklane check, backend mutate/restore,
    frontend no-reload HMR, two-instance isolation, stop-one, and cleanup with
    unique probes. Do not make Compose the default host path.
14. Complete accessibility/responsive review across parent/student SPA surfaces
    and full browser regression for pairing, checklist, dialogue, media,
    external waiting, revoke/re-pair, process restart, and completion recovery.

## End-to-End Test Plan

### Browser exploratory acceptance

- Invite/accept second parent, concurrently manage tasks, revoke one parent, and
  attempt stale REST/WS/confirmation/artifact access.
- Complete one task of each verification kind and reconstruct each in audit.
- Exercise retention dry-run/apply, legal hold, object/DB reconciliation,
  diagnostics, rate limits, provider/verifier disabled states, and export.
- Use a loopback service principal to read scoped completion/events, replay
  cursor/ack, deny wrong audience/scope/client/raw Stytch/missing tenant grant,
  and keep product working while consumer is down.
- Run dark/light desktop/mobile axe and keyboard matrix.

### Backup/restore and isolation acceptance

- Seed representative data/media, back up, destroy only a unique probe stack with
  exact confirmation, restore into a different instance, run consistency scan,
  resume workers, and compare domain facts/digests.
- Run two default host-path instances, health/user smoke, and stop-one isolation.
  Because this phase changes the opt-in Compose vector, also run two Stacklane
  instances, mutate/restore Go and React sources, stop one, and verify the other
  plus DB/object store remains intact.

### Promoted automation

- After exploratory PASS, promote invite/revoke/audit/retention/integration/auth
  and complete regression flows to Playwright.
- Add process E2E for service auth/event replay, consumer outage/backfill,
  retention restart, backup/restore consistency, graceful shutdown, rate limits,
  and production fail-fast.
- Add compatibility, generated-output tracking, vulnerability/license, secret
  scan, and planted-red policy tests.

Commands:

```bash
make tasks-all tasks-test tasks-cover tasks-lint tasks-clients tasks-web
make tasks-e2e tasks-external-e2e tasks-integration-e2e tasks-backup-restore-e2e
cd primer-tasks && go test -race ./... -count=1
# additive opt-in Compose/Stacklane vector changed by this phase:
primer-tasks/scripts/dev check
```

## Anti-Cheating Audit

- Verify invitation acceptance binds authenticated Identity subject and does not
  email-merge, auto-provision cross-tenant access, or trust body tenant/role.
- Recheck revoked membership across REST, WS replay, tools, confirmation,
  downloads, and service grants; client logout alone is not revocation.
- Trace immutable audit rows from source facts; reject synthesized UI timelines,
  mutable audit, hidden reasoning, or secrets.
- Verify retention deletes real object/DB content while preserving required
  decision facts and restart/retry does not over-delete.
- Inspect real PostgreSQL + object backup/restore manifests/checksums/fresh
  instance/worker reconciliation; reject fixture reseeding as restore.
- Trace service auth and tenant grants server-side. Reject fail-open shared
  secrets, raw Stytch acceptance, client filtering, or events without source state.
- Search for LMS imports/DSNs/cross-DB SQL/FDW/dblink/views/FKs or synchronous
  Primer dependency.
- Inspect real metrics/health/redaction; reject always-OK health or browser-only
  counters.
- Re-run earlier web/server anti-cheating checks: Fantasy tools, media bytes,
  external separate process, exactly-once decisions, no reasoning.
- Verify generated sources/contracts remain untracked and production registration
  owns clean generation.

## Completion Gate

- [ ] All phase-7 and regression BDD scenarios pass.
- [ ] Dedicated browser exploratory PASS precedes final Playwright promotion.
- [ ] Full Playwright suite is green against the release stack.
- [ ] Invitation/revocation and two-tenant/two-parent isolation pass everywhere.
- [ ] Audit/retention, backup/restore, session replacement, and consumer outage drills pass.
- [ ] Default host Make gates pass. Because this phase changes the opt-in
      Compose vector, Stacklane check/hot-reload/two-instance/stop-one proofs
      also pass.
- [ ] Service integration seam is authenticated, generated, replay-safe, and DB-decoupled; production Identity blockers are labeled honestly.
- [ ] Full race/coverage/vet/lint/build/client compatibility/security/diff gates pass.
- [ ] Final anti-cheating audit finds no fake evidence, fail-open auth, secret/media leak, tracked generated output, or Primer DB coupling.
