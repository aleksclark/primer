# Phase 14: Outbox and webhooks

**File:** `phase-14-outbox-webhooks.md`
**Depends on:** Phase 8 for plan events; Phase 11+ for materialization events
**Duration guess:** 4–6 days
**Handoff wave:** see index orchestrator map

## Goal

Operate durable domain event delivery: outbox dispatcher, webhook endpoint CRUD, signed HTTP deliveries, retries, idempotency, workspace isolation, and restart-safe backlog drain. Studio remains correct with zero subscribers.

## Scope

### In scope

- webhook_endpoints + webhook_deliveries APIs
- HMAC signature header scheme documented
- worker claims outbox FOR UPDATE SKIP LOCKED
- retry backoff; dead-letter after N
- list deliveries server-side
- event types exact crosswalk strings
- restart drain test

### Out of scope / YAGNI

- External event bus (NATS/Kafka) requirement
- Primer subscriber implementation inside LMS (consumer track)

## BDD Success Criteria

#### Scenario: P14-S1 — Outbox durable types

- **Given** actions that emit events
- **When** inspect outbox
- **Then** types include curriculum.created, plan_revision.published, materialization.* , materialized_item.superseded, plan_change.proposed as implemented

#### Scenario: P14-S2 — Signed delivery success

- **Given** endpoint active pointing at test receiver
- **When** event enqueued
- **Then** delivery delivered
- **And** signature validates
- **And** receiver got JSON body

#### Scenario: P14-S3 — Idempotent retry

- **Given** receiver succeeds after duplicate attempt simulation
- **When** retry same event
- **Then** unique (endpoint,event) respected
- **And** receiver dedupe key stable

#### Scenario: P14-S4 — Cross-tenant isolation

- **Given** endpoint on W1
- **When** event from W2
- **Then** W1 endpoint not invoked for W2 events

#### Scenario: P14-S5 — Receiver down and worker restart

- **Given** receiver 500s then recovers; worker restarted mid-backoff
- **When** process
- **Then** eventually delivered or marked failed per policy
- **And** no loss of outbox row
- **And** restart continues drain

## Implementation Instructions

1. Signature: `X-Studio-Signature: v1=<hmac-sha256 hex>` over body+timestamp.
2. Delivery timeout bounded; contextual cancellation.
3. Admin UI optional later; API sufficient.
4. Metrics: pending outbox age.

## End-to-End Test Plan

#### P14-E1 — Enqueue types

- **Setup:** publish+mat
- **Action:** query outbox
- **Assert:**
  - expected types
- **Command:** `make studio-test`

#### P14-E2 — Deliver signed

- **Setup:** httptestreceiver
- **Action:** emit event
- **Assert:**
  - delivered
  - sig ok
- **Command:** `go test ./internal/outbox -run Deliver`

#### P14-E3 — Isolation

- **Setup:** two workspaces
- **Action:** emit both
- **Assert:**
  - routing correct
- **Command:** `go test ./internal/outbox -run Tenant`

#### P14-E4 — Restart drain

- **Setup:** stop receiver; enqueue; restart server; start receiver
- **Action:** wait
- **Assert:**
  - delivered
- **Command:** `go test ./internal/outbox -run Restart`

## Anti-Cheating Audit

- Worker not best-effort goroutine without DB outbox
- Signature secret not logged
- Deliveries not claimed successful without HTTP 2xx
- No global endpoint receiving all tenants

## Completion Gate

- [ ] P14 scenarios green
- [ ] OpenAPI webhooks ops done
- [ ] Docs for signature verification


## Dependencies

- Upstream: Phase 8 for plan events; Phase 11+ for materialization events
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (when leaving test auth)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
