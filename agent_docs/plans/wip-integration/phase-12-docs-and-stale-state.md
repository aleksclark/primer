# Phase 12: Reconcile status and archive proven-stale state

## Goal

Make roadmap/runbook/status documents match the merged repository and archive
only branches/worktrees whose unique behavior is now proven integrated or
superseded. This removes misleading cursors before final certification while
retaining recoverable history and successor-initiative worktrees.

## BDD Success Criteria

### Scenario: Roadmap cursors match merged code

- **Given** protected history through all integration/repair PRs
- **When** a reader opens Studio delivery/platform/contracts, Identity, Agents,
  and Tasks indexes/runbooks
- **Then** Identity through I14, contracts through C12, and Studio
  S11/S12/S14/S19 are accurately recorded as merged/present
- **And** missing Studio S13/S15–S18, Tasks P7, and later Tasks Android work are
  clearly incomplete rather than silently implied complete.

### Scenario: Evidence claims are exact and bounded

- **Given** historical live/provider/emulator wording and current certification
- **When** status text is reconciled
- **Then** each claim cites a reachable commit/PR and actual command/evidence
- **And** physical-device, production deploy, live Stytch/model, or blocked
  evidence is not inferred from fixtures, titles, screenshots, or old branches.

### Scenario: Proven stale worktree is removed safely

- **Given** the registered uppercase S12 worktree is clean and its tip is an
  ancestor of merged master
- **When** it is unregistered/removed
- **Then** no untracked or modified file is lost
- **And** the commit remains reachable through master, remote PR head, or an
  explicit archive ref.

### Scenario: Diverged leaves remain recoverable after classification

- **Given** Tasks leaves and Agents donor branches are not direct ancestors
- **When** semantic comparison proves integrated/superseded behavior
- **Then** exact tips are pushed/renamed to non-force-updated archive/backup refs
  before active branch cleanup
- **And** any unresolved invariant keeps its branch active and blocks archival.

## Implementation Instructions

- Derive status from `git log --first-parent`, merged PR metadata, package/routes,
  migrations, and passing exact-head tests. Update at least:
  `curriculum-studio-delivery/{index,execution-index}.md`,
  `curriculum-studio-platform/index.md`,
  `curriculum-studio-contracts/index.md`,
  `primer-identity-service/index.md`, relevant phase headers/runbooks, Agents
  coverage/status runbooks, and both Tasks indexes/evidence status.
- Remove duplicate/stale rows and contradictory “current cursor” paragraphs.
  Preserve historical filenames/IDs where links depend on them.
- State the actual Studio present/missing matrix and distinguish implementation
  presence from full production/live acceptance.
- Re-run relative link and phase-number checks after edits.
- Before removing the stale registered worktree, record `git status`, HEAD,
  branch, ancestry, and archive reachability. Refuse removal if dirty.
- For diverged Tasks/Agents refs, create immutable remote backups or local
  `archive/...` refs before deleting/renaming active heads. The P6 leaf
  `7b0231c3` audit must be complete first.
- Do not archive initiative 01–04 worktrees; they are intentional successors.
- Recheck `deploy/.env` metadata only: ignored, untracked, mode 0600. Never
  include it in cleanup commands or docs with values.

## End-to-End Test Plan

- Mechanically resolve all Markdown links and assert each phase overview matches
  existing zero-padded files.
- Cross-check every completed status SHA/PR against `git merge-base
  --is-ancestor <sha> origin/master` or patch-equivalence evidence.
- Search for stale cursor phrases (`C10`, pre-I14, S11 “on branch”, S12 “next”)
  and manually review each hit in context.
- Before/after worktree cleanup, run `git worktree list --porcelain`, exact path
  `git status --short --branch`, and `git show-ref` for recovery refs.
- Verify archived remote heads with `git ls-remote`, then verify active branch
  list has only intended work.
- Run metadata-only `.env` checks and repository secret scans that do not open
  ignored files.

## Anti-Cheating Audit

- Check statuses against code/merged ancestry and tests, not commit/PR titles.
- Check “complete” does not combine credential-free implementation with blocked
  live/physical/production proof.
- Check no stale branch is blindly merged to make its status true.
- Check branch/worktree cleanup logs prove cleanliness and recovery before
  removal; reflog-only recovery is insufficient for non-ancestor tips.
- Check successor worktrees and backup refs remain intact.
- Check secret scans/cleanup did not read or stage ignored `deploy/.env`.

## Completion Gate

- [ ] Identity/Studio/contracts/Agents/Tasks indexes and runbooks agree with
      merged exact-tip reality.
- [ ] Missing S13/S15–S18, Tasks P7, later Tasks Android, and all blocked live
      evidence remain explicit.
- [ ] Links, phase numbering, SHAs, PRs, and command claims validate.
- [ ] Stale S12 worktree is removed only after clean/reachable proof.
- [ ] Diverged Tasks/Agents leaves are remotely/local-archive recoverable before
      active cleanup; unresolved refs remain.
- [ ] Initiative 01–04 and immutable backups remain untouched.
- [ ] `deploy/.env` remains ignored/untracked/0600 and uninspected.
- [ ] Docs/cleanup PR is independently reviewed, green, and merged.
