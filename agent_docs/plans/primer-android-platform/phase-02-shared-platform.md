# Phase 2: Shared Android UI and app foundation

## Goal

Create one Android build with three app identities consuming the same System C
Compose components. Move the existing Tasks student functionality into Student
without losing pairing/security behavior; establish Control's installable shell.
Do not pretend an unauthenticated Control shell is the completed Tasks slice.

## BDD Success Criteria

### Scenario: One visual system, distinct input models

- **Given** signed TV, Student and Control builds from the same checkout
- **When** a reviewer opens their real navigation, status, settings and form surfaces
  in dark and light mode
- **Then** shared controls have the same token-derived geometry, typography, state
  semantics and focus treatment
- **And** TV remains D-pad usable while phone/tablet controls support touch, large
  text, keyboard/IME and accessibility without clipping or inaccessible actions.

### Scenario: Existing Tasks behavior survives migration

- **Given** the existing Tasks web/server and a Student installation
- **When** a parent issues a QR, Student pairs, and the student opens Today/Upcoming
  and a task, starts and submits it
- **Then** the public server holds the corresponding student-bound state
- **And** process restart preserves pairing, a foreign occurrence stays unavailable,
  and revocation removes Tasks access without releasing device ownership.

### Scenario: Application identity migration is explicit

- **Given** a device previously paired through `com.aleksclark.primertasks`
- **When** the parent moves it to Primer Student through the documented process
- **Then** the new app requires a new authorized pairing and the old pairing is revoked
- **And** no private token is extracted/copied across packages, no old task data is
  deleted on the server, and TV retains its package identity and pairing.

### Scenario: TV remains a real functioning product

- **Given** a paired TV box and a Student-owned A16 with TV allowlisted
- **When** the user browses, plays, seeks where permitted, returns and opens settings
- **Then** server playback grants/accounting and entertainment seeking restrictions
  remain correct, with working D-pad focus on the box
- **And** TV does not attempt to become device owner on the A16.

### Scenario: Fresh checkout builds without hidden generated files

- **Given** a fresh checkout without ignored generated Kotlin/OpenAPI files
- **When** the documented root generation/build commands run
- **Then** all three app targets and their tests compile from their declared dependencies
- **And** a missing generation prerequisite fails rather than silently using stale output.

## Implementation Instructions

1. Keep `android/app/` and `android/core/` as TV targets for compatibility. Add
   `app-control` and `core-ui`, and wire Phase 1 Student into the same version
   catalog/toolchain. Select supported mobile minSdk deliberately; shared code
   must not raise TV's Android 9 floor accidentally. Keep TV FFmpeg/ABI configuration
   local to TV. Control and Student do not inherit TV orientation/leanback flags.
2. Move generic theme/color/type/spacing/motion into `core-ui`. Generate tokens once
   from `design-system/generated/PrimerTokens.kt` into that library; remove copied
   native tokens and per-app rewrites after migration. Bundle appropriately licensed
   Instrument Sans / IBM Plex Mono fonts. Use the canonical System C reference.
3. Implement shared buttons, inputs, ruled surfaces/rows, status labels, error/empty/
   loading states and dialogs with real disabled/focus/semantic behavior. Keep
   TV form-factor/domain classifications outside the library. Add previews/catalog
   and screenshot/semantics tests, but prove consumption through actual app screens.
4. Refactor TV to these primitives while preserving product-specific components,
   navigation/focus and playback boundaries. Implement actual light-theme selection
   rather than merely defining unused light palette objects. Maintain dark default.
5. Move existing native Tasks UI into `feature-tasks-student`, separating lifecycle/
   state and transport from the current monolithic `MainActivity.kt`. Preserve
   CameraX pairing, parser/origin restrictions, Keystore encryption, backup denial,
   deep-link semantics and system-picker documented fallback. Use managed launcher
   navigation for Tasks and approved apps; don't embed a browser substitute.
