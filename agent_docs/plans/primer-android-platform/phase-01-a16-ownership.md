# Phase 1: A16 ownership and recovery

## Goal

Qualify the actual Galaxy A16 5G for a recoverable, Play-free, multi-app managed
Student deployment. Produce the initial Student DPC/launcher target and prove a
signed same-package update before investing in fleet management. This phase is
usable for supervised qualification, not approved for unattended student use.

## BDD Success Criteria

### Scenario: Real setup-time ownership

- **Given** the parent's reset approval, preserved personal data, a stock A16 with
  its exact variant/firmware recorded, and a signed Student provisioning APK
- **When** the parent follows the documented factory-reset/Setup Wizard enrollment
- **Then** Android reports Student as device owner and Student becomes persistent home
- **And** no Play account, paid EMM or Knox enrollment subscription is required.

### Scenario: Managed multi-app use and reboot

- **Given** Student owns the device and two explicitly approved apps are installed
- **When** the student launches each app, presses Home/Back, locks/unlocks and reboots
- **Then** approved apps function and Home returns to Student
- **And** unapproved launcher/settings/store paths cannot provide unrestricted use,
  policy persists offline, and keyguard/emergency access remains available.

### Scenario: Parent recovery is real and bounded

- **Given** the A16 has no network and a parent holds a per-device one-use recovery code
- **When** the parent enters maintenance through the visible recovery surface
- **Then** necessary network/setup repair is possible for a bounded session
- **And** expiration or reboot reapplies policy, the code cannot be replayed, and a
  durable local audit records entry/exit without storing the secret in logs.
- **And** repeated wrong codes are rate-limited across process death and reboot.

### Scenario: Signed replacement retains ownership

- **Given** signed Student N is device owner with policy and recovery state
- **When** a verified same-key N+1 is installed through Student's PackageInstaller path
- **Then** no student confirmation is required on the certified A16 configuration
- **And** the running version becomes N+1 with ownership, policy and recovery state intact.
- **When** a wrong-key, wrong-package or older APK is offered instead
- **Then** it is rejected visibly without opening an unrestricted installer or launcher.

### Scenario: Unsupported setup does not masquerade as management

- **Given** an already provisioned/unmanaged phone, a competing owner, or Samsung
  security settings that block enrollment/installation
- **When** the parent tries to enroll or update
- **Then** the app reports the specific setup/blocker state, never "managed" based
  only on a local preference or screen pinning
- **And** no automated reset or security-setting bypass occurs.

## Implementation Instructions

1. Add proposed `android/app-student/` and the smallest used
   `android/core-device-policy/` / `android/core-updates/` modules. Keep existing
   `:app` and `:core` TV targets working. Pin the Student application ID and fully
   qualified receiver component before device-owner provisioning. Use the real
   release variant and controlled signing identity; no keystore in source control.
2. Implement required setup-time DPC activities/receiver/manifest declarations,
   including current Android provisioning-mode and policy-compliance callbacks
   where required. Qualify QR provisioning using an HTTPS APK location and correct
   Android provisioning checksum format. ADB `dpm set-device-owner` is a development
   aid only; it cannot stand in for the production setup-time test.
3. Extract/reuse mechanisms from TV's `KioskPolicy.kt`, not its TV-only eligibility
   rule or swallow-and-log success semantics. Use persistent preferred HOME,
   allowlisted lock task, Home support for returning from approved apps, and
   supported activity-start restrictions. Verify system readback. Distinguish
   true `LOCK_TASK_MODE_LOCKED` from removable screen pinning.
4. Provide parent-only local bootstrap configuration before enabling restrictions:
   approved package/signing identities, required system dependencies, and recovery
   material. Freeze this bootstrap surface after management is activated. No
   general exported policy setter, debug-only bypass or universal password.
