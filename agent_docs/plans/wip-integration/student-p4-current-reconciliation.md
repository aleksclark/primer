# Student / P4 reconciliation onto current master

## Scope and donor audit

Initial isolated base: `ccee6c8b114c73d52876021a3d93c98d70244050`.
This is a narrow source reconciliation, not full P4/native acceptance.

- Student tip `d4d56f74` itself changes A16 documentation. Audited its ancestry
  from shared `eeb04728`: ported the final eight-file Student feature delta
  (capability gating, unavailable restore result, back-navigation, pairing
  fallbacks and tests). The intermediate missing-capability fallback in
  `a6c556f8` was not adopted; final `c2142368` fails closed.
- Producer `51cad1c0` is already represented by master `218f749f`.
  `StudentRequirement`, `requirements` and `studentCapability` remain intact.
- P4 `89f3a593`, `fd34a7bd`, `825bef5c`: ported only product-local web,
  TypeScript facade/probe and Go API/test hunks. The seven web files matched
  the pre-P4 donor state on master. Did not import donor workflows, old producer
  files wholesale, migrations or historical PASS records.
- Educator donor feature/app files equal master: no port needed.
- Android integration `7eb4c4bc` context and documentation clarification
  `6cb46f6e` are represented by the explicitly historical A16 record in
  `test-artifacts/android-student/student-tasks-baseline-a16.md`. No hardware,
  signing, public rollout or native Control acceptance is inferred here.

## Reconciled contracts and local authority

- Browser manual start/submit now bind to the issued requirement and use a
  dedicated typed `StudentManualAction2` result, not donor `map[string]any` and
  not a response adapter that discards binding fields. Modern web always sends
  `requirementId`; omission is accepted only for exactly one supported manual
  requirement, resolved under the occurrence transaction.
- Every cookie manual mutation requires Origin/CSRF, including that legacy
  omission. Malformed raw queries fail before interpretation, rather than
  `URL.Query()` silently dropping a malformed explicit/duplicate selector.
- Native remains bearer `/device` only with unchanged typed `OccurrenceAction2`.
  No native query selection, cookie fallback or dialogue/WS support is added.
  Both native start and submit validate the entire single-manual envelope before
  mutation/idempotent success. Student/device rows stay locked through the
  occurrence transaction in archive-compatible order. Browser session lifetime
  checks and strict socket/inspect decoding remain separate and unchanged.
- Capability metadata and P4 attempt-state `verification` are additive, not
  alternatives. All-requirements completion, per-requirement attempts and retained
  inspection remain authoritative. Management migrations `00012`–`00015`,
  canonical `00001`–`00011`, identity policy and generated-client constraints
  are untouched by this port.
- Native unsupported states have neutral labels, not invented parent approval.
  Captured detail requests and a view generation prevent late results reopening
  a detail after Back/select B. Stale restore results cannot restore a revoked
  or replaced binding; session storage publication is serialized and checked,
  with a UI restore generation guarding already-returned stale results.
- Browser design authority remains **Primer System C**,
  `design-system/README.md`, `reference/system-c.html` and generated tokens.
  Student dialogue is Decide/Learn; retained parent history is Command/Inspect;
  task editing is Configure. No house palette/token replacement. Collections
  remain bounded and server-operated through the generated client.

## Review and checkpoint evidence (not final qualification)

L2 read-only reviewer `d9570a37-7dd0-41bb-8aa9-addc4d194b49` produced
`/tmp/student-p4-review-initial.md` and `...-followup.md`. Findings drove the
custody, typed-response, label, navigation and malformed-query corrections.
Reviewer withdrew mandatory selection for the safe legacy single-manual call
once unconditional CSRF and transactional binding were established. Final
source review and combined-source test receipts are recorded below when ready.

Local logs: `/tmp/student-p4-current-gates/`.

- `focused-go-corrected`: focused Phase2, public mixed-manual, capability and
  native compatibility tests passed (API 45.904s), before later query/restore
  hardening. Earlier RED preserved: native start-after-submit semantics,
  permissive missing-requirement test precondition, and an invalid-version
  fixture attempted through a producer that correctly rejects it.
