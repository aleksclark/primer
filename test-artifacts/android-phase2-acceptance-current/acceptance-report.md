# Android Phase 2 exploratory acceptance — current QR

**Result: FAIL CLOSED (incomplete)**

## Environment and constraints held

- Fresh, cold-booted Pixel AVD (`pixel`, Android 15) was already running with `-no-snapshot`; the Primer Tasks app was uninstalled, current APK installed, then launched from a clean app state.
- Built/installed `primer-tasks/android/app/build/outputs/apk/debug/app-debug.apk` with `PRIMER_API_ORIGIN=http://10.0.2.2:37949`.
- Docker runtime inspection found `TASKS_MODEL_PROVIDER=disabled` on the running `primer-tasks-primer-tasks-p2-api-1` container. No API was called directly.
- The supplied exact QR image was copied only into emulator Pictures storage and selected through the real Android system Photo Picker. No QR payload/code was read, typed, recreated, seeded, or sent to a pairing endpoint directly.
- The QR was issued at `2026-08-19T03:26:03-05:00`. System Photo Picker selection of the exact image began at `2026-08-19T03:29:38-05:00`, within five minutes.

## Observed Android evidence

1. The fresh pairing screen shows **Scan pairing QR** as the filled primary action and **Import pairing QR image** as the secondary action (`01-*`).
2. CameraX was exercised first: the app requested camera access through the Android system permission dialog; permission was granted; the live scanner showed **Scan pairing QR** and **Cancel** (`02-*`, `03a-*`, `03b-*`, `camera-permission.txt`). The emulator camera had no supplied QR feed, so this path was not the QR-consuming path.
3. After cancelling CameraX, the app opened the real Android Photo Picker (`05-system-photo-picker.*`). The selected newest item was the supplied exact QR image, identified by the Photo Picker's capture-time accessibility label; pairing succeeded (`06-photo-picker-pair-result.*`).
4. The paired identity is **E2E Student Call Two**. The app rendered **Today**, multiple server statuses, and an **Upcoming** heading (`06-*`).
5. A pending occurrence opened to a detail screen with **Status: pending** and **Start task** (`07-*`). Starting it changed the detail status to **awaiting_verification** (`08-*`).
6. Force-stop/relaunch retained the paired identity and loaded that occurrence as **awaiting_verification**, proving local credential/session persistence and server-state refresh (`09-*`).

## Blocking evidence / unverified acceptance

- The required authenticated public parent browser context was unavailable. Both Chrome DevTools MCP `chrome-devtools_new_page` and `chrome-devtools_list_pages` failed before a page could be driven: `The browser is already running for /home/aleks/.cache/chrome-devtools-mcp/paseo-e2e-profile. Use --isolated to run multiple browser instances.` An isolated browser would not have the required authenticated parent-A session. No browser session, cookies, or private API were substituted.
- Consequently, parent-side checked-state actions were **not** performed: reject-for-retry, re-open/retry, approve, skip occurrence, cancel schedule, and foreign/tenant-denial. Their Android refresh outcomes are also unverified.
- The Android **Upcoming** data is nonempty (the heading is conditionally rendered only when it is nonempty), but the current screen has ten Today rows and no scrolling container. The first Upcoming row is compressed at the bottom/navigation boundary with no accessible text in the UI dump (`09-after-force-stop.xml`), so an Upcoming item was not reliably selectable. This is recorded as a UI accessibility/visibility defect rather than counted as a pass.

No automated test was created or promoted.
