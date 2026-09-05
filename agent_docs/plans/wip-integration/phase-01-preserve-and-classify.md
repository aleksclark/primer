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

- [ ] Both immutable backup refs resolve to the exact recorded SHAs on GitHub.
- [ ] Every local branch and registered worktree has a disposition.
- [ ] `fa9c6770` and `4b8924b8` have distinct patch-equivalence evidence.
- [ ] All non-equivalent leaves remain recoverable.
- [ ] `deploy/.env` is ignored, untracked, mode 0600, and never inspected.
- [ ] No branch/worktree cleanup or master rewrite occurred prematurely.
- [ ] Inventory commands and `git status --short --branch` are recorded.
