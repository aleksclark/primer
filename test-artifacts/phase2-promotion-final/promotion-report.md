# Phase 2 final promotion verification — FAIL CLOSED

**Result:** **BLOCKED / no promotion claim.** No product or test source was edited and no commit was made.

## Live boundary attempted

- Real Tasks service: the root `make tasks-endpoints` command reported the live direct web origin; the current emulator was `emulator-5556`.
- Public Parent-A browser UI: signed in through the visible identity picker, created exactly one new Parent-A student, created/published/scheduled one Parent-A task, and observed its pending occurrence in the public occurrences UI.
- Android: assembled current debug and test APKs with the emulator API origin, cleared the app **before** pairing, installed the current debug APK, and exercised the unpaired app's public **Import pairing QR image** fallback. Parent-A issued the pairing QR in the public browser page; the exact rendered QR was placed in emulator-visible Pictures and selected in Android's real system Photo Picker (`com.google.android.providers.media.module`). The QR file was removed afterward and is not retained in this artifact directory.

## Blocking observation

After the photo-picker selection, the Android UI rendered **“Pairing failed. Check the server and try again.”** The app remained unpaired. This prevented the required completed own-occurrence instrumentation input and violated the prerequisite for the documented install-preserving `OccurrenceDeepLinkConnectedTest` command.

Per the requested fail-closed rule, this leaf did **not**:

- retry by manual code entry, raw transport, token extraction, private route, DB seed, mock, or invented ID;
- run the connected instrumentation claim with fabricated/unpaired inputs;
- run the promoted Playwright claim independently after the required Android acceptance was blocked;
- run the destructive aggregate connected Gradle task.

The existing Parent-B occurrence recorded by the public browser exploratory flow was not used as a substitute for the blocked live paired-device path.

## Artifacts

- `00-environment.txt` — timestamp, reviewed revision, and emulator state.
- `01-parent-a-own-pending.snapshot.txt` — public Parent-A pending own occurrence (identifier-bearing live UI evidence; no credential material).
- `03-apk-build.txt`, `03-apk-sha256.txt` — current APK build configured for the emulator origin.
- `04-pre-pair-clear.txt`, `05-pre-pair-install.txt`, `06-unpaired.uia.xml` — clear occurred only before pairing; current APK and unpaired UI.
- `08-system-photo-picker.uia.xml` — system picker package/privacy surface. The corresponding image screenshot was deliberately removed because it contained the QR thumbnail.
- `09-after-photo-picker-pairing.uia.xml` — pairing failure surface.
- `11-pairing-failure-logcat-scrubbed.txt` — scrubbed diagnostic log.

## Required next action

Diagnose the real public QR/photo-picker pairing failure, obtain a new Parent-A QR only through the public browser UI, and restart this acceptance from a clean pre-pair state. Do not claim promotion until the single Parent-A paired device has a real completed own occurrence and the documented install-preserving instrumentation command independently passes both own-detail and foreign-generic-unavailable cases.
