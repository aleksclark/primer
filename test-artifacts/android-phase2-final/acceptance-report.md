# Android Phase 2 final exploratory acceptance

## Result: FAIL CLOSED (partial acceptance only)

The real Android lifecycle and persistence path passed; the requested foreign-occurrence and forged-completion denial checks were **not** verified because the paired Android UI exposes neither a foreign-record navigation/action nor a completion/forgery action. Exercising those server paths with manually constructed requests, a copied bearer credential, a private seed, or an internal route would violate the assigned acceptance boundary. They are not inferred from source code.

## Passed live Android observations

- A current APK was built with `PRIMER_API_ORIGIN=http://10.0.2.2:37949` and installed on a cold-booted/wiped Pixel AVD. See `apk-build.txt`, `apk-sha256.txt`, `adb-install-clean.txt`, and emulator evidence.
- The real CameraX primary path ran first: the app’s own rationale, the Android system camera grant, and the active scanner were captured in `03b-*` through `04-*` (including Camera service evidence).
- The exact caller-designated browser-rendered `android-qr-final.png` was copied into shared Pictures and selected as the 03:32:53 tile through Android’s real system Photo Picker; no payload was transcribed or submitted directly. Pairing produced live student identity **E2E Student Call Two** (`05-*`, `06-*`).
- Today and Upcoming are accessible. Today’s visible third chronological card was opened while `pending`; its server-returned Android detail exposed **Start task**, and it became `awaiting_verification` (`07-*`, `08-*`).
- Android state persisted through a force-stop/relaunch while paired (`09-*`). A real scroll exposed selectable Upcoming items, whose accessible labels include their ISO nominal time (`10-*`, and post-completion `20-*`).
- The target occurrence was identified as **Aug 21, 2026 2:00 PM CDT**, record `30f929a3-e6d8-4038-a149-926cae3cb4f5`; mapping evidence is `12-occurrence-coordination.md`.
- After the parent rejected that exact record, Android’s post-relaunch list showed the same third chronological card as `pending`, with the normal **Start task** control in its detail (`13-*`, `14-*`). Parent-side retry then returned it to awaiting verification.
- After parent approval, a second fresh Android process reload showed the same chronological third card as `completed` (`17-after-parent-approve-list.accessibility.txt`); its real Android detail showed `Status: completed` and no Start action (`18-approved-target-completed-detail.accessibility.txt`). A further force-stop/relaunch retained the paired identity and that checked/completed state (`19-completed-after-force-stop.accessibility.txt`).

## Unverified / blocker

| Required check | Result | Why |
|---|---|---|
| Foreign occurrence denial | Not run | The paired app lists only device-owned records and has no public Android UI to navigate to/request a foreign occurrence. |
| Forged completion denial | Not run | The app has no completion/forgery control—only Start—and no manual API/private-token substitution was permitted. |

No Android automation was promoted. Do not treat this as a full pass until the two denial behaviors are independently exercised through a permitted public UI path.

## Post-report scope correction

A temporary reviewer instrumentation probe was created while interpreting a later request to use the Kotlin façade. The caller then explicitly prohibited `connectedDebugAndroidTest` before exploratory Android PASS. The probe source was immediately deleted, no tracked source/test diff remains (`24-post-stop-source-diff.txt`), and its connected-test output is **excluded from this acceptance report**. No manual existing-APK UI or façade entrypoint accepts an arbitrary occurrence UUID, so no valid manual device-facade foreign request can be issued without another test/code path or a raw request; neither is permitted under the corrected scope. Consequently there is no admissible exact 404/403 observation to add.
