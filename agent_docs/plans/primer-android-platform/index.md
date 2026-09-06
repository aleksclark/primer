# Primer Android platform — Student, Control, and TV

## Outcome

Deliver Primer Student and Primer Control as native Android apps, sharing a
Compose component library with Primer TV. Tasks is the first complete product
slice. Student is the device owner and approved-app launcher on a Samsung Galaxy
A16 5G; parents manage it from Control. All three apps can be distributed outside
Google Play. Student updates silently on the certified handset; Control attempts
supported unattended self-updates and otherwise requests system confirmation.

**Status: Phase 1 implementation started; supervised device-owner qualification
partially verified, not accepted as complete.** See the
[physical-device evidence](../../../test-artifacts/android-student/device-owner-qualification.md)
and [A16 runbook](../../runbooks/android-a16-provisioning.md). Student now has a
signed DPC/launcher build, approved-app policy, bounded one-use recovery, and
verified local-APK installation. Silent updates remain blocked by Play Protect
review in the tested configuration; reset-time QR and full hardening are untested.

The planning-base inventory below was performed at `34a4f5c2` before implementation.
It records historical starting points, not the status of the new qualification code.

Subsequent [read-only USB preflight](../../../test-artifacts/android-student/device-preflight.md)
identified the actual handset as **SM-S166V**, Android 16 / API 36, One UI 8,
security patch `2026-02-05`, with locked verified boot, completed setup and no
owners or installed Primer packages. Student's build target was missing at that
preflight. Subsequent authorized enrollment/build testing is recorded separately
in the linked qualification evidence; the phone was not factory-reset.

## Confirmed requirements and decisions

- Student hardware: **Samsung Galaxy A16 5G**, connected model **SM-S166V**
  (`a16xtfn` / `a16x`), not an assumed SM-A166 variant. Reconfirm exact Android/One UI
  build, security patch, and carrier firmware at Phase 1 acceptance.
- Factory reset is acceptable. Student permits other parent-approved apps.
- Distribution stays **outside Google Play**. No Play publishing, Managed Google
  Play, Android Management API, paid EMM, or Knox license is required by the design.
  OS provisioning may still use installed Google/Samsung system components;
  Play-free distribution is not a promise of Google-free firmware.
- Use standard Android Enterprise device-owner APIs first. Knox-specific features
  are a follow-up only if the real A16 gate reveals an essential unsupported case.
- One device owner per device: Student owns the A16; TV is an ordinary allowlisted
  app there. TV may remain the owner on a dedicated TV box.
- Proposed stable identities: `com.aleksclark.primer.student` and
  `com.aleksclark.primer.control`. Preserve `com.aleksclark.primer.tv`. The old
  `com.aleksclark.primertasks` prototype requires explicit revoke/re-pair, not an
  impossible cross-package private-storage migration. Freeze Student's receiver
  component and release signing identity before managed deployment.
- Host the first device-management backend as a **separate bounded context in the
  existing Tasks deployable/database**, not a new microservice. It shares current
  household authorization, but not Tasks device credentials or lifecycle. This
  is a deliberate first-release hosting choice, not a claim that tasks own devices.

## Planning-base inventory (historical)

