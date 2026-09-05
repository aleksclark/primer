# WIP integration and baseline repair

## Outcome

Complete this plan to put Primer Tasks phases 1–6 on the protected `master`
lineage in reviewable slices, preserve every relevant pre-integration ref, repair
the already-reviewed release/coverage/generation/Android baseline, reconcile the
roadmap to repository truth, and leave an exact-tip handoff for the authstack,
Ultracore, Forgejo, and Studio-completion initiatives.

The protected branch is never force-pushed or used as an ad-hoc integration
branch. Each independently reviewable phase uses a feature branch and PR, is
merged only after its required checks are green, and is re-based/re-tested if
`origin/master` moves.

## Current-state summary

Evidence was collected on 2026-09-05 after fetching GitHub heads and PR refs.
The authoritative baseline is `origin/master` at
`b7a2027c24fbf63be0c4473f15dfcc5911803079` (PR #69).
Phase 01 was independently re-verified in this worktree (exact backup object IDs,
`git cherry` `- fa9c6770` / `+ 4b8924b8`, branch/worktree dispositions, ignored
`deploy/.env` mode 0600). Phase 02 has a local plan transplant only; it is not
complete until a feature PR merges and local `master` is reset to fetched
`origin/master`.

### Refs and WIP

| Area | Verified state | Evidence / disposition |
|---|---|---|
| Local `master` | `4b8924b800a9090cc1d89d0d434144400b53008b`, 2 ahead / 12 behind by ancestry | `fa9c6770` is patch-equivalent (`git cherry` `-`) to merged `630f2fce`; `4b8924b8` is the only unique patch and adds 13 Primer Tasks plan files. Preserve, then transplant only `4b8924b8`. |
| Active Tasks WIP | `impl/tasks-p6-external` at `d6e89d4abad814c94f89fe857417059ef391eb6c`, 213 ahead / 38 behind, 463 files, 48,673 insertions / 34 deletions | Phases 1–6 are implemented in the donor lineage; Phase 7 and Android continuation after the phase-2 baseline remain incomplete. Integrate as six snapshots, not one 213-commit PR. |
| Remote preservation | Exact backups already pushed | `backup/local-master-pre-integration-20260905` → `4b8924b8`; `backup/tasks-p6-external-pre-integration-20260905` → `d6e89d4a`. Verify before any history operation. |
| Tasks P6 leaves | Four local-only branches diverge at `e196dd2c`; each is behind the final donor and most patches are patch-equivalent there | Treat as donor/audit refs, not merge candidates. `impl/tasks-p6-protocol-worker` has one non-patch-equivalent hardening commit, so prove the final donor semantically contains or supersedes it before archival. |
| Agents leaves | `agents-phase1-runtime` and `agents-phase1-foundation` diverge from current master | Merged PR #56 supplies the service, but the foundation branch contains a different coverage-test strategy. Treat as possible coverage donors only; never merge wholesale. |
| Studio leaves | Uppercase `impl/S11-materialization` is patch-equivalent to merged PR #64; uppercase `impl/S12-workflow`, lowercase `impl/s12-workflow`, and `impl/s14-outbox-webhooks` are merged/ancestor tips | Proven stale as integration branches. One clean registered worktree still points at uppercase `impl/S12-workflow`; archive only after final proof and ref preservation. |
| Other local branches | Historical product branches are ancestors of `origin/master`; initiative 01–04 worktrees intentionally remain at the predecessor baseline | Do not disturb successor-initiative worktrees. Classify historical refs in Phase 1 and archive only in Phase 12. |
| PR #66 | `chore/go-1-26-6-clean` at `a0ae7d5c`, mergeable but `UNSTABLE`, cancelled foundation check, behind master | Rebase/reassess after integration; do not blindly merge. It must include every maintained module introduced by Tasks and pass the complete gate matrix. |

### Independently reproduced baseline failures

| Gate | Result at exact baseline | Evidence |
|---|---|---|
| LMS/server coverage | **PASS 86.4%** against 85% | `make cover` in detached clean baseline |
| Studio raw clean-checkout test | **FAIL** before generation: missing protobuf and Go REST generated packages | `cd curriculum-studio && go test ./... -count=1` |
| Studio official generation | Generates clients but dirties tracked `go.work.sum` with `google.golang.org/grpc v1.83.0 h1:JeNZ...` | `make -C curriculum-studio clients-go-grpc-build`; generated test then passes |
| Studio coverage | **FAIL 71.8%** in this run (previous review observed 71.9%) against 85% | `make studio-cover` after official generation |
| Identity coverage | **FAIL 79.8%** against 80% | `make identity-cover` |
| Agents coverage | **FAIL 84.9%** against 85% | `make agents-cover` |
| LMS image | **FAIL** | `Dockerfile` cannot resolve replaced `../primer-agents/client/go/go.mod` because the stage never copies it |
| TV image | **FAIL** | Same missing local replacement in `Dockerfile.tv` |
| content-ingest image | **FAIL** | Same missing local replacement in `Dockerfile.ingest` |
| Android pairing stress | Current independent forced 5-run sample passed 5/5; prior reviewed runs failed initially and 1/5 | A non-reproduction does not clear a known race. Make synchronization deterministic and require a larger repeated gate. |
| Roadmap docs | Stale | Merged history includes Identity through I14, contracts through C12, and Studio S11/S12/S14/S19, while indexes still advertise earlier cursors. |
| Local secret-file mode | `deploy/.env` in the main checkout was 0644 | Values were never read. Mode was repaired to 0600 and the file remains ignored/untracked. |

A trial merge of `impl/tasks-p6-external` into current `origin/master` was also
reproduced. Git conflicts only in `Makefile` and `go.work.sum`; `go.work` merges
automatically. This is conflict-shape evidence, not semantic validation.

## Scope boundaries

### In scope

- Exact ref/worktree/PR inventory and remote preservation before risky history work.
- Transplanting the unique Primer Tasks plans through a feature PR.
- Integrating the existing Tasks phase 1–6 donor as six reviewable, sequential
  PRs while preserving current Studio, Identity, Agents, LMS, TV, and Compose
  root wiring.
- Revalidating Tasks through its public HTTP/WebSocket/browser/Android/process
  boundaries, including real PostgreSQL, MinIO, and the separate verifier
  fixture where the donor plan requires them.
- Fixing clean-checkout generation, generated drift checks, three production
  Docker builds, module coverage floors, and the Android pairing race.
- Strengthening pull-request CI so release failures cannot be masked by job
  ordering or master-only image builds.
- Updating roadmap/status/runbooks, safely classifying stale refs/worktrees, and
  recording the exact integrated tip and honest blockers.
- Reassessing/rebasing PR #66 against the final module set; merging it only if
  its own gates pass.

### Out of scope

- Primer Tasks Phase 7 product work, production packaging, invitations, or live
  operations beyond integration/readiness repairs already present in the donor.
- Primer Tasks Android dialogue/media/external-verifier/release phases. The
  pairing and checklist baseline is retained; later native work remains a
  successor plan.
- Authstack, Ultracore, Forgejo migration, or the missing Studio S13/S15–S18
  product waves.
- Live billable model, live Stytch, physical-device, production deploy, or other
  provider evidence unless it is actually run with explicit authorization.
- Lowering any coverage threshold, committing secrets, or treating stored
  screenshots/logs as a substitute for a live browser/process test.

## Global constraints

1. `AGENTS.md` and existing service ownership boundaries are authoritative.
2. Never force-push or push ad-hoc commits to `master`. Never discard an
   unpreserved ref, commit, or worktree change.
3. Use one sequential editor in the integration workspace. Review agents do not
   mutate while an implementer is active.
4. Every Tasks slice starts from the then-current merged master and preserves
   the exact donor endpoint. Root files are semantically reconciled; never
   resolve `Makefile`, `go.work`, or `go.work.sum` with blanket ours/theirs.
5. The six donor endpoints are immutable evidence:
   - P1 `62795d53`
   - P2 `8274d217`
   - P3 `dc8cedb0`
   - P4 `89583ced`
   - P5 `e196dd2c`
   - P6 `d6e89d4a`
6. Final Primer Tasks planning files come from `4b8924b8` and are excluded from
   intermediate donor snapshots. The remote backup preserves original history.
7. Generated Studio clients remain untracked by policy. Official generation
   must be deterministic and must leave tracked files unchanged.
8. Coverage floors remain LMS 85%, Studio 85%, Identity 80%, Agents 85%, and
   Tasks 85%. Add behavioral tests; do not manipulate package sets, generated
   code, or dead code merely to move percentages.
9. Real persistence/durability claims use the production repository path with
   real PostgreSQL. Media uses real S3-compatible object storage; external
   verifier proof uses a separate process. Permitted model/JWKS fixtures remain
   explicitly credential-free and cannot support live-provider claims.
10. Browser verification uses the running application and managed headless
    browser by default. Direct handler calls, fixture HTML, screenshots, and
    mocked browser transports do not satisfy browser acceptance.
11. Never read or print `deploy/.env`; only metadata/permissions may be checked.
    It stays ignored and mode 0600.
12. PRs merge only with required local gates and GitHub checks green at the
    exact head. If master moves, rebase/merge-forward and rerun affected gates.
13. Physical-device/live-provider/deployment evidence is reported as BLOCKED or
    not run unless actually observed.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 01: Preserve and classify WIP](./phase-01-preserve-and-classify.md) | Freeze exact refs, backups, branch/worktree classifications, and safe local secret metadata before integration. | None |
