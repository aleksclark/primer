# Phase 2: Closed-enum parity without second DTO source

## Goal

Mechanically enforce shared closed-enum **wire string** parity across protobuf
enum suffixes, OpenAPI string enums, and DB CHECK constraints — without creating
a third hand-maintained DTO/enum package.

**Depends on:** Phase 1
**Duration guess:** 1–2 days

## BDD Success Criteria

#### Scenario: P2-S1 — Canonical closed sets match across three sources

- **Given** committed proto enums, OpenAPI components enums, and DB CHECK values
  for at least: materialization status, materialized item kind, revision state,
  item lock state, event type strings, error codes, curriculum API status mapping
- **When** the parity gate runs
- **Then** every wire string in the crosswalk tables appears in all required
  layers (with documented API↔DB maps such as `archived` ⇔ `retired`)
- **And** proto values equal `PREFIX_` + `UPPER_SNAKE(wire)` (or the documented
  event-type transform)

#### Scenario: P2-S2 — Drift fails closed

- **Given** a planted extra OpenAPI enum value not in proto/DB
- **When** the parity gate runs
- **Then** it exits non-zero naming the set and value
- **And** after revert, the gate is green

#### Scenario: P2-S3 — No third enum catalog is introduced

- **Given** the parity tool implementation
- **When** a reviewer inspects sources of truth
- **Then** the tool **extracts** from `.proto`, OpenAPI YAML (baseline or
  emitted), and SQL CHECK / SCHEMA.md — it does not own a parallel
  `enums.yaml` that humans edit as primary
- **And** CI fails if a forbidden path `**/enum-catalog.yaml` (or similar) is
  added as a write-source

#### Scenario: P2-S4 — Curriculum status mapping is explicit

- **Given** DB `draft|active|retired` and API `active|archived`
- **When** parity runs with mapping rules file **derived from crosswalk**
  (checked-in **mapping rules**, not value lists)
- **Then** `archived` maps only to `retired`; `draft` is storage-only unless
  exposed by a documented exception

## Implementation Instructions

1. **Extractor design** (`curriculum-studio/tools/contract-gates/enum_parity.py`
   or Go equivalent):

   - Parse proto enums via `buf build` image or regex+protoc descriptor
   - Parse OpenAPI `components.schemas.*.enum`
   - Parse DB from `curriculum-studio/db/migrations/*.sql` CHECK constraints
     and/or a golden extract committed **as test fixture output** regenerated
     from SQL (prefer live parse)

2. **Mapping rules (meta only)** — small checked-in
   `tools/contract-gates/enum_mappings.yaml`:

   ```yaml
   # NOT the enum values — only cross-layer renames / storage-only values
   curricula.status:
     api_to_db: { archived: retired }
     storage_only: [draft]
   event_types:
     proto_prefix: EVENT_TYPE_
     wire_transform: proto_suffix_to_dot_snake  # CURRICULUM_CREATED -> curriculum.created
   ```

3. **Sets required (minimum)**
   From crosswalk + SCHEMA.md:

   - `MaterializationStatus` / `materialization_runs.status`
   - `MaterializedItemKind` / `materialized_items.kind`
   - `MaterializedItemStatus` / `materialized_items.status`
   - `ItemLockState` vs `locked` boolean mapping rule
   - `RevisionState` / `plan_revisions.status`
   - `EventType` ↔ event type wire names
   - `ErrorCode` OpenAPI ↔ proto
   - Resource/export formats if exposed on both surfaces

4. **Wire into** `make contracts-parity` and `validate.sh` optional step or
   sibling script `scripts/parity.sh`.

5. **Post-Huma note:** after Phase 7, OpenAPI input to parity is **emitted**
   OpenAPI, not hand YAML; hand YAML remains baseline artifact only. Phase 2
   must accept a `--openapi path` flag for that handoff.

6. **Do not** regenerate DB or rewrite proto values casually; fix drift by
   changing the incorrect layer with review.

### Focused verification

- Run parity green on current tree
- Plant OpenAPI-only value → red → revert
- Confirm no new DTO package

## End-to-End Test Plan

| ID | Setup | Action | Assert |
| --- | --- | --- | --- |
| E2-01 | current contracts+db | `make contracts-parity` | exit 0; prints set counts |
| E2-02 | temp edit OpenAPI enum | parity | non-zero; message includes value |
| E2-03 | grep for hand enum catalog | CI policy | no primary `enum-catalog` values file |
| E2-04 | mapping rules only | unit test archived↔retired | passes |

## Anti-Cheating Audit

- Hard-coding expected enums inside the test instead of extracting from sources
- “Parity” that only compares proto to OpenAPI and skips DB
- Checking in a full values YAML and generating all three from it (second/third SoT)
- Marking green while ignoring known crosswalk sets
- Boolean lock vs enum lock silently skipped

## Completion Gate

- [ ] P2-S1…P2-S4 pass
- [ ] E2-01…E2-04 evidence (including planted red)
- [ ] `make contracts-parity` documented in contracts README
- [ ] No third DTO/enum source introduced
- [ ] Mapping rules contain transforms only

## Dependencies and rollback

- **Depends on:** Phase 1
- **Unblocks:** Phases 6–7 (handoff), 10
- **Rollback:** remove parity tool; keep sources unchanged
