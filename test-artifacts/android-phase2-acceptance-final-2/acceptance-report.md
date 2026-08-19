# Android Phase 2 final acceptance — rerun after deep-link fix

**Result: PASS.** This is a manual acceptance rerun against the current debug APK, installed once on the already paired Android device. No product or test source was edited; no test harness, raw HTTP, token extraction, database seeding, private route, or test-only seam was used. No automation was promoted.

## Current-run boundary

- Current APK: SHA-256 recorded in `02-apk-sha256.txt`; `assembleDebug` and the single `adb install -r` both succeeded (`01-apk-build.txt`, `03-adb-install.txt`).
- The device was already paired to the Parent-A student by the real public pairing flow. That pairing and the required normal lifecycle evidence are retained below rather than recreated destructively.
- Parent-B's real foreign occurrence was obtained in the shared browser exploratory record solely through the public Parent-B UI: `.paseo-e2e/phase2-tasks-schedules/android-foreign-parent-b-occurrences.snapshot.txt` and call 14 of `.paseo-e2e/phase2-tasks-schedules/exploration.md`. The exact UUID was used only as the Android deep-link argument and is redacted from this run's command logs.

## Fresh current APK deep-link observations

| Case | Action | Observed Android UI | Result |
|---|---|---|---|
| Device-owned occurrence | `adb am start -W` launched the existing `primertasks://occurrences/{own-id}` path after a force-stop. | Dedicated **TASK DETAIL** rendered the Parent-A task title and **Status: completed**. | PASS |
| Parent-B foreign occurrence | `adb am start -W` launched the existing `primertasks://occurrences/{foreign-id}` path after a force-stop, using the real Parent-B UUID obtained through the public Parent-B occurrence UI. | Dedicated **TASK UNAVAILABLE** surface rendered: **This task is unavailable.** / **The requested task could not be opened.** / **Back to today**. It contains neither the foreign task title nor an ID or any other existence-specific detail. | PASS |

The exact launch commands are secret/identifier-scrubbed in `04-own-deep-link-am-start.txt` and `05-foreign-deep-link-am-start.txt`. The corresponding UI XML and screenshots are `04-own-deep-link-detail.*` and `05-foreign-unavailable.*`. `06-logcat-scrubbed.txt` is retained as a scrubbed diagnostic capture.

## Retained, independently observed required matrix

The following non-destructive observations remain valid on the paired Parent-A device and were reviewed from the retained real-device/public-parent acceptance report; the current install did not clear app data or replace the device identity.

| Requirement | Retained real observation |
|---|---|
| Parent-A pairing and accessible Today/Upcoming LazyColumn | Real public Parent-A QR pairing completed; Android checklist and upcoming rows rendered. |
| Today Start → awaiting verification | A visible Android task detail changed to awaiting verification after **Start task**. |
| Parent reject → Android pending → retry → approve | The mapped occurrence was rejected, retried, and approved through the public Parent-A UI. |
| Checked/completed persistence through force-stop | Completed state remained after force-stop/relaunch without reinstall. |
| Parent public skip/cancel | Parent used the public lifecycle controls and the resulting states were observed. |

The detailed historical evidence is retained in the tracked report
`test-artifacts/android-phase2-final-pass/acceptance-report.md`; bulky screenshots
and UI dumps are intentionally not part of the final source tree.

## Scope and integrity

- Browser exploration is the recorded public-UI PASS in `.paseo-e2e/phase2-tasks-schedules/exploration.md` (call 7); the Parent-B occurrence acquisition is separately recorded there at call 14.
- This rerun made no browser/API/database mutation. It rebuilt and installed the current APK once, then exercised only the documented Android intent/deep-link product path.
- Pairing QR values, bearer material, and full occurrence identifiers are not copied into the new command/log artifacts.
