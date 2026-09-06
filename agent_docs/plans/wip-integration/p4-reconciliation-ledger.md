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

**Current status: CP3 Go-producer/TypeScript checkpoint, not completed P4.**
CP1 `d24287d5` and CP2 remediation `65339f8` were independently accepted for staged
continuation. SAMEe24's BLOCK of `82d92597` and every earlier receipt remain
preserved. CP3 source/contract/client evidence below requires its own review;
UI, coverage/race10, browser, Android and matching-head CI gates remain open.

## CP1 foundation recovery (historical, independently accepted)

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
Pure policy helpers alone are not a durable acceptance path. At the CP1
foundation those runtime boundaries were explicitly deferred; CP2 below adds
them through actual production boundaries, not private-test substitutes.

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

## CP2 durable public runtime recovery

- `internal/verification/dialogue.go` now owns authenticated atomic start and
  message/job/ack admission, current-question/policy/version binding, durable
  question/evaluation stage commits, finite follow-up exhaustion, and the one
  automatic decision/completion path. All issued requirements must be satisfied.
  Exact message-key replay returns original evidence; changed payload conflicts.
- `internal/jobs/dialogue.go` recovers the donor queue with renewable generation
  leases, production expired-lease recovery, monotonic retry budgets and fresh
  clock predicates after lock waits. It does not own completion.
- `internal/agent/dialogue.go` recovers exactly three narrow tools with a separate
  student system policy/PrepareStep, bounded public Fantasy execution and no
  parent tools/repository handle. One typed proposal commits only after successful
  execution with measured usage. Raw text/reasoning/provider output never implies
  success. The curated fixture requires distinct factual concepts and rejects
  insufficient, contradictory and keyword-bearing injection answers.
- `internal/api/dialogue_worker.go` and the additive main hook run this real
  worker independently of sockets. A persisted evaluation -> next-question stage
  survives an actual running-server SIGKILL and a fresh process/lease generation.
- `student_ws.go`/`student_ws_protocol.go` use cookie-only upgrade, exact Origin/
  CSRF and locked lifetime authority. No bearer/query fallback or tenant hub.
  A sole writer uses durable32-event pages, a64-event acknowledgment window,
  bounded writes and the maintained coder transport. Network writes hold only
  revocation/immutable-binding locks, not mutable progress locks.
- `dialogue_routes.go` registers typed public start/state/inspect/override through
  the production registry (47 REST operations). Inspect uses bounded event and
  attempt pagination. Parent override preserves prior decisions/evidence, records
  separate actor/provenance/audit and original idempotent result, rejects completed
  reversal, and prevents late worker acceptance. Manual/mixed requirements remain
  independent; ordinary approval cannot bypass dialogue.
- `phase2.go` preserves canonical config arrays/manual workflow while supporting
  bounded dialogue attempt retry and final all-requirements completion. Existing
  parent decision bodies remain valid; optional requirement selection disambiguates
  multiple manual requirements.
- Only incoming010/011 were extended for immutable occurrence ownership and exact
  override receipt metadata. Canonical001–009 stay byte-identical.

### CP2 actual tests and limits

New public harness runs the real `tasks-server` executable, actual public pairing,
REST/WS, real PG and real Fantasy tool loop. SQL is a read-only outcome observer
except explicitly labeled schema/locking negative fixtures. Actual cases cover:
three correct concepts + insufficient/injection retry, reconnect2/3, distinct
message/CAS/foreign binding/concurrent tabs, one decision/event, malformed/timeout/
forbidden-tool failure with saved-answer retry, actual running process crash and
stage takeover, manual+dialogue all-requirements completion, finite attempt limits,
immutable bounded inspect/override and late-worker cancellation/override fences.

Public cookie/bearer/query/Origin/CSRF, same-household and cross-tenant negatives,
idle revocation, real TCP write-vs-public-archive winner ordering and stalled
write lock release are exercised. Real public answers produce >64 durable events
for healthy paged replay and a raw RFC6455 probe: exactly64 unacknowledged events,
1013, one Close reply and EOF. Managed Chrome is still a separate future gate.