5. Implement per-device cryptographically random one-use recovery codes shown to
   the parent during setup, stored only as hardened verifiers on Student. Persist
   consumption before unlocking, attempts/backoff and audit; keep a bounded
   maintenance lease and always relock on reboot. Phase 4 adds authenticated
   remote issuance/rotation. Document secure parent custody now. Rehearse physical
   recovery before disabling debugging; do not claim a stopped/crashing DPC can
   repair itself. Physical factory reset/reprovision is the last-resort procedure.
6. Persist nonsensitive enforcement state in device-protected storage where boot
   requires it; do not move Tasks or management bearers there. Reconcile on boot,
   unlock, resume and package replacement, respecting background-start limits.
   Before unlock, preserve Android keyguard; do not expose educational data.
7. Build a minimal verified PackageInstaller session mechanism for N -> N+1 with
   persistent attempt/session identity and actual installed-version readback.
   A parent maintenance action may supply the immutable release for qualification;
   this is not yet remote rollout. OS package verification remains final authority.
8. Record Auto Blocker/Maximum Restrictions settings, battery handling, SIM/carrier,
   accessibility/IME, emergency and ordinary communication behavior. The parent
   must approve and perform any required security-setting change. Test it rather
   than assuming Samsung defaults or exemptions. No Knox API is assumed available.
9. Add `agent_docs/runbooks/android-a16-provisioning.md` (new) with prerequisites,
   exact commands/QR procedure, recovery, supported firmware and negative outcomes.
   Do not actually reset/enroll a user's handset without explicit execution approval.

## End-to-End Test Plan

- **Setup:** dedicated resettable A16, exact firmware recorded, secure signing
  material, HTTPS provisioning APK endpoint, two real approved apps, network
  disconnect capability and parent-held recovery codes. No first-party fake.
- **Public actions:** parent Setup Wizard QR enrollment; launch apps from Student;
  Home/Back/notification/settings/share-link attempts; lock/reboot/offline; local
  maintenance entry; N -> N+1 install through Student (not `adb install -r`).
- **Assertions:** real owner/lock-task readback, visible screen recordings, package
  version/signing identity, persisted policy, recovery replay denial and audit.
  ADB may observe OS state during qualification, not apply policies for the app.
- **Negative cases:** unmanaged device, another owner, blocked sideloading,
  incorrect recovery code, expired/replayed code, wrong signing key/package,
  reboot during maintenance/install. No live emergency call is placed for testing;
  inspect emergency access safely with the operator.
- **Repeatability:** perform enrollment/recovery/replacement again from documented
  prerequisites. Supplemental emulator tests exercise API/version branches but
  cannot certify Samsung behavior.
- **Build gates:** `cd android && ./gradlew test assembleDebug` remains valid;
  introduce explicit Student release/lint targets and document their exact commands.

## Anti-Cheating Audit

- Inspect DPM calls and readback: no fake `isManaged`, screen-pinning-only proof,
  ADB-applied allowlist, hidden accessibility-service kiosk, or success after errors.
- Inspect provisioning/recovery entry points for exported setters, fixed codes,
  in-memory consumed-code state, unbounded attempts and debug-only behavior.
- Ensure launcher hiding is not the only escape protection; test intents, task
  transitions and Samsung surfaces instead of only checking launcher screenshots.
- Ensure APK replacement uses PackageInstaller and real signatures; no fixture APK
  bytes, privileged test installer or direct callback injection counts as proof.
- No swallowed policy failure, broad retry loop, skipped firmware case or capability
  claim without an asserted OS effect. Unit DPM mocks do not count as device evidence.
- Audit local persistence/audit writes before reported success; no secrets in
  recordings/logcat. Local setup authority is physical parent setup, not a future
  unimplemented server flag.

## Completion Gate

- [ ] All BDD scenarios pass on the actual A16; missing handset access blocks completion.
- [ ] Real QR/setup-time ownership, multi-app return, offline reboot, recovery and
  silent signed replacement have retained evidence.
- [ ] Firmware/security-setting/communication limitations are explicit.
- [ ] Production and development provisioning are distinguished; recovery is rehearsed.
- [ ] Anti-cheating review passes; unit/lint/build gates pass and generated output is current.
