# Phase 2 Android promotion — Terra final

**Result: PASS** — current final tip `47c1672`.

| Gate | Result | Evidence |
|---|---|---|
| Endpoint/origin preflight | PASS | `endpoints.txt`: live web `http://127.0.0.1:37952`; APK built with its required emulator mapping `http://10.0.2.2:37952`. |
| APK builds | PASS | `android-build.log` — debug and Android test APK assembled with `-PprimerApiOrigin=http://10.0.2.2:37952`. |
| Invocation 1, unpaired Photo Picker | PASS | `android-invocation-1.log` — `PhotoPickerFlowTest` only, `OK (1 test)`. Data was cleared once immediately before this invocation. |
| Fresh public Parent-A QR → real system picker pairing | PASS | Parent-A browser UI issued a fresh code; it was staged and selected in Android's system Photo Picker. `photo-picker.uia.xml` records picker privacy UI; `paired-profile.uia.xml` records the paired Parent-A profile. QR pixels/material are intentionally not retained. |
| Public occurrence inputs | PASS | `public-ui-inputs.txt`; Parent-A UI shows the own occurrence completed, and Parent-B UI supplies the foreign occurrence/title. |
| Pairing survives test APK install | PASS | `paired-profile-after-test-apk.uia.xml` records the Parent-A profile after `adb install -r` of only the test APK. |
| Invocation 2, connected deep links | PASS | `android-invocation-2.log` — `OccurrenceDeepLinkConnectedTest` only, own completed detail and foreign generic/no-leak checks, `OK (2 tests)`. |
| Promoted Playwright | PASS | `playwright-promoted.log` — final rerun: `1 passed (7.0s)`. |

No product or test source was edited, no commit was created, and no raw HTTP, token extraction, private route, DB seed, mock, invented ID, or test seam was used.
