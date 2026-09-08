# Repository reconciliation — 2026-09-08

**Disposition:** CI fixes and the independently qualified Student slice are in
local `master`. The full P4 candidate is reconciled and retained, **not merged**:
its coverage gate fails at 71.0% against 85%. Current mainline also fails coverage
at 71.1%. No threshold was changed and no clean/full-CI claim is made.

## Scope and baseline

User-authorized maintenance: repair the two observed CI failures, reconcile the
pending Student and Tasks P4 changes against current mainline, review the Control
stash, archive test evidence, and retire only confirmed-obsolete local branches
and worktrees. No push, publication, deployment, or device operation is included.

Starting mainline: `ccee6c8b114c73d52876021a3d93c98d70244050`, equal to live
`origin/master` when inspected. The main checkout and every original linked
worktree had no tracked modifications. Four original worktrees held 583 loose
files: 582 browser-test evidence files plus one private `.env.local`.

## Preservation and recovery

Private local archive root (mode 0700):

```text
/home/aleks/work/archives/primer/20260908-state-cleanup/
```

- `repository-before.bundle`: verified Git bundle containing all pre-cleanup refs,
  including the Control stash and its untracked-file parent.
- `refs-before.txt`, `branches-before.txt`, `worktrees-before.txt`,
  `stashes-before.txt`: original identities and topology.
- `workspaces-before.json`, `agents-before.json`: local Paseo ownership inventory.
- `worktree-archives.json`: SHA-256, branch/commit identity, archive size, and hashes
  of loose files for each of twelve complete original worktree archives.
- `<worktree-name>.tar.gz`: full worktree snapshots, excluding only the `.git`
  pointer. Ignored local configuration, evidence, and build outputs are retained.
  Every compressed archive was read through and every loose-file hash compared
  with its source before any retirement. These are historical snapshots, not
  evidence that the reconciled source passed tests.

Archives may contain private local configuration. Do not commit/upload their
contents or extract them into a public directory. To inspect an archive, use
`tar -tzf`; recover into a new private directory rather than over a live checkout.
A retired Git branch can be reconstructed from its recorded object ID or the
bundle. `git bundle verify` and `git bundle list-heads` can inspect the bundle
without altering current refs.

## CI repairs

- Tasks run `34167419424` failed its first `gofmt` gate. Commit `9ce3715` restores
  the missing blank line before `StudentRequirement` in
  `primer-tasks/internal/api/phase2.go`. The complete Tasks module passes the
  formatting check, and baseline `go vet ./...` passes.
- Android run `34171423383` failed
  `ControlViewModelTest.periodicCatalogTickDoesNotDownloadApks` at main-dispatcher
  teardown. `1f5febfe` gives test ViewModels a real `ViewModelStore` owner, clears
  it, and joins all child work before server shutdown/`resetMain`. Three new
  regressions cover suspended auth, a sleeping ticker, and held real HTTP IO.
  The exact original timing failure did not reproduce in local baseline repeats;
  deterministic fault injection fails if cancellation or the join is omitted.
- Continuing the full suite exposed three ManagementSession test failures per
  variant. `adec7ab7` supplies schema-valid HPKE/key/hash/UUID fixtures and a
  device-capability provider with unchanged production defaults, preserving the
  generated validator. It strengthens actual HTTP report/receipt assertions and
  checks invalid API levels fail before HTTP. The invalid server-time case now
  asserts the earlier generated-boundary rejection and absence of policy effects.
- These repairs were merged locally in `3237c404`; no hosted CI rerun or push was
  performed. No test was disabled or deleted.
- The mainline coverage attempt additionally exposed an old dialogue-test replay
  race: it could mistake a prior rejection for the new injection response.
  `e32d584f` binds both rejection checks to the current message acknowledgement,
  evaluation message ID, and question ID. Assertions are stronger, not removed.
  The next full internal coverage run passed its tests, then failed at 71.1%.
  Both the earlier test failure and later coverage failure remain in `checks/`.

CI repair evidence (before Student/P4 integration):

| Command / probe | Observed result |
| --- | --- |
| Complete Tasks `gofmt -l` check; `go vet ./...` | Pass |
| Control ViewModel class, debug | 20 consecutive passes, 25 cases each |
| Omit owner cancellation / omit completion join | Both regression probes fail; mutations restored |
| ManagementSession class, debug | 5 consecutive passes, 11 cases each |
| Complete Android `test --continue` | 776 executions pass; zero failures/errors/skips |

