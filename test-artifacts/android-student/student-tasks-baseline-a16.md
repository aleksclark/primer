# Primer Student Tasks — A16 hardware handoff

> Historical donor record, imported from `6cb46f6e` (clarifying `d4d56f74`).
> All device, rollout and signing observations below belong to the named donor
> inputs. They are not tests of the current reconciliation or instructions to
> perform device/deployment actions. Fresh combined-source gates are recorded in
> `agent_docs/plans/wip-integration/student-p4-current-reconciliation.md`.

## Scope and device

Physical validation occurred on Samsung Galaxy A16 5G **SM-S166V**, ADB serial
`R5GL44E9FVH`, Android 16 / API 36. This is factual handoff evidence, not a
claim that the current source head has been installed.

No factory reset, recovery-code access, security-control changes, or debug APK
installation occurred. The installed owner remained
`com.aleksclark.primer.student/.admin.PrimerDeviceAdminReceiver`; managed lock
state remained `LOCKED` after each replacement and access-revocation test.

## Source and local Student gate

Current strict-capability source head at handoff:

```text
c21423689705b08af13fc50552fc2ead22251090
```

Executed at that exact head:

```bash
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  ./android/gradlew -p android :feature-tasks-student:testDebugUnitTest \
  --no-daemon --max-workers=2
```

Result: **BUILD SUCCESSFUL** (Student feature unit tests).

Earlier Student-scoped evidence also includes successful `:app-student:assembleDebug`,
`:app-student:assembleRelease` (for the recorded candidates), and the focused
Tasks API metadata test. The repository-wide Android and Tasks API suites were
not clean: unrelated Control self-update and dialogue integration tests failed.
Those failures were not modified or disabled in this branch.

## APK actually exercised

The most recent APK installed and exercised on the A16 was **version 15**, built
from the source state containing commit `a6c556f8`, before strict commit
`c2142368`.

```text
package:     com.aleksclark.primer.student
versionCode: 15
versionName: 0.1.0-qualification.15
APK:         /home/aleks/.local/share/primer/android-qualification/apks/student-15.apk
SHA-256:     ab6fa84ace52d56307db1dca91142cb382219143579921d7da48bcf671d46a33
```

Version 15 contained a **temporary legacy fallback** that regarded omitted
`studentCapability` as parent approval. This was needed only because the live
Tasks host omitted capability metadata. It enabled an A16 exercise of the old
live contract, but it is **not** proof that strict source `c2142368` was
installed or accepted.

Current `c2142368` intentionally removes that fallback: omitted or blank
`studentCapability` is unavailable and cannot expose start/submit. Only explicit
`studentCapability == "parent_approval"` may expose those actions. Explicit
`"unsupported"` remains unavailable.

## Actual v15 A16 observations

Using a live educator-issued one-use QR for a temporary student:

- The Student pairing screen displayed primary camera scan plus clearly labeled
  image-import and paste fallbacks.
- Camera scanning and image import honestly reported that parent maintenance is
  needed while lock-task blocks the system permission/media-picker activity.
- Invalid pasted data failed closed with an invalid-Primer-pairing message.
- The fresh QR paired the handset and displayed Today/Upcoming under the student
  display name.
- A parent-approval occurrence showed instructions, then transitioned
  `pending -> in_progress -> awaiting_verification` after Start and Submit.
- A live parent rejection was displayed after Refresh as **Rejected — retry**;
  a second Start/Submit followed by live parent approval was displayed as
  **Approved**. The checklist reflected **A16 PARENT CHECK · APPROVED** after
  returning from detail.
- A same-signing-key replacement from version 14 to v15 preserved the Tasks
  pairing and checklist.
- With Wi-Fi disabled, checklist Refresh showed **Unable to reach the Primer
  server. Try again.** and retained the paired/cached checklist. Wi-Fi was
  immediately restored.
- Archiving the temporary student from the parent session revoked Tasks access.
  The following handset Refresh cleared Tasks to the pairing screen and showed
  **This device pairing is no longer active. Scan a new QR code.** Device owner
  and managed lock-task state remained intact.

The parent API has no dedicated public pairing-revoke operation; archive was the
server-owned revocation mechanism exercised for this temporary record.

## Latest strict-code build input

Do not use the v15 APK to qualify strict source. Parent signing custody should
build a new version code **greater than 15** (for example 16) after the backend
contract below is live:

```bash
# In a shell with the existing protected signing environment loaded; do not print it.
# Required: PRIMER_STUDENT_KEYSTORE, PRIMER_STUDENT_STORE_PASSWORD,
# PRIMER_STUDENT_KEY_ALIAS, PRIMER_STUDENT_KEY_PASSWORD.
make tasks-clients
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  PRIMER_API_ORIGIN=https://api.primerlms.com/tasks/api \
  ./android/gradlew -p android :app-student:assembleRelease \
  -PstudentVersionCode=16 \
  -PstudentVersionName=0.1.0-qualification.16 \
  --no-daemon --max-workers=1
/opt/android-sdk/build-tools/35.0.0/apksigner verify --verbose \
  android/app-student/build/outputs/apk/release/app-student-release.apk
```