| Area | Present / partial / missing | Evidence |
|---|---|---|
| TV application | Present: tablet/TV Compose shells, pairing, playback, tests | [`android/`](../../../android/README.md) |
| Student starting point | Present: native QR pairing, encrypted bearer, Today/Upcoming/detail/start/submit | [`MainActivity.kt`](../../../primer-tasks/android/app/src/main/java/com/aleksclark/primertasks/MainActivity.kt), [`connected acceptance`](../../../primer-tasks/android/CONNECTED_ACCEPTANCE.md) |
| Control | Missing: no parent Android app; parent web/manual-approval API exists | [`Tasks web`](../../../primer-tasks/web/src/App.tsx), [`phase2_openapi.go`](../../../primer-tasks/internal/api/phase2_openapi.go) |
| Parent identity | Present: Clerk session JWT verification plus local household membership and local session revocation; native login not proven | [`clerk.go`](../../../primer-tasks/internal/api/clerk.go), [`release auth notes`](../tasks-early-release-clerk.md) |
| Visual system | Present tokens/specification; TV theme/components are app-local; Tasks still uses a hard-coded Material accent; no shared Compose library | [`design-system`](../../../design-system/README.md), [`TV theme`](../../../android/app/src/main/kotlin/com/aleksclark/primer/tv/app/ui/designsystem/Theme.kt) |
| Device owner | Partial: TV-only kiosk/home/restrictions implementation; no A16 Student provisioning/recovery | [`KioskPolicy.kt`](../../../android/app/src/main/kotlin/com/aleksclark/primer/tv/app/admin/KioskPolicy.kt) |
| Update installer | Partial: TV download/archive checking and PackageInstaller, interactive fallback; blank digest/size permitted, lifecycle state largely in memory | [`AppUpdater.kt`](../../../android/app/src/main/kotlin/com/aleksclark/primer/tv/app/update/AppUpdater.kt) |
| Release hosting | Partial: TV-only mutable APK/version endpoint, not a durable multi-app rollout ledger | [`release.go`](../../../server/internal/tv/api/release.go) |
| Kotlin client | Partial: generated student models/routes plus façade; parent operations absent; app imports client source through a source-set path | [`TasksClient.kt`](../../../primer-tasks/clients/kotlin/src/main/kotlin/com/aleksclark/primertasks/client/TasksClient.kt), [`generator`](../../../primer-tasks/clients/kotlin/generate-client.mjs) |
| Device management / remote delivery | Missing: management enrollment, policy revisions, acknowledgements, release targeting, remote update status | No equivalent domain in the inspected Tasks API |

Older Tasks plans describe pairing/checklists as donor-only. That is historical,
not the state of this checkout. Do not claim later dialogue/media/external-verifier
server phases are present: the inspected production registry currently registers
foundation and Phase 2. The [Android continuation plan](../primer-tasks-android/index.md)
remains the follow-up for those features, subject to its server prerequisites.

## Scope boundaries

### In scope

- Shared System C Compose library consumed by all three real applications.
- Native Tasks parent/student manual-approval loop, schedules, retries, history,
  restart recovery, and server-enforced household/student isolation.
- Fully managed A16 provisioning, multi-app launcher, local restrictions,
  parent maintenance/recovery, durable remote allowlist and status.
- Signed APK release publication, silent Student update, managed installation of
  explicitly approved distributable APKs, and unattended-when-supported Control
  self-update with an honest user-confirmation fallback.
- TV component reuse and shared installer adoption without changing its identity,
  playback rules, credentials, or dedicated-box ownership behavior.
- Real-device security/escape testing, repeatable builds, diagnostics and runbooks.

### Out of scope

- Full LMS/tutor/assessment UI, Tasks AI dialogue/media/external verification,
  enterprise role delegation, general-purpose MDM, surveillance, remote shell.
- Play/Knox enrollment products, arbitrary APK scraping, redistribution without
  permission, or intercepting third-party apps' private data.
- URL/content filtering inside approved browsers/apps. Approving a browser is not
  equivalent to approving individual sites; broader network controls need their
  own design. Use a conservative initial app list.
- Root/custom firmware, bootloader protection supplied by an app, guaranteed
  resistance to physical recovery flashing/reset, or universal silent Control
  updates on untested parent hardware.
- Automated remote wipe or non-wiping device-owner transfer in the first release.

## Architecture and global constraints

### Android modules (proposed additions)

