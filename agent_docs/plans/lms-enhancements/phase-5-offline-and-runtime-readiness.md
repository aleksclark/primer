# Phase 5: Offline reconciliation and runtime capabilities

## Purpose

Make long-running courses reliable on managed student workstations. Assignments, cancellations, evidence, and runtime requirements must converge after disconnection without exposing work the device cannot execute.

## Current need

The server work endpoint supports cursors and changed assignment rows, including soft-cancelled states. The client currently requests the first 100 items with an empty cursor on every sync and does not paginate or persist the returned cursor. This can miss assignments and cannot provide a well-defined full-snapshot recovery.

Runtime contracts name `coreutils-basic` and `text-processing`, but the workstation installs one combined closure through the singular profile variable. The resolver accepts that closure for either known name. Health checks do not prove that every authored profile and declared binary is present. The handwritten binary list also differs from what the Nix closure may actually contain.

These are workstation and synchronization capabilities, but they are part of the LMS learning loop: the server must know device capabilities before assignment and must reconcile authoritative assignment state.

## Scope

## A. Offline work reconciliation

### 1. Durable cursor and pagination

Persist per-device work-sync state in broker-owned SQLite:

- last accepted cursor;
- snapshot generation/ETag if introduced;
- last successful full reconciliation;
- pagination continuation during an in-progress sync.

Fetch every page until exhaustion before advancing the durable cursor. A crash midway resumes safely or restarts from the previous committed cursor.

### 2. Explicit snapshot semantics

Define two server response modes:

- incremental upserts since a valid cursor;
- full snapshot when no cursor exists or the cursor expired.

A full snapshot is authoritative for assignment rows in scope. The client atomically upserts returned rows and reconciles cached rows absent from the snapshot according to server policy. Because assignments are soft-cancelled, incremental cancellation normally arrives as an upsert; deletion tombstones are added only if the data model later permits hard deletion.

### 3. Cancellation and offline races

- Cancelled work cannot start after the cancellation is synchronized.
- An already-running offline session may preserve local work but cannot create mastery automatically after server rejection.
- On reconnect, the server records the session for parent review if policy permits, labels it cancelled-after-work, and returns an explicit result.
- The client never discards unsynced student evidence merely to make cache state look clean.

### 4. Revision and enrollment changes

Published revisions remain immutable. If a parent migrates an unstarted course entry, the old assignment is cancelled and a new assignment references the new revision. Cached old revisions remain only while needed for retained sessions/evidence and are garbage-collected under a documented policy.

### 5. Acceptance coverage

Add tests for more than 100 assignments, long disconnection, cursor expiry, cancellation, enrollment pause, revision migration, duplicate responses, partial pages, reboot during sync, and server unavailability.

## B. Runtime capability readiness

### 1. Audit actual closures

Generate or verify each profile manifest from the Nix closure rather than maintaining an inaccurate handwritten list. Record:

- profile name and version/digest;
- available binaries;
- shell/instrumentation version;
- locale and required shared data;
- compatibility with runner/verifier versions.

Audit whether `whoami` and `id` are already supplied by coreutils. Add `file` and process inspection tools only if approved course objectives require them.

### 2. Independent runtime packages

Build separately named closures for authored profiles, including a deliberate `linux-fundamentals` profile if needed. Configure `PRIMER_RUNTIME_PROFILES_DIR` so profile names resolve to distinct directories. Remove production reliance on singular-profile fallback once migration completes.

Capability sets should remain narrow. Do not install a general development environment simply because a later lesson might use one command.

### 3. Device capability reporting

Student profile/heartbeat data reports installed profile names, digests, runner versions, and structured-evidence support. This replaces Phase 0's temporary boolean capability gate. The server stores recent capabilities without trusting them as an authorization boundary.

The Phase 4 eligibility engine excludes incompatible activities and gives the parent a clear deployment diagnostic rather than assigning work that fails at launch.

### 4. Health checks and fail-closed launch

Health verifies every configured profile directory and declared binary. Launch verifies the activity's required profile digest/version and isolation prerequisites. Missing capabilities block only affected work with a precise message; they never trigger host PATH fallback in production.

## Data and API changes

- Work response mode, cursor expiry response, and snapshot generation.
- Broker sync metadata and atomic reconciliation methods.
- Session status for cancellation/offline conflicts.
- Device capability payload and parent diagnostic fields.
- Course eligibility reason for missing runtime capability.
- Optional activity requirement fields for profile version/digest ranges.

## Implementation slices

1. Specify cursor, pagination, and snapshot contracts.
2. Persist sync metadata and implement atomic page application.
3. Add cursor-expiry/full-snapshot server behavior.
4. Implement cancellation and migration reconciliation.
5. Generate runtime manifests from Nix packages.
6. Package and configure separate profiles.
7. Report capabilities and integrate eligibility gating.
8. Expand workstation health and physical acceptance tests.

## Tests

- Pagination retrieves every item and advances the cursor only after atomic application.
- Cursor expiry converges through a full snapshot.
- Cancelled rows replace open cached rows.
- Unsynced evidence survives cancellation and is surfaced for review.
- Reboot during any sync stage is safe.
- Runtime manifests match actual closure binaries.
- An activity cannot launch against the wrong singular closure.
- Missing profile/binary blocks assignment or launch with a useful diagnostic.
- No production profile falls back to unrestricted host binaries.
- Existing downloaded compatible work remains usable offline.

## Physical UAT

1. Download several Linux lessons, disconnect the workstation, complete one, pause another, and reboot.
2. While offline, cancel an assignment and publish/enroll a replacement server-side.
3. Reconnect and verify evidence upload, cancellation handling, replacement work, and cursor advancement.
4. Temporarily remove a test runtime profile and verify eligibility/health failure.
5. Restore it and confirm the assignment becomes eligible without re-pairing.
6. Run text-processing and scripting lessons against their exact declared closures.

## Exit criteria

- Work sync persists cursors, paginates fully, and recovers through authoritative snapshots.
- Cancellation and offline-completion races preserve evidence without creating false mastery.
- Runtime profiles are separate, versioned, and verified against their actual closures.
- The server does not assign incompatible activities when current device capabilities are known.
- Production launch never treats one profile directory as every profile or falls back to host tools.
- A 72-hour offline/reconnect physical test passes with at least 150 assignment-state rows, cancellation and revision changes during disconnection, one reboot mid-sync, no lost local evidence, and exact convergence with the server snapshot.

## Out of scope

- General remote workstation management.
- Hard deletion of learning evidence.
- Installing unrestricted compilers, package managers, or network tools.
- Requiring continuous connectivity for already downloaded compatible work.
