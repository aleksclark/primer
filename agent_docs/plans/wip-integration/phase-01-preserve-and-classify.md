# Phase 01: Preserve and classify WIP

## Goal

Create a recoverable, exact inventory before any integration or branch cleanup.
This phase comes first because ancestry, patch equivalence, and a remote backup
are prerequisites to deciding what may be transplanted or archived. It enables
the plan-only PR and all later snapshot integrations without risking local-only
history.

## BDD Success Criteria

### Scenario: Exact Tasks and local-master history is recoverable remotely

- **Given** local `master` at `4b8924b8` and Tasks donor at `d6e89d4a`
- **When** an operator queries GitHub heads without relying on local reflogs
- **Then** `backup/local-master-pre-integration-20260905` resolves exactly to `4b8924b8`
- **And** `backup/tasks-p6-external-pre-integration-20260905` resolves exactly to `d6e89d4a`.

### Scenario: Duplicate and unique local-master patches are distinguished

- **Given** fetched `origin/master` at `b7a2027c`
- **When** `git cherry -v origin/master master` is evaluated
- **Then** `fa9c6770` is reported patch-equivalent to merged history
- **And** `4b8924b8` is reported unique
- **And** no integration procedure merges both local commits wholesale.

### Scenario: Local-only leaves are classified without data loss

- **Given** the Tasks P6, Agents, Studio, historical, and initiative refs
- **When** ancestry, patch IDs, tree differences, upstreams, and worktree
  registrations are recorded
- **Then** each ref is labelled active donor, possible donor, merged/patch
  equivalent, intentionally retained initiative, or unresolved
- **And** unresolved or non-equivalent refs remain preserved and unmodified.

### Scenario: Ignored deployment credentials have safe local metadata

- **Given** an ignored `deploy/.env` exists in the main checkout
- **When** its metadata is checked without opening the file
- **Then** mode is 0600 and it is not tracked
- **And** no value, hash, byte count, or content from the file appears in logs or commits.

## Implementation Instructions

- Fetch heads and PR refs without pruning local refs. Record the exact fetched
  `origin/master`, local branch tips, `git worktree list --porcelain`, and open
  PR metadata.
- Verify the two existing backup heads with `git ls-remote`. If either differs,
  stop; create a new immutable backup name rather than force-updating it.
- Build a classification table from `git rev-list --left-right --count`,
  `git merge-base --is-ancestor`, `git cherry`, and path-scoped tree diffs.
- Explicitly record that the four Tasks leaves diverge at `e196dd2c`. Inspect
  `7b0231c3` against the final P6 files because it is not patch-equivalent even
  though later donor commits appear to supersede it.
- Treat `agents-phase1-foundation` only as a possible source of individual
  behavioral coverage tests. Its different test layout is not authority to
  replace merged PR #56.
- Do not remove/rename branches or unregister worktrees in this phase. Cleanup
  is Phase 12 after integrated tree comparison.
- Use `stat`, `git ls-files`, and `git check-ignore` for `deploy/.env`; never use
  `cat`, `read`, checksum tools, editor APIs, or commands that expand its values.

## End-to-End Test Plan

- From a shell with GitHub access, run:
  `git ls-remote --heads origin refs/heads/backup/local-master-pre-integration-20260905 refs/heads/backup/tasks-p6-external-pre-integration-20260905`.
  Assert exact object IDs, not only head names.
- Run `git cherry -v origin/master master` and retain the `- fa9c6770` / `+
  4b8924b8` evidence.
- For every local branch, record ahead/behind, ancestry, patch-equivalent counts,
  upstream, and active worktree path. Manually inspect all non-zero unique sets.
- Run `stat -c '%a %U:%G %n' /home/aleks/work/projects/primer/deploy/.env`,
  `git ls-files deploy/.env`, and the appropriate in-repository `git
  check-ignore -v deploy/.env`. Assert 600, no tracked path, and an ignore rule.
- No fake remote is permitted: remote preservation is proven against `origin`.

## Anti-Cheating Audit

- Check that backup verification compares object IDs and does not merely test
  that similarly named branches exist.
- Check that `git cherry`/tree comparisons are used; matching commit subjects
  are insufficient proof of equivalence.
- Check that non-ancestor Tasks/Agents refs were not deleted because a newer
  branch “looks complete.”
- Check shell history and commits for accidental `deploy/.env` content, hashes,
  command substitution, or staged secret files.
- Check that initiative 01–04 worktrees were not classified stale merely because
  they currently share the predecessor SHA.

## Completion Gate

