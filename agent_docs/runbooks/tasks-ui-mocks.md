# Primer Tasks UI mocks

Use this runbook to construct, review, and verify production-reusable Tasks mocks across web and Android. Mock evidence proves rendering and interactions only. `make tasks-e2e` remains the real API/auth/persistence workflow gate.

## Prerequisites

- Node.js 22, npm, and Chromium supported by Playwright.
- JDK 17 and Android SDK (`JAVA_HOME` and `ANDROID_HOME` may override documented defaults).
- Docker and full Chrome for the separate real-stack acceptance. Set `CHROME_EXECUTABLE` when Chrome is not on the default Playwright path.
- Optional `CHROMATIC_PROJECT_TOKEN` for hosted pinned comments and approvals. Never commit it.

## Install and generate

From the repository root:

```bash
make design-system
make tasks-client
npm --prefix primer-tasks/web ci --no-audit --no-fund
```

## Author a mock

1. Add or change a production stateless component in `primer-tasks/web/src/screens/StudentScreens.tsx`.
2. Add its deterministic story in `StudentScreens.stories.tsx`. Do not call APIs, initialize auth, or use live data.
3. Add the equivalent deterministic Compose state to `TasksMockCases.kt` and preview it from `TasksPreviews.kt`.
4. Use `data-review-target` on web and `testTag` on Compose for stable component identity. Keep accessible labels intact.
5. Start the workbench:

```bash
npm --prefix primer-tasks/web run storybook
```

Open `http://localhost:6006`, choose **Tasks / Pilot workflow**, then inspect dark/light and desktop/mobile toolbar modes.

## Comment and approve

Publish to Chromatic only with an explicitly supplied project token:

```bash
CHROMATIC_PROJECT_TOKEN=... make tasks-ui-mocks-review
```

Use Chromatic UI Review to pin discussions to exact snapshot points, resolve each thread, and approve the changeset. The repository contains no fallback comment database.

## Local verification

Run all local mock gates:

```bash
make tasks-ui-mocks
```

This regenerates System C tokens and Tasks clients, then runs web typecheck/unit tests, Storybook build, Storybook interaction/accessibility tests, 14 dark/light Playwright screenshot comparisons, Android feature unit tests, and Android preview-bearing debug assembly.

Artifacts:

- Storybook: `primer-tasks/web/storybook-static/`
- Web visual report: `primer-tasks/web/test-artifacts/ui-mocks-report/`
- Web failure traces: `primer-tasks/web/test-results/`
- Android AAR: `android/feature-tasks-student/build/outputs/aar/`

## Update approved web baselines

Only after reviewing the visual change:

```bash
make tasks-ui-mocks-update
git diff -- primer-tasks/web/e2e-mocks/tasks-mocks.visual.spec.ts-snapshots
npm --prefix primer-tasks/web run storybook:visual
```

Never run the update command merely to clear an unexplained failure. Android previews are compile-gated in this pilot; stable native pixel baselines require a pinned emulator or compatible host screenshot plugin before they can be authoritative.

## Validate the real pilot workflow

Run the production-wired browser acceptance separately:

```bash
CHROME_EXECUTABLE=/path/to/google-chrome make tasks-e2e
```

The existing Phase 2 flow must create a student, task, and schedule; pair the student browser; start and submit work; reject and retry it; submit again; approve it; and observe completion. It uses the host/non-Compose stack and real public boundaries.

## Failure diagnosis

- Storybook build failure: run `npm --prefix primer-tasks/web run typecheck`, then `storybook:build`.
- Accessibility failure: open the story URL printed by Vitest and repair semantics; do not disable the rule.
- Visual mismatch: inspect expected/actual/diff in the HTML report and trace before deciding whether to update.
- Android failure: verify JDK 17, regenerate Tasks clients, then run the exact Gradle tasks from the failing output.
- Real workflow failure: inspect `primer-tasks/web/test-artifacts/playwright-phase1.json` and host-stack logs; do not substitute mock success.

## Validation record

Validated on 2026-09-08 in the `design-mocks` worktree:

| Gate | Result |
|---|---|
| Plan structure | PASS, index plus four phase files and required sections |
| Storybook build | PASS, 10 pilot stories |
| Storybook interaction/accessibility | PASS, 10 tests |
| Web screenshot comparison | PASS, 14 dark/light cases |
| Android fixture/preview compile and unit tests | PASS, `testDebugUnitTest` plus `assembleDebug` |
| Production Tasks pilot | PASS, 7 Playwright tests through the host stack with full Chrome; pairing, manual approval, retry, collections, tenancy, and parent-agent flows |
| Chromatic hosted review | Not executed; no project token was supplied |
