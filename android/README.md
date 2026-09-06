# Primer TV — Android client

This Gradle build also includes the initial **Primer Student device-owner
qualification** target (`:app-student`), with `:core-device-policy` and
`:core-updates`. See the [A16 qualification runbook](../agent_docs/runbooks/android-a16-provisioning.md)
and [physical-device evidence](../test-artifacts/android-student/device-owner-qualification.md).
It is not yet the full Student/Control platform, and silent Student updates remain
an open verification gate on the tested handset/signing configuration.

The student-facing half of [Video As Instruction](../agent_docs/plans/video-as-instruction.md).
One APK runs on the tablet and on the living-room Android TV box; the shell is
chosen at runtime from `UiModeManager.currentModeType`.

This phase covers **on-demand viewing only**. The programmed channel (a
scheduled linear stream with a fully locked player) is a later phase.

## Modules

| Module | Contents |
|--------|----------|
| `core` | Pure Kotlin/JVM: API client, domain model, playback session state machine, watch-time accounting. No Android dependencies, so all of it is unit-testable on the JVM. |
| `core-ui` | Shared System C Compose theme, fonts, and primitives (`com.aleksclark.primer.ui`). Consumed by TV and Student; Control must use this library, not a copy. |
| `app`  | Android: Compose UI (tablet + leanback), ExoPlayer host, DataStore persistence. |
| `feature-tasks-student` | Migrated Tasks pairing/checklist/start/submit for Student. Entry: `StudentTasksRoute`. |
| `app-student` | Device-owner launcher. Tasks, approved apps, and parent maintenance; recovery stays in `StudentRuntime`. |
| `core-parent-identity` | Official Clerk Android SDK 0.1.31 password sign-in adapter (Kotlin 2.0.21 pin). Publishable key only. Live Clerk acceptance is not claimed. |
| `feature-tasks-control` | Native parent roster/tasks/schedules/review using `:tasks-client`. |
| `feature-device-control` | Unprivileged parent device/release surfaces using generated parent JWT calls. |
| `app-control` | Primer Control (`com.aleksclark.primer.control`). Not a device owner. |

Keeping the interesting logic in `core` is deliberate: the grant lifecycle,
heartbeat cadence, and watch-once rules are tested without an emulator.

## Primer Control

Unprivileged parent app (`:app-control`, package `com.aleksclark.primer.control`).
It is not a device owner. Screens talk only through `:tasks-client` with a parent JWT.
**minSdk is 28** so Control can consume `:core-updates` `SelfUpdateSession`. There is no
accepted API 26 Control deployment; do not `overrideLibrary` or duplicate the installer.

```bash
cd android
PRIMER_CLERK_PUBLISHABLE_KEY=pk_test_... \
PRIMER_API_ORIGIN=https://api.primerlms.com/tasks/api \
./gradlew :app-control:assembleDebug --no-daemon --max-workers=1
```

External Clerk setup is parent-owned. This tree does not claim live JWT azp or
physical acceptance. Needed configuration, without changing canonical auth here:

- Publishable key only in the APK (`PRIMER_CLERK_PUBLISHABLE_KEY`). Secrets stay out of the binary.
- Native SDK is Clerk Android API **0.1.31** because 1.1.x ships Kotlin 2.4 metadata and this Gradle tree is Kotlin 2.0.21. Hosted Account Portal is not in 0.1.31; Control currently implements official `SignIn.create` password then `Clerk.setActive` of the **new** session ID only. If the live instance requires another factor, extend that native flow — do not weaken server policy.
- Declared 0.1.31 coordinates (POM, not live runtime): Kotlin stdlib 2.1.20, serialization-json 1.9.0, coroutines 1.10.2, androidx.browser 1.9.0. Those artifacts ship Kotlin 2.2 metadata. `:core-parent-identity` and `:app-control` force stdlib 2.0.21 / serialization 1.7.3 / coroutines 1.9.0 / browser 1.8.0 so Kotlin 2.0.21 / AGP 8.7.3 can compile. Compile success is not runtime-compatibility evidence. There is no root-wide force and no `-Xskip-metadata-version-check`. Unit tests do not initialize a live Clerk backend.
- Control refreshes the official SDK token before authenticated work and on resume. Server logout must succeed before provider sign-out. FLAG_SECURE, backup exclusion, and password IME are set. Live Clerk JWT claims are unmeasured here.
- Proposed additive server env (parent L0 owns the port): `TASKS_CLERK_AUTHORIZED_PARTIES=com.aleksclark.primer.control`. `PublicOrigin` must remain required; extra parties must not replace the web origin check.

### Control self-update

Control consumes `:core-updates` `SelfUpdateSession` through `SelfUpdateCommands`
(`install`, `handleResult`, `resumeUserAction`, `reconcile`, `snapshot`, `cancel`).
Callers do not read the adapter's SharedPreferences layout. The adapter snapshots
APK bytes (`copyVerified`), checks device ABIs, and compares candidate targetSdk
to the running OS floor. Callers delete their download temp in `finally`.
Verifier is Tink `Ed25519Verify` only — host digest||zeros is not accepted.

