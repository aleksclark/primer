# S17 concurrency corrections — re-review handoff

Follow-up to blocked backend commit `80021bdf72e52b42fe7a645caaf532c4d338da18`.
Same `impl/s17-collab` writer/workspace; no runtime changes, agents, reset, push,
PR, deployment, or production activity. **Awaiting reviewer b010fc06; not full
S17 acceptance.**

## Corrected protocol

- `internal/repo/collab_lock.go` requires a real `pgx.Tx` at READ COMMITTED.
  Autocommit Querier fallbacks and higher-isolation transactions fail closed
  rather than releasing locks early or mistaking an older transaction snapshot
  for a fresh statement snapshot.
- `ApprovalRepo.Decide` holds locks until transaction commit in this order:
  revision (`NO KEY UPDATE`) → local reviewer membership (`SHARE`, not KEY SHARE)
  → existing approval (`UPDATE`). A **subsequent statement**, after all waits,
  checks draft status, active human reviewer role and the supplied fingerprint.
  Publication, graph writes and reviewer revocation cannot commit while these
  locks are held. All transaction/lock/statement errors propagate; no retry,
  process mutex or swallowed serialization error.
- `ApprovalRepo.Current` also joins the current active human reviewer membership.
  A later revoke/demotion makes the stored decision ineffective (pending), while
  preserving the historical row. A legitimate decision committed *before*
  publication may still be displayed on the published revision.
- `UnitLibraryRepo.CopyIntoRevision` locks the destination revision and rereads
  its state after the wait, before any imports, retaining the lock until the
  complete copied graph commits or rolls back.
- Unmerged migration **00013** adds `a_studio_lock_content` to all authorable graph
  tables, before existing immutability/prerequisite guards. Direct INSERT,
  UPDATE and DELETE serialize on the same revision lock, with post-wait state
  checks. Reparenting checks old/new revisions in UUID order. Membership join
  and standard-mapping triggers resolve the owning revision too. This covers
  edits that would not otherwise take a conflicting revision FK lock. The
  existing publisher may lock its curriculum first; these new operations do not
  acquire a curriculum lock after acquiring a revision lock. No older migration
  bytes changed; Down removes the added triggers/function.

If publication/revocation/content change wins first, a stale decision fails. If
an approval/copy wins first, the later publisher/revoker/editor waits, then
proceeds after commit. A later edit invalidates the fingerprint; it cannot
silently change content while an approval is waiting on another required lock.

## Both requested nits

- Comment bodies have a 10,000 **Unicode-code-point** bound (not bytes): HTTP
  `maxLength`, `domain.ValidateCommentBody`, repo rejection, database
  `char_length` CHECK. Tests cover ASCII, CJK and emoji at/over the bound,
  including direct SQL rejection and persisted character/byte counts.
- `listComments` returns applied `limit` and `offset` alongside `items` and
  `totalCount`. API metadata and repo clamping share `CommentPageBounds`.
  HTTP still rejects out-of-schema limits; direct repo calls clamp to 100.

## Permanent regressions / reviewer repro mapping

Original repro sources and overlay JSON files in `/tmp` were read and left
unchanged. They assert BAD success, so their passing is not acceptance. Their
synchronous publish/revoke ordering after an approval-row gate would be an
artificial deadlock with the new locks. The permanent tests instead run the
publisher/revoker as separate workers and assert its real database wait before
releasing that gate.

`internal/repo/collab_concurrency_test.go`:

- `/tmp/s17_approval_publish_race_test.go` → `TestP17ApprovalPublishRace`:
  publish-first rejects; decision-first blocks publisher and commits before it.
- `/tmp/s17_approval_revocation_race_test.go` → `TestP17ApprovalRevocationRace`:
  revoke-first rejects; decision-first blocks revocation; neither order leaves
  an effective decision after revocation.
- `/tmp/s17_library_publish_race_test.go` → `TestP17LibraryPublishRace`:
  publish-first leaves no imported rows; copy-first holds publication at a real
  standard FK barrier until all units, outcomes, links, mappings and evidence
  commit. Subsequent imports into the published plan are refused.
- `TestP17ApprovalContentMutationRace`: both orderings of a real outcome edit and
  review, including an approval-row wait, verify fingerprint invalidation and
  durable decision state.
- `TestP17GraphWriteWaitsForPublication`: raw SQL UPDATE waiting behind publication
  is rejected by the new trigger; original outcome title remains durable.
- `TestP17CollabRequiresReadCommittedTransaction`: autocommit wrappers and
  repeatable-read/serializable outer transactions fail closed without decisions.

Each worker acquires a separate pool connection and starts real transactions.
`pg_blocking_pids` with exact waiter/holder PIDs is the barrier; no sleeps or
elapsed-time assertions determine ordering. Timeouts only bound failures.
Assertions inspect committed state before fixture cleanup. Only fixture-owned
outbox events are removed afterward to avoid polluting unrelated global metrics.

Additional tests: `internal/api/collab_bounds_test.go`,
`internal/repo/collab_comments_test.go`, `internal/domain/collab_test.go`.

## Verification (current fix)

Logs include exact commands and `EXIT=` markers under
`/tmp/primer-s17-race-fix/`. Cwd below is `curriculum-studio` except Python (`db`).

- `race.log`: exit **0**
  ```sh
  GOWORK=off go test -race ./internal/repo -count=10 -run 'TestP17(Approval(Publish|Revocation|ContentMutation)Race|LibraryPublishRace|GraphWriteWaitsForPublication|CollabRequiresReadCommittedTransaction)' -v -timeout 180s
  ```
- `affected.log`: exit **0**, focused S17/Comment/Collab/Share/Diff/Template/Approval
  tests in API/repo/domain/authz, `-count=1 -timeout 120s`.
- `affected-race.log`: exit **0**
  `GOWORK=off go test -race ./internal/api ./internal/repo ./internal/domain ./internal/authz -count=1 -timeout 180s`.
- `clients.log`: exit **0**, `GOWORK=off make clients-generate` (offline Huma,
  protobuf, Go REST, TS generation/build); generated outputs remain ignored.
- `full.log`: exit **0** after generation,
  `GOWORK=off go test ./... -count=1 -timeout 180s`, including migrations.
- `python.log`: exit **0**, `python3 -m pytest tests -q` against real PostgreSQL.
- `build.log`: exit **0**, `GOWORK=off go build ./...`,
  `GOWORK=off go run ./cmd/migrate -check-freeze`, generated-artifact guard and
  `npm run build` in `web`. Existing @theme minifier warning remains.
- `git diff --check`: exit **0**.

Earlier failures are preserved in `focused.log`/`library-debug.log` (fixture null
blueprint instead of valid `{}` prevented reaching a lock barrier) and
`*-before-fixture-cleanup.log` (committed fixture outbox events affected the global
metrics assertion). The fixture defects were fixed, not bypassed; all final
commands above pass. Existing pre-S17 vet warning at
`internal/repo/foundation_test.go:104` remains untouched.

## Still open

Independent re-review of these concurrency guarantees; UI/browser journeys,
item comments, policy configuration/enforcement and schema/contract acceptance.
Coverage remains RED against the unchanged 85% gate: reviewer reported 76.9%
(previous writer 77.0% on the earlier tip). No new coverage claim, threshold change,
coverage chase, test skips, browser substitutes or phase-completion claim.
