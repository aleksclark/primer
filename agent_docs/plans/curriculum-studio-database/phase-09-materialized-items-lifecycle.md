# Phase 9: Materialized items lifecycle

**Depends on:** Phase 8
**Duration guess:** 4–5 days
**Tables:** `materialized_items`, `materialized_item_edits`, `assessment_supports`

## Goal

Persist generated instructional items with provenance to plan nodes and runs, support author edits, explicit lock/unlock, supersession chains, and enforce assessment publication support requirements via DB triggers through repository APIs.

## BDD Success Criteria

#### Scenario: P9-S1 — Item create with provenance

- **Given** a run and plan revision (matching)
- **When** creating item kind `lesson` with body JSON, optional unit/outcome ids, provenance object
- **Then** row stores; run/revision mismatch rejected by trigger

#### Scenario: P9-S2 — Locked item overwrite rejected

- **Given** an item with `locked=true`
- **When** attempting body/status overwrite (rematerialize path)
- **Then** DB/repo rejects
- **And** edit insert path also rejects while locked

#### Scenario: P9-S3 — Explicit unlock allows edit

- **Given** locked item
- **When** unlock sets `locked=false` clearing `locked_at`
- **Then** subsequent edits succeed and append `materialized_item_edits`

#### Scenario: P9-S4 — Edit history

- **Given** editable item
- **When** applying patches from subject_ref
- **Then** edit rows accumulate ordered by `created_at`

#### Scenario: P9-S5 — Supersession chain

- **Given** item A
- **When** creating item B with `supersedes_item_id=A` and marking A `superseded`
- **Then** chain query returns B→A
- **And** outbox event type may be emitted in Phase 11 integration

#### Scenario: P9-S6 — Assessment publish requires support

- **Given** assessment item without rubric/answer_key support row
- **When** transitioning status to `published`
- **Then** trigger rejects
- **When** support link to rubric or answer_key exists
- **Then** publish succeeds
- **And** support kind CHECK rejects linking non-support kinds

## Implementation Instructions

1. `MaterializedItemRepo`, `ItemEditRepo`, `AssessmentSupportRepo`.
2. Methods: Create, Get, ListByRun, Lock, Unlock, ApplyEdit, Supersede, Publish.
3. Map trigger errors to typed `ErrLocked`, `ErrAssessmentSupportRequired`, `ErrRunRevisionMismatch`.
4. Provenance JSON must remain an object; encourage keys like `stage_key`, `model`, `prompt_hash` without enforcing LLM vendor.
5. Workspace isolation via run/workspace join.
6. Concurrency: lock vs edit race—final state respects lock trigger.

## End-to-End Test Plan

| ID | Setup | Action | Assertions |
| --- | --- | --- | --- |
| P9-E1 | run+revision | create lesson | provenance; mismatch fails |
| P9-E2 | locked item | overwrite body | error |
| P9-E3 | unlock then edit | edit row | success |
| P9-E4 | multiple edits | list history | order |
| P9-E5 | supersede | chain | statuses |
| P9-E6 | assessment±support | publish | fail then pass; bad support kind fails |

```bash
cd curriculum-studio && go test ./internal/repo/... -count=1 -run 'Item|Assessment|Lock|Supersede'
cd curriculum-studio/db && python3 -m pytest tests -q -k 'locked_item or assessment_publish'
```

## Anti-Cheating Audit

- Lock protection must hold even with raw SQL update attempts in tests.
- Do not implement lock only in API middleware.
- Assessment support must use `assessment_supports` table, not a boolean flag on the assessment row alone.
- Supersession must not delete prior item history by default.

## Completion Gate

- [ ] P9-S1–P9-S6 / P9-E1–P9-E6 green
- [ ] Typed errors
- [ ] Anti-cheating audit clean

## Dependencies and rollback

- **Depends on:** Phase 8 (runs/stages exist)
- **Enables:** Phase 10 exports; Phase 11 item events
