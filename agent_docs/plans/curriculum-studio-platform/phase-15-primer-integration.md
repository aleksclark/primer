# Phase 15: Primer integration

**File:** `phase-15-primer-integration.md`
**Depends on:** Phases 12–14; credential-free test Identity for local E2E; **I9 / IB5 Primer-owned service principals** for production machine JWT
**Duration guess:** 6–9 days
**Handoff wave:** see index orchestrator map

## Goal

Expose Primer integration surface: gRPC CurriculumIntegrationService Materialize/GetMaterializationBundle, service JWT auth, MaterializationContext→Bundle mapping including session_spec and tutor_context, idempotent replay, and negative proof of no LMS DB access. Completes product roadmap Phase 3 integration core.

## Scope

### In scope

- gRPC server in studio process (shared port via cmux or `STUDIO_GRPC_PORT`)
- Generate Go stubs via `buf generate` into build dir / module
- Map context snapshot into materialization_runs.input_snapshot
- Bundle builder from items + artifacts metadata
- Service auth middleware scopes
- Integration tests with bufconn or real port
- Contract conformance tests for golden context/bundle
- Failure injection: invalid context, unauthorized, provider fail already covered
- Document LMS client adapter package location **optional** thin client under `curriculum-studio/internal/integration/primer` only — not LMS code changes required for Studio gate

### Out of scope / YAGNI

- LMS production cutover PR (separate)
- Studio→LMS import push
- Live Identity service credentials; credential-free E2E may use a test Identity-issued JWT, but production machine auth requires I9/IB5 service-principal `client_credentials`

## BDD Success Criteria

#### Scenario: P15-S1 — Materialize RPC success

- **Given** service JWT + published revision + scripted workflow
- **When** call Materialize with MaterializationContext
- **Then** returns MaterializationBundle
- **And** bundle id matches run
- **And** context_fingerprint set
- **And** sessions/session_specs present when generated

#### Scenario: P15-S2 — Wrong audience denied

- **Given** JWT aud=primer-lms
- **When** Materialize
- **Then** unauthenticated/permission denied
- **And** no run created

#### Scenario: P15-S3 — GetMaterializationBundle

- **Given** existing ready run
- **When** GetMaterializationBundle
- **Then** same bundle bytes/fields
- **And** authz workspace via service grant/membership

#### Scenario: P15-S4 — Idempotent replay

- **Given** same context fingerprint + idempotency key/metadata
- **When** Materialize twice
- **Then** same bundle id
- **And** single run

#### Scenario: P15-S5 — No LMS DB

- **Given** Studio configuration
- **When** static analysis + runtime config
- **Then** no LMS DATABASE_URL used
- **And** no queries to LMS tables
- **And** test fails if import path to server/internal/db appears

#### Scenario: P15-S6 — tutor_context and session_spec

- **Given** scripted fixtures include session_spec items
- **When** Materialize
- **Then** bundle includes session_spec kind entries
- **And** tutor_context fields populated per proto

## Implementation Instructions

1. Keep protobuf as SoT; do not duplicate bundle in OpenAPI.
2. OpenAPI getMaterializationBundle may return summary only.
3. Service membership: workspace grant table via integration_identities or service role membership seed.
4. Golden vectors under `curriculum-studio/internal/integration/testdata/`.
5. Negative authz tests mandatory.
6. X-Service-Token alias optional dual-accept behind flag, default off in new Studio.

## End-to-End Test Plan

#### P15-E1 — gRPC materialize E2E

- **Setup:** server process grpc+db+scripted
- **Action:** Materialize
- **Assert:**
  - bundle ready
  - items linked
- **Command:** `make studio-test`

#### P15-E2 — Aud rejection

- **Setup:** wrong jwt
- **Action:** RPC
- **Assert:**
  - error codes
- **Command:** `go test ./internal/integration -run Aud`

#### P15-E3 — Get bundle

- **Setup:** ready run
- **Action:** Get
- **Assert:**
  - match
- **Command:** `go test ./internal/integration -run GetBundle`

#### P15-E4 — Replay

- **Setup:** double Materialize
- **Action:** compare ids
- **Assert:**
  - idempotent
- **Command:** `go test ./internal/integration -run Replay`

#### P15-E5 — No cross-DB

- **Setup:** grep+test
- **Action:** ensure isolation
- **Assert:**
  - no server/internal/db import
  - DSN name studio
- **Command:** `go test ./internal/integration -run NoCrossDB`

#### P15-E6 — session_spec

- **Setup:** fixture
- **Action:** Materialize
- **Assert:**
  - proto fields set
- **Command:** `go test ./internal/integration -run SessionSpec`

## Anti-Cheating Audit

- gRPC handler not returning hard-coded bundle without DB run
- Must use real workflow path or explicitly documented stub only in unit tests not E2E
- No LMS schema migrations
- Fingerprint stability tests required
- Service auth not disabled

## Completion Gate

- [ ] Proto RPC implemented
- [ ] P15-E* green
- [ ] Cross-DB negative proven
- [ ] Product integration Phase 3 core satisfied (credential-free)


## Dependencies

- Upstream: Phases 12–14
- Sibling tracks: schema (`curriculum-studio/db`), contracts (`curriculum-studio/contracts`), identity (credential-free test Identity is separate; production machine JWT requires I9/IB5)
- Downstream consumers: later phases listed in index

## Rollback

- Revert the phase PR(s); drop any additive migrations only via forward-fix sibling db track (never destructive rollback in prod without backup).
- Feature flags: prefer `STUDIO_*` env gates over half-applied routes.
- SPA: revert `curriculum-studio/web` routes; keep generated client in sync with OpenAPI.
