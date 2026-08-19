# Android Phase 2 final reviewer — failed-run report

## Result: FAIL CLOSED — no exploratory PASS or automation promotion

The live Android and Parent-A paths below passed, but full acceptance is blocked by the requested **foreign-occurrence denial**. No existing manual Android action or executable façade entry point accepts an arbitrary occurrence ID. `TasksClient` exposes `studentOccurrence(token, id)` and `startStudentOccurrence(token, id)`, but it is a Kotlin library called only from the app; invoking it for a foreign ID would require adding a Kotlin/test/automation harness or extracting the paired token for a raw request. Both are outside the assigned boundary. No arbitrary ID, token copy, direct HTTP call, database query, private seed, or source/test change was used.

Accordingly, this reviewer cannot claim the foreign denial as passed or fabricate a 403/404 result. Do **not** promote Playwright, connected Android, or other automation based on this run.

## Other transient environment issues (resolved without weakening the run)

1. The first Android build inherited conflicting SDK variables (`ANDROID_HOME=/home/aleks/Android/Sdk`, `ANDROID_SDK_ROOT=/opt/android-sdk`) and failed before installation. The final build set both to the same SDK and succeeded (`01-apk-build.txt`).
2. The initially requested Stacklane FQDN from `make tasks-endpoints` resolved but refused port 80. The same command reported the currently live Docker-assigned web endpoint `http://127.0.0.1:37949/`, which served HTTP 200 and was used for all real parent UI interactions (`00-run-environment.txt`).
3. Chrome DevTools initially reported a released/blank profile as busy. Its stale blank Chrome owner was stopped, then the provided MCP browser listed `about:blank`; Parent-A was authenticated through the visible test identity UI, not by a cookie/API shortcut (`10-browser-mcp-recovery.txt`, parent screenshots).

## Evidence

- Kotlin façade limitation: `primer-tasks/clients/kotlin/src/main/kotlin/com/aleksclark/primertasks/client/TasksClient.kt`; no new source/test/harness was added.
- Final repository check: `29-git-status.txt`, `29-git-diff-check.txt`.
- The emulator remains running with the one installed current APK: `29-android-still-running.txt`.
