# Phase 6: Authoring boundary DTOs and offline Huma emission

## Goal

Introduce Huma v2 authoring edge boundary types and handlers sufficient to
**offline-emit** OpenAPI 3.1 from the same registration path the server will
use — modeled on LMS `server/cmd/openapi-gen` — covering the Curriculum Studio
authoring surface without restating Primer integration payloads.

**Depends on:** Phases 2–3
**Duration guess:** 4–6 days
**Parallel with:** Phase 4/5 after spikes

## BDD Success Criteria

#### Scenario: P6-S1 — Offline OpenAPI emission without listener or DB

- **Given** `curriculum-studio/cmd/openapi-gen` (or equivalent)
- **When** it constructs the Huma API with nil/fakes for repos like LMS
- **Then** it writes OpenAPI 3.1 YAML to a gitignored path
- **And** process does not bind ports or require Postgres

#### Scenario: P6-S2 — Emission uses production registration function

- **Given** `internal/api.New` (name flexible) registers all authoring routes
- **When** server main and openapi-gen both call the same `New`
- **Then** operationIds/paths in emitted spec match runtime routes
- **And** a test fails if gen registry diverges (shared test hook)

#### Scenario: P6-S3 — Boundary DTOs are explicit wire types

- **Given** Huma input/output structs for workspaces, curricula, revisions,
  graph, standards, resources, materializations (authoring subset), items,
  locks, exports, webhooks, events list, health
- **When** OpenAPI components are emitted
- **Then** required/optional/nullable match spike rules
- **And** no ORM/DB models appear as exported components

#### Scenario: P6-S4 — Non-overlap preserved in emitted spec

- **Given** emitted OpenAPI
- **When** ownership scanner runs
- **Then** no `MaterializationContext` full Primer schema; authoring materialize
  request remains the thin `AuthoringMaterializeRequest` subset
- **And** integration-only RPCs are absent (gRPC-only)

#### Scenario: P6-S5 — Semantic compatibility target vs hand baseline

- **Given** hand `openapi/v1/curriculum-studio.yaml` baseline
- **When** emitted spec is diffed with openapi-diff / spectral-aware compare
- **Then** report lists gaps; Phase 6 may still be incomplete on 100% path parity
  but must cover core resources and document remaining path delta
- **And** enums match parity gate (emitted path)

## Implementation Instructions

1. **Mirror LMS pattern** (`server/cmd/openapi-gen/main.go`):

   ```go
   builders["studio"] = func() (huma.API, http.Handler) {
     return studioapi.New(nil, studioapi.Options{})
   }
   ```

   Prefer Studio-local cmd under `curriculum-studio/cmd/openapi-gen` to avoid
   coupling to LMS module — unless go.mod strategy from Phase 1 says otherwise.

2. **Handler coverage strategy:**
   Implement real Huma registrations with harness repos returning fixed data so
   types drive OpenAPI. Full CRUD behavior can be thin; focus on signatures,
   status codes, headers (`Idempotency-Key`, `If-Match`), problem+json errors.

3. **DTO location:** `internal/api/dto/` or colocated `*_api.go` types — these
   **are** the authoring contract source post-handoff. Map to DB in repo layer
   later (DB plan / platform).

4. **Enum types:** Go string types with huma enum tags whose values are the wire
   strings; parity gate compares emission ↔ proto ↔ DB (Phase 2 flag).

5. **Pagination:** `limit`/`offset` query + `PageMeta` body fields matching
   baseline names (`totalCount`, etc.).

6. **Materialization authoring:**
   POST create run with generic learner profile; GET status/items/lock/unlock;
   GET bundle may return authoring view **or** 501/redirect note — must not
   embed full proto bundle schema as OpenAPI component tree. Prefer opaque
   artifact ref + summary fields if needed.

7. **Emission output:** `contracts/.tmp/openapi.emitted.yaml` gitignored.

8. **Do not** delete hand baseline in this phase.

### Focused verification

- `go run ./cmd/openapi-gen -out ...`
- validate emitted with openapi-spec-validator
- parity `--openapi emitted`
- route count ≥ baseline major resources

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E6-01 | no DB | openapi-gen | file written; openapi 3.1.0 |
| E6-02 | gen + server package | same New() used | test asserts equality of operation list |
| E6-03 | emitted YAML | ownership scanner | no Primer context schema |
| E6-04 | emitted YAML | enum parity | green |
| E6-05 | emitted vs baseline | diff report | artifact stored; critical paths present |

## Anti-Cheating Audit

- Emitting hand-copied YAML from baseline instead of Huma reflection
- Separate route table for gen only
- Using `map[string]any` everywhere to “pass” emission
- Claiming parity while still pointing parity at hand YAML only
- Port bind during gen

## Completion Gate

- [ ] P6-S1…P6-S5 pass
- [ ] E6-01…E6-05 evidence
- [ ] openapi-gen documented in Makefile
- [ ] Hand baseline still present and authoritative for consumers until Phase 7
- [ ] Gap list for missing ops tracked

## Dependencies and rollback

- **Depends on:** Phase 2–3
- **Unblocks:** Phase 7–8
- **Rollback:** remove api package/emission; keep baseline
