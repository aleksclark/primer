# Phase 2: Review catalog

## Goal

Expose production screen components in a fast code-first workbench and connect visual discussions to stable stories and components.

## BDD success criteria

### Scenario: Agent opens any pilot state

- **Given** the local Storybook
- **When** a reviewer selects a pairing, checklist, start, submit, retry, waiting, or completed story
- **Then** the selected production screen renders without auth or backend setup
- **And** theme and viewport controls can inspect responsive parity

### Scenario: Android states are reviewable

- **Given** Android Studio or the Android preview build
- **When** a reviewer opens the Tasks preview catalog
- **Then** production composables render deterministic pilot states in dark and light themes

### Scenario: Reviewer comments precisely

- **Given** a published Chromatic Storybook build
- **When** a reviewer pins a discussion to a screen component
- **Then** the discussion remains attached to that snapshot and story until resolved

## Implementation instructions

- Install Storybook in `primer-tasks/web` using the Vite React integration.
- Add stories next to production screen components and use deterministic fixture objects.
- Configure System C global styles, dark-first theme selection, desktop/mobile viewports, accessibility, and interactions.
- Add Chromatic as the hosted review adapter; keep project tokens in environment variables only.
- Add Android `@Preview` functions and preview parameters around the same production composables.
- Represent Android review evidence in Storybook documentation or Chromatic attachments when exported images are available; do not invent browser replicas of native screens.

## End-to-end test plan

- Build Storybook statically.
- Execute Storybook interaction/accessibility tests through the configured test runner.
- Compile Android preview sources and inspect at least one dark and one light state.
- Publish only when `CHROMATIC_PROJECT_TOKEN` is explicitly supplied.

## Anti-cheating audit

- Confirm stories import production components rather than copies.
- Confirm CSS is loaded from generated System C output.
- Confirm review publication cannot leak secrets from environment or API calls.
- Confirm native previews use Compose production components rather than HTML approximations.

## Completion gate

- [ ] Storybook builds with all pilot states.
- [ ] Android previews compile.
- [ ] Dark/light and desktop/mobile controls are available.
- [ ] Chromatic publication is optional and credential-gated.