| [Phase 02: Restore lineage and land Tasks plans](./phase-02-lineage-and-tasks-plans.md) | Transplant only the unique plans through a PR and return local `master` to the fetched protected lineage without losing history. | Phase 01 |
| [Phase 03: Integrate Tasks foundation and pairing](./phase-03-tasks-foundation-pairing.md) | Land donor P1 as the first standalone Tasks service/client slice. | Phase 02 |
| [Phase 04: Integrate tasks, schedules, and manual approval](./phase-04-tasks-schedules-manual.md) | Land donor P2 with durable schedules, occurrences, checklist, and parent approval. | Phase 03 |
| [Phase 05: Integrate parent agent command chat](./phase-05-parent-agent-chat.md) | Land donor P3 Fantasy job/WebSocket/tool-confirmation path. | Phase 04 |
| [Phase 06: Integrate student dialogue verification](./phase-06-student-dialogue.md) | Land donor P4 web dialogue verification while leaving later Android continuation explicit. | Phase 05 |
| [Phase 07: Integrate media rubric verification](./phase-07-media-rubric.md) | Land donor P5 real object-storage artifact and asynchronous rubric path. | Phase 06 |
| [Phase 08: Integrate external verifiers](./phase-08-external-verifiers.md) | Land donor P6 signed, idempotent, SSRF-fenced separate-process verifier protocol. | Phase 07 |
| [Phase 09: Repair clean generation, images, and CI](./phase-09-generation-images-ci.md) | Make clean-checkout generation deterministic, fix all three production images, and enforce those gates on PRs. | Phase 08 |
| [Phase 10: Restore all coverage floors](./phase-10-coverage-floors.md) | Raise Studio, Identity, and Agents through substantive tests without lowering gates. | Phase 09 |
| [Phase 11: Eliminate Android pairing flake](./phase-11-android-flake.md) | Remove timing/order dependence and add repeated Android regression enforcement. | Phase 09 |
| [Phase 12: Reconcile status and archive proven-stale state](./phase-12-docs-and-stale-state.md) | Correct roadmap/runbooks and archive only refs/worktrees proven superseded. | Phases 10–11 |
| [Phase 13: Certify exact master and hand off](./phase-13-certification-handoff.md) | Run full clean-clone/public-boundary/image/CI certification, settle PR #66, and publish an exact-tip machine-readable handoff. | Phase 12 |

