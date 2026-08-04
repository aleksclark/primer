# Plan: reimplement Command Teacher as the Primer student client

## Goal

Reimplement the useful parts of `~/work/command_teacher` inside Primer as a
student-workstation application. The new client will keep the terminal-first,
hands-on teaching experience, but Primer will own identity, curriculum,
assignments, evidence, and mastery.

This is not a source import. `command_teacher` is a behavioral prototype and a
reference for interaction ideas. New code will use Primer's domain model,
security boundaries, API conventions, test standards, and NixOS deployment.

The first complete vertical slice is the digital-literacy command-line track:

1. A parent assigns or enables a standards-linked terminal activity in Primer.
2. The student workstation downloads the student's work queue.
3. The student completes the activity in an isolated local shell while the TUI
   presents instructions and brief coaching.
4. Deterministic checks decide whether the activity's observable requirements
   were met.
5. The workstation reports an idempotent session event stream and supporting
   evidence to Primer.
6. Primer updates mastery centrally and exposes the result to the parent.

The architecture must later support other local activity runners without
turning terminal-specific concepts into the core protocol.

## What to retain from the prototype

Retain these product ideas, reimplemented rather than copied:

- A keyboard-driven TUI with workspace, instructions, and tutor conversation.
- A real PTY-backed shell for authentic command-line practice.
- A disposable exercise workspace containing realistic files and directories.
- Short coaching that points students toward documentation and discovery rather
  than supplying commands to paste.
- Immediate transition after verified completion.
- A typing-practice activity mode, if the curriculum assigns it.
- A single local binary suitable for controlled workstation deployment.

## What not to carry forward

| Prototype behavior | Primer replacement |
|---|---|
| Student types a name at startup | Device is paired to one Primer student |
| Markdown files are the progress database | Server records sessions, events, evidence, and mastery |
| A fixed learning-plan checklist lives in code | Versioned activities and assignments come from Primer |
| The LLM chooses the curriculum and declares completion | Primer chooses work; deterministic checks establish task completion |
| The instructor writes arbitrary exercise files | Declarative fixtures are materialized by the local activity runner |
| Terminal state is polled from a global screen buffer | Per-session structured shell events and a bounded transcript |
| A chroot is rebuilt by copying host binaries and libraries | A Nix-built, bubblewrap-only runtime with a fixed tool closure |
| Isolation silently falls back through weaker mechanisms | Fail closed when the required sandbox cannot start |
| Cloud-provider credentials live on the workstation | Tutor inference is behind the Primer server |
| Exercise state disappears on exit | Explicit local cache, durable outbox, and resumable server sessions |
| Typing prompts and progression are hard-coded | Assigned, versioned prompt sets with reported metrics |

No package, type, prompt, test, or module should be mechanically moved from
`command_teacher`. Implement each capability against the contracts in this
plan and use the old project only for behavior comparison.

## Ownership boundaries

### Primer server is authoritative for

- Student identity and the device-to-student binding.
- Enrollments, curriculum sequence, parent overrides, and assignment priority.
- Activity definitions and immutable published activity revisions.
- Standards attached to each activity and its mastery criteria.
- Session identity and accepted event/evidence records.
- Tutor policy and model access.
- Mastery state, confidence, reinforcement dates, and parent-visible history.

### Student client is authoritative for

- Rendering the TUI and accepting local input.
- Running the terminal or typing activity locally.
- Materializing the exact downloaded activity revision.
- Capturing structured observations from the local runner.
- Running deterministic checks for responsive feedback.
- Persisting cached work and unsent events until acknowledged.

The client may report evidence, but it must never directly set a mastery status
or confidence score. Server-side policy derives mastery changes from accepted
evidence.

## Target architecture