The parent race build tag now controls explicit child `go build -race`. Actual
binary build information is checked; detector output, exit66, early exits and
shutdown hangs fail even in deliberate crash tests. An intentional crash is
accepted only with the observed requested SIGKILL and no detector failure.
Ordinary runs remain ordinary evidence. Focused instrumented public-flow and
harness qualification passed; no full/count10 race result is claimed.

A source-provenance defect in Go's embedded stamp was diagnosed: installed
Go1.26.6 recognizes only directory `.git` roots and skips the linked worktree's
`.git` file, selecting the enclosing user repo. A scoped read-only Git trace
confirmed this. Original4b8924b8/false receipts are preserved, not rewritten.
The harness now separately captures actual worktree/HEAD/module/target, hashes
tracked AND untracked source/tests/SQL/assets and selected embeds around build,
verifies stable inputs during qualification, and hashes the actual live process
executable. Embedded stamp mismatch is explicitly non-authoritative. No user-root
or global Git/toolchain change was made.

CP2 REDs retained under `/tmp/primer-p4-gates/cp2/`:

- `public-authority-first`: missing `net/url` test import; corrected, not a
  security PASS.
- `module-first`: old Clerk test expected one cookie; exact TEST-ONLY amendment
  authorized by L0. Fresh pair now asserts exact names, unchanged student-cookie
  protections, independent host-only eight-hour readable CSRF and CSRF-alone
  student/parent denial. All original other assertions remain.
- The same module run had one initial1008 in a provider-failure subcase. Its exact
  historical DB interleaving was not captured. A concrete missing progress-read
  lock was proven RED by an actual public HTTP/PG barrier test, then corrected:
  assemble the DB state under a short shared progress lock, release before network
  I/O, and revalidate authority after waiting. Focused corrected-input proof and
  the subsequent complete ordinary module run passed. Do not call the original
  failure harmless/flaky or assert uncaptured causality.
- `state-snapshot-after` was interrupted during a later fixture and has no exit
  receipt; completed subcase observations are preserved, NOT aggregate PASS.
  Owned process/container cleanup was verified and remaining focused cases got a
  separate complete exit0 receipt.

The corrected complete standalone module run passed (`module-corrected`, exit0,
API242.964s). Vet/build and focused receipts are source-scoped. The current runtime
workload is materially larger than P3; full race10 still has its unchanged40m
package/60m job ceilings. No deadline/budget increase, assertion removal or skip
is authorized. Actual full coverage85% remains to be measured; subprocess
functional evidence must not be misrepresented as parent-process coverage.

## CP2 BLOCK remediation (SAMEe24)

Review receipt `/tmp/primer-p4-review-cp2.md`, SHA256
`ba7a0da99f552c9b4eeb735e3904aa11576757e4d2694af1aa737f149798ddef`, was read fully.
Scope remains the existing named window; no additional service/root/config/pin/
Android scope or budget/gate changes.

### HIGH1 — terminal job failure stranded an open occurrence

- Reproduced RED through actual public provider retries to the maximum: job
  failed but attempt stayed open and occurrence awaiting_verification.
- Removed queue-only expiry/failure updates. Queue recovery and worker failure
  now delegate to the generic engine, which locks immutable admission rows,
  occurrence, attempt projection and job, then rechecks state/clock/generation.
- Nonterminal expired leases requeue the durable stage without resetting budget.
  Terminal provider/deadline/lease exhaustion atomically preserves evidence,
  records one negative decision/error event, exhausts the attempt and makes the
  occurrence parent-retryable. Recovery is explicitly negative system authority,
  never positive authority borrowed from an expired student credential.
- Completed/canceled/overridden or superseded contexts only clean stale queued/
  running jobs; their prior decisions/state/evidence are not rewritten.
- Public max-provider-retry recovery passed. Supplementary queued/running/failed
  deadline, running lease-budget and queued-budget preconditions are recovered by
  the real production process; real earlier accepted evidence is retained, parent
  retry creates the next permitted attempt, replay stays idempotent and stale
  generation writes fail. No accepted results were injected as public success.
- Actual public completed/overridden/canceled outcomes plus stale-expired queue
  preconditions retain byte-equivalent protected evidence after recovery.

### HIGH1 related note — mixed requirement retry ambiguity

- Retry now selects current attempts per requirement, not a global attempt-number
  winner or the first revision ordinal. A unique failed candidate preserves the
  simple legacy call. Multiple candidates require explicit requirementId and/or
  attemptId; foreign, mismatched, superseded or nonfailed selections fail closed.
