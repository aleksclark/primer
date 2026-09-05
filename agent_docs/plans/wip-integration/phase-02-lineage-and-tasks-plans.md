# Phase 02: Restore lineage and land Tasks plans

## Goal

Land the unique Primer Tasks planning package from local `master` on current
protected history without replaying the already-merged live-LLM patch. After the
PR merges, make the main local `master` worktree match fetched protected history
while the immutable backup retains its old topology. This creates the canonical
document authority for six implementation PRs and the later Android continuation.

## BDD Success Criteria

### Scenario: Only the unique planning patch is proposed

- **Given** `4b8924b8` follows patch-equivalent `fa9c6770` on local `master`
- **When** the plan feature branch is compared with current `origin/master`
- **Then** it adds the WIP-integration plan and the 13 files under
  `agent_docs/plans/primer-tasks{,-android}/`
- **And** it does not reintroduce any code/config diff from `fa9c6770`.

### Scenario: The planning PR is navigable and internally consistent

- **Given** the new plan directories
- **When** a reviewer resolves every phase link and compares phase tables to
  filenames
- **Then** every link resolves, numbering/names agree, BDD/E2E/audit/gates are
  present, and Tasks Phase 7 plus later Android work remain explicitly incomplete.

### Scenario: Protected lineage is restored without losing old history

- **Given** the feature PR is merged and both Phase 01 backups are verified
- **When** the clean main worktree updates local `master` to fetched
  `origin/master`
- **Then** local `master` has zero ahead/behind and is an ancestor/equal of the
  protected tip
- **And** `4b8924b8` and `fa9c6770` remain reachable from the remote backup.

### Scenario: A changed protected base blocks an unsafe merge

- **Given** `origin/master` advances while the PR is open
- **When** required checks or merge state are reassessed
- **Then** the feature branch is updated normally, never force-pushed over
  reviewed commits
- **And** links/diffs/gates are rerun before merge.

## Implementation Instructions

- On a feature branch based on fetched `origin/master`, transplant
  `4b8924b8` only (path checkout or cherry-pick followed by a strict diff). Do
  not merge local `master` and do not transplant `fa9c6770`.
- Include this `agent_docs/plans/wip-integration/` package in the planning PR.
  Keep plan/status claims evidence-backed; do not mark implementation phases
  complete merely because the donor has archived screenshots.
- Validate markdown links and required sections mechanically. Compare the final
  Tasks planning paths against `4b8924b8` and the donor tip; they should be
  tree-equal.
- Open a feature PR, wait for all applicable checks, review the exact head, and
  merge through GitHub only when green.
- Only after merge and backup verification, confirm the main checkout is clean
  (ignored `deploy/.env` is allowed) and update local `master` to
  `origin/master`. A hard reset is allowed only because the old exact tip is
  already immutable on the backup head; never push local `master` directly.

## End-to-End Test Plan

- `git diff --name-status origin/master...HEAD` must list only the intended plan
  package. Use `git diff origin/master...HEAD -- primer-agents server` to assert
  no duplicate live-LLM code.
- Resolve every relative Markdown link in both new plan directories and assert
  exactly one index plus one zero-padded file per enumerated phase.
- `git diff --quiet 4b8924b8 HEAD -- agent_docs/plans/primer-tasks
  agent_docs/plans/primer-tasks-android` must pass after transplant.
- Verify the PR through `gh pr view`/`gh pr checks` at the exact head. After
  merge, fetch and assert the merge is reachable from `origin/master`.
- In the main worktree, assert `git status --short --branch` is clean and
  `git rev-list --left-right --count origin/master...master` is `0 0`.

## Anti-Cheating Audit

- Inspect the PR patch for hidden code from `fa9c6770`, generated artifacts, or
  unrelated status rewrites.
- Reject link checks that only grep text without resolving paths.
- Reject a direct push, force update, or history rewrite of remote `master`.
- Verify old history through the remote backup after local lineage repair; a
  local reflog alone is insufficient.
- Ensure no phase status claims physical-device/live-provider evidence.

## Completion Gate

- [ ] The plan feature PR contains only intended planning files.
- [ ] `fa9c6770` is absent as a unique PR patch.
- [ ] All plan structure/link/traceability checks pass.
- [ ] Required GitHub checks and independent review are green at exact head.
- [ ] PR is merged through protected-branch flow.
- [ ] Local main `master` matches fetched `origin/master` after backup recheck.
- [ ] Working trees remain clean and all old commits remain remotely reachable.

## Execution status (2026-09-05, this worktree)

Local transplant only. No PR, merge, push, or local-`master` reset has happened, so this phase is **not** complete.

- Checked out the 13 files from `4b8924b8` onto `initiative/00-wip-integration`. Did not merge local `master` and did not replay `fa9c6770`.
- `git diff --quiet 4b8924b8 HEAD -- agent_docs/plans/primer-tasks agent_docs/plans/primer-tasks-android` and the same comparison against `impl/tasks-p6-external` both pass after transplant.
- Relative Markdown links and required phase sections resolve in `primer-tasks/`, `primer-tasks-android/`, and `wip-integration/`.
- `git diff origin/master --` the five `fa9c6770` paths is empty. Remaining Phase 02 gates (feature PR, GitHub checks, protected merge, local `master` reset) are blocked until a later editor is authorized for remote operations.