```text
┌──────────────────── Primer LMS ──────────────────────┐
│                                                      │
│  curriculum + activities + assignments               │
│                 │                                    │
│                 ▼                                    │
│          student work API                            │
│                 │                                    │
│  tutor API ◄── session/event ingest ──► mastery      │
│     │                                     engine     │
└─────┼───────────────────────▲────────────────────────┘
      │ HTTPS                 │ idempotent batches
      ▼                       │
┌──────────── student workstation ─────────────────────┐
│ primer-student TUI                                   │
│  ├── work queue / instructions                       │
│  ├── tutor chat client                               │
│  ├── terminal activity runner                        │
│  │    └── PTY ── bubblewrap ── fixed Nix toolset     │
│  ├── typing activity runner                          │
│  └── local SQLite cache + event outbox               │
└──────────────────────────────────────────────────────┘
```

Use one new `primer-student` executable with `broker` and `tui` subcommands. A
root-owned system service runs the broker and exclusively owns credentials,
SQLite, synchronization, and sandbox launch; the unprivileged TUI talks to its
Unix socket using peer-credential checks. Keep server and client in the existing
`server` Go module initially so contracts and compatibility tests evolve
together. Standardize this module on Bubble Tea v2 and select/test a maintained
PTY/emulator before TUI implementation; do not copy the prototype's bubbleterm
wrapper blindly.

Suggested packages:

```text
server/cmd/primer-student/
server/internal/studentclient/
  api/             typed HTTP client and auth
  app/             Bubble Tea root model and navigation
  cache/           SQLite cache, resumable sessions, outbox
  activities/      runner interface and shared lifecycle
  terminal/        PTY, shell event parser, verifier, transcript
  typing/          typing runner and metrics
  tutor/           server-backed chat and coaching
  sandbox/         bubblewrap command construction and policy
server/internal/api/student_*.go
server/internal/repo/student_*.go
```

Do not couple the root TUI model directly to HTTP, SQL, PTY, or model-provider
implementations. Define narrow interfaces and drive all long-running work with
Bubble Tea commands and cancellable contexts.

## Server data model

The current schema sequences standards but does not contain executable learning
activities or student work. Add the following concepts in new migrations.

### `learning_activities`

An authoring identity that survives revisions.

- `id`, `slug`, `title`, `summary`
- `kind`: initially `terminal` or `typing`
- `subject_id`
- `status`: `draft`, `published`, `retired`
- timestamps

### `learning_activity_revisions`

An immutable payload used for reproducible sessions.

- `id`, `activity_id`, monotonically increasing `revision`
- `schema_version`
- `content` JSONB containing instructions, fixtures, runner configuration, hints,
  and checks
- `content_sha256`
- `published_at`
- uniqueness on `(activity_id, revision)` and the digest

Published revisions are append-only. Editing an activity creates a revision so
an offline client and a later evidence review refer to exactly the same work.

### `learning_activity_revision_standards`

- `activity_revision_id`, `standard_id`
- `role`: `primary` or `reinforcement`
- `weight`
- mastery-criterion identifier and evidence policy version

Standards links are immutable revision data. A changed relationship publishes a
new activity revision, so historical sessions and evidence remain reproducible.

### `student_assignments`

A concrete work item selected by the parent or a future overseer.

- `id`, `student_id`, `activity_revision_id`
- optional `enrollment_id`
- `state`: `available`, `in_progress`, `completed`, `cancelled`
- `priority`, `available_at`, optional `due_at`
- `assigned_by`, `reason`, timestamps
- optional parent constraints JSONB

The initial release requires explicit assignment. Automatic daily planning may
create the same records later without changing the workstation protocol.

### `learning_sessions`

- `id`, `assignment_id`, `student_id`, `device_id`
- client-generated `client_session_id` with a uniqueness constraint per device
- `activity_revision_id` snapshot reference
- `state`: `started`, `paused`, `completed`, `abandoned`
- `started_at`, `last_event_at`, `completed_at`
- duration and summary fields derived by the server

Legal transitions are `started -> paused -> started`, and either active state to
`completed` or `abandoned`. Starting a session atomically moves an available
assignment to `in_progress`. Only one nonterminal session may exist per
assignment; an existing one is returned to another paired device. Completion
locks the assignment and session, accepts the first valid completion, and
atomically writes events, evidence, mastery effects, assignment state, and the
completion result. Later attempts return that result unchanged.