```text
android/                         # one Gradle build / version catalog
  app/                           # existing TV target; avoid gratuitous rename
  core/                          # existing pure Kotlin TV domain
  app-student/                   # launcher + stable DPC receiver + app wiring
  app-control/                   # parent identity + app wiring
  core-ui/                       # generated tokens, theme, accessible primitives
  core-device-policy/            # mechanism, policy/readback, recovery
  core-updates/                  # verification/install/reconciliation mechanism
  feature-tasks-student/          # moved existing student flows
  feature-tasks-control/          # native parent flows
  feature-device-control/        # enrollment/policy/release status UI (Phase 4)
primer-tasks/clients/kotlin/      # actual Gradle module, generated internals + façades
```

Modules are introduced when used, not as empty abstractions. UI never imports TV
API/domain types or device-policy capabilities. TV density/focus and product
badges stay in TV adapters; mobile and TV share visual grammar, not navigation.
Control must not declare a device-admin receiver or manage the parent's phone.

### Server and authority

Add a proposed `primer-tasks/internal/devicemanagement/` domain, thin typed Huma
registration in `internal/api/`, and additive migrations in `internal/db/migrations/`.
Use management-prefixed tables, separate management enrollment/credentials,
revisioned policy and release intent, and append-only audit. Parent membership
continues to come from the existing local household ledger, never Clerk roles or
client-selected tenant IDs. Existing `/device/*` routes retain Tasks-only meaning.

Control -> authenticated server -> durable desired state -> Student reconciliation
-> OS readback/install result -> durable report -> Control. No direct Control-to-
Student socket is required. Parent logout, Tasks revoke, management quarantine,
and physical decommission are different operations. Revoking Tasks must not
remove device ownership or delete recovery material.

### Non-negotiable implementation rules

1. **Physical-device gate first.** Samsung Auto Blocker / Maximum Restrictions,
   battery management, telephony, and Setup Wizard can differ by firmware. Record
   blockers, not a green status inferred from emulator behavior.
2. **Recoverable enforcement.** Retain last-known policy offline. A task/API outage
   neither unlocks the phone nor bricks basic operation. Recovery is per-device,
   parent-held, rate-limited, one-use, audited, and bounded; no universal PIN,
   hidden tap bypass, unrestricted exported activity, or indefinite maintenance.
3. **Threat boundary.** Certified stock firmware and locked bootloader; no claim
   to defeat a rooted OS or every physical recovery path. Preserve keyguard,
   emergency access, approved accessibility and parent-approved communication.
4. **Credential separation.** Parent JWT, Tasks bearer, management bearer and
   release-publisher authority are separate. Store sensitive native state with
   Keystore protection; exclude it from backups, URIs, logs and screenshots.
   Enrollment codes are short-lived, single-use, and never long-lived credentials.
5. **Generated API clients.** Huma executable boundary types -> offline OpenAPI ->
   ignored generated internals -> committed façades -> apps. Fix untyped operation
   results used by native code; no copied DTOs, ad-hoc endpoint URLs or optimistic
   "completed" facts. Preserve deployed mount prefixes such as `/tasks`.
6. **Release trust.** Per-package signing custody; mandatory digest/length and
   archive identity checks; immutable artifacts. No arbitrary download URLs from
   a push message, no student-supplied APK install command, no shared admin key.
7. **Delivery semantics.** Durable intent and at-least-once reconciliation, not
   "push sent = applied". Online foreground checks plus WorkManager catch-up;
   Android schedules background work inexactly. An optional later wake-up channel
   cannot carry authority. Offline, asleep or OEM-throttled devices remain pending
   with last-seen status; no guaranteed real-time delivery promise.
8. **System C.** Generated tokens from `design-system/`, dark primary/light parity,
   square/ruled structure, visible focus, real font assets and scalable text.
9. **Testing.** Default host Make path, real Tasks/PostgreSQL and public APIs;
   emulator exploratory PASS before connected promotion. A16 tests additionally
   prove OS effects. Test JWT issuers prove local boundaries, not live Clerk login.
   No mocked first-party backend or DPM replaces end-to-end acceptance.
