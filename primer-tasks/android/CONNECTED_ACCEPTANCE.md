# Connected Phase 2 acceptance runner

Student migration (`:feature-tasks-student` / `com.aleksclark.primer.student`) is in progress. Keep this prototype runner until the Student connected path is proven. Do not copy tokens from `com.aleksclark.primertasks` into Student.

`OccurrenceDeepLinkConnectedTest` is the connected promotion test for the real `MainActivity` deep-link path. It does not seed occurrences, create a fake transport, or extract credentials. The occurrence IDs and foreign title must come from the recorded public Parent-A/Parent-B browser acceptance flow.

Prerequisites:

1. Start the real Tasks API and use an emulator-reachable origin (`http://10.0.2.2:<api-port>` for a loopback API).
2. Pair the emulator through the public Parent-A QR flow. Keep app data and that pairing intact while installing the debug APK (`adb install -r`, not a data-clearing install).
3. Use a completed own occurrence and a real Parent-B occurrence. Do not invent IDs.

Run without the Gradle connected task's uninstall/data-reset behavior:

```bash
cd primer-tasks/android
ANDROID_HOME=/opt/android-sdk PRIMER_API_ORIGIN=http://10.0.2.2:<api-port> \
  ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest
/opt/android-sdk/platform-tools/adb install -r app/build/outputs/apk/debug/app-debug.apk
/opt/android-sdk/platform-tools/adb install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
/opt/android-sdk/platform-tools/adb shell "am instrument -w \\
  -e ownOccurrenceId <public-own-id> \\
  -e foreignOccurrenceId <public-foreign-id> \\
  -e foreignTitle '<public-foreign-title>' \\
  com.aleksclark.primertasks.test/androidx.test.runner.AndroidJUnitRunner"
```

The `adb install -r` steps preserve the target app's encrypted DataStore. Do not
run `connectedDebugAndroidTest` after pairing unless its uninstall behavior is
explicitly controlled.

The own case asserts `TASK DETAIL` and `Status: completed`. The foreign case asserts `TASK UNAVAILABLE`, the generic unavailable copy, and absence of both the foreign ID and foreign task title. Missing arguments fail explicitly rather than skipping. A Gradle connected test install that clears the existing pairing is an environment failure; re-pair using the public QR flow before rerunning.

When Gradle's connected-test task would reinstall/uninstall the paired app, use this equivalent install-preserving route after the APK is built (do not run `clean`, `uninstall`, or `pm clear`):

```bash
./gradlew --no-daemon :app:assembleDebug :app:assembleDebugAndroidTest \\
  -PprimerApiOrigin=http://10.0.2.2:<api-port>
ADB=/opt/android-sdk/platform-tools/adb
$ADB -s <serial> install -r app/build/outputs/apk/debug/app-debug.apk
$ADB -s <serial> install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
$ADB -s <serial> shell am instrument -w -r \\
  -e ownOccurrenceId=<public-own-id> \\
  -e foreignOccurrenceId=<public-foreign-id> \\
  -e foreignTitle=<title-with-escaped-spaces> \\
  com.aleksclark.primertasks.test/androidx.test.runner.AndroidJUnitRunner
```

The `-e` arguments are the same instrumentation arguments as the Gradle invocation; escape spaces in the title for the device shell. Re-check the paired profile after both `install -r` commands and re-pair through the public QR flow if the install has removed it.
