# Phase 09: Repair clean generation, images, and CI

## Goal

Repair baseline release mechanics after Tasks integration: an official Studio
bootstrap works from a clean checkout without dirtying tracked sums, all three
production server images can resolve the local generated Agents Go client, and
PR CI exercises generation drift and image builds before merge. Reassess PR #66
against the complete maintained-module set rather than merging its stale tip.

## BDD Success Criteria

### Scenario: Clean Studio bootstrap is deterministic

- **Given** a fresh clone with ignored generated Studio client directories absent
- **When** the documented/root Studio test gate runs
- **Then** it generates required protobuf and Go REST clients before `go test
  ./...`
- **And** a second generation is byte-stable and `git status --porcelain` remains
  empty, including `go.work.sum`.

### Scenario: Every production image resolves local Agents client source

- **Given** the repository-root Docker context and server `replace` directive to
  `../primer-agents/client/go`
- **When** `Dockerfile`, `Dockerfile.tv`, and `Dockerfile.ingest` build
- **Then** each dependency/build stage copies the required client module before
  `go mod download`/compile
- **And** the final LMS, TV, and ingest binaries/images are produced without
  bypassing the replacement.

### Scenario: Pull requests cannot hide release failures

- **Given** a PR touching server modules, local client, Dockerfiles, Studio
  contracts/generation, workspace sums, or Tasks root wiring
- **When** GitHub Actions evaluates it
- **Then** clean generation plus clean-tree checks and relevant Docker builds run
- **And** generation is not pre-populated by an earlier step in a way that masks
  raw checkout/bootstrap defects.

### Scenario: Go 1.26.6 proposal is reassessed on current code

- **Given** PR #66's stale one-commit head and newly integrated Tasks modules
- **When** it is rebased/recreated on current master
- **Then** every maintained Go module, builder image, workspace, workstation pin,
  and generation gate is handled consistently or an explicit exclusion is
  documented
- **And** it merges only if all exact-head checks are green; otherwise it is
  closed/superseded with the blocker recorded.

## Implementation Instructions

- Commit the legitimate `google.golang.org/grpc v1.83.0` module checksum produced
  by official Studio generation. Change root/module targets so the supported
  clean-checkout test path generates prerequisites first, while retaining the
  no-tracked-generated policy.
- Add a clean-room script/workflow step that removes only ignored generated
  client directories in an isolated clone/worktree, runs official generation
  twice, tests, then fails on any tracked diff/status. Do not delete live WIP.
- In each server Dockerfile stage that runs `server` module commands, copy
  `primer-agents/client/go/go.mod`/`go.sum` before download and copy its source
  before compile. Keep cache-friendly ordering and do not replace the local
  module with an unreviewed remote pseudo-version merely to make Docker pass.
- Update Docker CI to run on relevant pull requests. Separate build from push:
  PRs build without registry credentials/push; protected-master runs may login
  and push. Include path triggers that cover the local client and Dockerfiles.
- Add/adjust Tasks CI for its module, generated clients, web, verifier fixture,
  coverage, and supported Android JVM/APK gates without claiming Phase 7.
- Rebase PR #66 normally after these repairs and Tasks integration, extend it to
  maintained Tasks modules if appropriate, and rerun all modules/images. Do not
  force-push over an exact head still under review; use a superseding branch if
  needed.

## End-to-End Test Plan

- In a fresh detached worktree/clone, assert raw absence of generated Studio
  dirs, run the official bootstrap/test target twice, and then:
  `git status --porcelain`, `git diff --exit-code`, and
  `curriculum-studio/tools/contract-gates/check_no_tracked_generated.sh`.
- Run direct `cd curriculum-studio && go test ./... -count=1` after bootstrap and
  all contract/client conformance targets.
- Build full images (not only intermediate targets):
  `docker build -f Dockerfile -t primer:<sha> .`,
  `docker build -f Dockerfile.tv -t primer-tv:<sha> .`, and
  `docker build -f Dockerfile.ingest -t content-ingest:<sha> .`.
  Smoke-run binaries where configuration-free help/health startup permits; do
  not fake unavailable production dependencies.
- Plant a temporary clean-room red by omitting/changing the local client copy and
  prove each relevant image gate fails; revert it and prove green.
- Plant a generated checksum/client drift in an isolated fixture and prove the
  generation gate fails and leaves the real checkout untouched.
- Inspect PR #66 at its new exact head and run the full phase/module/workstation
  matrix required by its changed files.

## Anti-Cheating Audit

- Check Docker stages compile real server commands; copying an unused client
  directory or building only a no-op target is insufficient.
- Check CI has a `pull_request` path and does not require unavailable registry
  secrets for PR builds.
- Check clean-generation starts with generated dirs absent and fails on tracked
  `go.work.sum` drift; `git diff --check` alone is not a clean-tree test.
- Check generated files remain ignored/untracked and tests import actual output.
- Check PR #66 includes/excludes Tasks modules deliberately and that cancelled
  historical checks are not reported green.
- Check no coverage or other gate was removed to shorten image/upgrade CI.

## Completion Gate

- [ ] Fresh-clone generation twice, test, no-tracked-gen, and clean status pass.
- [ ] The grpc v1.83 checksum drift is resolved legitimately.
- [ ] Full primer, primer-tv, and content-ingest images build at exact head.
- [ ] PR CI enforces image and clean-generation gates with planted-red proof.
- [ ] Tasks has truthful CI coverage for integrated phase 1–6 surfaces.
- [ ] PR #66 is rebased/recreated and green, or honestly closed/superseded.
- [ ] All release-mechanics changes are independently reviewed and merged.
