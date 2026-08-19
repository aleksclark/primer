# Phase 1: Standalone identity, pairing, and client shells

## Goal

Create the independently runnable Primer Tasks foundation and prove the first
cross-platform identity slice. A parent signs into the browser through a real
BFF flow, creates a tenant-scoped student, issues a one-use pairing QR, and a
student browser plus a fresh Android emulator bind themselves to that student.
The phase also establishes the Stacklane Compose lifecycle, handler-derived
client generation, System C shells, and two-tenant authorization pattern that
all later phases must preserve.

This phase deliberately ends with an empty student checklist. Task semantics
start in phase 2; login, tenant, pairing, and both client shells are real here.

## BDD Success Criteria

#### Scenario: Parent signs in and manages a student

- **Given** the dev Compose stack with a protocol-compatible test Identity issuer
  and a pre-provisioned tenant membership
- **When** the parent completes the browser authorization-code/PKCE login and
  creates a student through the parent SPA
- **Then** the browser holds only a host-only, HttpOnly, SameSite BFF cookie
- **And** the student appears by display name in a bounded server-owned list
- **And** refresh/navigation preserves the parent session and student row.

#### Scenario: Tenant boundaries deny IDOR

- **Given** parent A belongs to tenant A and a student belongs to tenant B
- **When** parent A guesses tenant B's student, pairing, device, or list filter
  identifiers through REST or BFF routes
- **Then** no tenant B data is returned or mutated
- **And** the denial is recorded without logging credentials.

#### Scenario: Android pairs by scanning a QR

- **Given** a parent has selected a student and requested a short-lived pairing
  code
- **When** a fresh Android emulator scans the web-rendered QR
- **Then** the app exchanges the code exactly once, stores the returned token in
  a Keystore-encrypted local store, and displays the bound student's name and
  empty checklist
- **And** neither the token nor a reusable credential was present in the QR.

#### Scenario: Student browser pairs without exposing a bearer token

- **Given** an unpaired browser on the student route and a valid one-use code
- **When** the student enters/scans it
- **Then** the BFF establishes a host-only student session bound to that student
- **And** browser JavaScript cannot read an Android-style device bearer token
- **And** the student shell displays only that student's identity and empty list.

#### Scenario: Pairing is single-use, expiring, and revocable

- **Given** a used, expired, or revoked pairing code/device
- **When** another client tries to redeem the code or an old client calls the
  profile endpoint
- **Then** the server returns the documented 401/403 error and creates no second
  device
- **And** revocation takes effect on the next request while the original pairing
  and revocation remain auditable.

#### Scenario: Parent updates and archives a student safely

- **Given** a student in the parent's tenant
- **When** the parent edits the display name and later chooses delete/archive
- **Then** the update is durable and archive semantics are explicit
- **And** active paired credentials are revoked on archive
- **And** a foreign tenant cannot perform either action.

#### Scenario: Development stack is isolated and reloadable

- **Given** two worktrees with distinct `STACKLANE_INSTANCE` values
- **When** each runs the documented lifecycle
- **Then** Compose project names, loopback ephemeral ports, Stacklane labels,
  FQDNs, databases, and volumes are distinct
- **And** a Go source change reloads the server and a React source change HMRs
  the DOM without a full browser reload
- **And** stopping one stack does not affect the other.

#### Scenario: Production rejects test identity

- **Given** `TASKS_ENV=production`
- **When** test issuer/test-login configuration is supplied or required auth
  secrets are missing
- **Then** `tasks-server` fails before migrate/listen
- **And** no allow-all parent is installed.

## Implementation Instructions

1. Scaffold `primer-tasks/` as its own Go module and add it to `go.work` without
   importing `server/internal/*`. Use Huma v2, chi, pgx v5, goose, envconfig,
   slog, and testcontainers following repository patterns while preserving a
   separate module and DB.
2. Add `cmd/tasks-server`, `cmd/tasks-migrate`, and offline
   `cmd/openapi-gen`. Configuration uses only `TASKS_*`; require a Tasks-safe
   database name and reject LMS/TV/Studio/Identity DB names. Use a distinct
   goose table.
3. Add migrations/repositories for tenants, parent memberships, BFF sessions,
   students, pairing codes, student devices/sessions, and audit records.
   - Store only hashes of pairing codes, device tokens, and session handles.
   - Use composite tenant ownership constraints where rows reference each other.
   - Archive students rather than deleting referenced history; revoke devices in
     the same transaction.
4. Implement a product BFF authorization-code + S256 PKCE client matching Primer
   Identity's public contract. Compose may run a narrow test issuer that emits
   protocol-compatible signed tokens and fixed test principals; production
   binaries must reject that issuer/mode. Browser JS never receives provider,
   access, or refresh tokens.
5. Implement explicit parent guards and `Scope{TenantID, SubjectRef}` repository
   inputs. Avoid generic unscoped CRUD. Initial memberships accept only role
   `admin`.
6. Implement one-use pairing claim transactionally. QR payload contains only
   product origin/API origin, display-safe pairing ID/code material, expiry, and
   a version; it must not contain the eventual device token. Android receives a
   bearer once; student web receives an HttpOnly BFF session instead.
7. Scaffold `primer-tasks/web` with parent and student route groups, consuming
   generated System C CSS and the repository's exact fonts/icons. Classify the
   parent student list as **Explore/Configure** and the student empty checklist
   as **Operate**. Include intentional loading, empty, error, denied, revoked,
   and expired states, dark primary/light parity, keyboard focus, and mobile
   reflow.
