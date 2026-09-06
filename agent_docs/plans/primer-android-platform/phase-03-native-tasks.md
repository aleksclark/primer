# Phase 3: Native Tasks vertical slice

## Goal

Deliver the first complete educational product loop entirely in the two native
apps: parent signs in, creates/assigns work and pairs a student; student works and
submits; parent decides; both observe the same durable outcome. Existing web
clients remain compatible and are useful independent observers, not substitutes
for missing Control screens.

## BDD Success Criteria

### Scenario: Native parent login preserves household authority

- **Given** a parent account with local Tasks household membership
- **When** the parent signs into Control through the real configured identity provider
- **Then** Control loads only that household and can issue student pairing QRs
- **And** an authenticated account without membership is denied, while expired,
  revoked or wrong-issuer sessions cannot read or mutate parent records.

### Scenario: Create, assign, submit, approve

- **Given** a parent signed into Control and an unpaired Student app
- **When** the parent creates a student, pairs the handset by QR, creates a task with
  instructions/manual verification, and assigns a one-off schedule
- **Then** Student displays the correct occurrence and instructions
- **When** the student starts/submits and the parent reviews/approves in Control
- **Then** both apps and the existing web client show the same completed occurrence
- **And** persisted decisions identify the authorized parent and include their reason.

### Scenario: Recurrence, rejection and retry are server-owned

- **Given** a recurring schedule in an explicit IANA timezone
- **When** the parent edits future scheduling, rejects a submitted occurrence and
  uses the supported retry action
- **Then** future occurrences follow server rules without changing old revision
  snapshots, and Student shows the returned state and available next action
- **And** replay/restart does not duplicate occurrences or decisions, and invalid
  transitions produce a visible conflict with a refreshed authoritative record.

### Scenario: Revocation and foreign identifiers remain isolated

- **Given** two real households/students and a paired Student credential
- **When** either app requests another household's occurrence/student/schedule or
  Student sends its bearer to a parent operation
- **Then** no foreign data or mutation is exposed
- **When** the parent revokes the pairing, archives the student or signs out of Control
- **Then** the appropriate server session is revoked, stale credentials fail, and
  clearing Tasks access does not remove device owner, kiosk policy or recovery.

### Scenario: Interrupted requests do not fabricate completion

- **Given** a paired student and pending work
- **When** a submit/decision response is lost, either app dies, or the API restarts
- **Then** reopening/retrying reconciles the persisted server state without creating
  a duplicate result or turning a transport failure into completion
- **And** offline/stale state is labeled, errors are actionable and retained cached
  content is removed on revocation without leaking into logs or backups.

## Implementation Instructions

1. Implement `feature-tasks-control` using `core-ui`: native roster/student detail,
   pairing QR/revoke, task create/edit/archive, one-off and supported recurring
   schedules, assigned-work/review detail and history, approve/reject/retry. Keep
   unsupported verification methods hidden or explicitly unavailable, not simulated.
   Retain search/pagination for collections rather than fetching an entire household.
2. Qualify the configured Clerk instance with its supported native Android flow/SDK.
   Use the provider's supported secure browser handoff when required, not a WebView
   password collector, copied browser cookies, or JWTs in deep-link query strings.
   Pin SDK and callback configuration; test the real native token's issuer, session
   ID, audience and authorized-party semantics against `internal/parentauth` and
   `internal/api/clerk.go`. Add only explicitly justified native validation policy;
   do not disable web audience/issuer/authorized-party checks to make login work.
   Provider setup/access is an explicit external gate. Do not resurrect the retired
   production `/auth/callback` flow as a shortcut.
3. Keep short-lived parent access tokens in memory under the identity adapter;
   protect any required persisted session material per provider guidance and
   Keystore/backup policy. Call server logout/revocation before provider signout,
   preserve the current fail-closed semantics, and report failure honestly.
4. Extend the Kotlin code generator and façade for real parent operations from
   `openapi.go` and `phase2_openapi.go`. Make it generate methods as well as paths,
   request/response/error types and pagination. Replace raw JSON/string operation
   results needed by apps with typed executable Huma boundaries while keeping
   existing wire behavior compatible. Test null/absent, enums, nested schedules,
   timestamps, integer widths and error statuses; fail on unsupported schema shapes
   rather than defaulting unknown objects to strings.
