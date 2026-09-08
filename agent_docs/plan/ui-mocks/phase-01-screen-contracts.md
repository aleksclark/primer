# Phase 1: Reusable screen contracts

## Goal

Separate rendering from transport and orchestration for the pilot workflow. This makes each screen directly constructible by a coding agent while preserving the production API and security boundaries.

## BDD success criteria

### Scenario: Production and mock checklist share rendering

- **Given** deterministic student and occurrence state
- **When** the state is rendered in production or the mock catalog
- **Then** both paths use the same checklist component
- **And** no mock path calls the Tasks API

### Scenario: Actions remain explicit

- **Given** each supported occurrence status
- **When** the detail screen renders
- **Then** only the server-permitted start, submit, refresh, and navigation actions are visible
- **And** unsupported verification never becomes manual approval

### Scenario: Failure states remain visible

- **Given** pairing or occurrence failure copy
- **When** its screen renders
- **Then** the failure is announced and the recovery action remains available

## Implementation instructions

- Add plain web screen models and stateless student pairing, checklist, and occurrence components under `primer-tasks/web/src/screens/`.
- Adapt existing production containers in `App.tsx` to those components without changing transport behavior.
- Make Android Tasks screen composables reusable by previews, with deterministic fixture constructors outside session orchestration.
- Add stable semantic review targets while retaining accessible names.
- Keep generated client types at production adapter boundaries where practical.

## End-to-end test plan

- Run web typecheck, unit tests, lint, and build.
- Run `:feature-tasks-student:testDebugUnitTest` and assemble the feature.
- Run existing real-stack Tasks Playwright for the pilot workflow after visual tooling is complete.

## Anti-cheating audit

- Search production routes for mock query parameters or environment branches.
- Confirm fixtures do not import `tasksClient`, initialize auth, or contain tokens.
- Confirm Android previews do not construct `TasksSession`, DataStore, camera, or generated network clients.
- Confirm action visibility is derived from the same production presentation rules.

## Completion gate

- [ ] Shared production screen components exist on web and Android.
- [ ] Deterministic pilot states compile.
- [ ] No production runtime mock switch exists.
- [ ] Focused web and Android tests pass.
