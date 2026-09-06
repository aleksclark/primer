# Phase 5: Play-free release delivery

## Goal

Deliver real remotely requested updates without Play: publish immutable signed
APKs, target Student devices from Control, silently replace Student on the A16,
and report installed versions after restart. Control self-updates unattended when
Android permits it and otherwise presents the system consent flow. Reuse installer
mechanisms in TV without coupling its service credentials to Tasks management.

## BDD Success Criteria

### Scenario: Parent pushes Student N+1

- **Given** a trusted published Student N+1 and an enrolled A16 running N
- **When** an authorized parent selects N+1 for that device in Control
- **Then** durable desired state is saved; Student downloads/verifies/installs it
  without student confirmation on the certified A16
- **And** Student restarts with Tasks pairing, management, approved apps and recovery
  intact, and Control confirms N+1 only after actual installed-version reporting.

### Scenario: Offline/replayed/interrupted delivery converges

- **Given** a pending release target while the A16 is offline
- **When** the server restarts, the request is replayed, the device reconnects, or
  download/process/reboot interruption occurs during the update
- **Then** the same immutable target resumes/reconciles without duplicate active
  installs or loss of device policy
- **And** Control distinguishes queued, downloading, verifying, installing,
  confirmed, blocked and failed, including stale last-seen information.

### Scenario: Untrusted/incompatible artifacts never install

- **Given** a release with missing/wrong digest or size, wrong package/signature,
  stale version, incompatible SDK/ABI, excessive size, or disallowed URL/redirect
- **When** publication or download/verification is attempted
- **Then** it is rejected with a sanitized reason and no package replacement
- **And** another household or a Tasks-only credential cannot target a device,
  change its channel, publish artifacts, or access private deployment records.

### Scenario: Control updates outside Play

- **Given** Control N installed by a parent from the documented HTTPS distribution
  page and a trusted N+1 release
- **When** Control checks for updates and Android permits unattended self-replacement
- **Then** it installs N+1 and reconciles the result without losing parent session
  state beyond the identity provider's normal reauthentication rules.
- **When** the OS, permissions or Samsung settings require confirmation/block install
- **Then** Control shows the reason and a user-initiated system install/permission
  action, handles cancel/resume, and never reports success before replacement.

### Scenario: Approved app delivery is constrained

- **Given** a parent-approved package/signing identity and a lawfully distributable
  trusted APK for that package (including Primer TV)
- **When** Control requests installation/update on the managed A16
- **Then** Student installs only that authorized artifact and reports the actual
  installed version, while launch availability follows the approved-app policy
- **And** arbitrary student-supplied APKs/URLs or a package outside policy cannot install.

### Scenario: TV compatibility and forward recovery

- **Given** the existing TV release API and a managed TV box or Student-owned A16
- **When** TV uses the shared verifier/installer or Student updates allowlisted TV
- **Then** package identity, TV credentials, playback state and owner role stay correct
- **And** a paused rollout does not downgrade devices; a corrected higher-version
  release can replace a faulty build through the documented recovery process.

## Implementation Instructions

1. Extend the Phase 4 domain with proposed release/artifact, rollout-target and
   install-attempt tables. Identity includes package, channel, versionCode (wide
   integer), versionName, signing identity, minSdk/ABI, byte length, SHA-256 and
   immutable artifact reference. Per-device target mutations are audited/CAS and
   idempotent. Release publishing authority is separate from household parent
   authority; a parent selects approved releases but cannot sign/publish arbitrary
   replacements through Control.
2. Add a documented operator publication boundary (new `tasks-release` CLI and/or
   narrowly authorized API) sharing real validation/persistence with the service.
   Verify APK metadata, `apksigner` result and build provenance before atomically
   marking an immutable artifact available. Storage can initially be a persistent
   filesystem volume, not a new object service; retain content-addressed artifacts,
   atomically publish and back up metadata/bytes together. Never overwrite an APK
   behind the same version URL. Explicitly configure storage; no temporary-dir fleet.
3. Produce signed release metadata using a separately controlled release trust key
   pinned in clients, covering the package/version/digest/size/compatibility fields.
   APK publisher identity and Android signing verification remain required; metadata
   does not override them. Provisioning QR must refer to the intended immutable APK
   and Android's required checksum encoding, not a mutable latest file.
4. Harden `core-updates` from the Phase 1 mechanism/TV donor: require digest and
   positive bounded length, bound bytes during streaming (including unknown content
   length), reject wrong package/version and signer set, verify installed identity,
   minSdk/ABI, use private staging and atomic rename, clean partials/sessions safely.
   Keep signing identities stable for v1; do not treat any intersecting historical
   certificate as sufficient. Future rotation requires explicit Android signing
   lineage support and separate compatibility acceptance. Never skip OS verification.
5. Restrict download origin/redirects and avoid forwarding bearer credentials to
   artifact hosts outside the intended trust boundary. Use the owned API/binary
   façade, HTTPS and scoped artifact grants where needed. Public Primer APK bytes
   may be anonymously distributable; device/household rollout metadata is not.
   No secrets in query strings, published manifests or install callback payloads.
6. Persist desired version, attempt/session IDs and progress. Reconcile committed
   PackageInstaller sessions with actual package state after process death,
   `MY_PACKAGE_REPLACED`, boot/unlock and resume. Treat callback success as provisional
   until version readback; use explicit protected callback components and correct
   PendingIntent mutability for supported APIs. Preserve sensitive app storage.
