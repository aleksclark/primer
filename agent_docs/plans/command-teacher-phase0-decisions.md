# Command Teacher Phase 0 decisions

Recorded during Phase 0 of
`agent_docs/plans/command-teacher-reimplementation.md`. These decisions bound
trust, retention, packaging, and the parent-auth prerequisite. Implementation
details for later phases should not contradict this document without an
explicit update here.

## Trust model

### Student-device trust

- A student workstation is paired to exactly one Primer student via a
  short-lived parent-issued pairing code exchanged for an opaque device token.
- The device token is root-owned on the workstation and never exposed to the
  unprivileged TUI process. The broker alone holds it.
- Evidence reported by a paired device is **continuous practice evidence**, not
  a formal proctored assessment. Server mastery policy may weight it below
  parent-observed or formal checks.
- The client may report check observations and artifacts. It must never set
  mastery status or confidence directly.
- Device revocation is enforced on the next authenticated request. Fully offline
  work is bounded by a configured cache-expiry window (chosen in Phase 1/2).

### Managed-workstation trust

- Workstations are NixOS-managed family devices under parent administrative
  control, not hostile multi-tenant hosts.
- Isolation (bubblewrap + fixed Nix tool closure) protects the host from the
  exercise and keeps exercises reproducible. It is not a defense against a
  malicious local root or a fully compromised student account with physical
  access.
- Deterministic checks run against a session workspace the broker materializes.
  Paths are constrained with safe-path validation so curriculum cannot escape
  the workspace via `..` or absolute paths.

### Local-root threat

- A process with local root can read the device token, mutate the SQLite cache,
  forge outbox events, or disable the sandbox. Primer does not attempt to stop
  that adversary on-device.
- Mitigations are operational and server-side: parent-visible device activity,
  revocation, anomaly review of evidence patterns, and treating student-client
  evidence as non-authoritative for high-stakes claims.
- Formal grade-affecting mastery still requires the broader evidence mix defined
  by Primer mastery policy.

## Offline cache, transcripts, and artifacts

| Data | Authority | Durability | Retention (initial) |
|---|---|---|---|
| Device token | Server (hash); root file on device | Persist across reboot under `/var/lib/primer-student` | Until revoke/rotate |
| Assignment + activity revision cache | Server revisions immutable | SQLite + content files | Until tombstone or cache expiry |
| Event outbox | Client until ack; server append-only | SQLite then PostgreSQL | Server audit retention (Phase 1 policy) |
| Bounded terminal transcript | Client coaching aid | Local, redacted before upload | Short local TTL; server tutor-event retention separate from mastery |
| Session artifacts | Optional evidence | Object store + metadata | Parent-access policy; default align with monitoring retention below |
| Mastery aggregates | Server only | PostgreSQL | Long-lived academic record |

Offline rules for Phase 0 design (implemented later):

- Previously downloaded revisions remain runnable.
- New assignments and tutor calls require connectivity.
- Local deterministic checks still give the student feedback.
- Completion displays as awaiting sync until the server accepts the completion
  UUID/digest pair.

## Screenshot and exec-audit monitoring

The workstation already captures:

- Periodic screenshots under `/persist/monitoring/screenshots` (7-day cleanup).
- Process accounting and auditd `execve` logging for student UIDs.
- Sway window-focus tracking under `/persist/monitoring/windows`.

**Decision (Phase 0):** Primer student TUI/terminal windows are **not** excluded
from screenshot or exec-audit monitoring. Those stores keep the **same parent
access and retention policy** as the rest of the workstation monitoring stack
(screenshots: 7 days; exec/window logs: follow existing persist + rotation).

Rationale: excluding Primer would create a blind spot exactly where command
practice happens, and the parent already expects continuous light monitoring on
this managed device. If privacy or storage pressure later requires a narrower
policy, revisit with an explicit parent-facing setting rather than a silent
client-side exclusion.