6. Turn `primer-tasks/clients/kotlin/` into a proper Kotlin Gradle module included
   from the shared build. Apps import its façade, not its source directory or
   generated internals. Declare contract/client generation prerequisites. Extend
   the generator only as needed here; parent operation expansion is Phase 3.
7. Add the Control shell with truthful not-yet-configured/sign-in-entry state,
   shared navigation/settings and no seeded household/tasks. Do not add DPC or
   blanket install/management privileges to the parent's app.
8. Document explicit prototype migration: record old pairing, issue new Student QR,
   confirm identity, revoke old pairing, then optionally uninstall old APK. Preserve
   any deployed occurrence link compatibility via a verified routing decision;
   resolve ambiguous old/new scheme handling rather than forwarding credentials.
9. Update root `Makefile`, `primer-tasks/Makefile`, `.github/workflows/android.yml`
   and applicable Tasks CI. Include `design-system/**` and Kotlin client/server
   contract inputs in change triggers. Keep `make tasks-android` as a compatibility
   entry point delegating to the new build. Retire duplicate Gradle app implementation
   only after the new flow passes; update `CONNECTED_ACCEPTANCE.md` paths explicitly.

## End-to-End Test Plan

- **Setup:** real Tasks host stack (`make tasks-host-up`), two parent households,
  shared builds, an emulator and managed A16; real TV backend/media and dedicated
  TV input environment for TV regression. No fake first-party API.
- **Mobile action/assertion:** public parent QR -> Student camera scan -> visible
  correct profile/tasks -> start/submit -> parent sees persisted pending review.
  Kill/reopen; try foreign deep link; revoke and verify denial while kiosk persists.
  Use original camera-to-system-picker exception only as already documented.
- **Visual/input action/assertion:** run common controls in all three actual apps,
  both themes, enlarged font, TalkBack/keyboard and TV D-pad. Capture screen evidence
  with secret screens excluded. A gallery screenshot alone cannot prove adoption.
- **Migration action/assertion:** leave old prototype installed, move through the
  real re-pair flow, verify old credential revocation and unchanged server history.
- **TV action/assertion:** real grant/playback/heartbeat/resume and seeking policy;
  no re-pair or owner change caused by component migration.
- **Build:** existing `make design-system`, `make tasks-clients`, `make tasks-android`,
  and `cd android && ./gradlew test assembleDebug`; add documented all-app lint and
  release builds. Follow install-preserving connected acceptance, not data-clearing
  Gradle reinstalls after pairing. Run exploration before promoted automation.

## Anti-Cheating Audit

- Dependency graph must show all apps using `core-ui`; no three copied implementations
  or unused token library alongside hard-coded Material colors.
- Inspect real screens, not just catalogs; no fixture task lists or hard-coded
  successful pairing. Control placeholder must not claim live parent functionality.
- Inspect generated dependencies, package boundaries, primitive imports and CI
  triggers. No stale source-tree generation, ignored missing files or bypassed build.
- Inspect Keystore/metadata migration and revocation: no raw token extraction,
  client-only authorization, hidden cross-package data sharing or in-memory sessions.
- Inspect TV playback and focus assertions, rather than build-only non-regression.
- No test-only data/flags, swallowed migration errors, broadened retries, disabled
  accessibility assertions or skipped device cases to satisfy the gate.
- Events/status must follow real persisted server state; screenshots and docs must
  not claim first-slice completion before Phase 3.

## Completion Gate

- [ ] All BDD scenarios pass with real Tasks/TV dependencies and migrated Student.
- [ ] Three separately installable APKs use shared components in real screens.
- [ ] Dark/light/input/accessibility and TV playback evidence is reviewed.
- [ ] Prototype re-pair migration and owner/credential isolation are proven.
- [ ] Clean generation, client boundaries, unit/lint/build/connected gates pass.
- [ ] Anti-cheating review passes; no generated client outputs are committed.
