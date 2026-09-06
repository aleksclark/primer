# Original Tasks P4 reconciliation ledger

## Authority and scope

- Recovery source: non-plan delta
  `dc8cedb021e2e14c38ac020e85133b388409f736..89583ced7555f5559934320ee93d84185cb0b00e`.
- Canonical integration base: `2383669e674bfbffbec2da007bb9a1231054ddc4`
  (merged P3 PR84). Original phase06/P4 acceptance criteria are unchanged.
- Owner: native leaf `150cf0d1-d2f7-47d3-b8a1-d93d79fd4fd6`, existing
  `integration/tasks-p4-reconciled` worktree, Astra/high. No delegation.
- L0's updated named window releases the central Go producer seams for additive
  P4 work. Kotlin and legacy Tasks Android remain held by their separate owners;
  native dialogue transport/action is deferred. No peer management files or
  provisional producer commits are overlaid.
- Confirmed migration order: canonical00001–00009 remain immutable; incoming
  P4 is00010/00011, then separately owned management/release012–015. Local
  checkpoints are allowed; push/PR/integration remains L1/L0 coordinated.

**Current status: CP1 policy/persistence/authoring foundation, not completed P4
or student-dialogue acceptance.** The durable evaluation/completion adapter,
worker, public student WS, generated dialogue contract, UI and independent
browser gates are still forthcoming. Do not infer acceptance from the presence
of a manifest or a migration.

## Source recovery and adaptation completed in this checkpoint

| Original source | Current adaptation |
|---|---|
| `internal/domain/dialogue.go` config/validation/snapshot helpers | Retain the original versioned config concepts/functions; enforce exactly three questions, bounded inline or explicit embedded source, retain-only evidence, exact known config keys, and canonical requirement-array parsing. Add immutable issued revision/requirement/source version/hash binding. |
| `internal/domain/tasks.go` kind expansion | Validate the complete dialogue envelope/config instead of widening a kind string alone. Manual requirements retain their prior validation behavior. |
| `internal/verification/dialogue.go` context/question/evaluation authority | Derive readiness from distinct bound questions AND messages; reject wrong policy/snapshot/current question/version, duplicate evaluations, premature advancement and excess follow-ups. Never trust an accepted-count projection. Rationale is a short server-owned vocabulary, not provider prose with a keyword blacklist. |
| `internal/verification/engine.go` manifest | Add versioned dialogue manifest/config validation and shared all-issued-requirements policy. The current parent decision path uses the generic all-requirements rule, so a manual acceptance cannot bypass remaining dialogue work. Automatic dialogue acceptance is not implemented yet. |
| `internal/repo/dialogue.go` persistence seam | Use canonical `tasksdb.Database`/pgx transactions, not donor `*pgxpool.Pool`. Recover policy storage/read intent; no repository completion operation. Resolve and persist policy in the real publication transaction. Future attempts must copy it from the issued revision, never template-latest. |
| Donor `00008_dialogue_verification.sql` | Reconcile as incoming `00010_dialogue_verification.sql`; tenant/attempt/question/message/policy FKs, distinct-question/message uniqueness, bounded content, admitted session/job identity and monotonic attempt/lease counters. Add publish-time `dialogue_revision_policies` because source resolution must survive a delayed student start. |
| Donor `00009_dialogue_evaluation_criteria.sql` | Reconcile as incoming `00011_dialogue_evaluation_criteria.sql`; bound criteria and append-only policy/question/message/evaluation/event/override evidence. Protect dialogue decisions without changing the old manual-decision retention contract. |
| `internal/api/phase2.go` create/revise/publish hooks | Hunk adaptation only: full dialogue validation, transactional published policy, generic all-requirements gate, and typed conflict when ordinary parent approval would bypass dialogue. Preserve array storage/config->0, manual start/submit, current publication transactions and CAS. |
| Migration tests | Update both current migration counts9→11; retain immutable Clerk hash test. Add real PG fresh/repeated migration and released00009→00011 preservation of existing local identities, credentials, manual records, parent runs, authority, effects and previews. |

The schema stores evidence and enforces structural/retention constraints; it
**does not itself execute the worker, authenticate a socket or complete a task**.
Pure policy helpers likewise are not a durable acceptance path. These missing
runtime boundaries remain explicit work, not private-test substitutes.

## Preserved versus omitted/deferred

- Canonical00001–00009 byte hashes, module/workspace pins and checksum files are
  unchanged. Tasks direct coder1.8.14 remains; this checkpoint's Go commands use
  `GOWORK=off` and `-mod=readonly`. Workspace-selected1.8.15 qualification is a
  later actual gate, not implied here.
