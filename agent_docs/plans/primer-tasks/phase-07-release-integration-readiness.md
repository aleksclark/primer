# Phase 7: Multi-user operations and Primer-ready release

## Goal

Turn the completed standalone feature set into an operable, integration-ready
product without embedding it into Primer yet. Add tenant-admin parent invitations
(all parents still have the single `admin` role), comprehensive audit/retention,
backup/restore and device replacement drills, observability and limits, and
authenticated service/event seams that a later Primer adapter can consume.
Re-prove Stacklane isolation/hot reload and run the complete browser/Android
acceptance matrix.

The phase ends with a standalone release candidate and an explicit future
integration contract, not a cross-database shortcut or premature LMS sync.

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
  actor/device/agent/provider/verifier provenance, overrides, and domain events
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
- **When** PostgreSQL and object-store backup are taken, the stack is restored to
  a fresh instance, and workers resume
- **Then** tenants, memberships, students, tasks, schedules, occurrences,
  decisions, audit, outbox, and referenced artifacts are consistent
- **And** no terminal task duplicates, stale lease, or cross-instance credential
  is accepted inadvertently.

#### Scenario: Device replacement does not duplicate completion

- **Given** a lost/revoked Android device and a replacement paired to the same
  student
- **When** the replacement loads work and retries any server-known terminal state
- **Then** it observes existing completion, cannot replay old device credentials,
  and creates no duplicate decisions/submissions.

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
- **When** parents/students complete every verification type
- **Then** Primer Tasks continues normally and queues any outbound integration
  events durably
- **And** no query, migration, FK, view, or fallback touches a Primer database.

#### Scenario: Operational failures are diagnosable

- **Given** stale scheduler/job/outbox leases, provider failures, external
  dead-letters, object inconsistencies, pairing attacks, slow WebSocket clients,
  or unsupported app versions
- **When** they occur
- **Then** health/readiness, metrics, structured logs, and parent diagnostics show
  bounded actionable state without secrets/student media/chat bodies
- **And** readiness fails only for documented critical dependencies.

#### Scenario: Full Stacklane and client matrix remains green

- **Given** two release-candidate worktree instances
- **When** the full browser and Android acceptance suites, hot-reload probes, and
  stop-one isolation run
- **Then** each instance remains isolated and all phase 1–6 user flows pass
- **And** clean contract/client generation leaves the worktree clean.

## Implementation Instructions

1. Add invitation records and accept/revoke membership domain services. Invitations
   bind tenant, intended subject/email hint only as non-authoritative display,
   inviter, expiry, one-use digest, and the actual authenticated Identity subject
   at acceptance. Never merge users by email. Keep the only initial role `admin`.
2. Add last-admin protection, session/confirmation revocation, membership audit,
   and two-parent concurrency tests. Recheck membership on every sensitive
   request/tool/WS replay rather than trusting a long-lived role snapshot.
3. Consolidate append-only audit events and parent **Inspect/Monitor** UI across
   all verification kinds. Define redaction/visibility separately for student
   text, parent text, agent safe rationale, artifact metadata, provider usage,
   external verifier data, and operational logs.
4. Implement configurable retention jobs with leases/fencing for conversations,
   transient run events, uploads/partials, originals/derivatives, external
   payloads, audit, and outbox. Preserve immutable decision/provenance digests and
   explicit tombstones. Add dry-run/report and legal-hold/admin exclusions.
5. Add backup/restore scripts and runbook for the Tasks PostgreSQL database and
   object storage, including ordering, encryption, checksums, manifest, restore
   to a distinct instance, worker lease reconciliation, consistency scan, and
   credential/session handling. Never put secrets or student media in git test
   artifacts.
6. Add observability: OpenTelemetry hooks/Prometheus metrics and structured
   redacted logs for HTTP/WS, pairing, schedule lag, job leases, agent usage/
   latency/failure, verification outcomes, upload/scan/retention, external
   delivery/dead letters, outbox age, and client versions. Add health/readiness
   dependency semantics and alert thresholds/runbooks.
7. Define a future Primer integration adapter as authenticated REST/event
   surfaces owned by Tasks:
   - read-only occurrence/completion projections with stable source refs;
   - durable versioned outbox event feed or signed webhooks with cursor/ack;
   - explicit tenant service grants and scopes;
   - audience/client identity validation aligned with Primer Identity once its
     service-principal milestones are available.
   Generate a Go client package for future Primer consumption, but do not add LMS
   code or a shared database. Credential-free qualification may use a loopback
   issuer; production cutover remains gated on Identity service principals.
8. Add integration idempotency/source refs so a future consumer can replay safely.
   Tasks never waits synchronously for Primer to mark its own occurrence complete.
   Document event versioning, retention, backfill, and consumer outage behavior.
9. Harden security: request/body limits, rate limits for login/pair/message/upload,
   CSP/CORS/origin, secret rotation, object URL TTL, dependency/vulnerability
   scans, WebSocket connection/subscription limits, agent cost quotas, Android
   network security config, and production fail-fast config.
10. Finish deployment artifacts for standalone Tasks while preserving dev hot
    reload: production multi-stage image, migration command/job, non-root runtime,
    read-only filesystem except declared paths, graceful drain for HTTP/WS/workers,
    and documented rollback. Do not rewrite existing Primer deploys.