Android runs used Java 17, `/opt/android-sdk`, `--no-daemon --max-workers=1`, and
isolated temporary test output. Full commands, XML, and earlier negative results
are preserved under `checks/control-ci-lifecycle/` in the archive.

### Separate reproduced defect, not fixed by CI repair

Successful management enrollment with `replace=true` increments the authorization
epoch in both `enroll` and `enrollLocked`; the latter invalidates the epoch
captured by the former. A schema-valid replacement success probe retained the
old token and returned the stale-enrollment message. This requires a separate
replacement/concurrent-revocation fix with preserved fencing, not removal of
security checks to make the CI suite pass. The temporary probe was restored and
its failure retained in the archive. No management-device replacement or hardware
acceptance is claimed here.

## Control stash disposition

The static review covered all 32 changed/stashed files against fixed `9ce3715`:
3 current-identical files, 9 exact blobs already in mainline history, and 20
semantically superseded drafts. No safe unique runtime behavior or stash-added
regression test required recovery. Restoring the draft would undo newer session
fencing, exact QR mount validation, schedule/DST handling, and real UI wiring.
The hosted Clerk SDK alternative is unqualified design history, not a lost
working integration.

Full local findings: `stash-semantic-review.md` and `stash-file-review.json` in the
archive. One inherited, not stash-added, cleartext-origin test assertion is noted
as optional future coverage work; no runtime policy change is justified by it.

The obsolete stash entry was dropped only after checking its exact object ID.
It remains recoverable as:

```text
refs/archive/state-cleanup-20260908/stash-control-wip
bc6e0f7adbca1f199172ae82b3aba9f9520191ab
```

The original verified bundle independently contains the same stash parent graph.
A standalone mirror restored from that bundle passed `git fsck --full`; all 32
stash file blob IDs matched the inventory. The restoration log and private mirror
remain in the archive directory.

## Student slice integrated independently

Commit `d337b0ae`, merged as `fbebc11b`, ports the final Student baseline plus
reviewed navigation and storage corrections from P4 candidate `08bff024` without
importing the coverage-blocked P4 Go/browser changes:

- Server-capability-controlled actions and truthful unsupported/status labels.
- Unavailable restore state and Back-to-Today behavior that stays inside Tasks.
- Immutable detail request identity and generation fences for late responses.
- Serialized pairing publication, initial restore snapshots/incomplete cleanup,
  conditional clearing, and final guarded publication. Network calls remain
  outside the storage mutex. Deterministic tests cover the old-null-read/new-pair
  race and replaced/revoked binding outcomes.
- Labeled image/paste pairing fallbacks and a generated `StudentRequirement`
  typealias, not a copied DTO.
- Historical A16 notes retained with explicit historical-only qualification.

Standalone Student/mainline checks at exact `d337b0ae`:

| Gate | Result |
| --- | --- |
| Current-source OpenAPI/Kotlin regeneration | Pass |
| Full Android `test --continue`, unchanged rerun | 806 pass, zero failures/errors/skips |
| Student unit tests | 48 per variant pass |
| Control unit tests | 47 per variant pass |
| Student `lintRelease` | Pass, 3 warnings / no errors |
| Control `lintDebug` | Pass, 4 warnings / no errors |
| Full `assembleDebug --continue` | TV, Student, and Control APKs pass |

**Reliability caveat:** an unchanged TV pairing-token timeout failed the first
full run and 1 of 5 focused class repeats before the full rerun passed. This is
not an unqualified reliably green all-Android gate. No TV source/test change was
made to conceal it. All failed and passing receipts are preserved under
`checks/android-final-student/`. These are JVM/build checks, not device acceptance.

## P4 candidate retained behind the coverage gate

```text
branch: reconcile/student-p4-current
candidate tip: ec042b4c02d69f8ad97bda55361c99b43ed4f8ae
final code: 08bff02470089616e9f90389b509116967ecd2a3
worktree: .worktrees/13yu1btw/student-p4-current
```