Phases 10 and 11 may be developed on separate branches after Phase 09, but they
must not edit the same workspace concurrently and both must be merged before
Phase 12. All Tasks phases remain strictly sequential because each consumes the
previous service schema and client contract.

## Requirement traceability

| Requirement | Phases |
|---|---|
| Preserve local master, Tasks WIP, leaves, and stale refs before history work | 01, 12 |
| Land only unique Tasks plans; avoid duplicate `fa9c6770` | 02 |
| Integrate 48k-line Tasks WIP in reviewable slices | 03–08 |
| Preserve newer Studio/Identity/Agents/root work | 03–09 |
| Fix three production Docker builds | 09, 13 |
| Clean-checkout generation and generated drift | 09, 13 |
| Studio/Identity/Agents coverage floors without lowering gates | 10, 13 |
| Android pairing flake and repeated checks | 11, 13 |
| Public Tasks UI/API boundaries with real browser/process dependencies | 03–08, 13 |
| Reconcile Identity I14, Studio S11/S12/S14/S19, contracts C12 docs | 12 |
| Safe ignored `deploy/.env` permissions without secret inspection | 01, 12 |
| Reassess PR #66 rather than blindly merge | 09, 13 |
| Exact master/CI/status/handoff for next orchestrator | 13 |

## Completion rule

This plan is complete only when all 13 phase gates are satisfied; all merged PR
heads are reachable from the same fetched `origin/master`; a fresh clone at the
recorded SHA passes generation, full tests, all four coverage floors, Tasks
public-boundary tests, Android repeated tests, generated-drift checks, and the
three production Docker image builds; GitHub required checks are green; git
status is clean; stale state is archived with recoverable refs; and
`handoff.json` truthfully records the exact tip, merged PRs, commands/results,
remaining Tasks/Studio/authstack blockers, and all unrun live/physical evidence.