Foreground confirmation is dispatched only through a resumed Activity. A silent
`startActivity` return is not presentation. Otherwise Control checks
`POST_NOTIFICATIONS`, `areNotificationsEnabled`, and the update channel, then
posts a real notification or records **Deferred**. Continue Confirmation / onResume
re-dispatches through `resumeUserAction`. Missing session, cancel, and denied
notifications are observable; they are not `presented=true`.

Control candidates are selected by running package **and** the stable channel,
newer than the installed version. The decoded signed manifest must match outer
package/channel/version/signer/size/hash before download. Device rollout still
uses `selectedReleaseId`; Control self-update does not.

Install/hash/copy run on IO. Catch-up of a pending confirmation happens on resume.
Parent settings may enable catalog checks on resume and a 15-minute-floor periodic
refresh. Unattended catch-up is opt-in and only when the shared adapter reports
`EligibleUnattended`; confirmation-required, deferred, and failed states never
auto-install. Discovery never skips signed-manifest verification. This is not a
silent-install guarantee and not live Clerk/self-update acceptance. Missing
`PRIMER_RELEASE_TRUST_ROOT` fails closed. minSdk **28**; no `overrideLibrary`; no
duplicate installer. Production `ReleaseTrust` verifies only; fixture sign helpers
are not on the production type.

### Exact Clerk identity acceptance (external; not the test issuer)

The local Tasks test issuer is **not** Clerk. Native Control acceptance must use
the real Clerk instance that already serves the Tasks web parent. Do not paste
JWTs, scrape cookies, or revive `/auth/callback`. Do not put production secrets
in source or APKs. Creating test users and household memberships needs **operator
approval**. Actual Clerk policy and JWT claims are **external measurements** —
this README does not assert observed native `azp` or that password is enabled /
MFA is disabled on the live instance.

Control 0.1.31 implements official `SignIn.create` password strategy because
hosted Account Portal is not in that SDK. If the configured instance requires an
unsupported factor (MFA, magic link, SSO, new password), **extend the native
flow** to that Clerk-supported method. Do not weaken server authorized-party,
issuer, or membership policy to make login work.

**Operator checklist (approval required; do not change instance policy here):**

1. Confirm whether Android application ID `com.aleksclark.primer.control` is
   registered on the Clerk instance. Measure the resulting session `azp` from a
   real Control login; do not assume it.
2. Confirm which first factors the instance actually allows. Do not enable
   password or disable MFA from this tree.
3. After operator approval, provision two parent users and two household
   memberships (A and B). Do not reuse browser test-issuer principals.
4. Publishable key only on the device (`PRIMER_CLERK_PUBLISHABLE_KEY`). Secret
   keys stay in operator env.

**Tasks host (operator env, parent L0 owns the port; Control does not edit these files):**

```text
TASKS_AUTH_MODE=clerk
TASKS_CLERK_ISSUER=https://<clerk-frontend-api>
TASKS_CLERK_JWKS_URL=https://<clerk-frontend-api>/.well-known/jwks.json
TASKS_PUBLIC_ORIGIN=https://<tasks-web-origin>   # remains required
TASKS_CLERK_AUTHORIZED_PARTIES=com.aleksclark.primer.control
```

`TASKS_CLERK_AUTHORIZED_PARTIES` is proposed additive config. PublicOrigin must
stay required. Optional `TASKS_CLERK_AUDIENCE` only if this Clerk instance emits
`aud`. Household membership remains the local Tasks ledger, not Clerk orgs.

**Control debug build (no production mutation):**

```bash
cd android
PRIMER_CLERK_PUBLISHABLE_KEY=pk_test_... \
PRIMER_API_ORIGIN=https://<tasks-host>/tasks/api \
./gradlew :app-control:assembleDebug --no-daemon --max-workers=1
```

Acceptance measurements after a real Control login (not claimed): issuer matches
`TASKS_CLERK_ISSUER`, session id present, authorized-party matches whatever Clerk
emits for this app, no-membership denial, household B isolation, Tasks logout
before Clerk.signOut.


## Build

Requires JDK 17 (AGP 8.7 rejects newer JDKs) and the Android SDK.

```bash
cd android
./gradlew test           # unit tests, no device needed
./gradlew assembleDebug  # app/build/outputs/apk/debug/app-debug.apk
```

Production versions are supplied by `primerVersionCode` / `primerVersionName`
Gradle properties (or `PRIMER_ANDROID_VERSION_CODE` /
`PRIMER_ANDROID_VERSION_NAME`). Version codes must increase for every update.
Production signing is enabled only when all four environment variables are set:
`PRIMER_ANDROID_KEYSTORE`, `PRIMER_ANDROID_STORE_PASSWORD`,
`PRIMER_ANDROID_KEY_ALIAS`, and `PRIMER_ANDROID_KEY_PASSWORD`. The equivalent
Gradle properties are `primerSigningStoreFile`, `primerSigningStorePassword`,
`primerSigningKeyAlias`, and `primerSigningKeyPassword`; keep them in the ignored
`keystore.properties` file and pass `-PprimerSigning...` when building locally.
Devices must be provisioned initially with that same production signing identity;
Android will reject later APKs signed with another key.