The candidate reconciles `89f3a593`, `fd34a7bd`, and `825bef5c` against the current
producer/identity/client boundaries rather than merging old branches wholesale.
It preserves native capability metadata and all migration bytes; adds typed
requirement-bound browser manual actions and retained inspection; keeps native
bearer `/device` separate from cookie/Origin/CSRF browser custody; and validates
native single-manual envelopes and current device authority transactionally.

Independent source review found no remaining source blocker after malformed-query,
late-navigation, and stale-restore corrections. The full Tasks suite, vet/build,
TS generation/boundary/runtime/generation/public-conformance tests, Kotlin
qualification, web lint/typecheck/26 unit tests/build passed on their recorded
source snapshots. Fresh real-stack Chrome exercised mixed manual/dialogue work,
rejected answers, reload at 2/3, retained inspection, and all-requirement completion.
This is not independent browser promotion, full race/count10, native Control,
live-provider, release, or deployment acceptance.

**Coverage is still failed: 71.0% < 85%.** The independently rerun repaired
mainline is also below the unchanged gate at 71.1%. A read-only measurement audit
confirmed that public server subprocesses are not coverage-instrumented and the
gate reads only the parent test profile. No child counters were fabricated,
percentages averaged, package denominator reduced, or instrumentation scheme
silently introduced. The measurement audit does not establish that proper child
collection alone would reach 85%; residual gaps may require more tests.

Retained candidate records:

- `agent_docs/plans/wip-integration/student-p4-current-reconciliation.md`
- `agent_docs/plans/wip-integration/student-p4-current-review.md`
- `test-artifacts/student-p4-current/receipts.json`

Those files live on the **candidate branch**, not mainline. Complete local logs
and browser evidence are copied to `checks/student-p4-current/` in the archive;
`coverage-measurement-review.md` describes the legitimate collection approach and
required negative qualifications. The earlier Android checkpoint compilation
failure is separately retained under `checks/android-p4-checkpoint/`, not relabeled
as a final-code failure or pass.

Next integration gate: implement and qualify accurate same-source child-process
coverage collection and/or add real missing coverage; remeasure with the full
internal-package denominator and unchanged 85% floor. Complete remaining required
race/browser qualification before treating P4 as accepted. Do not merge the
candidate merely because source review and functional tests passed.

## Cleanup result

- **13 worktrees retired:** 11 original inactive worktrees plus 2 temporary
  verification worktrees. All had complete verified archives first. The old P4
  worktree copies were retired after handoff; their original branch refs remain
  available alongside the retained candidate because integration is blocked.
- **65 local branch heads retired:** 63 original heads and 2 temporary verification
  heads. Original proofs cover 57 mainline ancestors, 4 fully patch-equivalent
  branches, the content-reviewed Educator branch, and the reconciled Student
  branch. The temporary CI checkout was an ancestor of the retained P4 candidate;
  its actual CI repair commits are already on mainline. The temporary Student
  checkout was a mainline ancestor.
- Every deleted branch tip is preserved at
  `refs/archive/state-cleanup-20260908/heads/<original-branch>`.
- **No active stash entries:** the reviewed Control stash has its own archive ref
  and verified standalone bundle restoration.

`retired-worktrees.json` and `retired-branches.json` record paths, exact heads,
proof categories, recovery refs, and archived workspace IDs. New verification
and retained-candidate snapshots are in `reconciliation-worktree-archives.json`.
Associated idle Paseo workspaces were archived before worktree removal where
registered. Targets were checked for live non-agent processes and changed
source/loose files. No blanket `git clean`, remote branch deletion, or PR closure
was performed.

Four worktrees remain: mainline, the blocked P4 candidate, the independent active
pairing-flow task, and the live `wet-parrot` development/device workspace.
Nineteen local branches remain, including unmerged donor/roadmap work and the open
Go-pinning PR lineage. Those are not obsolete merely because they are old.

Explicit exclusions:

- `update-primer-student-pairing-flow` / `anxious-chipmunk` is an independent active
  user task. Its branch, worktree, and agent are not cleanup targets.
- `implement-primer-tasks-android-apps` / `wet-parrot` has live emulators and a
  Tasks development stack. Its worktree and processes remain untouched even if
  its source changes are superseded by the reconciliation.
- Unmerged donor implementations and roadmap work are not obsolete merely because
  they are old. Ancestry, patch/content equivalence, and runtime ownership must be
  checked before retirement. No remote branch or pull request is deleted here.