### `learning_session_events`

An append-only audit stream.

- client-generated event UUID as the idempotency key
- `session_id`, monotonically increasing client sequence
- event type, client occurrence time, server receipt time
- schema-versioned JSONB payload
- uniqueness on `(session_id, sequence)` and event UUID

Initial event types include `session_started`, `task_viewed`, `command_finished`,
`check_evaluated`, `hint_requested`, `tutor_message`, `typing_sample`,
`session_paused`, and `session_completed`. Do not upload raw keystrokes.

### `learning_session_completions` and artifacts

Store one immutable completion result per session, keyed by the client's stable
completion UUID. It contains the request digest, accepted check results,
evidence IDs, mastery transition snapshots, and response JSON returned by every
retry. A reused UUID with a different digest is a conflict.

Artifacts are optional and uploaded before completion through an idempotent
artifact route. Store metadata and content SHA-256 in PostgreSQL, and bytes in a
configured object store or filesystem outside PostgreSQL. Enforce per-file and
per-session limits, allowlisted media types, safe filenames, malware scanning
where available, parent-only download authorization, and a retention/deletion
policy. Completion references accepted artifact IDs and digests.

### Mastery evidence

Keep `mastery_records` as the authoritative aggregate. Extend evidence ingest so
a completed check creates one idempotent `mastery_evidence` row for each linked
standard. Define `source_ref` canonically as
`student-client:<session-id>:<check-id>:<standard-id>` and enforce uniqueness on
`(mastery_record_id, source_ref)`. Upsert the mastery record, lock it during
completion, retain session/check IDs and verifier versions in structured
metadata, and deterministically recompute the aggregate from accepted evidence.

A server-side mastery service must:

1. Validate the assignment, activity revision, device, event order, and check ID.
2. Derive evidence from the published check definition and accepted observations.
3. Insert evidence idempotently.
4. Recalculate status/confidence according to Primer's mastery policy.
5. Record why a transition occurred.

Do not expose generic mastery CRUD to the student credential.

## Activity revision contract

Start with a versioned JSON contract represented by Go types and emitted in
OpenAPI. A terminal activity revision contains:

- Learning objective and concise student instructions.
- Primary and reinforcement standard references supplied by the server.
- A fixture tree with directories, text files, and permissions.
- Allowed runtime profile, such as `coreutils-basic` or `text-processing`.
- Ordered tasks, each with stable IDs, prerequisites, optional graduated hints,
  and an explicit `all`/`any` tree of required and optional check IDs.
- Deterministic completion checks with versioned result/observation schemas.
- Attempt/reset behavior, progression rules, resume policy, and
  artifact-retention policy.
- A minimum compatible runner version and unknown-field/version rejection rules.
- Tutor context that explains misconceptions and pedagogical constraints.

Checks should assert outcomes rather than exact command strings. Initial check
kinds can include:

- File or directory exists or does not exist.
- File content equals, contains, or matches a constrained pattern.
- Path has expected type or mode.
- Working directory reached.
- A command completed with expected executable/argument properties.
- A pipeline produced expected normalized output.

Avoid arbitrary shell snippets in activity definitions. Implement a small,
typed verifier vocabulary so published curriculum cannot escape the exercise
workspace and checks can be repeated by tests and the server.

A typing revision contains a versioned prompt set, categories, ordering policy,
time or completion bounds, and success thresholds. Report WPM, corrected and
uncorrected accuracy, completed prompts, and category-level errors. Do not infer
terminal-command mastery merely because a command was copied in a typing game;
typing evidence supports typing/digital-fluency standards only.

## Student API

Create a student-specific API surface instead of composing the generic admin
CRUD routes. All routes are under `/api/v1/student` and infer the student from
the authenticated device.

### Device enrollment

Use a separate student-device credential, not `SERVICE_TOKEN`, the TV device
token, or an admin key.

