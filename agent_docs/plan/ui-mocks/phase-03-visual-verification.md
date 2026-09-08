# Phase 3: Visual verification

## Goal

Turn approved mock states into deterministic regression evidence that can be checked while implementation evolves.

## BDD success criteria

### Scenario: Approved web visuals remain stable

- **Given** committed Playwright reference images
- **When** the mock visual suite runs at fixed viewport, locale, theme, and motion settings
- **Then** unchanged screens pass pixel comparison
- **And** changed screens produce actual, expected, and diff evidence

### Scenario: Intentional visual change

- **Given** a reviewed mock change
- **When** a maintainer explicitly updates baselines
- **Then** only selected baseline files change
- **And** the normal comparison passes afterward

### Scenario: Android implementation remains constructible

- **Given** every pilot fixture
- **When** Android unit tests and preview-bearing sources compile
- **Then** each fixture maps to a production Compose screen without network state

## Implementation instructions

- Add a Playwright configuration dedicated to Storybook visual cases, separate from real-stack acceptance.
- Use `toHaveScreenshot` with fixed Chromium, viewport, locale, timezone, animations, and fonts.
- Add named scripts for compare and explicit baseline update.
- Compile and test Android fixtures and previews on the host; add platform screenshot baselines only when the repository has a stable emulator or official host screenshot plugin compatible with its AGP/Compose versions.
- Keep Chromatic approval and local Playwright baselines complementary.

## End-to-end test plan

- Build and serve Storybook through Playwright `webServer`.
- Compare all pilot web states.
- Deliberately perturb one local render, confirm failure, restore it, and confirm pass while validating the runbook.
- Run Android feature unit tests and debug compilation.

## Anti-cheating audit

- Reject `test.skip`, empty screenshots, broad masking, arbitrary threshold increases, and automatic baseline updates.
- Confirm visual cases select stories by stable IDs and assert meaningful text before screenshots.
- Confirm screenshots do not replace the real workflow assertions.
- Confirm Android compilation includes preview and fixture files.

## Completion gate

- [ ] Web baselines are committed and comparison passes.
- [ ] Explicit update command is documented.
- [ ] Failure artifacts are generated when a mismatch occurs.
- [ ] Android fixture and preview compilation passes.
