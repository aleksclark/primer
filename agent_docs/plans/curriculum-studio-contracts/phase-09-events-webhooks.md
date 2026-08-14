# Phase 9: Domain events and webhook envelopes

## Goal

Prove domain-event and webhook **envelopes** end-to-end: protobuf `DomainEvent`
ownership, closed event type strings, UI event read + webhook CRUD on authoring
REST, pull/ack on integration gRPC, and delivery header conventions — with
harness outbox, not a full production bus.

**Depends on:** Phases 5, 8
**Duration guess:** 2–4 days

## BDD Success Criteria

#### Scenario: P9-S1 — DomainEvent envelope fields stable

- **Given** harness emits `plan_revision.published` (and other closed types)
- **When** gRPC `PullEvents` and REST workspace events list return data
- **Then** envelope includes id, type, workspace_id, optional resource ids,
  occurred_at, and Struct/JSON data
- **And** type wire strings match crosswalk / parity gate

#### Scenario: P9-S2 — Event types closed set

- **Given** the seven product-plan event names
- **When** unknown type is injected in fixture
- **Then** server rejects publish to outbox
- **And** clients only expose known enum/string union

#### Scenario: P9-S3 — Webhook CRUD and delivery attempt records

- **Given** authoring client with workspace admin token
- **When** create/list/update/delete webhook endpoint and list deliveries
- **Then** REST shapes match emitted OpenAPI
- **And** harness delivery records store http_status/attempt/timestamps
- **And** POST to subscriber uses documented headers (e.g. event id, signature
  placeholder interface owned with platform secrets — signature algorithm
  documented; test may use test secret)

#### Scenario: P9-S4 — PullEvents and AcknowledgeEvent

- **Given** consumer id from service subject
- **When** pull pages events then ack
- **Then** acked events are not redelivered to that consumer
- **And** at-least-once redelivery occurs if ack omitted (harness policy test)

#### Scenario: P9-S5 — Non-overlap: event payload richness

- **Given** OpenAPI UI event read model
- **When** compared to proto DomainEvent
- **Then** OpenAPI may use shared envelope fields + type name string but must
  not redefine Primer-only materialization payload schemas inside event data
  beyond free-form object/Struct equivalent

## Implementation Instructions

1. **Outbox harness** in grpcapi/api shared store:
   - append-only events
   - consumer offsets / ack table in memory

2. **Emit events** from harness mutations (publish revision, materialize
   status changes) to keep E2E realistic.

3. **Webhook dispatcher** (optional goroutine in test): POST JSON envelope to
   an `httptest` test receiver; record delivery.

4. **Headers (document in contracts README):**
   - `Content-Type: application/json`
   - `X-Curriculum-Studio-Event-Id`
   - `X-Curriculum-Studio-Event-Type`
   - `X-Curriculum-Studio-Signature` (HMAC placeholder; key from platform secrets
     interface — do not invent KMS)

5. **REST list events** supports type filter + pagination.

6. **LMS interface:** LMS plan consumes gRPC pull or webhooks via generated
   client only.

### Focused verification

- event type parity
- pull/ack tests
- webhook receiver test

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E9-01 | publish revision harness | PullEvents | type `plan_revision.published` |
| E9-02 | all event types seeded | parity + list | seven names present |
| E9-03 | webhook CRUD + mock receiver | trigger delivery | delivery row + headers |
| E9-04 | pull/ack | second pull | empty after ack; redelivery without ack |
| E9-05 | REST list events | generated TS client | envelope decodes |

## Anti-Cheating Audit

- Returning empty event lists always green
- Webhook CRUD without delivery attempt proof
- OpenAPI restating full bundle inside event schema
- Ack that does not change pull results
- Unsigned webhooks claimed “production ready” without documenting residual risk

## Completion Gate

- [ ] P9-S1…P9-S5 pass
- [ ] E9-01…E9-05 evidence
- [ ] Event header docs in contracts README
- [ ] Parity includes event types
- [ ] LMS interface note updated

## Dependencies and rollback

- **Depends on:** Phase 5, 8
- **Unblocks:** Phase 11
- **Rollback:** disable dispatcher; keep envelope types
