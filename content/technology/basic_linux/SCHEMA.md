# Activity and course schema process

## Canonical sources

| Artifact | Role |
|---|---|
| Go types in `server/internal/studentclient/contracts` | Semantic source of truth for decoding, staged validation, and publish |
| `content/technology/basic_linux/activity.schema.json` | Authoring JSON Schema (Draft 2020-12) checked into git |
| `content/technology/basic_linux/course.schema.json` | Course manifest JSON Schema |
| `server/internal/studentclient/contracts/testdata/conformance/` | Fixtures that must agree between Go validation and JSON Schema |

Phase 1 keeps a checked-in schema and enforces agreement via conformance tests.
Generating schema from Go remains preferred later; until then, CI must fail on drift.

## Strict decoding

- JSON: `DisallowUnknownFields`, duplicate-key rejection, single value only.
- YAML: `KnownFields(true)`, duplicate-key rejection.
- Unknown `schemaVersion` values are rejected; they are never interpreted as latest.

## Check stages

Each check may declare `stages`: `fixture`, `task`, and/or `final`.

| Stage | When evaluated |
|---|---|
| `fixture` | Immediately after fixture materialization |
| `task` | While a referencing task is active (may describe initial conditions) |
| `final` | At activity completion |

Omitted `stages` default to `final` only so repair activities do not treat outcome
checks as baseline fixture assertions.

Optional `invariantAt`: `fixture`, `after_task`, `final`.

Optional `evidenceBearing` (default true) marks whether a pass may contribute
student evidence. Authoring-only checks may set it false.

## Reference solutions

`referenceSolution` is authoring-only. Publish stores `content` only; the
reference never becomes student-delivered verifier code. Replay with:

```bash
go run ./cmd/activity-validate -file path/to/activity.json -replay-reference
```

## Course manifests

`CourseDocument` supports ordered activities, prerequisites, gates, remediation
refs, modules, continuity placeholders, and descriptive pacing. Calendar-driven
eligibility fields are rejected.

## Validation entry points

```bash
# Curriculum samples
make activity-validate

# Linux course manifest
cd server && go run ./cmd/activity-validate -course ../content/technology/basic_linux/course.json

# Single lesson with materialize (default)
cd server && go run ./cmd/activity-validate -file ../content/technology/basic_linux/lessons/20-capstone-verification/activity.json

# Python schema pass used by the course loader
python3 content/technology/basic_linux/load.py --validate-only
```