- Retry numbers increment only the selected requirement. Explicit selection can
  recover another failed requirement while the occurrence awaits verification,
  without duplicating the first retry. Inspect exposes bounded generic attempt
  identities/kinds so callers do not guess IDs.
- Public tied-number tests passed for both manual/dialogue orderings, one failed
  candidate, ambiguous failures, foreign/mismatched selectors and exact selected
  retry outcomes. Legacy manual/Clerk boundary and DB-failure checks passed.

### HIGH2 — answer-bearing provider question prose

- Reproduced RED by an adversarial provider question-tool output containing the
  reviewer's answer-bearing sentence; it reached the public stream on blocked82d.
- Removed the blacklist/suffix-based prose acceptance path. Visible wording now
  comes only from an affirmative three-question plan/version bound into the
  issued immutable snapshot/digest. Curated chapter prompts are server-owned;
  arbitrary bounded inline sources use closed source-neutral comprehension
  templates, with no source text interpolated into them.
- The real Fantasy question tool accepts ONLY the current question identity.
  Extra/duplicate/case-aliased/free-form fields and wrong identities reject the
  turn, including valid identity plus adversarial prose. The engine resolves
  wording from its snapshot, pure evidence validation checks that binding, and
  incoming011 enforces it again at the question INSERT boundary.
- Both curated and distinct inline public three-concept flows passed through real
  Fantasy/PG. Adversarial prose/identity cases persist no question and no unsafe
  question event. This is closed egress authority, not a semantic blacklist or
  an educational-quality claim about live evaluation.

### Remediation receipts and limits

Receipts live under `/tmp/primer-p4-gates/cp2-remediation/`:
`findings-before` is preserved RED for BOTH blocking cases; `question-unit`,
`question-public-after`, `exhaustion-after`, `expiry-selection-first`,
`affected-packages`, `coherent-regressions`, `preservation-expiry`, and vet/build
record actual focused results. Later frozen binding receipts supersede only for
final-input qualification, never relabel earlier source snapshots.

Child-race build-tag/effective-setting, fail-closed exit/detector checks, actual
running-executable hashes and checkout manifests remain intact. No long race or
full unchanged module rerun was used as a default remediation shortcut. Original
82d source/RED/PASS/interruption/VCS-provenance records remain untouched.

## CP3 producer / TypeScript facade checkpoint

The generated-client skill and ownership/build references were refreshed before
work. CP3 prioritizes a pre-merge producer handoff, without waiting on or editing
Kotlin/legacy Android and without large UI work.

- Actual `studentCommand` and `verification.DialogueEvent` Go declarations carry
  variant membership/scalar constraints. `student_ws_contract.go` reflects those
  declarations into the SAME rules used by inbound/outbound runtime validation,
  offline JSON Schema and generated TS unions. Unknown fields/wrong variants,
  missing binding, unsafe numeric/cursor values and false terminal status/count
  combinations fail closed. Corrupt persisted events are rejected, not stripped.
- The Go socket path/protocol/read/page/window/write bounds are shared by actual
  route/transport code and schema metadata. State now reports real active job
  phases so reconnect/conflict recovery cannot invite an answer while busy.
  No cookie/tenant/student/prose policy is accepted from the client.
- Parent config metadata reflects the actual `domain.DialogueConfig` and the
  constants used by server validation/source resolution. It includes retained-
  evidence, bounded source/rubric/retry limits and the manifest envelope. Closed
  published question plans remain server-only, never parent-input or student DTOs.
- Existing `cmd/agent-protocol-gen -bundle` emits a complete v2 bundle: parent
  schema/TS, student schema/TS, parent-config schema/TS, REST JSON/YAML, manifest
  and completion marker. The marker is invalidated before derivation/writes;
  default Node generation invalidates before invoking real Go too. No new Go
  producer command, Docker/context/build-key or dependency change.
- The normalized digest covers the four JSON contracts with recursive object-key
  ordering/Go-compatible string encoding; array order and all semantic fields,
  nullability, enums/constraints are preserved. Node independently recomputes it.
  Optional expected-digest binding rejects coherent-but-stale artifacts. Integrity
  is not authenticity: explicit bundle trust remains the current-source Go→Node
  build chain, not a claim that arbitrary supplied artifacts are current.