1. An authenticated and authorized parent creates a short-lived, one-use pairing
   code for a selected student in the admin UI. Before exposing this operation
   or activity/assignment administration, add parent sessions and role checks to
   the LMS; do not build them on today's unguarded generic CRUD surface.
2. `POST /student-devices/pair` exchanges the code plus device name/public
   metadata for an opaque random token.
3. Store only a token hash server-side and persist the token in a root-owned,
   student-unreadable workstation credential file.
4. Support revocation, last-seen time, and token rotation.
5. Bind every student endpoint to the device's student ID and reject IDs supplied
   in request bodies when they do not match.

### Work and session routes

- `GET /student/profile` returns display identity and server capabilities.
- `GET /student/work?after=<cursor>` returns assignment upserts and tombstones,
  immutable revisions, and a sync cursor/ETag. Expired cursors cause an explicit
  full snapshot; conditional requests may return not-modified.
- `POST /student/sessions` starts or resumes by `clientSessionId` idempotently.
  This maps an offline client ID to the server ID before queued events upload.
- `POST /student/sessions/{id}/events` accepts ordered event batches and returns
  the highest contiguous acknowledged sequence.
- `POST /student/sessions/{id}/artifacts` reserves/uploads bounded evidence
  artifacts idempotently and returns their IDs and digests.
- `POST /student/sessions/{id}/complete` takes a persisted completion UUID,
  request digest, final check observations, and artifact IDs; it returns the
  immutable accepted evidence and mastery-transition result.
- `POST /student/sessions/{id}/tutor/messages` returns brief policy-constrained
  coaching grounded in the activity revision and accepted session observations.

The event endpoint must accept safe retries, reject sequence conflicts with a
clear recovery response, cap payload and transcript sizes, and update a session
inside one transaction. A completion retry must return the original result.

Generate the OpenAPI spec after adding these routes, but implement a small Go
client from the shared request/response types initially. Do not make the
workstation execute a generated TypeScript client or depend on admin endpoints.

## Local state and synchronization

The root-owned broker uses SQLite at `/var/lib/primer-student/state.db`; NixOS
impermanence backs `/var/lib/primer-student` under `/persist`. The interactive
student process never opens this database or reads the bearer token. Store:

- Paired device identity metadata, excluding the token itself.
- Downloaded assignment and activity-revision cache.
- Active session state and last acknowledged server sequence.
- Append-only event outbox.
- Durable completion intent, request digest, retry state, and acknowledged
  immutable response.
- Pending artifact metadata and upload state.
- Bounded transcript segments needed for coaching/evidence.
- Explicitly retained student artifacts.

Write every event to SQLite before updating the UI as reported. A background
sync loop sends ordered batches, deletes or marks rows only after acknowledgment,
and uses exponential backoff with jitter. Process termination, reboot, duplicate
responses, and network loss must not lose or duplicate evidence.

Offline behavior:

- Previously downloaded activities remain runnable.
- New assignments and tutor responses require connectivity.
- Deterministic local checks continue to provide feedback.
- Completion is shown as `awaiting sync` until the server accepts it.
- Server tombstones win over cached state. If cancellation races with offline
  completion, the server records the session for review but creates no evidence
  unless the cancellation policy explicitly permits it.
- Revocation takes effect on the next request; a configured cache-expiry window
  limits how long fully offline work may continue.

Do not make offline inference a first-release requirement. The UI should explain
that coaching is temporarily unavailable while preserving the exercise.

## Terminal runner and isolation

Build the terminal environment as a Nix package and launch it only through
bubblewrap. The workstation already uses NixOS, so the environment can reference
an immutable store closure rather than copying whatever happens to be installed
on the host.

Required policy:

- New user, PID, IPC, UTS, and network namespaces.
- No network namespace connectivity.
- Read-only binds for the selected command-tool closure and required locale/docs.
- A fresh writable exercise directory and temporary filesystem per session.
- No bind of the student's home, `/persist`, device credential, SSH material, or
  host runtime sockets.
