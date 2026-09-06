# Phase 4: Remote device management

## Goal

Make Control the real parent console for the Student-owned A16: bind management
separately from Tasks, change approved apps remotely, inspect actual enforcement,
and recover safely. Use a bounded management domain in the current Tasks service
so the first household does not need another service or identity migration.

## BDD Success Criteria

### Scenario: Parent-bound enrollment is not Tasks pairing

- **Given** an authenticated household parent and a Student-owned A16
- **When** the parent issues a management enrollment QR in Control and the handset
  consumes it through its parent-only enrollment surface
- **Then** exactly one management device identity is bound to that household
- **And** the single-use code expires/consumes atomically, another household cannot
  claim the device, and management credentials cannot authenticate Tasks routes.
- **When** Tasks is subsequently paired or revoked
- **Then** management identity, owner policy and recovery remain intact.

### Scenario: Parent changes approved apps

- **Given** an enrolled device with a last-applied policy revision
- **When** the parent approves an installed app or removes approval in Control
- **Then** a new desired revision is persisted and eventually reconciled on Student
- **And** approved apps launch, removed apps are no longer usable through the
  launcher or tested escape routes, and Home still returns to Student
- **And** Control distinguishes requested, applied, partial/failed and stale status.

### Scenario: Offline and out-of-order delivery are safe

- **Given** the A16 is offline and several policy changes are saved
- **When** Control/server restart and the A16 later reconnects
- **Then** the latest authorized revision is reconciled, stale responses/duplicate
  reports do not overwrite newer status, and local protection remained active offline
- **And** revoked management credentials cannot fetch new policy or report success.

### Scenario: Unsafe policy changes cannot remove the control path

- **Given** a parent submits a policy excluding Student, a required system component,
  a mismatched app signer, or a capability unsupported by this firmware
- **When** the policy is validated and reconciled
- **Then** unsafe changes are rejected or explicitly reported unsupported before
  destroying the known-good launcher/recovery path
- **And** a partial OS failure is visible with individual operation/readback results,
  not represented as an atomically applied policy.

### Scenario: Recovery, quarantine and decommission are different

- **Given** a managed A16 with per-device parent-held recovery material
- **When** an authorized parent grants bounded maintenance or rotates recovery codes
- **Then** the correct device alone receives the change and entry/exit is audited
- **And** offline one-use recovery still works; rotation is shown pending until applied.
- **When** management credentials are quarantined/revoked
- **Then** last-known restrictions remain and a local parent recovery route survives;
  only explicit physical decommission removes management, not logout or Tasks archive.

## Implementation Instructions

1. Add proposed `primer-tasks/internal/devicemanagement/` for management domain,
   repository and policy validation. Add typed route registration in `internal/api/`
   and additive management-prefixed tables under `internal/db/migrations/` for
   devices, hashed enrollment codes, hashed opaque credentials, policy revisions,
   device reports, recovery-operation metadata and audit. Persist through PostgreSQL;
   no process-local queue as source of truth. Keep optional Tasks student binding
   separate and non-cascading: Tasks archival cannot delete management enrollment.
2. Proposed public route groups are `/managed-devices/*` (parent administration)
   and `/management-device/*` (enroll, fetch desired state, report actual state).
   These are new APIs, not existing endpoints. Register them in production and
   offline Huma emission with explicit parent versus management-device auth.
   Audit `clerk.go` routing carefully: adding device routes must not accidentally
   remove guards from parent routes or trust an arbitrary bearer kind.
3. Reuse current local parent-household authorization, denying tenant/actor fields
   supplied by clients. Composite tenant/device foreign keys and scoped queries
   prevent cross-household policy/report/enrollment access. Do not embed parent
   tokens or TV admin keys in Student. Never reuse `/device/*` Tasks bearer tables.
4. Model enrollment as a parent-issued short-lived capability, atomic consumption
   and separate credential establishment stored with Android Keystore protection.
   Require an explicit parent setup/maintenance action to bind or replace household
   management; normal Student use cannot scan an attacker's household QR. Reject
   silent household rebinding. On lost exchange response, allow only a proven
   device-bound authenticated retry, or require fresh parent enrollment with clear
   cleanup of the abandoned identity; never replay a consumed code for a new device.
5. Policy v1 covers approved package IDs and signer identities, minimal required
   system exceptions, launcher/lock-task features, supported user restrictions and
   bounded maintenance configuration. Keep user app inventory minimal: package,
   label, signer/version and policy status; no personal-content collection. Use
   supported package visibility declarations and DPC APIs, not accessibility scraping.
