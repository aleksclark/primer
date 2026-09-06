# Release verifier — Android 9 / API 28 runtime evidence

## Result

**PASS for the scoped release-signature verifier test on real Android API 28.**
This is not an APK installation, TV playback, live Clerk, or full Phase 5 acceptance.

- Dedicated emulator: `primer-release-api28`, Google APIs x86_64 Android 9 image.
- `adb shell getprop ro.build.version.sdk` returned `28`.
- No device-owner enrollment or physical A16 change was performed for this test.
- Integration included `8a9bbd15` (A's `09966456`), Tink Ed25519 verification,
  independent test vectors and the Android instrumentation test.

## Executed boundary

```bash
JAVA_HOME=/usr/lib/jvm/java-17-openjdk ANDROID_HOME=/opt/android-sdk \
  ./android/gradlew -p android --no-daemon --max-workers=1 \
  :core-updates:assembleDebugAndroidTest :core-updates:testDebugUnitTest

adb -s emulator-5582 install -t \
  android/core-updates/build/outputs/apk/androidTest/debug/core-updates-debug-androidTest.apk
adb -s emulator-5582 shell am instrument -w -r \
  -e class com.aleksclark.primer.updates.ReleaseTrustAndroidTest \
  com.aleksclark.primer.updates.test/androidx.test.runner.AndroidJUnitRunner
```

Observed instrumentation result:

```text
com.aleksclark.primer.updates.ReleaseTrustAndroidTest:
.
Time: 0.073
OK (1 test)
INSTRUMENTATION_CODE: -1
```

Assertions executed on Android, not merely JVM 17:

- RFC 8032 section 7.1 TEST 1 empty-message signature validates against its
  published Ed25519 public key.
- The independent Go `crypto/ed25519` signature over the UTF-8 ReleaseManifest
  payload validates.
- `SHA256(publicKey || message) || zeros` does **not** validate.
- `SignedManifestCodec.verifyEnvelope` verifies the exact payload and parses it
  through the generated release document type, including escaped/Unicode text.

## Security correction and scope

Earlier unaccepted candidate code had an unconditional digest-plus-zeros
verification fallback labeled as a host fixture. It was a real verification
bypass, not safe test scaffolding. It was removed in `20938a7c` (A's `ed7b1e8e`).
Production `ReleaseTrust` now only verifies genuine Ed25519 signatures. Fixture
signing helpers were removed from production and replaced by independent vectors
in test sources.

The physical A16's qualification build 13 predates this shared verifier. No
production deployment or accepted release containing that fallback is claimed.
Older local candidate builds must not be deployed.

Raw instrumentation/emulator logs remain in ignored
`.paseo-e2e/android-release-api28/`. The public test key and signatures are test
vectors, not production trust roots.

## Remaining gates

- Real signed APK download/replacement and confirmation handling on unprivileged
  Control and on the TV hardware.
- Actual operator publication of a matching TV signed sidecar and API/client test.
- A16 silent-update behavior remains blocked by the observed Play Protect review.
- Canonical P4-first producer reconciliation, full regressions, and live identity
  acceptance remain separate requirements.
