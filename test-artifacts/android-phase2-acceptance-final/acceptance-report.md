# Android Phase 2 final acceptance — fresh leaf

**Result: FAIL CLOSED.**

The latest Go source and current Android APK built successfully, and one fresh Parent-A Android pairing was completed through the public browser parent UI and Android's real system Photo Picker flow. The required fresh own and foreign deep-link observations were then performed with `adb am start` against the installed current APK.

The own link passed. The foreign link did **not** meet the requested exact acceptance condition: it returned the device to its ordinary paired checklist rather than rendering a generic unavailable/404 surface. I therefore cannot report PASS.

## Scope and boundary

- No product or test source was edited; no harness, private route, raw HTTP, credential/token extraction, database seeding, or automation promotion was used.
- The live Parent-A and Parent-B browser flows used the public UI. Parent browser authentication used the existing browser-visible test sign-in route, as retained in the Phase 2 exploration record.
- The final artifact directory deliberately contains no QR image, manual pairing value, token, or QR payload. The fresh QR was rendered by Parent-A UI, staged only transiently to Android's system Photo Picker, used once, and deleted from both the host and emulator staging location.

## Current build and fresh pairing

| Check | Result | Evidence |
|---|---|---|
| Latest Go source builds | PASS | `00-latest-go-build.txt` (`make build`) |
| Current Android APK builds | PASS | `01-current-apk-build.txt` (`./gradlew assembleDebug`; SHA-256 recorded) |
| APK installed once, app cleared/launched | PASS | `04-install-clear-launch.txt`, `04-unpaired.uia.xml` |
| Parent-A student selected and pairing material issued through public UI | PASS | `02-parent-a-students.snapshot.txt`, `03-parent-a-student-detail.snapshot.txt` |
| One Android pairing from that fresh material | PASS | `06-qr-stage.txt`, `07-system-photo-picker.uia.xml`, `08-after-pairing.uia.xml` |

`08-after-pairing.uia.xml` shows the named Parent-A student and task rows, establishing that the device is paired to Parent-A's student. The system Picker artifact establishes a genuine Android picker (`com.google.android.providers.media.module`) and its selected-photos privacy surface, without preserving the selected QR pixels.

## Required public occurrence IDs and deep links

| Case | Public-source observation | Android command/result | Verdict |
|---|---|---|---|
| Own historical occurrence | Parent-A public Occurrences UI lists `359952d8-7d14-4bd5-96f1-84804d957cc6` for the paired student, `E2E Brushing Call Three`, `COMPLETED`. | `adb am start -W -a android.intent.action.VIEW -d primertasks://occurrences/359952d8-7d14-4bd5-96f1-84804d957cc6 com.aleksclark.primertasks` opened `TASK DETAIL`, title `E2E Brushing Call Three`, `Status: completed`. | **PASS** |
| Foreign Parent-B occurrence | Parent-B public Occurrences UI lists `62b79178-af55-4ca7-8cb8-8f4000252d2b` for Parent-B student `3E2A2310-90C1-484E-953F-3EBDA5101FFD`, title `Android Foreign Parent B Task`, `PENDING`. | The same `adb am start -W` form with that UUID was independently executed twice (normal four-second observation and one-second immediate observation). Both Android UI dumps show the ordinary Parent-A paired checklist, not a generic unavailable/404 surface. No Parent-B title, UUID, or other foreign occurrence detail appears in either dump. | **FAIL: exact generic unavailable/404 requirement not met** |

Evidence: `11c-parent-a-occurrences-default.snapshot.txt`, `12-own-deep-link-am-start.txt`, `12-own-deep-link-detail.uia.xml`, `13-parent-b-occurrence.snapshot.txt`, `14-foreign-deep-link-am-start.txt`, `14-foreign-deep-link-result.uia.xml`, `14b-foreign-deep-link-am-start-immediate.txt`, `14b-foreign-deep-link-immediate.uia.xml`.

The foreign observation does not expose Parent-B record content, but an ordinary checklist is not the required generic unavailable/404 response. This distinction is intentionally not relaxed.

## Previously retained matrix evidence (not repeated)

The already-observed matrix remains retained under `test-artifacts/android-phase2-final-pass-2/` and is indexed below rather than re-run or weakened:

- accessible Today and Upcoming LazyColumn rows: `23-lazycolumn-upcoming.uia.xml`, `24-upcoming-row-detail.uia.xml`;
- Start to awaiting verification: `12-today-pending-detail.uia.xml`, `13-today-started-awaiting.uia.xml`;
- parent reject, retry, and approve, followed by Android completed refresh: `17-parent-own-rejected-pending.png`, `18-own-after-parent-reject-detail.uia.xml`, `19-parent-own-retry-awaiting.png`, `20-parent-own-approved-completed.png`, `21-own-refresh-completed.uia.xml`;
- checked/completed persistence through force-stop: `22-completed-force-stop-relaunch.txt`, `22-completed-after-force-stop.uia.xml`;
- public Parent skip/cancel: `25-parent-public-skip-cancel.png`.

These prior artifacts are sufficient for the matrix items, but cannot cure this leaf's required fresh foreign-deep-link result.

## Required follow-up

Make the foreign/deleted/non-owned deep-link path visibly render one common generic unavailable/404 surface (without record-specific content), then rerun only the two independent `adb am start` observations against a freshly built/installed current APK. Do not replace that check with a raw client call or test seam.
