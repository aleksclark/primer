# Phase 8: Publish immutability

**File:** `phase-08-publish-immutability.md`
**Depends on:** Phase 7
**Duration guess:** 3–5 days
**Handoff wave:** see index orchestrator map

## Goal

Implement revision publish flow: validation gate, transition draft→published, DB-enforced immutability, supersede lineage, audit, and outbox `plan_revision.published`. This completes the durable plan control-plane core.

## Scope

### In scope

- publishRevision endpoint
- reject graph mutations on published
- supersede published → new draft copy optional helper
- publish blocked on error-severity validation
- outbox event with payload ref
- audit_events

### Out of scope / YAGNI

- Materialization
- Webhook delivery worker (enqueue only; deliver Phase 14)

## BDD Success Criteria

#### Scenario: P8-S1 — Publish success

- **Given** draft with passed validation
- **When** POST publish
- **Then** status published
- **And** published_at set
- **And** immutable thereafter

#### Scenario: P8-S2 — Published GET still works

- **Given** published revision
- **When** GET graph/revision
- **Then** 200 read
- **And** names present

#### Scenario: P8-S3 — Reject published edit

- **Given** published revision
- **When** PATCH graph or update node
- **Then** 409/403
- **And** DB trigger or app; row unchanged

#### Scenario: P8-S4 — Supersede

- **Given** published revision
- **When** create superseding draft
- **Then** old superseded
- **And** new draft editable lineage link

#### Scenario: P8-S5 — Publish blocked on errors

- **Given** draft failing validation
- **When** publish
- **Then** 409/400
- **And** remains draft

#### Scenario: P8-S6 — Outbox published event

- **Given** successful publish
- **When** inspect outbox_events
- **Then** event_type plan_revision.published
- **And** same transaction durability

## Implementation Instructions

1. Service method Publish(ctx, revID) runs validate → status update → outbox.
2. Rely on DB triggers from 00004; assert errors mapped.
3. Copy-on-supersede strategy documented (deep copy graph tables).
4. Tests attempt UPDATE via SQL in integration to prove trigger (optional) plus API path.

## End-to-End Test Plan

#### P8-E1 — Publish happy path

- **Setup:** valid draft
- **Action:** publish
- **Assert:**
  - status
  - outbox row
- **Command:** `make studio-test`

#### P8-E2 — Immutability

- **Setup:** published
- **Action:** edit attempts
- **Assert:**
  - fail
  - select unchanged hash
- **Command:** `go test ./internal/plan -run Immutable`

#### P8-E3 — Supersede lineage

- **Setup:** published
- **Action:** supersede
- **Assert:**
  - states correct
- **Command:** `go test ./internal/plan -run Supersede`

#### P8-E4 — Blocked publish

- **Setup:** invalid draft
- **Action:** publish
- **Assert:**
  - rejected
- **Command:** `go test ./internal/plan -run PublishBlocked`

#### P8-E5 — Outbox durability

- **Setup:** publish then kill before worker
- **Action:** row remains undelivered
- **Assert:**
  - outbox pending
- **Command:** `go test ./internal/outbox -run Enqueue`

## Anti-Cheating Audit

- Handler must not flip status without validation
- Immutability not only app-layer if DB trigger exists — both should agree
- Outbox not written after commit separately without transactional outbox pattern
- Supersede must not mutate old graph

## Completion Gate

- [ ] P8-S*/E* green
- [ ] Planning MVP backend-ready
- [ ] Event type string exact match crosswalk


## Dependencies

- Upstream: Phase 7
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
