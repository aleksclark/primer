# Phase 6: Fleet acceptance and operations

## Goal

Certify the exact signed apps, A16 firmware and backend through realistic daily
use, failure and recovery. Publish a repeatable Play-free provisioning/update
runbook and a capability matrix grounded in observed behavior. This phase closes
cross-cutting gaps; it cannot waive incomplete earlier phase gates.

## BDD Success Criteria

### Scenario: A real managed school-day loop

- **Given** a freshly provisioned A16 and Control installed from the documented
  Play-free release channel
- **When** a parent assigns work, the student switches between Tasks and approved
  apps, submits, and the parent reviews during a full day including screen-off time
- **Then** the complete Tasks loop works, the allowlist remains enforced and battery/
  network behavior is recorded without disabling production restrictions for the test
- **And** the next-day/reboot session restores the same student and management state.

### Scenario: Escape attempts do not bypass the approved-app policy

- **Given** the certified stock A16 firmware and signed release build
- **When** a tester exercises the documented Home/Back/Recents, quick settings,
  notifications, Settings links, share/open-with, browser deep links, IME settings,
  accessibility, multi-window/pop-up, assistant, store, USB and safe-mode entry paths
- **Then** unapproved use is prevented or an explicit supported-policy limitation is
  recorded and resolved before rollout
- **And** approved accessibility, keyguard, emergency access and parent-selected
  normal communications still work; the test does not place an emergency call.

### Scenario: Offline and update recovery survives real interruptions

- **Given** queued policy/release changes, unfinished work and an enrolled device
- **When** Wi-Fi/mobile data are interrupted, the phone sleeps/reboots, the server
  restarts and an update occurs during the acceptance window
- **Then** last-known policy stays enforced, Tasks does not invent decisions, and
  management/update reconciliation eventually reports actual state
- **And** latency, blocked operations and stale status are visible to the parent.

### Scenario: Lost access and decommission are recoverable

- **Given** a parent loses a session or the management service is unavailable
- **When** the documented offline recovery and network-repair process is followed
- **Then** the parent can recover without a universal secret or disabling isolation
- **And** maintenance expires/relocks and its audit is uploaded after reconnection.
- **Given** an explicit parent decision to retire/reset/replace the device
- **When** the physical decommission procedure is followed
- **Then** old Tasks/management credentials are revoked, factory-reset protection
  requirements are understood, and a replacement needs fresh parent authorization.

### Scenario: A release is reproducible and supportable

- **Given** a clean checkout, restored artifact/metadata backups and controlled signing access
- **When** an operator follows the build/publish/pilot/update/forward-fix procedures
- **Then** the three packages and their provenance are identifiable, a pilot alone
  receives its target, and support can diagnose failure without student secrets
- **And** an OS/One UI update invalidates only the affected certification until the
  firmware-sensitive provisioning/lockdown/update/recovery subset is rerun.

## Implementation Instructions

1. Create `agent_docs/runbooks/android-client-ops.md` and finalize the Phase 1 A16
   provisioning runbook. Include exact app IDs/receiver, firmware/build numbers,
   security settings, setup QR, bootstrap trust, signing custody, approved-app
   policy, recovery/code custody, install permission and Control consent behavior.
2. Define the initial approved-app baseline with the parent. Test actual chosen
   apps and their system dependencies; package allowlisting does not constrain
   websites/content inside an approved app. Do not silently approve all browsers,
   stores, Settings or accessibility services to make integration tests pass.
3. Record the supported firmware matrix including exact model/variant (the first
   connected target is SM-S166V), carrier/SIM, Android/One UI/security patch and
   Samsung security settings. Separate standard
   Android API support, actual observed enforcement, untested cases and unavoidable
   physical-reset/root limitations. No "fully locked" blanket badge.
4. Add real-device escape and cross-feature regression coverage around enrollment,
   task revocation, app switching, application replacement and maintenance. Improve
   unsupported failures in the production implementation, or stop rollout and
   obtain an explicit scope/hardware decision. Do not solve failures with hidden
   debug flags or global validation bypasses.
5. Rehearse last-resort recovery if Student crashes or cannot start. The app cannot
   repair itself in that condition; document physical access, Samsung account/FRP
   prerequisites and factory-reset/reprovision. Do not erase a phone or bypass FRP
   without its owner's explicit execution approval and legitimate credentials.
6. Rehearse backup/restore of management policy/audit/artifact metadata and immutable
   release bytes in an isolated environment; retained old versions/signing access
   must support a forward fix. Do not restore a stale database over live devices
   and assume consumed credentials/recovery codes are valid. Document revocation
   and reconciliation safeguards for disaster recovery.