- [x] Both immutable backup refs resolve to the exact recorded SHAs on GitHub.
- [x] Every local branch and registered worktree has a disposition.
- [x] `fa9c6770` and `4b8924b8` have distinct patch-equivalence evidence.
- [x] All non-equivalent leaves remain recoverable.
- [x] `deploy/.env` is ignored, untracked, mode 0600, and never inspected.
- [x] No branch/worktree cleanup or master rewrite occurred prematurely.
- [x] Inventory commands and `git status --short --branch` are recorded.

## Execution status (2026-09-05, this worktree)

Independently re-verified without pruning, deleting, renaming, or unregistering anything. No history rewrite. Secret-file values were never read, hashed, counted, grepped, expanded, or printed.

| Check | Result |
|---|---|
| `git fetch --no-prune` `origin/master` | still `b7a2027c24fbf63be0c4473f15dfcc5911803079` |
| `git ls-remote` `backup/local-master-pre-integration-20260905` | `4b8924b800a9090cc1d89d0d434144400b53008b` |
| `git ls-remote` `backup/tasks-p6-external-pre-integration-20260905` | `d6e89d4abad814c94f89fe857417059ef391eb6c` |
| `git cherry -v origin/master master` | `- fa9c6770` (patch-id equal to merged `630f2fce`); `+ 4b8924b8` unique |
| `git rev-list --left-right --count origin/master...master` | `12 2` |
| `stat -c '%a %U:%G %n'` on main-checkout `deploy/.env` | mode `600`; `git ls-files` empty; `git check-ignore -v` matches `.gitignore` `.env` |

### Branch / worktree dispositions (no archival)

| Ref / worktree | Tip | Disposition |
|---|---|---|
| local `master` @ `/home/aleks/work/projects/primer` | `4b8924b8` | preserve; transplant only unique plan commit |
| `impl/tasks-p6-external` | `d6e89d4a` | active donor |
| `impl/tasks-p6-db-concurrency` | `325541b2` | Tasks leaf; diverges at `e196dd2c`; `git cherry` `-` vs donor |
| `impl/tasks-p6-fixture-security` | `fff6c1c5` | Tasks leaf; diverges at `e196dd2c`; both unique commits `git cherry` `-` vs donor |
| `impl/tasks-p6-web-clients` | `b5c9f87d` | Tasks leaf; diverges at `e196dd2c`; `git cherry` `-` vs donor |
| `impl/tasks-p6-protocol-worker` | `7b0231c3` | Tasks leaf; diverges at `e196dd2c`; `06c962c0` is `-`, hardening `7b0231c3` is `+` vs donor. Keep recoverable until Phase 12 proves semantic supersession |
| `agents-phase1-runtime` | `1b967b91` | possible coverage/runtime donor only; not a merge candidate |
| `agents-phase1-foundation` | `09c0d7a5` | possible coverage-test donor only; different test layout; never merge wholesale |
| `impl/S11-materialization` | `486e647d` | Studio; `git cherry` `-` vs `origin/master` (PR #64) |
| `impl/S12-workflow` | `976a254c` | Studio ancestor of `origin/master` (behind 8) |
| `impl/s12-workflow` | `3b5993dd` | Studio ancestor of `origin/master` (behind 1) |
| `impl/s14-outbox-webhooks` | `f2b3121d` | Studio ancestor of `origin/master` (behind 6) |
| worktree `.worktrees/impl-S11-materialization` | clean `impl/S12-workflow` @ `976a254c` | registered stale clean worktree; archive only in Phase 12 |
| `/tmp/primer-tasks-readonly` | detached `d6e89d4a` | read-only donor checkout; leave registered |
| `initiative/00-wip-integration` | this branch | active integration editor |
| `initiative/01-authstack`, `04-studio-completion` | `b7a2027c` | intentionally retained successor worktrees |
| `initiative/02-ultracore`, `03-forgejo-migration` | unique planning tips | intentionally retained successor worktrees |
| `chore/go-1-26-6-clean` (`a0ae7d5c`) / `chore/go-1-26-6` | unique vs master | PR #66 reassessment later; not a merge candidate now |
| `primer-standalone-task-verification-plan` | unique `ec36e559` | preserve; not an integration donor |
| remaining historical product branches | ancestors of `origin/master` | merged/ancestor; archive only in Phase 12 |

`git status --short --branch` in this worktree was clean on `initiative/00-wip-integration` before the Phase 02 transplant. Main checkout: `master...origin/master [ahead 2, behind 12]`, clean. Stale S12 worktree: clean and behind 8.
