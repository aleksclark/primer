# Phase 2: Trustworthy terminal evidence

## Purpose

Make terminal activity evidence correspond to completed commands and observable workspace changes rather than idle timing and screen scraping. This phase restores reliable procedural assessment while preserving freedom to solve tasks with reasonable equivalent commands.

## Current need

The command-box fallback can wrap one submitted line and observe its process result. The deployed PTY path instead waits after a newline, assumes success, labels the event `pty-shell`, and uses the visible terminal screen as stdout. It cannot reliably distinguish prompts, commands, output, errors, interactive programs, or completion. Exact `command_properties` checks therefore conflict with both the real runner and the original design requirement for structured shell events.

Primer's architecture already anticipates a root-run launcher with a dedicated authenticated event pipe. This phase completes that design.

## Scope

### 1. Trusted launcher and structured shell event channel

Do not place the event MAC key or writable control channel in the student-controlled shell process. A broker-owned launcher outside the sandbox owns the key and event sequence, supervises the PTY/shell process, and converts trusted process-boundary observations into authenticated frames. Shell-specific instrumentation may send untrusted candidate metadata over a separate channel; the launcher validates and combines it with observed process exit and PTY stream boundaries before signing an event. Student commands never inherit the signed event channel or key.

Support one explicitly versioned interactive Bash integration first. If Bash cannot provide trustworthy executable/argv or pipeline structure for a construct, mark those fields unavailable and rely on outcome checks rather than inventing values. Other shells remain unsupported until they have an equivalent tested integration.

Events include:

- session ID and monotonically increasing sequence;
- command start and completion timestamps;
- original submitted line for bounded audit display;
- normalized executable and argument vector when determinable;
- working directory before and after;
- exit status or terminating signal;
- bounded stdout and stderr digests plus separately bounded display excerpts;
- pipeline/redirection structure sufficient for typed predicates;
- runner, shell-instrumentation, and verifier versions.

The trusted launcher signs frames with a per-session MAC. The broker rejects malformed, replayed, cross-session, or out-of-order frames. Candidate metadata from inside the shell is never accepted as trusted solely because it came from a startup hook. Ordinary terminal output remains ordinary PTY data and is not trusted as a control channel.

### 2. Remove synthetic PTY observations

Delete the idle-screen conversion into a successful shell result. Checks run after a completed-command event or explicit student verification request. Filesystem checks may also run after fixture materialization/reset.

If instrumentation fails, the activity stops with a diagnostic. Do not downgrade silently to screen polling in production.

### 3. Command history evidence

Persist bounded structured command history per session. Task checks can evaluate:

- at least one matching successful command since task activation;
- executable/argument properties with permitted equivalence;
- expected exit status or failure;
- stdout/stderr value, substring, or pattern;
- pipeline/redirection properties;
- cwd transition;
- command ordering when pedagogically necessary.

Avoid exact syntax surveillance. Prefer filesystem and output outcomes. Command-structure predicates are justified only when the learning objective is the command, pipeline, redirection, or error-handling mechanism itself.

### 4. Workspace manifests and provenance

Capture a safe workspace manifest before and after each completed command:

- relative path;
- type and permission bits;
- bounded content digest for files;
- no host paths, secrets, or file contents in the event by default.

Store a manifest digest on check observations and completion evidence. Derive write sets by comparing manifests. This supports script provenance and write-boundary checks without adding arbitrary audit hooks.

### 5. Script execution predicates

Build script-oriented checks from command history and workspace deltas:

- a named executable ran with expected arguments;
- it completed within the activity timeout;
- expected stdout/stderr and exit status were observed;
- writes remained under allowed paths;
- generated artifact digests match required outcomes;
- repeated execution is stable when the activity requires it.

Do not introduce an opaque verifier that executes author-supplied shell. The same typed evidence vocabulary must serve interactive commands and scripts.

### 6. Evidence upload and server validation

Command events are written to the durable local outbox before the UI reports them. The server validates sequence, schema version, size limits, activity digest, and runner capability. Completion observations reference accepted event sequences and workspace manifest digests.

The server still treats this as continuous evidence from a managed paired workstation, not formal assessment or proof that the host was uncompromised.

## Security requirements

- Startup files and instrumentation binaries are read-only in the sandbox.
- Dangerous inherited environment values are cleared; the supported shell integration is installed by the trusted launcher.
- The signing key and signed control descriptor exist only in the broker-owned launcher outside the student shell and sandbox.
- Closing or corrupting the untrusted candidate-metadata channel yields missing evidence or a blocked activity, never forged success.
- No network access is added.
- Event excerpts are bounded and redact configured sensitive patterns.
- Workspace traversal remains beneath the session root.
- Instrumentation cannot grant access to broker credentials or SQLite.
- Resource limits and parent-death behavior remain enforced.

## Implementation slices

1. Define versioned structured command event contracts.
2. Implement launcher event framing, MAC, and sequence validation.
3. Persist command history and workspace manifest digests locally.
4. Remove idle-screen shell-result synthesis.
5. Extend verifier predicates to task-scoped history and output streams.
6. Add script/write-set predicates from existing evidence primitives.
7. Upload events idempotently and bind completion observations to accepted sequences.
8. Update representative Linux activities away from exact wrapper strings.

## Tests

- Interactive commands, builtins, pipelines, redirections, failures, signals, and cwd changes produce correct events across a fixture corpus of at least 50 supported Bash command forms.
- Prompt text and prior scrollback never appear as trusted command stdout.
- Student processes cannot forge, replay, or inherit control frames.
- Equivalent valid commands satisfy outcome-oriented tasks where exact syntax is irrelevant.
- A task teaching `>` does not pass when the final file was hand-edited through another mechanism unless policy explicitly permits outcome-only evidence.
- Script write-set checks reject writes outside the allowed project paths.
- Reboot/offline retries preserve event order and do not duplicate evidence.
- Instrumentation failure blocks the activity clearly.
- Existing filesystem checks remain deterministic.

## Physical UAT

Run lessons 1, 9, 13, 15, 16, and 19 through the actual PTY. Verify:

- executable, arguments, exit status, cwd, stdout, and stderr;
- pipelines and redirection without wrapper-specific exact strings;
- command failure and `$?` lessons;
- script execution and generated reports;
- reset, resume, offline completion, and later sync;
- parent evidence views show bounded structured observations rather than terminal dumps.

## Exit criteria

- No production terminal check depends on idle timing or visible-screen parsing.
- Every supported command fixture produces the expected event in 100 consecutive PTY test runs, and required command-sensitive checks pass in 20 consecutive physical runs per representative lesson.
- Evidence includes accepted event sequences and workspace manifest digests.
- Script provenance and write boundaries are verifiable with typed predicates.
- Command checks allow pedagogically valid equivalents unless exact structure is the objective.
- Physical UAT demonstrates accurate success and failure evidence across representative lessons.

## Out of scope

- Proving the workstation is uncompromised.
- Arbitrary shell-based verifier plugins.
- Full terminal transcript upload.
- LLM interpretation as a completion check.