6. Use per-device monotonic desired revision and server compare-and-swap for parent
   edits. Reports include policy revision, installed Student version, timestamps,
   per-control desired/readback results and sanitized errors. The server validates
   report ownership/revision; label this as device-reported, not cryptographic proof
   against a compromised OS. A stale acknowledgement cannot mark newer policy applied.
7. Reconcile using the Phase 1 mechanism on startup/resume, boot/unlock and bounded
   WorkManager catch-up. Persist desired/local applied state before/after OS actions.
   API calls may partially succeed: retain control/recovery and retry boundedly,
   reporting specifics rather than rolling back blindly or claiming atomicity.
   Guard against removing Student, network-management dependencies and recovery.
8. Extend `feature-device-control` with enrollment, device list/detail, last seen,
   desired/applied revision, approved-app editor, failure explanation and maintenance
   actions. Installed-but-unapproved, approved-but-missing and applied are distinct.
   Phase 5 adds delivery of missing distributable APKs; approval alone never claims
   installation. Pin approved signer sets; no general remote intent/shell execution.
9. Integrate Phase 1 recovery with authenticated parent issuance/rotation. Secret
   material is one-time disclosed and parent-held, never in ordinary reports/logs.
   Persist used-code/lease state and audit locally, then upload when reconnected.
   Explain that offline revocation/rotation takes effect only after sync; reboot
   closes maintenance. Device credentials remain usable for protected catch-up only
   while authorized; quarantine does not clear OS ownership.
10. Add documented host targets/migrations and generated Kotlin/TypeScript façades.
    Current hosting shares the Tasks database intentionally; future extraction
    would migrate this domain through public contracts, not teach TV about Tasks SQL.

## End-to-End Test Plan

- **Setup:** real host Tasks/PostgreSQL with management migrations, two parent
  households, Control and managed A16; two approved distributable apps already
  installed under parent supervision. One emulator may act as a foreign device.
- **Action/assertion:** Control issues enrollment; A16 enrolls; Control changes
  allowlist; device fetches and applies; launch from Student and attempt share,
  deep-link, settings, store, notification, browser/IME and same-task escapes.
  Compare Control reports with actual OS policy and on-screen behavior.
- **Server security:** public requests with foreign IDs, Tasks bearer on management,
  management bearer on Tasks/parent, consumed/expired enrollment, report revision
  forgery, tenant-field injection and concurrent parent edits. Assert no foreign
  rows/mutations and explicit denial/conflict, not only status codes.
- **Durability:** disconnect, change multiple revisions, restart service and app,
  reconnect and replay stale reports. Confirm last policy enforcement and latest
  revision convergence. Revoke Tasks and management independently. Kill during an
  actual policy application and reconcile without losing the launcher.
- **Recovery:** real one-use code while offline, wrong/replayed code, expiry/reboot,
  rotation pending/applied, parent maintenance repair and quarantine. No fixture
  success or injected policy acknowledgement.
- **Commands:** `make tasks-clients tasks-test tasks-cover tasks-android` plus the
  new documented management connected suite after exploratory PASS. Keep default
  `make tasks-host-up`; additive Compose is not the assumed runtime.

## Anti-Cheating Audit

- Inspect separate credential/table/auth guards and non-cascading Tasks association;
  no reuse of Tasks tokens, parent JWTs or TV admin keys as management authority.
- Inspect enrollment binding, API tenant scoping, CAS/revision validation and report
  persistence. No client-only ownership, in-memory command queue, hard-coded applied
  status or events without committed desired state.
- Inspect actual DPM readback, launcher/app lifecycle and same-task intent behavior;
  hiding icons is not denial. A device acknowledgement alone is insufficient proof.
- Inspect recovery consumption/lease/rate-limit persistence and audit retry. No
  permanent unlock, fixed password, secret log output or test-only management bypass.
- Reject mocked first-party API/DPM acceptance, internal setter tests posing as E2E,
  skipped OEM escape cases, swallowed partial failures and overly broad retries.
- Inventory and diagnostics must not drift into private-content collection. The
  capability matrix must distinguish requested, supported, applied and untested.

## Completion Gate

- [ ] All BDD cases pass through Control, Student, real API/DB and actual A16 OS state.
- [ ] Separate enrollment/credentials, household isolation and replay/CAS gates pass.
- [ ] Multi-app policy enforcement, offline catch-up and partial-failure reporting are proven.
- [ ] Maintenance/recovery/quarantine and independent Tasks revoke are rehearsed.
- [ ] Generated clients, migrations, affected lint/build/test/coverage gates pass.
- [ ] Phase-specific anti-cheating review passes; no silent-install claim yet beyond Phase 1 proof.
