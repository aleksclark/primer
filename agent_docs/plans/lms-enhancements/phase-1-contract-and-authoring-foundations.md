# Phase 1: Contract and authoring foundations

## Purpose

Create one strict, versioned source of truth for activity authoring before expanding instruction, response, and evidence capabilities. Curated curriculum should fail during review or CI, not after publication or on a student's workstation.

## Current need

Primer's Go contract performs substantial semantic validation, but JSON decoding accepts unknown fields. The Linux course adds a separate JSON Schema that rejects unknown fields while leaving check parameters broadly open. The offline validator also infers whether a check describes initial state from whether its path appears in fixtures. This fails for activities that intentionally ask a student to correct an existing fixture, such as capstone debugging.

A second schema hand-maintained beside Go types will drift. More importantly, schema validity alone does not prove that an activity can be completed. Primer needs authoring validation that distinguishes initial conditions, outcomes, and an optional reference solution without allowing arbitrary author-supplied shell verification.

## Scope

### 1. Strict document decoding

- Reject unknown JSON fields with `json.Decoder.DisallowUnknownFields`.
- Add equivalent strictness for YAML node decoding.
- Reject duplicate JSON/YAML keys.
- Keep explicit schema-version dispatch; never interpret an unknown version as the latest.
- Produce errors with document path and field location suitable for authors and CI.

### 2. Canonical JSON Schema

Choose one generation direction and enforce it in CI:

- preferred: generate Draft 2020-12 JSON Schema from dedicated contract metadata and Go types, then add explicit semantic validation for cross-references and safe paths;
- acceptable first step: retain a canonical checked-in schema, add conformance fixtures that must agree with Go decoding/validation, and fail CI on drift.

Define conditional `params` shapes for every check kind. Unknown check kinds and parameters must fail. Preserve the small typed verifier vocabulary; do not add arbitrary shell snippets.

### 3. Explicit staged validation lifecycle

Replace inferred baseline semantics with explicit observation stages:

- fixture assertions prove the materialized starting state;
- task checks are evaluated while their task is active and may intentionally prove an initial condition before mutation;
- final outcomes are required at completion;
- invariants are evaluated at declared boundaries such as fixture, after-task, and final.

Each check declaration states the stages where it is expected and whether it is evidence-bearing or authoring-only. This supports Lesson 20's inspect-before-repair task without pretending the incorrect initial content is a final outcome. The validator evaluates fixture assertions and invariants at materialization, while reference replay records task-scoped observations and final outcomes at their declared stages.

### 4. Reference solution replay

Add an optional authoring-only reference solution outside the published student revision. It may contain a sequence of runner inputs or a test harness fixture, but must never become executable verifier code delivered to students.

CI can:

1. materialize the fixture;
2. prove initial checks and invariants;
3. run the reference solution in the same sandbox/runtime profile;
4. prove required outcomes and post-run invariants;
5. reset and repeat to test determinism where requested.

A course may omit a reference solution, but high-complexity activities and all capstones should require one by authoring policy.

### 5. Course manifest schema

Define a strict `CourseDocument` contract for git-authored course metadata:

- stable slug, title, subject code, and version;
- ordered activity slugs and revision-resolution policy;
- prerequisite relationships;
- evidence and parent-review gates;
- remediation and reinforcement branch references;
- optional modules/capstone markers;
- continuity policy placeholders (`fresh`, `optional_previous`, `required_project`, `portfolio_review`);
- parent-facing description;
- descriptive effort metadata, but no dates as progression requirements;
- no student IDs or mutable assignment state.

Phase 4 persists and executes these semantics; Phase 6 implements non-`fresh` continuity and the final guarded import. Unknown future policy variants fail explicitly rather than being ignored.

### 6. Authoring diagnostics and guidance

Enhance `activity-validate` to report:

- schema and semantic failures;
- initial/outcome/invariant counts;
- runtime capability requirements;
- unreachable tasks or cyclic prerequisites;
- unused checks and hints;
- oversized instruction/check warnings;
- missing reference solution warnings for capstones;
- standards that are unknown or lack an evidence policy.

Warnings should inform craftsmanship without turning arbitrary style preferences into hard validation failures.

## Contract migration

Publish a v2 activity schema only if new serialized fields cannot be safely optional in v1. The server and workstation must support v1 during a documented migration window. Immutable v1 revisions remain runnable under their original interpretation, subject to Phase 0's conservative evidence policy.

Provide a deterministic migration tool for source documents. It should produce reviewable diffs and never republish automatically.

## Implementation slices

1. Add strict decoding and negative fixtures.
2. Define conditional schemas for checks and conformance tests.
3. Add explicit validation applicability to checks.
4. Update materialized validation to respect applicability.
5. Add reference-solution replay behind a validator flag.
6. Define and validate `CourseDocument`.
7. Add authoring warnings and migration tooling.
8. Wire validation into CI and deployment publication.

## Tests

- Unknown and duplicate fields fail in JSON and YAML.
- Every accepted schema fixture is accepted by Go semantic validation.
- Every rejected conformance fixture fails both paths for the intended reason.
- A task-scoped initial-condition check against an existing fixture path is recorded during the inspection task without being mistaken for a final outcome.
- Invariants run at every declared boundary during reference replay.
- Reference replay cannot access network, host home, credentials, or arbitrary verifier execution.
- v1 immutable revisions remain decodable during migration.
- Course manifests reject duplicate order, missing slugs, cycles, and calendar-driven eligibility fields.

## Exit criteria

- There is one documented canonical activity schema process.
- Unknown fields and check parameters fail before publication.
- Full materialized validation and staged reference replay succeed for intentional repair activities without `--no-materialize` workarounds.
- Capstone reference solutions run deterministically in CI.
- The Linux course and existing curriculum activities pass strict validation.
- A strict course manifest is ready for Phase 4 persistence.

## Out of scope

- Rendering instructional blocks.
- Student short-answer input.
- Runtime command instrumentation.
- Persisting or assigning course sequences.