10. **Compatibility.** Preserve TV package/receiver identity, Tasks web API and old
    clients. Make targets and CI must regenerate from clean checkout. Keep APK N
    usable against the service while APK N+1 rolls out. No app data deletion as an
    upgrade test. Retire old native build only after replacement acceptance.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: A16 ownership and recovery](./phase-01-a16-ownership.md) | Prove real provisioning, recoverable multi-app kiosk and silent same-package replacement before committing to hardware assumptions. | None |
| [Phase 2: Shared Android UI and app foundation](./phase-02-shared-platform.md) | Three build targets consume one component library; migrate existing Student behavior without TV regression. | Phase 1 |
| [Phase 3: Native Tasks vertical slice](./phase-03-native-tasks.md) | Control creates/assigns/reviews; Student starts/submits; both show the same durable decision. | Phase 2 |
| [Phase 4: Remote device management](./phase-04-device-management.md) | Parent-bound management enrollment, durable approved-app policy, recovery and truthful applied status. | Phase 3 |
| [Phase 5: Play-free release delivery](./phase-05-release-delivery.md) | Immutable signed releases, pushed silent Student updates, conditional unattended Control updates, shared TV installer. | Phase 4 |
| [Phase 6: Fleet acceptance and operations](./phase-06-acceptance-operations.md) | Certify exact firmware/builds through daily use, escape attempts, offline/restart/update/recovery and repeatable rollout. | Phase 5 |

Phase 1 is a hardware/platform qualification slice, not a substitute for the
Phase 5 update service. Phase 3 is the first complete educational product slice.
Do not distribute Phase 1–4 builds as fully certified managed devices.

## Requirement traceability

| Requested capability | Owning acceptance |
|---|---|
| Primer Student and Primer Control | Phase 2 installable identities; Phase 3 native parent/student loop |
| Shared look/feel across all three apps | Phase 2 shared components/theme and real TV/mobile visual/accessibility tests |
| Tasks first vertical slice | Phase 3 create/schedule/submit/decision/retry/restart scenarios |
| Student device owner, A16, approved other apps | Phase 1 provisioning/kiosk/recovery; Phase 4 remote allowlist; Phase 6 escape matrix |
| Push Student updates | Phase 5 durable target -> real silent OS replacement -> reported installed version |
| Both apps self-update outside Play | Phase 5 Student silent; Control supported unattended path plus consent fallback |
| Safe repeated operation | Phase 6 physical recovery, soak, firmware and release receipts |

## External platform references

Reviewed to establish platform constraints, not evidence of this handset's behavior:

- [Android dedicated devices](https://developer.android.com/work/dpc/dedicated-devices)
- [Build a DPC](https://developer.android.com/work/dpc/build-dpc)
- [Multi-app lock task](https://developer.android.com/work/dpc/dedicated-devices/lock-task-mode)
- [PackageInstaller Session.commit](https://developer.android.com/reference/android/content/pm/PackageInstaller.Session#commit(android.content.IntentSender))
- [SessionParams.setRequireUserAction](https://developer.android.com/reference/android/content/pm/PackageInstaller.SessionParams#setRequireUserAction(int))
- [Samsung Auto Blocker](https://www.samsung.com/uk/support/mobile-devices/protect-your-galaxy-device-with-the-new-auto-blocker-feature/)

Android documents device-owner installs without user intervention. Ordinary
self-updaters have additional permission, target-SDK and OS requirements, and
must always handle `STATUS_PENDING_USER_ACTION`. Samsung documents sideload/update
and device-admin restrictions; any required setting change belongs in explicit
parent provisioning, never a covert bypass. Parent handset model/OS remains a
Control update-matrix input, not a blocker to implementing the consent fallback.

## Completion rule

All six phase gates must pass against exact commits, signed APKs and firmware,
with public-boundary evidence, physical A16 proof, clean generation/builds,
server isolation/replay tests, recovery rehearsal, and phase-specific anti-cheat
reviews. Document unsupported OEM behavior and Control consent requirements.
No missing hardware/auth dependency is counted as a pass. This plan alone claims
none of those implementation gates are complete.
