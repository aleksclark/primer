# Phase 2 promoted automation — final PASS

Exploratory-before-promotion gates were satisfied by browser call 7 and the
fresh Terra Android deep-link PASS in
`test-artifacts/android-phase2-acceptance-final-2/acceptance-report.md`.

## Browser

```text
PRIMER_TASKS_BASE_URL=http://127.0.0.1:37952
PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1 make tasks-e2e

phase1.spec.ts ... passed
phase2-tasks-schedules.spec.ts ... passed
2 passed
```

The Phase 2 Playwright test uses the real public UI and server-owned
collections; it does not seed data or use a private transport.

## Android

The promoted connected class is
`primer-tasks/android/app/src/androidTest/java/com/aleksclark/primertasks/OccurrenceDeepLinkConnectedTest.kt`.
It was run with the documented install-preserving instrumentation command in
`primer-tasks/android/CONNECTED_ACCEPTANCE.md`, against the real configured
API and IDs sourced from public Parent-A/Parent-B UI:

- own deep link: `TASK DETAIL`, completed status — PASS;
- foreign deep link: `TASK UNAVAILABLE`, generic copy, no foreign ID/title — PASS;
- pairing accessibility test — PASS.

The existing `PhotoPickerFlowTest` was run separately in its required clean,
unpaired emulator state and passed. It is intentionally not combined with the
paired deep-link invocation because its setup clears pairing state.

No mock server, raw HTTP, token extraction, private route, DB seed, invented ID,
or test-only product seam was used. Historical failed promotion and pairing
runs remain in `test-artifacts/phase2-promotion-final/` and are not final
claims.
