# Device-client negative rerun: preflight blocker

The caller authorized a narrow test through the Kotlin `TasksClient` façade using the paired emulator token and a **real Parent-B occurrence ID obtained through public UI evidence**.

## Verified façade scope

`primer-tasks/clients/kotlin/src/main/kotlin/com/aleksclark/primertasks/client/TasksClient.kt` exposes:

- `studentOccurrence(token, id)` → generated `DEVICE_OCCURRENCE` route;
- `startStudentOccurrence(token, id)` → generated `DEVICE_OCCURRENCE_START` route;
- profile/checklist/today/upcoming and pairing methods.

It exposes **no direct completed/approve/decision method**, so no Kotlin device-client call can directly write a completed state.

## Blocker

No actual Parent-B occurrence ID is present in the supplied public UI evidence. The recorded Parent-B public Tasks screen is empty (`.paseo-e2e/phase2-tasks-schedules/call2-parent-b-tasks-isolated.snapshot.txt`), and the cross-tenant detail evidence is a Parent-A *student* ID returning 404 (`call2-parent-b-cross-tenant.snapshot.txt`), not a Parent-B occurrence.

Chrome DevTools MCP cannot attach to the existing authenticated browser profile (`The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile`), so a current Parent-B Occurrences collection cannot be read without taking over/killing the parent context. That was not done.

A random/non-occurrence UUID would only establish missing-record behavior, not the requested foreign-record authorization denial. No raw transport, database, repository inspection, private seed, or fabricated identifier was substituted.

Provide the Parent-B public-occurrence UUID (or a public snapshot containing it) to run the paired-app instrumented façade check without printing/revealing the device token.