If Gradle cannot find the SDK, create `android/local.properties`:

```
sdk.dir=/path/to/Android/Sdk
```

The APK ships `arm64-v8a` and `armeabi-v7a`, which covers both the tablet and
the RK3318 box.

## Sideloading to the TV box

The T9 box has no Play Store, so install over ADB. Enable **Developer options →
USB debugging / Network debugging** on the box first, then:

```bash
adb connect 192.168.1.50:5555        # the box's LAN address
adb -s 192.168.1.50:5555 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s 192.168.1.50:5555 shell monkey -p com.aleksclark.primer.tv 1
```

`-r` reinstalls in place and keeps the pairing. Use `adb logcat` for
troubleshooting.

The app declares both `LAUNCHER` and `LEANBACK_LAUNCHER`, so it appears in the
tablet's app drawer and on the TV home row.

## Android 9 dedicated-device provisioning

Kiosk behavior is deliberately gated by both TV mode and device ownership, so
the same APK remains a normal app on tablets. Device ownership can only be set
on a factory-reset/unprovisioned device. For development provisioning:

```bash
adb install app/build/outputs/apk/debug/app-debug.apk
adb shell dpm set-device-owner \
  com.aleksclark.primer.tv/.app.admin.PrimerDeviceAdminReceiver
adb shell dpm list-owners
```

After provisioning, `MainActivity` allowlists only Primer TV for lock task and
enters lock task whenever it resumes. The device restrictions block factory
reset, safe boot, extra users, overlays, and removable media, but intentionally
do **not** restrict Wi-Fi/network repair, Bluetooth/audio/volume, or package
installation. Validate these APIs on the production RK3318 firmware because
some Android 9 TV ROMs have incomplete device-policy implementations.

Device-owner updates use a `PackageInstaller` session for silent same-package
replacement. Downloads are size/checksum checked and must contain the same
package, a newer published version, and a compatible signing certificate.
Unmanaged devices, and device-owner ROMs that reject silent sessions, use the
interactive system installer fallback.

## Pairing

1. In the admin SPA, open **Devices** and register the device to get a pairing
   code.
2. Launch the app. Enter the server address (`tv.local:8081`, or paste the admin
   URL — a trailing `/api/v1` is stripped automatically) and the code.
3. The device exchanges the code for a token, stored in DataStore. The code is
   single-use; re-issue one from the admin UI to pair again.

**Settings → Unpair** forgets the token but keeps the server address.

## Playback rules

Enforced by the server; the app reflects them rather than deciding them.

- **Educational / mixed** — pause and seek. This is study material, so
  re-watching a passage is expected.
- **Entertainment** — pause, but **no seeking**. These are rationed by the
  server's watch-once ledger, and scrubbing to the last minute would spend the
  single viewing without watching it. Seek commands are withdrawn from the
  player itself, so the transport controls, the D-pad, and any media session all
  obey the rule.

Progress is reported every ~30s. Backgrounding stops the heartbeats but keeps
the session open, so an interrupted film resumes on the same grant instead of
spending a second play.

## Direct play only

The app never asks Jellyfin to transcode. The RK3318 box cannot keep up with a
server-side transcode, and a NAS usually cannot either, so the library must be
in a format the client can decode without a server-side re-encode:

| | Supported |
|---|---|
| Video | H.264 (AVC), H.265 (HEVC), up to 4K (SoC hardware) |
| Container | MKV, MP4 |
| Audio | AAC, MP3 (hardware MediaCodec); AC3, EAC3, DTS via bundled Media3 FFmpeg software fallback |

### Bundled AC3 / EAC3 / DTS (FFmpeg)

The T9 / RK3318 (and many other Rockchip TV boxes) ship without AC3/EAC3/DTS
MediaCodec decoders. The app vendors the official AndroidX Media3 1.4.1
`decoder_ffmpeg` extension built against FFmpeg 6.0 with **LGPL-compatible**
audio decoders only (`ac3`, `eac3`, `dca`). ExoPlayer is wired with
`DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON`, so platform AAC/MP3 stay
on hardware and FFmpeg is used only when MediaCodec cannot handle the track.

**Limits**

- Software decode burns CPU. Multichannel AC3/EAC3/DTS is fine on RK3318 for
  typical TV content; very high-bitrate tracks can still hitch.
- **TrueHD / Atmos truehd is not enabled** (heavy and uncommon in this library).
- DTS-HD MA core may play via the DTS (`dca`) decoder; full lossless MA is not
  a target.
- x86 / x86_64 `.so` files are included in the AAR for emulator completeness;
  the APK still ships only `arm64-v8a` and `armeabi-v7a`.

Rebuild / provenance: see
[`third_party/media3-ffmpeg/`](third_party/media3-ffmpeg/README.md).

The admin SPA flags items that fail the direct-play allowlist, and the server
refuses to offer them in the catalog — an unplayable title should never reach
the student as a black screen.