8. Scaffold `primer-tasks/android` as a dedicated Kotlin/Compose app with a new
   application ID. Consume generated `PrimerTokens.kt`; use CameraX plus a
   bundled QR decoder so pairing does not depend on Play Services. Store the
   token encrypted with an Android Keystore key and DataStore metadata; disable
   token backup/export. The app reports no user-selectable student switch.
9. Emit OpenAPI from production Huma registration offline. Generate distinct
   TypeScript and Kotlin client packages into ignored build roots, with committed
   façades for base URL, cookie/device auth, typed errors, and cancellation. Add
   source scans banning copied DTOs and ad-hoc REST calls in web/app code.
10. Add `compose.yaml` and one `scripts/dev` lifecycle vector. Public services
    (`web`, `api`, test issuer if browser-addressable) use Stacklane labels and
    loopback ephemeral publishes. Containers communicate by Compose DNS; Vite
    proxies `/api`, `/auth`, and later `/ws` to `api`. Add healthchecks, named
    Go/npm/Gradle/Postgres caches, worktree source binds, and pinned Go/Vite
    watchers. `down` preserves volumes; `destroy` requires exact confirmation.
11. Add `check`, `up`, `status`, `endpoints`, `logs`, `down`, and guarded
    `destroy`; `check` renders Compose to a private temp file and fails closed on
    missing labels, fixed/wildcard ports, host networking, absent healthchecks,
    missing source binds/named state volumes, or `.local` domains.
12. Add product-local build/test/coverage/client/web/android/E2E commands and a
    root forwarding surface only where needed. Do not lower existing gates or
    commit generated clients/contracts.

## End-to-End Test Plan

### Browser exploratory acceptance

- Start the real Stacklane stack with PostgreSQL and test issuer.
- Log in as parent A, create/update a student, render a pairing QR, and verify
  persisted state after server/browser refresh.
- In a separate context, pair the student web route and assert the empty student
  shell, HttpOnly cookie properties, denied parent routes, and revocation.
- Log in as tenant B and run cross-tenant URL/body/filter attempts.
- Capture dark desktop, dark mobile, and light parity screenshots plus axe output.

### Android emulator acceptance

- Boot a clean emulator, install the real APK, scan the QR displayed by the real
  web page, and verify the bound profile survives app process death and emulator
  reboot.
- Attempt QR replay from a second fresh emulator and verify denial.
- Revoke from parent web and verify the first emulator is rejected on its next
  request and returns to a clear re-pair state.
- Inspect app-private storage/logcat/backups to ensure no plaintext token or QR
  token leakage.

### Promoted automation

- After exploratory PASS, add Playwright login → student CRUD → QR issuance →
  student-browser pair/revoke specs against the real stack, including two-tenant
  negatives, mobile viewport, axe, and cookie assertions.
- Add emulator-backed Compose/UI tests for QR decode fixture, real pair/profile,
  persistence after process restart, replay denial, and revocation. Use a virtual
  scene/image injection for QR camera automation rather than bypassing the scan
  activity.
- Add process E2E that migrates a fresh Postgres, starts the real binary, and uses
  freshly generated TS/Kotlin clients for successful and typed-error calls.
- Run the Stacklane hot-reload and two-instance proof with exact source restore.

Planned commands (implemented in this phase):

```bash
primer-tasks/scripts/dev check
primer-tasks/scripts/dev up
make tasks-test tasks-cover tasks-clients tasks-web tasks-android
make tasks-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Trace browser login through a real code/PKCE exchange and BFF session store;
  reject localStorage JWTs, hard-coded user objects, and production test-login.
- Verify every student/device query includes server-derived tenant scope and DB
  ownership constraints; reject client filtering or request-supplied tenant
  authority.
- Inspect pairing SQL for atomic claim and unique hash behavior; reject plaintext
  codes/tokens, reusable QR credentials, or in-memory revocation.
- Inspect Android storage and backup rules; reject plain preferences/DataStore
  bearer values or student switching by edited local metadata.
- Confirm OpenAPI and clients are generated from production registration, are
  untracked, and are actually imported by both consumers.
- Inspect rendered Compose JSON, not only YAML. Reject fixed ports, ambient
  project names, fake health, shared worktree volumes, or `down -v` defaults.
- Prove HMR without reload and Go rebuild without container restart; reject mere
  process restart as hot-reload evidence.
- Browser/Android E2E must assert persisted rows/revocation through public
  behavior, not only screenshots, status 200, mocks, or repository calls.

## Completion Gate

- [ ] All BDD scenarios pass for two tenants and fresh clients.
- [ ] Dedicated browser and Android exploratory agents report PASS.
- [ ] Playwright and emulator suites are promoted only after that PASS and are green.
- [ ] Offline contract emission and clean client generation/build are deterministic.
- [ ] No generated contract/client source is tracked.
- [ ] System C dark/mobile/light and axe evidence is reviewed.
- [ ] Stacklane check, hot reload, and two-instance isolation proofs pass.
- [ ] Go tests/race/vet/build/coverage, web lint/typecheck/build, Android unit/build/connected tests, and `git diff --check` pass.
- [ ] The anti-cheating audit finds no auth, tenancy, pairing, client, or Compose substitution.
