# Phase 11: Outbox, webhooks, and idempotency

**Depends on:** Phase 5 (min for publish events); integrates Phases 7–9 event types before completion
**Duration guess:** 4–6 days
**Tables:** `outbox_events`, `webhook_endpoints`, `webhook_deliveries`, `idempotency_keys`

## Goal

Guarantee durable domain events via transactional outbox, at-least-once webhook delivery with idempotency keys and leases, and inbound idempotency key storage for API/callback dedupe—without in-memory-only buses.

## BDD Success Criteria

#### Scenario: P11-S1 — Transactional outbox with domain write

- **Given** a draft revision publish path
- **When** `PublishRevision` commits
- **Then** in the **same transaction**, an `outbox_events` row is inserted with `event_type=plan_revision.published`
- **And** if outbox insert fails, publish rolls back

#### Scenario: P11-S2 — Webhook delivery at-least-once

- **Given** active webhook endpoint subscribed to event type
- **When** outbox dispatcher creates delivery rows
- **Then** `UNIQUE (endpoint_id, event_id)` holds
- **And** failed HTTP attempts increment `attempt_count` and keep status retryable until delivered/failed terminal policy
- **And** re-dispatch does not create duplicate delivery identities

#### Scenario: P11-S3 — Delivery idempotency key

- **Given** a delivery with `idempotency_key`
- **When** inserting a second delivery with same key
- **Then** unique constraint rejects
- **And** dispatcher treats conflict as already-scheduled

#### Scenario: P11-S4 — Inbound idempotency keys

- **Given** workspace scope `materialize` and key `K`
- **When** first BeginIdempotent stores request_hash and response_ref
- **Then** second call with same key returns prior response_ref
- **And** same key different request_hash conflicts per policy
- **And** unique `(workspace_id, scope, key)` enforced

## Implementation Instructions

1. `OutboxRepo.Enqueue(tx, event)` must accept ambient transaction/Querier.
2. Wire Phase 5 publish and Phase 7/9 state changes to enqueue:
   - `curriculum.created`, `plan_revision.published`, `materialization.requested|ready|failed`, `materialized_item.superseded`, `plan_change.proposed` as applicable
3. `WebhookEndpointRepo` CRUD; secrets only as `secret_ref` pointers—never raw HMAC secrets in plaintext if avoidable (if stored, document encryption-at-rest expectation; prefer secret manager ref).
4. `WebhookDeliveryRepo` claim/lease pattern similar to Phase 8 for pending deliveries.
5. Dispatcher worker interface with HTTP client injected; tests use fake HTTP server.
6. `IdempotencyRepo` for inbound keys.
7. Mark `published_at` on outbox when all required deliveries terminal **or** when event is published to bus—document chosen semantics (at minimum: published_at set when dispatcher successfully finished fanout attempt cycle).
8. Payload JSON must not include credential material; subject_refs ok.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P11-E1 | draft graph | publish in UoW with outbox | event row exists iff revision published; forced outbox error aborts publish |
| P11-E2 | endpoint+event | dispatch with flaky HTTP then success | attempt_count≥2; final delivered; no dup endpoint/event |
| P11-E3 | duplicate idempotency_key | insert | conflict |
| P11-E4 | inbound key | two begins | replay safe; hash mismatch path |

```bash
cd curriculum-studio && go test ./internal/repo/... ./internal/outbox/... -count=1 -race -run 'Outbox|Webhook|Idempotency|Publish'
cd curriculum-studio/db && python3 -m pytest tests -q -k 'webhook or idempotency'
```

## Anti-Cheating Audit

- Outbox insert must share the domain transaction—tests must fail publish when outbox constrained.
- Do not use channel-only events without DB rows in default path.
- Webhook “delivered” must record DB status, not only HTTP 200 mock without update.
- secret_ref must not print secrets in test logs.
- At-least-once means duplicate delivery *attempts* possible; uniqueness is on delivery identity, not single HTTP call.

## Completion Gate

- [ ] P11-S1–P11-S4 / P11-E1–P11-E4 green
- [ ] Event type strings match product plan / SCHEMA.md
- [ ] Race tests on claim/dispatch
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 5; completes after wiring 7–9
- **Rollback:** stop dispatcher; retain outbox rows
- **Risk:** poison payloads—keep payload schema version field early
