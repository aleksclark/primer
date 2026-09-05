# Android Phase 4: Release hardening

## Goal

Make the native client operationally ready after the reviewed main Phase 7
release contract. Prove lost-device replacement, revoke/re-pair, full native
verification regression, process/network/server restart, privacy/backup/cache
cleanup, diagnostics, generated-client compatibility, and release packaging.

## BDD Success Criteria

#### Scenario: Device replacement preserves authority

- **Given** a lost/revoked device with completed and active work
- **When** a replacement pairs to the same student
- **Then** it observes existing completion and authoritative active state, old
  bearer/replay fails, and no duplicate submission/decision is created.

#### Scenario: Full native verification regression passes

- **Given** release-candidate Tasks and native app
- **When** the student executes manual, dialogue, media/rubric, and external tasks
- **Then** every state transition matches server facts across refresh/restart
- **And** no earlier completion/evidence is duplicated or weakened.

#### Scenario: Network and server failure recover

- **Given** active dialogue/upload/external waiting
- **When** network drops, Tasks restarts, or workers reconcile
- **Then** the app shows bounded offline/reconnecting state and later reloads the
  durable result without local success or uncontrolled retry.

#### Scenario: Revocation removes local authority

- **Given** a paired release app
- **When** parent revokes/archive or membership policy denies access
- **Then** the next request/WS fails, token and sensitive caches are cleared, and
  re-pair is required while server evidence remains.

#### Scenario: Privacy and backup policy survive release build

- **Given** paired sessions and captured/uploaded media
- **When** storage/logcat/dumpsys/backup/export/cache inspections run
- **Then** no plaintext bearer, pairing material, hidden reasoning, retained
  sensitive media, signed object URL, or verifier secret is found
- **And** backup remains disabled as designed.

#### Scenario: Compatibility and diagnostics are actionable

- **Given** unsupported app/contract versions, stale device, failed worker, or
  unavailable provider/verifier
- **When** parent/native diagnostics are viewed
- **Then** human-readable version/last-seen/sync/failure state is visible without
  secrets
- **And** unsupported operations remain cached/blocked rather than misexecuted.

## Implementation Instructions

1. Finalize release build/versioning/signing configuration, application/network
   security policy, min/target SDK, dependency pins, shrinking rules, and CI APK
   artifacts without embedding environment secrets.
2. Add app version/protocol/capability reporting through generated device
   heartbeat/diagnostic contracts. Show parent-readable compatibility and native
   actionable error states.
3. Harden token rotation/revocation, Keystore failure recovery, no-backup rules,
   screenshot/clipboard/share/export behavior, cache encryption/retention, and
   sensitive media cleanup.
4. Add migration logic for native local metadata across versions; fail closed on
   unknown/corrupt state and never silently switch student/origin.
5. Implement bounded network retry/backoff/jitter, connectivity transitions,
   graceful app/background shutdown, WorkManager cleanup, and server-version
   compatibility negotiation.
6. Add explicit replacement/re-pair flow and diagnostics while preserving
   server-known terminal state and idempotency keys.
7. Complete native accessibility, dark/light parity, keyboard/switch access where
   applicable, orientation/size-class behavior, permission denial, and low-memory
   recovery.
8. Add release runbooks for pairing/replacement, revoke incident, backup/privacy
   inspection, cache reset, signing/update, rollback, and unsupported protocol.
9. Keep generated Kotlin outputs ignored and reproduce them from the reviewed
   server release contract; fail build on stale façade/schema/version drift.

## End-to-End Test Plan

- Fresh release APK pair and full manual/dialogue/media/external task cycle against
  the real release Tasks stack on the default host Make/non-Docker path.
- Force-stop/reboot during each active verification type; network loss and Tasks/
  worker/object/verifier restart at declared safe points; verify durable recovery.
- Revoke and replace device; replay old QR/bearer; load existing completion and
  active work with no duplicate effects.
- Run storage/logcat/dumpsys/backup/shared-file/cache/media-metadata/clipboard/
  screenshot scans before and after revocation/replacement.
- Exercise unsupported app/server protocol versions, failed providers/verifiers,
  stale device/sync lag, and parent diagnostics.
- Test orientation, font scale, dark/light, TalkBack/accessibility, permission
  denial, low storage/memory, and process recreation.
- After exploratory PASS, promote the complete matrix to connected instrumentation
  plus pinned black-box emulator suites; run on a freshly wiped AVD and release
  APK, not mocked screens/services.

Commands:

```bash
cd primer-tasks/android
./gradlew testDebugUnitTest lintRelease assembleRelease connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Verify replacement/re-pair uses public one-use pairing and old credential
  revocation, not local student switching or copied token.
- Trace every native terminal state to server facts/idempotency; reject cached
  success or UI-only completion.
- Inspect real release APK manifest/resources/network config/storage/backup/logs/
  media for secrets, debug endpoints, cleartext exceptions, or retained artifacts.
- Verify generated-client compatibility gate regenerates cleanly and consumers do
  not copy DTOs/raw transport.
- Confirm emulator suites use real Tasks/PostgreSQL/object/external processes and
  preserve exploratory-before-promotion order.
- Ensure release docs do not claim offline authority, live-model quality, or
  physical-camera proof beyond observed evidence.

## Completion Gate

- [ ] Full native manual/dialogue/media/external regression passes on release APK.
- [ ] Process/reboot/network/server/worker/object/verifier recovery passes.
- [ ] Revoke, old credential replay denial, replacement pair, and no-duplicate state pass.
- [ ] Privacy/backup/cache/media/logcat/storage scans pass.
- [ ] Compatibility/version/diagnostic and generated-client gates pass.
- [ ] Accessibility/theme/orientation/permission/low-resource matrix passes.
- [ ] Dedicated exploratory PASS precedes complete promoted emulator automation.
- [ ] Fresh release anti-cheat review approves server authority and privacy boundaries.