7. Produce redacted diagnostics with app/version, firmware, owner/readback status,
   desired/applied revision, last contact, installer attempt/session and sanitized
   error codes. Never include tokens, recovery values, task evidence media or full
   student communications. Make status useful from Control without a debug cable.
8. Pin and document exact root commands for clean generation/build/test/coverage,
   signed release validation, host E2E, per-device install-preserving acceptance,
   publication and pilot targeting. Retire duplicate legacy native entry points
   only after compatibility wrappers/runbooks are current. Preserve historical
   test artifacts rather than rewriting them as if they covered these apps.
9. Keep optional future work separate: fast wake-up transport, additional device
   models, Knox-specific features, advanced web/content policy and the existing
   native Tasks dialogue/media/external-verifier continuation. None is silently
   counted as delivered by this initial fleet release.

## End-to-End Test Plan

- **Setup:** signed release builds from a clean commit, real Tasks/management service
  and PostgreSQL, persistent release storage, actual A16, parent Control device,
  dedicated TV box, documented parent-approved app baseline and recovery materials.
- **Day-in-use:** provision by public setup flow, pair/create/assign/review through
  Control/Student, run approved apps, lock/sleep/reboot, install N+1 by remote target,
  and continue Tasks afterward. Record battery, time-to-reconcile and failures over
  at least a full day plus next-day restart; no simulated clock replaces this evidence.
- **Escape matrix:** exercise each BDD entry route in the shipped configuration;
  capture redacted screens and actual OS state. Include OEM-specific surfaces and
  allowed-app redirects rather than merely asserting launcher icon absence.
- **Fault matrix:** service restart, interrupted connectivity, screen-off/OEM battery
  limits, repeated/stale reports, constrained storage, process/reboot during install,
  expired parent session, independent Tasks/management revocation and offline recovery.
- **Operations:** operator follows runbook without implementation-only knowledge,
  restores isolated backups, targets a pilot, pauses further rollout, publishes a
  higher-version fix and physically decommissions/reprovisions a test device with
  explicit approval. Verify old credentials fail and histories remain auditable.
- **Compatibility:** rerun TV pairing/playback/seek/updates and native/web Tasks flows;
  same APK identities and service boundaries survive. Control consent cases must
  remain genuine OS outcomes, not automation clicking approval and calling it silent.
- **Commands:** use root `make design-system`, `make tasks-clients tasks-test
  tasks-cover tasks-android`, `make tv-test` and shared Android Gradle lint/test/
  build gates, plus exact connected/release commands introduced by prior phases.
  Run all affected repository gates; unavailable dependencies block the relevant gate.

## Anti-Cheating Audit

- Match every capability-matrix row to an assertion, firmware/build and retained
  evidence. No emulator-only A16 claim, future test presented as executed, or skip
  counted as pass. State exact unattended-Control support, not universal support.
- Verify real first-party production wiring, PostgreSQL/artifact persistence and
  OS effects throughout; no mocked DPM/backend/release channel or in-memory success.
- Confirm observations follow public UI/API/CLI setup/action boundaries; no direct
  database seeding, ADB policy application, privileged install or injected token
  stands in for the behavior under test. Read-only diagnostics are permissible.
- Review security guards, replay/lease/consumed-code state and device/household scope
  after backup restore, revoke and update. Client-only isolation is insufficient.
- Check no test-only permissions, relaxed app allowlist, disabled Samsung control,
  swallowed failures, excessive retries or hard-coded success differ from the
  documented shipped configuration. Parent-approved Samsung setting changes must
  appear explicitly in the certification receipt.
- Reports/events must derive from durable source state and actual device readback;
  notifications/push/installer callbacks alone are not acceptance.
- Audit logcat, recordings, backups, APK assets and source for credentials/recovery
  material. Verify no broad telemetry was added to compensate for weak diagnostics.

## Completion Gate

- [ ] All six phases' BDD scenarios and real-dependency gates pass.
- [ ] Exact A16/parent-device/TV firmware and signed APK receipts are retained.
- [ ] Full-day/reboot, escape, offline/restart/update and physical recovery matrices pass.
- [ ] Emergency/accessibility/communication behavior and content-policy limits are explicit.
- [ ] Another operator can follow provisioning, Play-free release and recovery runbooks.
- [ ] Clean generation, lint/build/test/coverage and connected regression gates pass.
- [ ] Anti-cheating review finds no substitutions; remaining external/unsupported cases
  are named rather than labeled complete.