- Resource limits for processes, files, memory, and session duration, applied by
  a hardened systemd transient scope/cgroup plus launcher rlimits.
- Parent process death terminates the sandbox.
- A narrow, explicit artifact export path when an activity requires persistence.
- Failure to create the required isolation blocks the activity with a diagnostic.

The tutor service never receives a host-path file tool. Fixture creation occurs
before sandbox start from the typed activity contract, and verification resolves
all paths beneath the session workspace using safe path handling.

Instrument the exercise shell through a root-run launcher and a dedicated pipe
file descriptor not inherited by student commands. Frames carry a per-session
MAC and sequence; the broker rejects malformed, replayed, or out-of-order frames.
Mount startup files read-only, clear dangerous shell environment hooks, and keep
verification independent of control records where filesystem outcomes suffice.
The PTY layer strips no trusted data from ordinary terminal output. Keep a
bounded transcript for coaching, but redact environment values and configured
sensitive patterns before upload.

The client cannot prove an uncompromised workstation to the server. Evidence is
therefore classified by provenance: deterministic checks from a paired managed
workstation are accepted as continuous evidence, not formal assessment. Each
check event carries the activity digest, runner/verifier version, normalized
observation (path type/mode/content digest or bounded normalized output), and
workspace manifest digest. Formal mastery still requires the evidence mix in
Primer's server policy.

Completion checks are triggered after command completion and on explicit student
request, not by periodically asking an LLM to interpret the visible screen.

## Tutor behavior

The terminal runner and verifier work without an LLM. The tutor is a server
capability layered on top.

The tutor request includes the activity revision ID, current task/check state,
recent bounded observations, prior hints, and the student's message. The server
loads the authoritative activity and mastery context rather than trusting prompt
material supplied by the client.

Tutor policy must require:

- One or two short sentences by default.
- Questions, documentation pointers, and graduated hints before solutions.
- No command intended for copy-paste unless the activity explicitly teaches
  syntax by worked example.
- No authority to mark a task complete or mutate mastery.
- No arbitrary tool access to the workstation.
- Clear distinction between deterministic check results and model inference.

Store tutor exchanges as session events for parent review and continuity, with a
retention policy separate from durable mastery evidence.

## TUI behavior

Keep the prototype's effective layout but make the work queue the entry point:

1. Startup validates local state and pairs only if no credential exists.
2. Sync status and assigned work are shown before a session begins.
3. Selecting an assignment opens the activity workspace.
4. Terminal activities show terminal, current task/instructions, and tutor chat.
5. Typing activities use a dedicated runner rather than a shell imitation.
6. A visible status line distinguishes `online`, `offline`, `syncing`, and
   `awaiting sync`.
7. Exit pauses the session safely; a separate explicit action abandons it.
8. Completion displays checks passed, evidence submitted, and server acceptance,
   without presenting a noisy numeric mastery score to the student.

Identity is never selected by typing a name. A different student requires the
parent to revoke/re-pair or deploy a separate workstation profile.

## Workstation deployment

Add a NixOS module, for example
`workstation/hosts/workstation/primer-student.nix`, and include it from the host
configuration.

The module should:

- Package `primer-student` with `buildGoModule` and a pinned vendor hash. Because
  the current flake root is `workstation/` and cannot implicitly include sibling
  source, either move the flake to the repository root or pass a clean repository
  source as an explicit flake input; choose and test this packaging layout in
  Phase 0.
- Install bubblewrap and only the runtime dependencies the client needs.
- Create a locked-down system identity owning `/var/lib/primer-student`, the
  credential, cache, and broker socket; authorize only the student group through
  socket permissions plus peer credentials.
- Persist `/var/lib/primer-student` and the root-only credential path explicitly
  in the impermanence configuration.
- Start `primer-student broker` as a hardened system service and make all API,
  cache, pairing, rotation, outbox, completion, and sandbox operations explicit
  versioned IPC calls. The TUI never receives the token.
- Provide a `primer` launcher in the student environment and optionally launch
  it in Ghostty at Sway startup after the vertical slice is stable.