- Separate TS package consumes generated types/rules and existing openapi-fetch
  middleware. No parallel REST fetch lane bypasses Clerk. Student constructor
  owns same-origin/base-path/cookie-CSRF, strict events, ack/cursor/reconnect and
  identical pending-message replay. Stale-CAS recovery retains unsent text and
  never silently rebases it. Completion is an explicit server result, not a count
  or answer-level acceptance. Parent inspect/override/config remain separate.
- Only named parent/student facades may construct sockets; source/DTO/transport
  checks and no-tracked-generated-output checks are executable. The existing
  empty `.gitkeep` is the sole narrow output-root marker exception.
- `make -C primer-tasks clients-typescript` runs fresh generation, TS typecheck,
  boundaries, client runtime tests, genuine Go-overlay/Node-no-Go tests and the
  dedicated tagged real generated-client public conformance test. The existing
  `clients` target retains its subsequent Kotlin edge; it was not run during the
  Kotlin source hold. Root Makefile and other shared build files stay unchanged.

### CP3 observed qualification

- Parent WS schema/TS, REST YAML and generated REST TypeScript are byte-identical
  to the pre-CP3 `65339f8` baseline. Existing additive P4 REST operations/retry
  selectors are preserved; no new REST operations were needed in CP3.
- Real Go source overlays rename a runtime student command JSON field and change
  an actual config bound; both generated artifacts AND actual Go runtime checks
  follow. A stale TS consumer fails compilation after the generated wire rename.
- Fresh double generation is deterministic. Real Node with no Go consumes the
  explicit bundle and typechecks; missing/corrupt/mixed/unsupported/wrong-normalized-
  digest/expected-stale bundles fail before replacing outputs. Default mode invokes
  actual Go and refuses an existing-output fallback when Go is unavailable.
- Eight client tests cover generated guards/config limits, terminal truthfulness,
  base/tasks/api/CSRF, current-version and identical retry payloads, busy/conflict
  recovery, URL/foreign-frame refusals, Clerk middleware and typed errors. Boundary
  mutations prove parallel sockets/fetch/copied DTOs/tracked generated output fail.
- A real generated-client Node consumer calls the actual Tasks process/PG/Fantasy
  through public start/WS/inspect, rejects an incorrect answer, reconnects2/3,
  completes once and observes a typed404. Node cookie/Origin bridging is an explicit
  test-platform bootstrap, not a mock server/model or product transport bypass.
- Go emitter/runtime contract tests, focused server concurrency/override/IDOR
  regressions, vet/build, existing web typecheck and15 preserved web unit tests
  pass. No browser exploration, full race10, coverage or native acceptance claimed.

CP3 REDs remain under `/tmp/primer-p4-gates/cp3/`: initial generated public client
rejected Huma's documented REST `$schema` link with502; the facade now removes
ONLY that generated REST metadata before the strict WS-state guard, and actual
public response metadata/passing flow verifies the adapter. Initial no-generated-
output gate rejected the existing empty `.gitkeep`; exact marker allowlisting
preserves rejection of real generated source. These are not hidden or called
flaky. Raw artifact hashes/normalized digest and final source binding are in the
producer handoff report, clearly pre-merge and pending independent CP3 review.

## Next checkpoint / remaining acceptance

1. SAMEe24 independent review of the CP3 producer/facade checkpoint. Its exact
   commit and normalized contract digest are a PRE-MERGE handoff through L1/L0,
   not full-P4 or peer/native acceptance.
2. After that review, additive TaskEditor/Decide-Learn/Inspect web surfaces using
   the generated facade; preserve editing/presets/history/Clerk/P3 behavior.
   No Playwright authoring before independently owned Chrome exploration.
3. Kotlin/Android stays held pending its explicit consolidated handoff. Do not
   wait for native acceptance to hand off the Go producer; native dialogue remains
   deferred and ordinary existing build compatibility is still owed.
4. Actual85% coverage and original race10 with effective runtime instrumentation,
   generated clients/web, independent Chrome before promotion, Android build and
   all exact-head CI/review before integration. No full-P4/native/live-provider/
   phase13/publication/deployment claim.