- `client-web`: fresh TypeScript generation/typecheck/boundary/runtime tests,
  generation mutation tests and real generated-client public dialogue/manual
  probe passed; Kotlin generation and generator tests passed; web lint,
  typecheck, 26 unit tests and build passed. These are pre-merge/source-scoped,
  not final combined-head qualification. Vite reported its existing native
  config-loader and >500kB chunk warnings; no thresholds changed.
- `full-tasks`: FAILED solely at the old Clerk browser manual fixture, which
  supplied a cookie but no CSRF. The test now uses the separately asserted
  issued CSRF cookie/header and configured Origin; no identity guard was changed.
- `full-tasks-corrected` and `coverage`: interrupted before aggregate receipt;
  no exit file, no PASS. Fresh combined-master reruns remain required.

## Final candidate and source binding

- Narrow implementation checkpoint: `32ac8b3d`.
- Parent-authorized master `3237c404` merged as
  `0655effe2ec9f8177b7d9b454d9fb2342de6d9f9`. This includes parent-owned format,
  Control test lifecycle and management strict-fixture repairs. No donor branch
  was merged wholesale and this lane did not merge its candidate into master.
- Final **code**: `08bff02470089616e9f90389b509116967ecd2a3`, tree
  `d754383ec79deec6ca4d4cb1186e3f358b6f3f0d`.
- The last code commit serializes the initial restore snapshot and incomplete
  cleanup under the pairing-publication mutex. The parent-discovered null-read /
  new-pair / stale-clear race has a deterministic `UNDISPATCHED` +
  `CompletableDeferred` store-barrier test using the production publication seam.
  Network I/O remains outside the mutex. It also exports the missing
  `StudentRequirement` façade **typealias**, not a duplicate model.
- Full Go/web receipts name `0655effe`; the only subsequent code changes are
  `TasksSession.kt`, `TasksLateResultTest.kt`, and Kotlin `Models.kt`. Their stamp
  is not rewritten. Final focused Go/TS tests ran again after `08bff024`; corrected
  Kotlin tests compiled/exercised the final bytes immediately before that commit.
- Final read-only reviewer found **no remaining source blocker** at `08bff024`.
  The exact source-bound review is committed alongside this report as
  [student-p4-current-review.md](student-p4-current-review.md). Reviewer inspected
  receipts but did not rerun tests/browser; review does not waive coverage.

## Fresh gate results

Exact commands, exit codes, source trees, log hashes, contract digest and screenshot
hashes are committed in `test-artifacts/student-p4-current/receipts.json`.
Go module commands below use `GOWORK=off GOFLAGS=-mod=readonly` unless noted.

| Gate | Observed result / source scope |
|---|---|
| `cd primer-tasks && go test ./... -count=1` | PASS at combined `0655effe`; API334.839s. All subsequent Go source bytes unchanged. |
| `cd primer-tasks && make cover` | **FAIL**, total **71.0% < 85%**, make exit2. API336.667s. Original floor/measurement preserved; public child-process coverage is not invented. |
| `go vet ./...`, `go build ./...` | PASS, combined Go tree. |
| `make clients-typescript` | PASS both combined and final code checkpoints: generation, typecheck, boundaries, 8 runtime tests, 3 generation/mutation tests, actual tagged generated-client public manual/dialogue conformance. Final public API5.995s. |
| Final focused Go public tests | PASS at `08bff024`, API15.119s: native compatibility/custody/query negatives, mixed selected manual, safe capability projection and Clerk browser fixture. |
| Workspace native compatibility | PASS with default root workspace: `GOFLAGS=-mod=readonly go test ./primer-tasks/internal/api -run '^TestPublicNativeManualRequirementCompatibility$' -count=1`. Not a full workspace suite. |
| `node clients/kotlin/generate-client.mjs`, `node --test clients/kotlin/generate-client.test.mjs` | PASS from the fresh combined producer; strict constraints preserved. |
| Web `lint`, `typecheck`, `test:unit`, `build` | PASS, 26 web unit tests; build regenerates client. Existing Vite config-loader/chunk-size warnings retained, not suppressed. |
| Kotlin client + Student feature debug/release JVM tests, feature `lintDebug` | PASS: 37 client tests +96 Student variant executions, zero failures/errors/skips. Three late-result tests pass in both variants. Gradle workers=1, forced rerun, isolated output under `/tmp/student-p4-current-gates/gradle-output`. |