Structured Primer session events remain the primary pedagogical audit trail;
screenshots are a coarse operational backstop, not the completion mechanism.

## Packaging layout

**Decision:** keep the flake root at `workstation/` for now. Package
`primer-student` by passing an **explicit repository source input / path** from
the workstation flake into `buildGoModule` (for example a clean filtered copy of
the monorepo or a `path:`/`git+` input that includes `server/`). Do **not**
require moving the flake to the repo root in Phase 0.

A repo-root flake remains a viable later cleanup if packaging friction grows.
Phase 5 should implement the explicit-input approach first and document the
vendor hash + allowed source paths in the workstation module.

## Bubble Tea and PTY / emulator

**Decision for Phase 0 tooling:** use the module’s existing **Bubble Tea v1**
stack (`github.com/charmbracelet/bubbletea v1.x` and `bubbles` already in
`server/go.mod`) for the `activity-validate --tui` helper and any Phase 0 demos.
This avoids a cross-cutting migration while contracts land.

**Decision for the student client (Phase 3 target):** standardize on **Bubble
Tea v2** before building the real `primer-student` TUI. Track the migration as
its own change set (module-wide), not as a side effect of activity contracts.

**PTY / terminal emulator:** prefer **`github.com/creack/pty` plus a thin custom
wrapper** owned under `server/internal/studentclient/terminal` for lifecycle,
resize, and bounded transcript capture. Do not vendor or copy the prototype’s
bubbleterm wrapper. Revisit `bubbleterm` (or another maintained emulator) only
if the custom wrapper cannot meet rendering/test needs; any such dependency must
be evaluated for maintenance and Bubble Tea v2 compatibility first.

## Parent authentication prerequisite (design only)

Phase 1 must not hang new pairing or activity administration off today’s open
LMS CRUD. Outline:

1. **Sessions**
   - Parent/educator authenticates (password or passkey; exact mechanism Phase 1)
     and receives a server-side session (HTTP-only secure cookie or opaque bearer
     stored server-side).
   - Sessions have idle and absolute lifetimes, rotation on privilege use, and
     explicit logout/revoke.
2. **Roles**
   - Reuse `educators.role` values (`parent`, `admin`, `tutor`) with explicit
     permission checks per route.
   - Pairing-code creation: `parent` or `admin` for a student in scope.
   - Activity authoring/publish: `admin` (and optionally `parent` for drafts
     later); published revisions remain immutable.
   - Assignment create/cancel: `parent` or `admin` for students in scope.
3. **Scope**
   - Early single-family deployment may treat all non-admin parents as full
     household scope; still thread `actor_id` through audit fields (`assigned_by`,
     pairing creator).
4. **Student API separation**
   - Student device token auth remains distinct from parent sessions and from
     `SERVICE_TOKEN` / TV device tokens.
5. **Ordering**
   - Land session middleware + role helpers before student-device pairing routes
     and before activity/assignment admin mutations.

This document is design-only for auth; no session implementation is required to
close Phase 0.

## Curriculum and contracts (Phase 0 deliverables)

- Versioned Go contracts live under
  `server/internal/studentclient/contracts`.
- Fixture materialization and typed verification live under
  `server/internal/studentclient/terminal`.
- Source activities: `curriculum/activities/{slug}/activity.yaml`.
- Digital-literacy standard seeds: `curriculum/standards/digital-literacy.yaml`.
- Offline validate entrypoint: `server/cmd/activity-validate` and
  `make activity-validate` (no database). Full DB publish/reconcile is Phase 1.

## Open items deferred past Phase 0

- Exact offline cache TTL and revocation grace parameters.
- Object-store backend choice for artifacts.
- Bubble Tea v2 migration execution.
- Parent session credential mechanism (password vs passkey).
- Whether tutor transcript retention differs numerically from the 7-day
  screenshot window (policy hook exists; defaults may match initially).