- Existing parent worker/policy/Authstack/Clerk/local identity/authority locks,
  receipts, stale confirmation behavior, public `/tasks` mounting, TaskForms,
  presets/DST and web source are not overlaid.
- Original donor `student_ws.go`/global tenant hub, unbounded stream delivery,
  non-atomic admission, unfenced worker, arbitrary completion SQL, handwritten
  TS/Kotlin dialogue DTOs, and fallback completion projection are **not adopted**.
- No parent-memory-confirmation production store is restored to make the donor
  unrelated API test compile.
- No source/retention fallback: unknown/remote refs, source substitution,
  retentionDays, redaction/deletion and unknown config keys fail validation.
  There is no remote fetching service or timed-deletion control.
- No `/device/ws`, Kotlin/Models/legacy Android changes, native action, Compose,
  Docker, Identity, root Make/workspace, dependency pin, release or deployment
  changes. Shared CI amendment is authorized but not needed for this checkpoint
  and has not been applied yet.
- No donor `.paseo-e2e` evidence/PASS copied. No Playwright source authored; fresh
  independently owned Chrome exploration must precede later promotion.

## Verification and evidence classification

Current local receipts are under `/tmp/primer-p4-gates/cp1/`; checkpoint source
binding and final command receipts are recorded there. They are local delivery
receipts, not portable CI or independent acceptance.

Executed during this checkpoint:

- Domain/verification/repository tests: exact-key/retain-only/source validation,
  immutable snapshots, three distinct bound evaluations, incorrect follow-up,
  stale/current-question/version and cross-message negatives, all-requirement
  policy and unsupported manifest checks.
- Real PostgreSQL schema tests: fresh11/repeat migration; released00009 upgrade
  plus preserved prior rows; retained evidence UPDATE/DELETE refusals;
  same-tenant cross-attempt/question/policy/session binding; distinct accepted
  question/message/event keys; criteria validation; active-job uniqueness and
  monotonic lease/retry counters.
- Public TCP REST against the actual production registry and real PG: valid
  create/revise/publish, original source retained after a newer revision,
  canonical editable config->0 projection, tenant denial, invalid config
  refusal, and transactional rollback of an invalid stored draft publication.
- Supplemental negative fixtures construct OPEN attempt prerequisites and call
  the real parent decision route to prove no manual dialogue shortcut and no
  bypass of another required driver. They do not inject accepted evaluations as
  purported public dialogue success.
- Complete standalone-module `go test ./... -count=1`, including existing P3 API
  tests; Go vet/build. Exact final receipts distinguish any earlier source
  snapshots. No full coverage/race10/client/web/Android/browser/CI claim follows.

Historical REDs retained:

1. `postgres-first.log`: synthetic student seed reused one placeholder as UUID
   and text; corrected explicit cast. Not a product migration failure.
2. `postgres-second.log`: an already evaluated message hit uniqueness before the
   intended cross-question FK. Corrected the fixture to use a new unevaluated
   message/job and separately assert both constraints. No constraint weakened.

No source mutation was justified by calling those setup failures “flaky.” Owned
PostgreSQL containers were terminated by their test cleanup. No live household
or another initiative's fixture was used.

## Next coherent work and remaining gates

1. Finish CP1 durable authority: atomic authenticated message/job/ack, locked
   current-question CAS, leased/idempotent evaluation and exactly one generic
   decision/completion event, separate audited override and bounded inspect.
2. Requirement-scoped Fantasy runtime with student system policy/PrepareStep,
   real usage/safe streaming, durable initial/next-question stages, bounded
   retry and restart/takeover fencing. No provider error/fallback prose success.
3. Cookie-only public student WS with Origin/CSRF and locked lifetime authority
   at admission/subscribe/replay/commands/every bounded private write; durable
   page/window/backpressure/healthy-reader behavior on maintained coder.
4. Actual Go-derived REST/WS contract and verified bundle/facades; additive web
   TaskEditor/Decide-Learn/Inspect, never copied DTOs or local completion state.
5. Supply an exact coherent producer/contract commit and normalized digest to
   L0/L1 for the peer additive handoff. This foundation is not that final
   producer freeze. Kotlin/Android remains held until its explicit handoff;
   actual existing Android build compatibility is still required.
6. Actual85% coverage, original race10 (including verification/agent/jobs/API,
   same budgets), generated clients/web, independent Chrome then promoted
   Playwright, all matching-head CI and review before integration. No native,
   live-provider quality, publication, deployment or phase13 completion claim.