- Configure the Primer base URL and public TLS trust declaratively, never a
  shared secret in the Nix store.
- Expose a local health command used by `workstation/deploy.sh` after activation.

Deploy through the existing guarded workstation deployment workflow. Do not add
self-update logic or permit the workstation to build arbitrary repository code.

## Implementation phases

### Phase 0: contracts and threat model

- Record student-device, managed-workstation trust, offline-cache,
  transcript/artifact retention, local-root, and existing screenshot/exec-audit
  monitoring assumptions. Decide whether Primer windows are excluded from
  screenshots or whether those stores adopt the same parent access and retention
  policy.
- Choose the root-flake/source-input packaging layout, Bubble Tea v2 migration,
  and maintained PTY/emulator.
- Add authenticated parent sessions/authorization as a prerequisite design.
- Define versioned activity, observation, event, completion, artifact, and IPC
  Go types with validation.
- Add golden JSON compatibility tests before database or UI work.
- Author two sample terminal activities: basic navigation and file organization.
- Store source definitions under `curriculum/activities/`, validate them in CI,
  and add an idempotent publish/reconcile command that creates immutable database
  revisions by slug and digest. Draft admin edits export through the same
  validation/publish path.
- Decide the first digital-literacy standard codes and add them through the same
  versioned curriculum-data reconciliation path.

**Exit:** contracts can represent both activities, invalid paths/checks are
rejected, and fixtures/checks can be evaluated in temporary directories.

### Phase 1: server work queue and evidence slice

- Add parent authentication/authorization before pairing or admin authoring.
- Add activity, revision-standard link, assignment, session, event, completion,
  artifact, and device migrations, repositories, and domain types.
- Add pairing and authenticated student routes.
- Add parent admin CRUD for authoring/assigning activities, while keeping
  published revisions immutable.
- Add idempotent event ingest and completion transactions.
- Add the mastery service that converts accepted checks into evidence and updates
  records without accepting client-supplied mastery values.
- Regenerate LMS OpenAPI and the admin client.

**Exit:** an API integration test pairs a device, downloads an assignment,
starts twice with the same client ID, retries event batches, completes twice,
and produces exactly one set of evidence/mastery effects.

### Phase 2: headless local engine

- Implement SQLite cache/outbox and sync protocol.
- Implement typed fixture materialization and verifier checks.
- Implement the bubblewrap/Nix terminal profile and structured shell records.
- Build a noninteractive harness that runs known command sequences against the
  sample activities.

**Exit:** the harness downloads work, completes one activity online, repeats it
through a forced disconnect/restart, and reaches the same single server result.
It also proves the sandbox cannot read a host canary file or access the network.

### Phase 3: student TUI

- Implement work queue, session lifecycle, three-pane terminal activity view,
  sync state, errors, pause/resume, and completion summary.
- Add tutor endpoint and chat only after deterministic progression works.
- Add bounded transcript handling and retention cleanup.
- Test resize, focus, cancellation, delayed responses, and reconnect behavior.

**Exit:** a student can complete the basic-navigation assignment without an LLM,
then request a hint in a second run, and the parent sees accepted evidence.

### Phase 4: typing activity

- Reimplement the typing runner using assigned prompt sets and explicit metrics.
- Link it only to appropriate typing/digital-fluency standards.
- Reuse the same assignment, session, event, offline, and completion contracts.

**Exit:** typing sessions sync idempotently, metrics survive restart, and command
knowledge mastery is not changed by typing-only evidence.

### Phase 5: NixOS deployment and operations

- Add the workstation Nix module, persistence, credentials, launcher, and health
  check.
- Extend guarded deployment verification to start the client, inspect health,
  and confirm the cache is writable and the sandbox can launch.
- Pair the physical workstation and run a real student acceptance session.
- Add parent-visible device/session diagnostics and revocation.

**Exit:** a clean workstation deployment can pair, download, run, reboot offline,
resume, reconnect, and report exactly one completion.

