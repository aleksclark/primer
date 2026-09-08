# Phase 4: Pilot runbook

## Goal

Provide one reproducible procedure that takes a coding agent from a System C change through mock construction, review, visual verification, and real Tasks workflow validation.

## BDD success criteria

### Scenario: Local pilot from a clean checkout

- **Given** documented Node, Chromium, JDK, and Android prerequisites
- **When** an operator follows the runbook
- **Then** design tokens regenerate, Storybook builds, visual checks pass, Android previews compile, and artifact locations are printed

### Scenario: Real workflow remains authoritative

- **Given** mock checks pass
- **When** the operator runs the Tasks host acceptance
- **Then** pairing, checklist, start, submit, reject/retry, resubmit, and approval pass through real public boundaries
- **And** mock results are not reported as server acceptance

### Scenario: Missing hosted-review credential

- **Given** no Chromatic token
- **When** the local pilot runs
- **Then** all local gates still run
- **And** publication is reported as not requested rather than passed

## Implementation instructions

- Add `agent_docs/runbooks/tasks-ui-mocks.md` with setup, authoring, review, baseline update, failure diagnosis, and clean-up.
- Add Make targets that compose existing token, web, Storybook, visual, Android, and real-stack commands.
- Execute the runbook exactly once and record commands and outcomes in the runbook validation section or a linked artifact.

## End-to-end test plan

- Run the complete local mock target.
- Start Storybook and verify the pilot flow in a real browser.
- Run the existing real-stack Playwright pilot through `make tasks-e2e` or its focused equivalent.
- Stop all started services and preserve only non-sensitive reports.

## Anti-cheating audit

- Confirm the wrapper exits nonzero on every failed required gate.
- Confirm hosted review is not claimed without a Chromatic build URL.
- Confirm real-stack acceptance is not replaced by Storybook interaction tests.
- Confirm no credentials or endpoint files enter committed artifacts.

## Completion gate

- [ ] Runbook commands execute as written.
- [ ] Local mock checks pass.
- [ ] Browser inspection passes without console errors.
- [ ] Real Tasks pilot passes.
- [ ] Validation evidence distinguishes mock, visual, and real-stack results.
