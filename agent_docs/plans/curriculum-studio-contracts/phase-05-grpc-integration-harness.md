# Phase 5: gRPC integration service harness

## Goal

Stand up a real loopback gRPC server implementing `CurriculumIntegrationService`
with **contract adapters/harnesses** (in-memory or test DB fixtures) so the
generated Go client exercises success, precondition, auth metadata, and
idempotency paths — without implementing full agent materialization.

**Depends on:** Phase 4
**Duration guess:** 3–5 days

## BDD Success Criteria

#### Scenario: P5-S1 — Generated client Materialize + GetMaterialization

- **Given** loopback gRPC server with harness store
- **When** client calls `Materialize` with valid context + idempotency key
- **Then** response has materialization id and status `requested` or `running`
- **And** `GetMaterialization` returns the same id/status progression

#### Scenario: P5-S2 — Bundle precondition until ready

- **Given** a run not yet `ready`
- **When** client calls `GetMaterializationBundle`
- **Then** gRPC status maps to failed precondition and `ErrorDetail.code`
  is `failed_precondition` or `materialization_failed` per harness rules
- **And** after harness marks `ready`, bundle returns with session specs and
  Struct `tutor_context`

#### Scenario: P5-S3 — Idempotent Materialize

- **Given** a prior Materialize with key K
- **When** client retries Materialize with same key and equivalent context
- **Then** same materialization id is returned
- **And** conflicting body with same key yields `idempotency_key_conflict`

#### Scenario: P5-S4 — Published revision reads and validation RPC

- **Given** harness seeded published revision + graph
- **When** client calls `GetPublishedRevision`, `GetPlanGraph`, `ValidateRevision`
- **Then** immutable revision payload returns; draft ids are not found
- **And** validation report structure matches proto

#### Scenario: P5-S5 — Auth metadata required

- **Given** server interceptor requiring Bearer JWT (fixture JWKS)
- **When** client omits metadata
- **Then** unauthenticated error
- **And** with valid `aud=curriculum-studio` token, call succeeds

#### Scenario: P5-S6 — PullEvents / AcknowledgeEvent harness

- **Given** outbox fixtures
- **When** client PullEvents then AcknowledgeEvent
- **Then** events use closed EventType wire mapping; ack is durable in harness

## Implementation Instructions

1. **Server package** `internal/grpcapi`:
   - Register `CurriculumIntegrationServiceServer` generated interface
   - Unary interceptor: extract `authorization` metadata, validate JWT via
     `internal/authn` fixture JWKS
   - Map domain harness errors → `status.Error` + typed details

2. **Harness store** `internal/grpcapi/harness`:
   - In-memory revisions, runs, items, events, idempotency records
   - Status state machine for LRO
   - Seed helpers for tests

3. **Test binary / `go test`**:
   - `bufconn` or `localhost:0` server
   - Use **generated client package only** (no raw grpc invoke of hand stubs in
     consumer tests — server tests may use generated server APIs)

4. **Wire enums** through shared encode helpers in `internal/boundary` that call
   proto enums — not a parallel string catalog.

5. **Do not** implement real LLM workflows; returning fixture bundle JSON is OK
   if Struct round-trips.

6. **Platform interface:** document `grpcapi.Register(server, deps)` for the
   future studio binary.

### Focused verification

- `go test ./internal/grpcapi/... -count=1`
- Client tests under `clients/go-grpc` or `internal/grpcapi/e2e`

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E5-01 | bufconn + fixture JWT | Materialize + Get | ids match; status progresses |
| E5-02 | run not ready | GetBundle | precondition + ErrorCode |
| E5-03 | ready run | GetBundle | Struct tutor_context deep equal fixture |
| E5-04 | idempotency | double Materialize | same id; conflict path tested |
| E5-05 | no auth | Materialize | unauthenticated |
| E5-06 | seed revision | GetPublishedRevision/Graph/Validate | success |
| E5-07 | events | Pull + Ack | types match crosswalk strings via enum |

## Anti-Cheating Audit

- Hard-coded always-ready bundle skipping LRO
- Tests calling server impl methods instead of generated client
- Auth interceptor disabled in production build tags
- Fake success without registering real generated service descriptor
- Skipping idempotency conflict case

## Completion Gate

- [ ] P5-S1…P5-S6 pass
- [ ] E5-01…E5-07 green
- [ ] Server uses generated pb service registration
- [ ] No business agent code required for green
- [ ] Fixture JWKS only; live Identity marked not done

## Dependencies and rollback

- **Depends on:** Phase 4
- **Unblocks:** Phases 8–9, 11
- **Rollback:** remove grpcapi harness; keep client package
