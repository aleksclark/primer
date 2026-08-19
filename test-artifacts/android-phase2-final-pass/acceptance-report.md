# Terra Android Phase 2 final exploratory acceptance

**Reviewed source:** `ec5b8bdf79c43536cbbff2640ec8d55119425310`, containing the requested `3e97ff2` LazyColumn accessibility fix.  
**Overall result:** **FAIL CLOSED / partial acceptance** — all exercised public Android and Parent-A flows passed, but the required foreign-occurrence device-boundary denial has no permitted executable entry point. No automation was promoted.

## Environment and boundary

- `make tasks-endpoints` identified the live web endpoint as `http://127.0.0.1:37949/` and API as `http://127.0.0.1:37948/` (`00-run-environment.txt`). The advertised Stacklane FQDN itself refused port 80, so the live endpoint printed by the same task was used rather than inventing an origin.
- A prior `pixel` emulator was stopped; a clean `pixel` AVD was started with `-no-snapshot -no-boot-anim -gpu software -port 5556`. The current debug APK was built once and installed once. Evidence: `01-apk-build.txt`, `02-apk-sha256.txt`, `03-emulator.log`, `04-adb-install.txt`.
- The emulator and installed app remain running; there was no APK reinstallation after the initial install (`29-android-still-running.txt`).
- Parent-A was explicitly signed out, then authenticated through the web sign-in and visible **Parent A** identity choice. No API login or cookie injection was used. Browser screenshots are `11-parent-a-students-authenticated.png` and `12-parent-a-fresh-qr-issued.png`.

## Passed observations

| Requirement | Live observation | Evidence |
|---|---|---|
| CameraX primary precedes fallback | Fresh unpaired page exposes **Scan pairing QR** before the image importer. The in-app rationale, Android runtime permission, and active scanner were exercised before any picker. | `05-unpaired.*`, `06-camerax-rationale.*`, `07-camera-system-permission.*`, `08-camerax-running.*`, `08-camerax-dumpsys.txt` |
| Exact fresh QR through Android system Photo Picker / SAF | Parent-A issued a new QR in the real UI. Its rendered QR element was captured as `12-fresh-parent-a-qr-exact.png` (SHA-256 in `12-fresh-parent-a-qr-sha256.txt`), placed only in shared Pictures, and selected as the sole tile from package `com.google.android.providers.media.module`. Picker accessibility says **This app can only access the photos you select**. Pairing reached the real student checklist. | `12-parent-a-fresh-qr-issued.png`, `13-*`, `14-system-photo-picker.*`, `15-after-exact-photo-picker-pairing.*` |
| Many Today rows remain visible and tappable | After pairing, many visible **Today** cards were available. A visible pending Today card was opened and its real detail contained **Start task**. | `15-after-exact-photo-picker-pairing.uia.xml`, `16-today-pending-detail.*` |
| Start → awaiting verification | The Android public **Start task** action changed that detail from `pending` to `awaiting_verification`. | `16-today-pending-detail.uia.xml`, `17-today-started-awaiting.uia.xml` |
| Pairing and list persist through force-stop | Without reinstalling, the app was force-stopped/relaunched and reloaded the paired student/list. | `18-after-force-stop-*` |
| Upcoming LazyColumn survives many Today rows | A real swipe through the same scrollable Android LazyColumn exposed **Upcoming** and multiple accessible/tappable rows with ISO nominal times. The first Upcoming row was opened and returned to the list. | `19-upcoming-after-many-today.*`, `20-upcoming-row-tapped-detail.*`, `20-upcoming-row-returned.uia.xml` |
| Exact parent rejection and Android refresh | The Android-started target was the same Parent-A occurrence `4ea32491-3a0e-4d21-acfe-1ab373f6b2f8` (Nov. 1, 2026, 10:00 AM EST). Parent-A rejected it through the visible occurrence row. A fresh Android process loaded that exact card as `pending` with **Start task** again. | `21-parent-awaiting-target-before-reject.png`, `22-after-parent-reject-*`, `23-rejected-exact-target-pending-detail.*` |
| Retry same ID then approve | Parent-A used **Retry** on that same ID, observed it back at `AWAITING_VERIFICATION`, then used **Approve**, observing `COMPLETED`. | `24-parent-same-id-retry-awaiting.png`, `25-parent-same-id-approved-completed.png` |
| Android completed refresh and persistence | The already selected exact Android detail was refreshed through its public button and showed `Status: completed` without **Start task**. A second force-stop/relaunch retained the paired student and completed card. | `26-exact-target-after-parent-approve-detail.*`, `27-completed-after-force-stop-list.*` |
| Parent public skip and cancel | Parent-A used the public **Skip** action on `64427745-73dd-4efd-bb3b-427c37af9976` (observed `EXCUSED`) and public **Cancel** on `5c8c4a63-268e-4adc-b606-8d9ff675d140` (observed `CANCELED`). | `28-parent-public-skip-and-cancel.png` |

## Required but not accepted

| Requirement | Result | Fail-closed reason |
|---|---|---|
| Foreign occurrence denial via existing Kotlin TasksClient/device façade | **NOT VERIFIED — blocker** | The existing `TasksClient` library has no user-invokable device façade/CLI and the installed Android UI only lists device-owned records. Exercising `studentOccurrence(token, foreignId)` would require a new test/harness or exposing/copying the paired bearer token for raw transport. Neither is permitted. No random ID or source-inferred result was substituted. |

The detailed fail-closed decision is in `failed-run-report.md`.

## Scope integrity

- No product or test source was modified.
- No `connectedDebugAndroidTest`, Playwright, or other automation promotion was run.
- Browser console check found no errors during the parent flow.
- UI XML, screenshots, logcat, camera dumps, build/install logs, and an artifact inventory are retained in this directory.