### Phase 6: broader Primer integration

- Let the session overseer create assignments through the same server model.
- Add reinforcement activities selected from mastery scheduling.
- Introduce other activity runners only through the shared runner interface.
- Replace temporary admin authoring forms with validated curriculum tooling if
  authoring volume warrants it.

## Testing strategy

### Server

- Migration up/down and constraint tests in PostgreSQL testcontainers.
- Repository and API tests for student isolation, revoked devices, malformed
  revisions, revision immutability, event ordering, retries, and completion.
- Transaction tests proving evidence and mastery updates are all-or-nothing.
- Authorization tests proving one paired device cannot read or write another
  student's data or access generic admin/mastery routes.

### Client

- Unit tests for activity validation, safe paths, checks, shell-event parsing,
  transcript bounds/redaction, sync cursors, and backoff.
- SQLite restart tests around every outbox transition.
- HTTP fake tests for timeouts, 401 revocation, 409 sequence recovery, duplicate
  acknowledgment, stale work, and server upgrades.
- Bubble Tea model tests for asynchronous messages and stale responses.
- PTY integration tests against the pinned Nix tool profile.

### End to end

- Pair, assign, download, run, verify, complete, and inspect mastery.
- Disconnect before start, during event upload, and after server commit but
  before client acknowledgment.
- Kill and restart the client during an active terminal session.
- Revoke the device and verify cached work expires without exposing other data.
- Attempt host-file reads, network access, process escape, fork exhaustion, path
  traversal, and malicious fixture/check payloads.
- Validate the NixOS configuration with `nix flake check` and build the host
  system before deployment.

Run focused Go tests after each change, then `make test`, `make lint`, OpenAPI
regeneration checks, and workstation flake checks at phase boundaries.

## Observability and parent controls

Add structured server logs and metrics for pairing, work sync, active sessions,
event lag, rejected event batches, tutor failures, completion latency, and
outbox age as reported by device heartbeat. Never log tokens, raw chat bodies,
or full terminal transcripts.

The parent UI should eventually show:

- Paired devices, last seen, app version, sync lag, and revoke action.
- Assignments and their current state.
- Session timeline with commands/check outcomes at an appropriate level of
  detail.
- Evidence created from each session and resulting mastery transitions.
- Activities awaiting sync or requiring review.

A parent correction adds or supersedes evidence through an auditable server
operation; it does not rewrite the device event history.

## Migration from `command_teacher`

There is no runtime data migration in the first release. The prototype's
`STUDENT_<name>.md`, `TYPING_<name>.md`, cached instructions, and chroot contents
are not authoritative enough to import as mastery evidence.

If preserving history is desired, provide a one-time parent-reviewed report
that parses the markdown into dated notes attached to the student. It must not
create completed assignments or mastered standards automatically.

Use the old application as an acceptance oracle only:

- Compare the ergonomics of terminal/instructions/chat interaction.
- Recreate representative beginner exercises as declarative Primer activities.
- Confirm coaching remains concise and discovery-oriented.
- Remove the standalone prototype from operational use only after the Primer
  client completes the deployed vertical slice.

## Decisions and deferred work

Decisions for the initial implementation:

- Primer server is the source of curriculum, assignments, tutor policy, and
  mastery; the workstation is an offline-capable executor and evidence producer.
- Completion is deterministic; an LLM may coach but may not grade observable
  terminal tasks.
- Student APIs use per-device credentials bound to one student.
- Local changes are queued durably and server writes are idempotent.
- Bubblewrap on the pinned NixOS workstation is mandatory and fails closed.
- No Bedrock, OpenAI, or OpenRouter credential is deployed to the workstation.

Deferred until after the vertical slice:

- Automatic overseer-generated daily assignments.
- Offline/local tutor inference.
- Collaborative or multi-student workstation sessions.
- General-purpose coding containers or network-enabled exercises.
- Importing prototype progress as anything stronger than parent-reviewed notes.
- Real-time screen streaming to the tutor; structured observations are preferred.
