# Phase 3: Hard-type qualification spikes

## Goal

Run throwaway-but-retained qualification spikes that prove generators and
runtime codecs correctly handle the hardest Curriculum Studio wire shapes
**before** locking client stacks and mass generation. Emit
**PROCEED / CONDITIONAL PROCEED / STOP** per shape.

**Depends on:** Phase 1 (layout, pins)
**Duration guess:** 2–4 days
**Parallel:** may run beside Phase 2

## BDD Success Criteria

#### Scenario: P3-S1 — Nullable vs optional / absent

- **Given** fixture messages/fields that distinguish null, absent, and empty
  string (OpenAPI nullable + proto optional/presence)
- **When** emit → generate → round-trip through REST JSON and gRPC
- **Then** clients preserve semantic differences required by Studio
  (e.g. unset window.end vs explicit null policy documented)
- **And** gate records PROCEED or STOP with evidence path

#### Scenario: P3-S2 — Timestamp and duration encodings

- **Given** `google.protobuf.Timestamp` fields and any duration-like minutes
  fields (`available_minutes` int vs timestamps)
- **When** JSON/REST (RFC3339) and gRPC binary codecs run
- **Then** UTC normalization is stable; invalid timestamps fail typed errors
- **And** double-emit is deterministic after normalization

#### Scenario: P3-S3 — Typed application errors

- **Given** `ErrorCode` + `ErrorDetail` on REST problem+json and gRPC status
- **When** harness returns each major code
- **Then** generated clients surface machine code + location, not only HTTP/gRPC
  status strings
- **And** unknown codes fail closed or map to `internal` per written rule

#### Scenario: P3-S4 — Pagination envelopes

- **Given** REST `limit`/`offset`/`PageMeta` and gRPC `PageRequest`/`PageResponse`
- **When** multi-page list fixtures run
- **Then** clients can walk all pages; empty next token/offset end is clear
- **And** transports remain non-unified (no fake domain Page type shared)

#### Scenario: P3-S5 — Long-running materialization

- **Given** Materialize → status poll → bundle fetch precondition
- **When** harness moves `requested → running → ready` and failure path
- **Then** client does not treat create response as bundle; GetBundle fails until
  ready with `failed_precondition` / matching ErrorCode
- **And** idempotent Materialize with same key returns same run id

#### Scenario: P3-S6 — `google.protobuf.Struct` tutor_context and event data

- **Given** `MaterializationBundle`/`MaterializedItem.tutor_context` and
  `DomainEvent.data` as Struct
- **When** nested JSON objects round-trip gRPC and (if exposed) REST authoring
  bundle **view**
- **Then** structurally equal JSON after parse; no silent key drop for known
  fixture keys
- **And** if a generator cannot support Struct, gate is STOP or CONDITIONAL with
  explicit adapter (e.g. `map[string]any` / JSON raw) documented

#### Scenario: P3-S7 — Spike outcomes gate later phases

- **Given** spike report `tools/contract-gates/spikes/REPORT.md`
- **When** Phase 4 or 6 starts
- **Then** each required shape is PROCEED or CONDITIONAL with listed mitigations
- **And** any STOP blocks dependent generation adoption

## Implementation Instructions

1. Create `curriculum-studio/tools/contract-gates/spikes/` with:
   - `fixtures/` JSON + proto text fixtures
   - `cmd/spike-runner` or scripts per shape
   - `REPORT.md` template with columns: shape, stack, result, evidence, mitigations

2. **Stacks under test (minimum):**
   - protoc-gen-go + grpc-go (pinned in buf.gen.yaml)
   - openapi-typescript + openapi-fetch (LMS versions as starting pins)
   - optional: oapi-codegen or huma-compatible Go REST client candidate

3. Prefer **in-process / loopback** servers; no Docker dependency for spikes
   unless unavoidable.

4. For each shape: generate twice → compare normalized outputs; call real
   codec path; record unsupported constructs.

5. Do **not** delete spike fixtures after success — they become conformance
   seeds for Phases 5/8/11.

6. STOP examples: generator collapses null/absent; Struct becomes `string` only;
   error bodies untyped `any` without code field.

### Focused verification

- REPORT.md committed with six shapes
- At least one intentional failing fixture proves assertion teeth
- Pins recorded (buf plugins, openapi-typescript version)

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E3-01 | null/absent fixtures | spike runner REST+gRPC | REPORT PROCEED/STOP; assertions on presence |
| E3-02 | timestamp fixtures | spike runner | RFC3339 stable; invalid → typed error |
| E3-03 | error fixtures | spike runner | ErrorCode round-trip |
| E3-04 | pagination fixtures | spike runner | full walk |
| E3-05 | LRO fixtures | spike runner | precondition on early bundle |
| E3-06 | Struct fixtures | spike runner | deep equal JSON |
| E3-07 | double generate | two runs | normalized digest equal |

## Anti-Cheating Audit

- Marking PROCEED from hello-world string fields only
- Mocking codecs instead of generated stubs
- Ignoring Struct by deleting fields from fixtures
- Claiming TS support without compiling generated types under `strict`
- Hiding STOP by not running a stack that will be used in production

## Completion Gate

- [ ] P3-S1…P3-S7 pass
- [ ] E3-01…E3-07 evidence paths linked in REPORT.md
- [ ] No STOP remaining without explicit plan change
- [ ] Pins documented for adopted generators
- [ ] Fixtures retained for later phases

## Dependencies and rollback

- **Depends on:** Phase 1
- **Unblocks:** Phases 4, 6
- **Rollback:** keep REPORT as historical STOP; do not adopt failing generators