11. Add parent diagnostics for device/app version, last seen, revocation, job/
    outbox lag, failed verification requiring action, artifact retention, and
    provider/verifier availability.
12. Run full contract compatibility and generated-client policy gates. Attach
    normalized contracts/digests as CI/release artifacts rather than committing
    schema/generated source. Add breaking-change comparison against the first
    immutable release artifact.
13. Re-run Stacklane check, backend mutate/restore, frontend no-reload HMR,
    two-instance isolation, stop-one, and cleanup with unique probe instances.
14. Complete accessibility/responsive review for all parent/student SPA surfaces
    and full emulator matrix for pairing, checklist, dialogue, media, external
    waiting, revoke/re-pair, process restart, and device replacement.

## End-to-End Test Plan

### Browser exploratory acceptance

- Invite/accept second parent, concurrently manage tasks, revoke one parent, and
  attempt stale REST/WS/confirmation/artifact access.
- Run one complete task of each verification kind and reconstruct each in audit.
- Exercise retention dry-run/apply, legal hold, object/DB reconciliation,
  diagnostics, rate limits, provider/verifier disabled states, and export.
- Use a loopback service principal to read scoped completion/events, replay
  cursor/ack, deny wrong audience/scope/client/raw Stytch/missing tenant grant,
  and keep product working while consumer is down.
- Run full dark/light desktop/mobile axe and keyboard matrix.

### Android emulator acceptance

- Execute fresh pair, persistent login, manual task, dialogue reconnect, photo/
  video/audio submission, no-chat rubric, external wait, process restart, device
  revoke, replacement pair, and existing completion recovery against the real
  release stack.
- Run network loss and server restart at declared safe points. Inspect logcat,
  app storage, backups, media cache, and exported files for secret/privacy leaks.

### Backup/restore and isolation acceptance

- Seed representative data/media, back up, destroy only a unique probe stack with
  exact confirmation, restore into a different instance, run consistency scan,
  resume workers, and compare domain facts/digests.
- Run two Stacklane instances, full health/user smoke, mutate/restore Go and React
  sources, stop one, and verify the other plus its DB/object store remains intact.

### Promoted automation

- After exploratory PASS, promote invite/revoke/audit/retention/integration/auth
  and complete regression flows to Playwright.
- Extend emulator black-box automation to the full matrix; do not replace camera,
  QR, file, process, or real API boundaries with mocked screens.
- Add process E2E for service auth/event replay, consumer outage/backfill,
  retention restart, backup/restore consistency, graceful shutdown, rate limits,
  and production fail-fast.
- Add compatibility, generated-output tracking, vulnerability/license, secret
  scan, and planted-red policy tests.

Commands:

```bash
primer-tasks/scripts/dev check
make tasks-all tasks-test tasks-cover tasks-lint tasks-clients tasks-web tasks-android
make tasks-e2e tasks-external-e2e tasks-integration-e2e tasks-backup-restore-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
cd primer-tasks && go test -race ./... -count=1
```

## Anti-Cheating Audit

- Verify invitation acceptance binds the authenticated Identity subject and does
  not email-merge, auto-provision cross-tenant access, or trust invitation body
  tenant/role.
- Recheck revoked membership across REST, WS subscription/replay, agent tools,
  confirmation, object download, and service grants; client logout alone is not
  revocation.
- Trace audit rows from immutable source facts; reject synthesized UI timelines,
  mutable/delete-capable audit, or raw hidden reasoning/secrets.
- Verify retention deletes real object/DB content while preserving required
  decision facts, and restart/retry does not over-delete.
- Inspect backup/restore evidence for actual PostgreSQL + object bytes, manifests,
  checksums, fresh instance, and worker reconciliation; reject fixture reseeding
  presented as restore.
- Trace service API/event auth and tenant grants server-side. Reject shared-secret
  fail-open, raw Stytch acceptance, client filtering, or event rows without
  transactional source state.
- Search code/schema for LMS imports, DSNs, cross-DB SQL, FDW/dblink/views/FKs, or
  synchronous Primer dependency.
- Inspect observability for real counters/state and redaction; reject health that
  always returns OK or metrics derived only in browser.
- Re-run all earlier phase anti-cheating checks on the release tip, especially
  real Fantasy tools, media bytes, external separate process, exactly-once
  decisions, and no raw reasoning.
- Verify generated sources/contracts remain untracked and clean generation/build
  uses production registration.

## Completion Gate

- [ ] All phase-7 and regression BDD scenarios pass.
- [ ] Dedicated browser and Android exploratory agents PASS before final promotion.
- [ ] Full Playwright and emulator suites are green against the release stack.
- [ ] Invitation/revocation and two-tenant/two-parent isolation pass across every boundary.
- [ ] Audit/retention, backup/restore, device replacement, and consumer outage/backfill drills pass.
- [ ] Stacklane check/hot-reload/two-instance/stop-one proofs pass.
- [ ] Service integration seam is authenticated, generated, replay-safe, and DB-decoupled; production Identity blockers are labeled honestly.
- [ ] Full race/coverage/vet/lint/build/client compatibility/security/diff gates pass.
- [ ] Final anti-cheating audit finds no fake evidence, fail-open auth, secret/media leak, tracked generated output, or Primer database coupling.
