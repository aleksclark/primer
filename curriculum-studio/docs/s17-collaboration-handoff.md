# S17 backend implementation handoff

Status: **backend concurrency corrections awaiting re-review; full phase 17 is
not complete**. Reviewer b010fc06 blocked `80021bdf` on real PostgreSQL races.
See [the concurrency-fix handoff](s17-concurrency-review-fix.md) for the corrected
locking protocol, permanent regressions, and current verification. The original
verification table below is historical evidence for `80021bdf`, not acceptance
of the race fixes. Continued the inherited dirty tree on `impl/s17-collab`, parent
`4842b22b95dad0c2392885663b41d4a71598adb3`. No reset, workspace/runtime changes,
agents, push, PR, merge, deployment, or production data operations.

## Implemented

- `internal/api/collab.go`, `internal/repo/collab*.go`: durable, bounded node
  comments with server-derived membership display name and JWT subject; reviewer
  decisions tied to a content fingerprint. GET approval supplies the fingerprint;
  POST requires `decision` and `contentFingerprint`. A real READ COMMITTED
  transaction locks revision, local reviewer membership and existing approval
  before a fresh statement checks authority, draft status and fingerprint.
  Graph-write triggers use the same revision lock. Changed content or revoked/
  demoted review authority returns pending; stale decisions return 409; authors
  cannot approve.
  Approval is separate from the existing publication/validation policy.
- `GET /studio/v1/revisions/{revisionId}/diff?fromRevisionId=...`: SQL comparison
  of stable outcome codes and names. Renames produce removed/added names. Both
  revisions must be readable and belong to the same curriculum.
- `internal/api/library.go`, `internal/repo/library.go`: workspace library list,
  get and save (`POST /workspaces/{workspaceId}/unit-library`, body
  `revisionId`, `unitId`, optional `name`); transactional copy at
  `POST /revisions/{revisionId}/unit-library/{entryId}`. Copies retain names,
  objectives/arcs, outcomes, mappings, evidence, internal prerequisites, projects
  and resource links while remapping plan IDs/codes. Missing dependencies roll
  back every inserted graph row. Pool-backed saves use repeatable-read snapshots.
- Workspace template list/create at `/workspaces/{workspaceId}/templates`;
  curriculum creation accepts `templateCode` for a workspace seed or the existing
  built-in `template` enum. Seeds populate draft outcomes/units and unit-outcome
  relationships inside the curriculum/outbox transaction. Template identity is
  preserved in curriculum metadata and responses.
- Read-only curriculum grants at `/curricula/{curriculumId}/shares` and revocation
  at `/curricula/{curriculumId}/shares/{targetWorkspaceId}`. Only curriculum list,
  curriculum/revision get, revision list, graph and diff reads consult grants.
  Comments, approvals, library saves, exports, materializations and mutations do
  not. W2 gets no synthetic membership; W3 and revoked grants cannot read.
- Additive migration `db/migrations/00013_collaborative_authoring.sql`, schema
  documentation, and exact Go/Python migration inventories (no frozen baseline
  edits). `workspace_policies` is storage only in this slice.
- `internal/api/authoring.go`: additive DTOs, plus explicit materialized-item
  list query fields. Regenerating the SPA client exposed the older unexported
  embedded-query omission; the source DTO fix restores its implemented
  limit/offset/q contract without hand-editing generated clients.

## Evidence

HTTP tests use production Huma/chi routes and middleware, signed JWT test keys,
Studio-local memberships, and real PostgreSQL testcontainers. Signing keys and
JWKS are auth fixtures; no fake first-party persistence or success handlers.

- `internal/api/collab_test.go`:
  `TestP17S1CommentPersistsWithAuthor` (two principals, persisted names/decision,
  pagination, missing/stale fingerprint rejection, rejection state).
- `internal/api/collab_scenarios_test.go`:
  `TestP17S2RevisionDiffNamesAndAuthorization`,
  `TestP17S3LibraryAndTemplatePopulateDraftAtomically`,
  `TestP17S4ShareReadOnlyAndRevocation`,
  `TestP17ApprovalRequiresLocalReviewerAndDraft`.
  Includes HTTP import rollback after removal of a saved catalog dependency,
  mixed-workspace role checks, read/write share matrix, unshared W3, and revoke.