The original focused Kotlin run failed compilation because the donor test imported
`StudentRequirement` without a public façade alias. That RED receipt remains;
`final-kotlin-corrected` proves the alias correction and all final store regressions.
No generated OpenAPI/client sources are committed. Final normalized contract digest:
`34f650af536e8b0d44655ad88f9eb7f67df4ca90fd02f68c512553c0d045a1f7`.

## Fresh browser evidence

Real isolated PostgreSQL + Tasks process + test issuer + Vite, using the repository
scripted Fantasy provider; managed Chrome context `student-p4-current`. The owned
host-stack name is `student-p4-current-0655effe`, not the live `wet-parrot` stack.
No route interception, direct SQL success rows, ad hoc browser transport or device
operation was used. This is L1 implementer exploration, not independent Chrome or
promoted Playwright acceptance.

Observed through the UI:

1. Parent signs in through the test issuer, creates a named student and mixed
   parent-approval + curated dialogue task. Generated config limits disable an
   incomplete draft; save/publish/schedule use named server-backed choices.
2. Browser pairing uses the freshly issued one-use code. Student selects manual
   requirement, starts/submits, then explicitly selects and starts dialogue.
3. A correct first answer is accepted. An insufficient answer and a second answer
   rejected by the narrow scripted fixture remain in history without advancement.
   The fixture's canonical second answer succeeds; reload at2/3 retains the third
   question. Third success accepts dialogue while explicitly leaving the whole
   task incomplete because manual approval remains outstanding.
4. Parent inspector displays original answers, safe evaluations, criteria, source
   version/hash and3/3. Next events advances the bounded URL cursor to20 and shows
   the final original answer/evaluation. Inspector fields have unique id/name.
5. Parent selects manual approval; only then does assignment become completed.
   Student reload reads explicit server completion and no answer composer.
6. Foreign requirement query and unavailable occurrence both show refusal/error,
   with no inferred manual or dialogue fallback.

Reviewed dark desktop/mobile and mobile light screenshots preserve System C,
Instrument Sans and ruled structure. Mobile document width equals390px with no
horizontal overflow. Screenshots and saved completion/inspection/unavailable
snapshots are hashed in the receipt; detailed local procedure is
`.paseo-e2e/student-p4-current/exploration.md`.

Diagnostics remain explicit: development mount/reload emitted a WebSocket
closed-before-established warning before successful connected/replayed flow;
Student selector/composer emitted missing id/name diagnostics. Some old-tab MCP
screenshot/evaluation calls timed out; fresh-tab reads and mobile capture succeeded
on the same URL, but the timeout cause was not established. These tool calls are
not silently relabeled PASS. No full long-form keyboard/IME or browser adversarial
matrix is claimed.

## Disposition and gaps

**Candidate reconciled; full qualification is blocked by the existing85% gate.**
Parent owns baseline coverage/measurement analysis and final combined Android
app suite/lint/build. This lane does not introduce a new coverage scheme, change
CI floors/budgets or touch the separate management replacement double-epoch bug.

Still not claimed: full race/count10, full workspace suite, complete CI, independent
Chrome/Playwright promotion, native lock-wait revocation winner-order race proof,
full Android app/device/camera/Control loop, live Clerk/provider, signing, release,
publication or deployment. No push or merge of the candidate into master occurred.
The owned browser-stack API/issuer/Vite sessions were stopped after testing; its
uniquely named PostgreSQL containers were stopped, not removed, preserving their
temporary data. No live emulator, `wet-parrot` service, donor worktree, independent
pairing branch or archived repository state was changed. Chrome tool cleanup
closed its last test tab but subsequently returned a stale-selected-page error;
remaining isolated test tabs were not force-cleaned through a shared browser.