The current handset was read-only confirmed as version 15 at handoff; no version
16 APK was built or installed.

## Required backend rollout for strict capability

The live host `https://api.primerlms.com/tasks/api` currently omits both fields
below. Strict Student source therefore correctly treats live assigned work as
unavailable.

Before strict hardware acceptance, deploy the shared public API represented by
this branch's `51cad1c0` / educator `1c94d159`:

- `GET /device/today`, `GET /device/upcoming`, and
  `GET /device/occurrences/{id}` (under `/tasks/api`) return student-safe
  occurrence `requirements` containing only `id`, `kind`, `configVersion`,
  `interaction`, and `executor`.
- Those responses return `studentCapability` as exactly `parent_approval` or
  `unsupported`.
- The server emits `parent_approval` only for exactly one requirement whose
  values are `kind=parent_approval`, `configVersion=1`,
  `interaction=parent_action`, and `executor=human`; all mixed/missing/other
  requirement sets are `unsupported`.
- Device submit refuses unsupported work with `409 unsupported_task` rather than
  selecting an arbitrary requirement.

Once that deployment is observed, rebuild strict source as a new same-key
release and repeat pair → Today/Upcoming → Start/Submit → parent
reject/retry/approve → Refresh → server-side revocation. Do not treat the v15
legacy-fallback exercise as strict-capability acceptance.

## Post-handoff production schema observation

At `2026-09-07T11:18:55Z`, after the parent-owned production rollout, a
non-authenticated schema request returned `200`:

```text
GET https://api.primerlms.com/tasks/api/openapi.yaml
SHA-256: a985aba291e285fc5eeb4f090fd270e8253c59169a99c5a09fb83fb0edee2d0c
```

The live `Occurrence2` schema now exposes `studentCapability` with enum
`parent_approval | unsupported` and `requirements.items` referencing
`StudentRequirement`. The live `StudentRequirement` schema contains exactly
`id`, `kind`, `configVersion`, `interaction`, and `executor` as required
fields.

The ignored Kotlin generated client was regenerated locally from this
worktree's Go registry with:

```bash
cd primer-tasks
go run ./cmd/openapi-gen -out build/openapi.yaml
node clients/kotlin/generate-client.mjs
```

A forced `:tasks-client:test` and `:feature-tasks-student:testDebugUnitTest`
pass followed that generation.

## Strict capability A16 acceptance after rollout

After the schema observation, a strict Student APK was built from the current
worktree (strict app behavior in `c2142368`; later commit `8caff804` is this
handoff documentation only), with the production origin pin. No legacy
fallback was restored.

```text
version 16 SHA-256: 0cf8b99d6049ebe526ae79513a17158d8a1bdb1240b51fe53dc99003f85de33c
version 17 SHA-256: ad5cac8458fb38db9544a195039642f7aedd3222d686434c64947d18c3a4f655
package:              com.aleksclark.primer.student
origin:               https://api.primerlms.com/tasks/api
```

Both were signed using the existing protected qualification signing environment
without printing or extracting any signing material. `apksigner verify` passed
for each. Version 16 installed over version 15 with `adb install -r`; version
17 installed over version 16 the same way. At final observation the A16 ran
version 17, remained the same device owner, and reported managed lock-task
state `LOCKED`.

A temporary parent-created student and a single parent-approval task were used
for the strict live loop, then archived for cleanup. Parent create/assign/reject/
retry/approve actions were authenticated public Tasks REST calls from a live
signed-in parent web session, **not native Primer Control UI**. This proves the
Student hardware side; it does not prove the complete two-native-app workflow.
The Student implementer explicitly confirmed that distinction after the run.

The live parent GET for the issued occurrence returned:

```text
studentCapability: parent_approval
requirements: exactly one item with
  kind=parent_approval, configVersion=1,
  interaction=parent_action, executor=human
```

Observed on the version-16 handset:

1. An educator-issued one-use QR paired successfully through the explicitly
   labeled paste fallback.
2. The occurrence appeared in both Today and Upcoming and opened its
   instructions.
3. The explicit `parent_approval` capability exposed Start; Start changed
   `pending` to `in_progress`.
4. Submit changed `in_progress` to `awaiting_verification` and showed the
   waiting-for-parent state.
5. A live parent rejection changed the server occurrence to `pending`; handset
   Refresh showed **Rejected — retry** and exposed Start again.
6. A second Start/Submit followed by live parent approval showed **Approved**
   after handset Refresh.
7. The version-16-to-17 same-key replacement retained the paired student and
   approved occurrence, proving pairing persistence across this strict-code
   replacement.
8. Archiving only the temporary student revoked server access. Handset Refresh
   cleared Tasks to the pairing screen and showed **This device pairing is no
   longer active. Scan a new QR code.** Device ownership and kiosk state stayed
   intact.

This is strict parent-approval acceptance. Unsupported/mixed work is prevented
by the explicit-capability gate and unit tests; no dialogue/media task was
misrepresented as parent approval during the hardware run.