5. Give parent and device façades distinct credential providers so a student bearer
   cannot accidentally be used for parent calls. Centralize mount/base URL handling;
   retain server origin restrictions and exercise the deployed `/tasks/api` mount
   as well as local origin. No per-screen raw HTTP or copied DTOs.
6. Student uses existing server start/submit behavior and refreshes occurrence state
   after transitions. Add durable UI/lifecycle recovery and bounded cache if used;
   do not invent offline authoritative decisions. Retry only operations with proven
   idempotency; expose conflicts and refresh rather than blindly retrying writes.
7. Preserve current task/schedule/revision/occurrence semantics. If native work
   reveals a missing public history/pairing-management operation, implement a typed,
   tenant-scoped server endpoint first; don't inspect the DB from the client. Add
   API/regression tests and regenerate TypeScript as well as Kotlin.
8. Refresh documentation in the old Android continuation plan to point subsequent
   native dialogue/media work at Student modules. Preserve historical evidence;
   do not import unreviewed donor phases or mark continuation features complete.

## End-to-End Test Plan

- **Setup:** real Tasks host service/PostgreSQL and scheduling worker, Control
  emulator/parent phone, managed A16 Student, two distinct households. Use public
  bootstrap/membership setup, not client-provided tenant IDs. Existing local test
  issuer may support automated auth boundary cases only; real Clerk login is a
  separate mandatory native acceptance path.
- **Actions:** through Control UI create roster/task/schedules, issue QR; Student
  camera pairs and works; Control reviews/approves/rejects/retries; both reopen.
  Observe independently through existing web/API and persisted audit/decision rows.
- **Negatives:** cross-household IDs and student-to-parent bearer substitution,
  revoked/expired parent sessions, consumed QR, pairing revocation, approval before
  submit, two parents deciding concurrently, duplicate submit and request loss.
- **Durability:** kill both apps, restart service, disconnect during acknowledged
  writes and around response delivery, relaunch and compare server history.
  Include recurring timezone/DST regression via the existing real schedule worker.
- **Commands:** `make tasks-clients`, `make tasks-test`, `make tasks-cover`,
  `make tasks-android`, and existing Tasks web typecheck/build/test targets after
  API changes. `make tasks-host-up` supplies the default live host environment;
  only run promoted browser/connected suites after genuine exploratory PASS.
- **Automation:** add native instrumentation/UIAutomator workflows with documented
  serials/setup; install-preserving tests must not erase pairing. Do not use direct
  API seeding to replace the user workflow under test or inject tokens into apps.

## Anti-Cheating Audit

- Inspect Control screens for embedded Tasks web pages, fixture lists or hard-coded
  parent/session identity masquerading as a native vertical slice.
- Inspect Kotlin generated methods and typed Huma output: no copied DTOs, arbitrary
  object-to-string coercion, manual URLs or status-only tests.
- Inspect server membership/session revocation on every parent path; no client-only
  tenant selection, Clerk-role authority or bypass for "native" tokens.
- Inspect occurrence/decision transactions, worker durability and UI reconciliation;
  no in-memory approval ledger or success event preceding committed state.
- Test assertions must cover persisted decisions/actor/reason and foreign-data
  absence, not just HTTP 200 or mocked client calls.
- No test-only auth shortcuts in production, seeded in-app credentials, disabled
  conflict checks, swallowed logout/retry errors or skipped real-provider acceptance.
- Ensure management/kiosk persistence is untouched by Tasks revoke; future-feature
  matrices must not claim dialogue/media/external verification evidence here.

## Completion Gate

- [ ] All BDD cases pass through native Control/Student and real backend boundaries.
- [ ] Actual configured-provider native sign-in/logout/membership denial is evidenced.
- [ ] One-off/recurring/manual decision/retry/history and restart paths are proven.
- [ ] Server isolation and replay/concurrency tests pass; web compatibility remains.
- [ ] Clean generated clients, all affected lint/build/test/coverage gates pass.
- [ ] Anti-cheating review passes; first Tasks slice is complete without pretending
  later verification methods are available.
