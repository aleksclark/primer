# Final Student/P4 code review — blockers only

## Verdict

**No remaining code blocker identified in the reviewed Student/P4 reconciliation at `08bff02470089616e9f90389b509116967ecd2a3`.** The previously reported malformed-query and stale-restore defects are corrected in the current source. This closes this review's source findings; it is **not** full-P4, release, native/device, race-count10, or CI acceptance.

**The existing coverage gate is still failed:** the supplied receipt reports **71.0% < 85%**, exit 2. This code-review verdict does not waive or relabel that gate.

Safe legacy single-manual browser omission remains acceptable: full browser custody is enforced and the single issued requirement is resolved/validated within the transaction. Explicit selection remains necessary for mixed work. No requirement to remove that compatible API behavior is imposed.

## Actual reviewed source identity

```text
HEAD:      08bff02470089616e9f90389b509116967ecd2a3
Git tree:  d754383ec79deec6ca4d4cb1186e3f358b6f3f0d
Base:      3237c404 (merged master)
Worktree:  clean at inspection
```

`git diff 3237c404 HEAD --check` produced no errors. Both the tree identity and the file SHA-256 hashes below were obtained from the actual workspace, not copied from the handoff.

```text
7f75ef42bca348a78d1670a9aaafaaba6053d89289c75047eb0f9b6faf4ab35b  primer-tasks/internal/api/phase2.go
b5c89d6b54e144228a1529778fcfff8538bfb67b0f0c995d44b698b72135ea3e  primer-tasks/internal/api/phase2_openapi.go
15ce5d11243072fdc22f48d09543254d2792a8348bbc6f99317683a924ffd82e  primer-tasks/internal/api/manual_device.go
f8b278b91f86ea1b9de50bceef218cb043498f6be2a8be008b872faf851747a7  primer-tasks/internal/api/manual_native_reconciliation_test.go
bb0e4112c948d929a83a4ae3a3136e3acae88d245b9a5b32d1083ccf24e76fce  android/feature-tasks-student/src/main/kotlin/com/aleksclark/primer/student/tasks/TasksSession.kt
ba4a9a9b05bd05b216479b391d130e7a98ab611feb56846aecc53a13c9575449  android/feature-tasks-student/src/main/kotlin/com/aleksclark/primer/student/tasks/TasksScreens.kt
4493498b161f21c234986833b6ff1ab5361233d6550238e0fb41024b5ee96020  android/feature-tasks-student/src/main/kotlin/com/aleksclark/primer/student/tasks/OccurrenceViewGate.kt
3fc4c1614f26a02677af81b5d2556cb989fedc8abfef6cbc2ec5d25174d06dcf  android/feature-tasks-student/src/main/kotlin/com/aleksclark/primer/student/tasks/OccurrencePresentation.kt
5943a31f79733fac648d0cd6bd5d8f2e9c56abef8024bc6c3f942e3fd75bce71  android/feature-tasks-student/src/test/kotlin/com/aleksclark/primer/student/tasks/TasksLateResultTest.kt
471f7ad486a1f1769a96f3bb0f46f85d10b1cbe5332667379833be4cfa3d00a9  primer-tasks/clients/kotlin/src/main/kotlin/com/aleksclark/primertasks/client/Models.kt
```

## Closure assessment

### F1: malformed query silently became implicit selection — closed

`manualMutation2` now parses `RawQuery` exactly once using `url.ParseQuery`, rejects any parse error, and uses the same resulting map for selector presence, multiplicity, and unknown-parameter checks. A malformed explicit selector can no longer become the safe omission case. Native still rejects all nonempty query strings before mutation.

The public reconciliation test includes invalid percent escape, valid-plus-malformed duplicate, semicolon pair and malformed unknown-key requests, and observes that denied requests leave pending state and attempt count unchanged. The original valid optional/explicit paths are preserved.

### F2: delayed restore republished revoked/replaced custody — closed

`TasksSession` now serializes the storage snapshot and incomplete-state cleanup with the **same `pairingMutex`** used for publication, conditional clearing and final guarded load publication. `load` publishes successful profile/checklist metadata or transient-failure retained custody only after checking the current token/student/origin/pairing identity under that mutex; otherwise it returns `Superseded`. Network I/O stays outside the storage mutex.