- `internal/repo/collab_test.go`:
  `TestP17LibraryCopiesProjectRelationshipsAndRollsBack`,
  `TestP17ApprovalRepoRejectsForgedStaleAndRevokedDecisions`.
- `internal/authz/role_test.go`: `TestP17ApprovalRoleMatrix`.
- `internal/api/openapi_test.go`: additive paths and fingerprint/template fields.

Commands below are relative to the repository root unless a cwd is shown.

| Command | Observed result |
|---|---|
| `cd curriculum-studio && GOWORK=off go test ./internal/api ./internal/repo ./internal/domain ./internal/authz -run '^$'` (initial inherited compile) | Exit 1: exactly the five missing symbols ListPage, Decide, Current, DiffRevisions, registerLibraryRoutes |
| `cd curriculum-studio && GOWORK=off make clients-generate` | Exit 0: offline Huma emission, protobuf, Go REST client tests, TS generation/build; generated outputs remain ignored |
| `cd curriculum-studio && GOWORK=off go test ./internal/api ./internal/repo ./internal/domain ./internal/authz -count=1 -run 'P17\|Comment\|Collab\|Share\|Diff\|Template\|Approval'` | Exit 0; domain package compiles but has no matching tests |
| `cd curriculum-studio && GOWORK=off go test ./... -count=1` | Exit 0 after generation; includes migration/inventory tests |
| `cd curriculum-studio && GOWORK=off go build ./...` | Exit 0 |
| `cd curriculum-studio && GOWORK=off go run ./cmd/migrate -check-freeze` | Exit 0, freeze check ok |
| `cd curriculum-studio/db && python3 -m pytest tests -q` | Exit 0, real PostgreSQL schema suite |
| `cd curriculum-studio/web && npm ci --ignore-scripts --no-audit --no-fund` | Exit 0 |
| `cd curriculum-studio/web && npm run build && npm run lint` | Exit 0; existing @theme minifier warning and unused GraphEdge/revisions lint warnings |
| `cd curriculum-studio && bash tools/contract-gates/check_no_tracked_generated.sh` | Exit 0 |
| `GOWORK=off make studio-cover` | Exit 2 from make (gate exit 1): **77.0% < unchanged 85%**, RED |
| `cd curriculum-studio && GOWORK=off go vet ./internal/api ./internal/repo ./internal/domain ./internal/authz` | Exit 1: pre-existing `internal/repo/foundation_test.go:104` copies sync.WaitGroup to `_`; blame 0e276cf66; left unchanged |
| `git diff --check` | Exit 0 |

During repair, the new SQL fingerprint parameter initially collided with the
integer revision column; it was renamed. Rollback-test setup initially tried to
delete a still-linked resource; it now removes links before the dependency.
Go/Python tests exposed exact inventory counts needing migration 13 (Python also
omitted the already-existing migration 12). All were rerun successfully. An
initial generated-artifact check used the wrong directory and exited 127;
rerunning the existing `tools/contract-gates` script succeeded.

Local run logs: `/tmp/primer-s17-focused.log`, `/tmp/primer-s17-full.log`,
`/tmp/primer-s17-clients.log`, `/tmp/primer-s17-python.log`,
`/tmp/primer-s17-coverage.log`.

## Unmet phase gates / next owner

- **No new SPA UI in this slice.** P17-S2 diff rendering and the comment inspector
  drawer are still missing. The existing shell build is not browser evidence.
- The root `make studio-e2e` target only builds the shell and prints that the
  browser journey requires configured Studio DB/Identity fixtures. There is no
  `curriculum-studio/web/e2e` directory or browser-test script in its package.
  No replacement e2e stack or mocked browser journey was introduced. Required
  two-session browser journeys and responsive screenshots remain unproven.
- Workspace policy configuration/enforcement is not implemented beyond the new
  JSON storage table and existing Studio-local role rules. Comment targets are
  graph nodes; materialized-item comments are not part of this slice.
- Approval notification events are optional and not emitted here.
- Coverage remains **RED against 85%**. The original writer measured 77.0% on
  `80021bdf`; reviewer b010fc06 measured 76.9%. Neither is acceptance or a new-tip
  measurement. The concurrency fix does not rerun/chase coverage or alter gates.
- Independent review, schema/contract track acceptance and the phase completion
  gate remain open. Do not mark the whole roadmap phase complete from API tests.