7. Device-owner Student uses silent PackageInstaller for itself and policy-approved
   artifacts. Do not fall back to an unrestricted student-accessible installer if
   Samsung rejects it: report blocked/needs-parent-maintenance. Foreground checks
   and bounded periodic WorkManager catch-up deliver durable targets; record actual
   latency. WorkManager is inexact (periodic minimum is normally 15 minutes), so no
   exact asleep/offline delivery SLA. Optional wake-up transport is a later adapter,
   never the authority or required for eventual catch-up.
8. Control shares release verification/download/reconciliation, not DPC privileges.
   Declare appropriate install permissions and, on supported Android, request
   `USER_ACTION_NOT_REQUIRED` with `UPDATE_PACKAGES_WITHOUT_USER_ACTION` and meet
   the platform's advancing target-SDK requirements. Check unknown-source install
   authorization through the system. Always implement `STATUS_PENDING_USER_ACTION`:
   foreground user action or notification/deferred action when background launch
   is forbidden, cancel/retry, blocked installer and actual version confirmation.
   Qualify parent's OS when known; absent hardware means no unattended claim there.
9. Add per-app CI release signing/version inputs, monotonic version validation,
   APK signature checks, source/contract/toolchain receipts and fail-closed unsigned
   release publication. Keep keys/credentials out of PR builds/logs; back up signing
   keys securely. Do not reuse development keys on managed production devices.
10. Migrate TV's `AppUpdater` onto `core-updates` with a TV release-source adapter.
    Preserve existing server routes/token auth and box owner behavior. Old server
    metadata missing mandatory trust fields must produce a clear upgrade-required
    diagnostic, not silent weakened verification; ship additive metadata support
    server-first. No management credential is sent to the TV server. On A16, Student
    handles silent TV replacement; unmanaged TV retains interactive behavior.
11. Add Control release/status pages, bootstrap HTTPS download instructions and
    the operational pause/pilot/forward-fix procedure. Pause prevents future attempts
    where possible, not an already committed OS transaction. A higher version with
    compatible local migrations is the rollback strategy; Android downgrades are not
    assumed. App updates are separate from Samsung OTA OS/security updates.

## End-to-End Test Plan

- **Setup:** real service/PostgreSQL/persistent artifact store, real publication
  boundary, signed N/N+1/N+2 release APKs and deliberately invalid alternatives,
  managed A16, parent Control handset/emulator, real TV box/backend for compatibility.
- **Main action:** publish through operator CLI/API, select rollout in Control,
  wait for actual device reconciliation, watch silent replacement, reopen Tasks
  and approved apps and verify preserved identity/state. Compare PackageManager
  version/signer with server report and Control UI; record timestamps at each stage.
- **Failure actions:** offline target, restart server, interrupt stream and process,
  reboot during installation, low storage, revoked management token, stale target,
  wrong digest/key/package/ABI/minSdk, oversized response and cross-host redirect.
  Assert no replacement for invalid artifacts, bounded retries and actionable state.
- **Control action:** first sideload via documented user flow, grant system install
  permission where appropriate, test conditional unattended update on eligible OS,
  then genuine permission/consent/cancel/block cases. Do not use a device-owner
  parent emulator or ADB-granted privilege to prove ordinary-app self-update.
- **App/TV action:** publish approved TV APK, install via Student policy, verify
  playback/pairing; run TV's independent updater on the dedicated box. Test forward
  fix N+2, pilot-only targeting and paused rollout without claiming instant reversal.
- **Commands:** root `make tasks-clients tasks-test tasks-cover tasks-android`, TV
  server tests (`make tv-test`) when its release API changes, shared Android tests/
  lint/builds plus new documented release/connected commands. ADB may observe or
  recover under operator supervision; `adb install -r` is not update acceptance.

## Anti-Cheating Audit

- Inspect publisher role, immutable storage and signed metadata: no mutable latest
  artifact, optional checksums, client-supplied trusted URLs or parent-as-publisher.
- Inspect session persistence/readback; no in-memory rollout truth, success after
  download alone, manually injected callbacks or reports masquerading as OS replacement.
- Inspect signing checks (especially TV's historical-certificate intersection),
  streaming byte limits, redirects, version monotonicity and signer policy.
- Inspect API tenant/credential scope on targets/reports/artifact grants; no client-
  only authorization or Tasks bearer controlling installation.
- No fake APK byte tests as install proof, mocked first-party release service,
  ADB sideload as silent update evidence, test-only DPC privileges in Control,
  swallowed installer errors or unconditional broad retries.
- Consent UI must be real system UI; do not auto-click it and label that unattended.
  Missing firmware/parent-OS cases remain unsupported, not skipped green cases.
- Notifications/events follow durable targets; push delivery and WorkManager enqueue
  alone are not applied evidence. Check secret exposure and premature cleanup.

## Completion Gate

- [ ] Every BDD case passes with real signed artifacts, service persistence and OS installs.
- [ ] A16 Student N -> N+1 is silent, retains ownership/pairing/policy and reports actual version.
- [ ] Approved-package delivery is scoped; invalid artifacts and foreign actors are denied.
- [ ] Control unattended path is proven on declared supported OS; genuine consent/
  permission/block/cancel fallback works elsewhere with no universal-silent claim.
- [ ] TV compatibility, durable offline/restart retries and higher-version recovery pass.
- [ ] Signing custody, immutable publication, generated clients, lint/build/test/coverage
  and the anti-cheating audit pass; release receipts identify exact artifacts.