The parent-discovered older-null-read/new-pair-clear race is corrected in `08bff024`: `restore()` cannot capture an incomplete snapshot, allow new pairing publication, then clear it afterward. The `UNDISPATCHED` + `CompletableDeferred` test forces the real storage/publication seam to wait on the mutex without sleep-based ordering. It asserts that the new token is published only after incomplete cleanup and survives afterward.

At the UI boundary, `restoreCurrent()` fences pending restore results by generation; denial/replacement invalidates that generation. `Superseded` does not republish UI custody. Detail results retain the occurrence-view generation and immutable token/origin/request binding; navigation does not suppress a current-pairing revocation.

The late-result tests exercise delayed real client/session responses for revoked/replaced storage and the view gate after Back/selection. This is meaningful JVM/session evidence, not an assertion of on-device Compose lifecycle acceptance.

### Earlier producer/native and client findings — remain closed

- Browser manual mutations always require cookie identity and Origin/CSRF, including genuine omitted-selection single-manual calls. Bearer/query credential fallback is not introduced.
- Native manual actions remain bearer `/device` only. Student/device rows are locked before occurrence progress; exactly one full supported manual envelope is required before mutation or idempotent success. No native dialogue/WS path is added.
- Browser `StudentManualAction2` uses the real typed Huma boundary; native `OccurrenceAction2` remains unchanged. Selected requirement and attempt fields are not discarded into the old action projection or weakened to a generic dictionary.
- `requirements` / `studentCapability` coexist with the additive attempt-state `verification` projection. Mixed work is not reclassified as native parent approval; completion remains governed by all issued requirements.
- Unsupported/absent-capability Student records use neutral status labels and do not expose manual buttons.
- Missing Kotlin `StudentRequirement` export is now a **typealias to the generated model**, not a copied DTO. The compatibility fixture verifies the additive verification/requirement identity and existing `/device` route.

## Preservation checks

The reconciliation diff against merged master does not alter migration bytes, parent identity implementation, student WS protocol/decoder/authentication, or the parent inspect durable-event decoder. Kotlin generator/constraint implementation is not weakened. Management-only defects are outside this review and have not been imported as new scope.

I found no blocker in the reviewed narrow source preservation, supported manual transition compatibility, generated facade alias, or post-review storage changes.

## Receipt assessment and limits

I inspected existing receipts only; **I did not execute tests, Gradle, a browser, servers, or device operations.** No repository edits were made. This report is the only reviewer-authored output for the final pass.

- `kotlin-test-summary.json` and an independent sum of the XML reports both give **133 test executions**, not 143:
  - Tasks client: 37 tests in 3 suites.
  - Student feature: 96 tests in 26 suites, including debug/release variants.
  - Zero failures, errors, or skips.
- Both debug and release `TasksLateResultTest` XML files contain all three tests, including `incompleteRestoreSnapshotCannotClearNewPairingPublication`, with zero failures/errors/skips.
- `final-kotlin-corrected.log` records `:tasks-client:test`, Student debug/release unit tests, Student `lintDebug`, and `BUILD SUCCESSFUL`; its exit receipt is 0. This is focused module qualification, not full native app/device acceptance.
- `final-full-tasks.log` records the API package at **334.839s**; full-test exit receipt is 0.
- `final-coverage.log` explicitly records total **71.0%** and failure below the unchanged **85%** threshold; exit receipt is 2.
- Inspected exit receipts for TypeScript, Kotlin generation/generator tests, web build/typecheck/unit/lint, Go build and vet are 0. These are producer-owned receipts, not reviewer reruns.
- `final-source.txt` still names `0655effe...`, the pre-`08bff024` checkpoint. Git confirms the only changes from that checkpoint to reviewed HEAD are `TasksSession.kt`, `TasksLateResultTest.kt`, and the Kotlin facade alias. Go/TypeScript/web source inputs are unchanged by that last correction; the corrected Kotlin XML includes the new regression. Nevertheless, do not relabel the older provenance file as an exact `08bff024` stamp. The actual reviewed source identity is recorded above.
- The reported isolated browser mixed-work completion/retained inspection was not independently rerun in this final pass. No release, native/device, live identity, deployment, full race-count10 or complete CI acceptance follows from this report.

**Final disposition:** no new source blocker; initial/follow-up source findings closed. Coverage remains an explicitly failed acceptance gate, and broader qualification claims remain limited to their actual receipts.
