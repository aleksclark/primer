# Command-teacher platform gaps for the basic Linux course

The course files are valid Primer v1 activities, can be published, assigned, run in isolated workspaces, and can report deterministic completion evidence. The following gaps prevent command-teacher from delivering the intended course exactly as written without parent mediation or platform changes.

The verified, dependency-ordered implementation roadmap is in [`agent_docs/plans/lms-enhancements/`](../../../agent_docs/plans/lms-enhancements/README.md). That roadmap corrects several claims below where the platform already has part of the needed capability and preserves parent judgment, mastery-based progression, typed deterministic checks, and reproducible project work.

## Must close before student UAT

### Instructional Markdown is not displayed

Each lesson has a substantial `LESSON.md`, but the student client receives only `ActivityContent` strings and does not load or render that file. As a result, command-teacher presents terse objective/task text rather than the explanation, examples, vocabulary, misconceptions, and reflection prompts.

**Close with:** versioned instructional blocks or a hashed Markdown resource in activity content, safe Markdown rendering in the TUI, and offline caching with the immutable revision.

### Runtime profiles are not independently deployed

Lessons 12–14 and 19–20 declare `text-processing`. The contract and sandbox registry know that profile, but the workstation currently installs a single `coreutils-basic` Nix closure and supplies it through `PRIMER_RUNTIME_PROFILE_DIR`. The combined closure happens to include many required tools, but profile selection is not actually enforced as authored.

**Close with:** build and install both named closures, configure `PRIMER_RUNTIME_PROFILES_DIR`, verify each profile's binaries at health check, and fail assignments clearly when the minimum runtime is absent.

### Conceptual mastery lacks a response surface

Filesystem roles, terminal-versus-shell, permission meaning, process concepts, pipeline reasoning, and capstone defense require student explanation. Current checks can validate files and the latest command/output, but cannot capture a short answer or parent-confirmed oral explanation. Several activities therefore use constrained `echo` responses as a temporary proxy.

**Close with:** short-answer tasks, rubric criteria, persisted evidence, parent attestation, and a policy that deterministic state may grant procedural evidence while conceptual mastery requires an appropriate response or human review.

### Capstone continuity is missing

Every activity receives a new isolated fixture. Lesson 20 therefore has to provide a deliberately imperfect copy rather than verify the student's actual Lesson 19 project.

**Close with:** explicit artifact promotion from one completed revision into a later assignment, immutable provenance, malware-safe file limits, parent visibility, and reset semantics that distinguish disposable practice from a retained project.

### Course progression is bulk assignment, not mastery-gated sequencing

`load.py` publishes and assigns all 20 activities because Primer has no course sequence model. Assignment order and priority do not guarantee that the student sees only the next mastered-eligible lesson. This conflicts with the intended use of the four-week outline as quantity, not schedule.

**Close with:** persist a course manifest, prerequisites, mastery gates, remediation/review branches, and capstone eligibility; have the overseer assign only the next eligible activity. Until then, use the loader with `--no-assign` and assign one slug at a time for supervised UAT.

## Verification gaps

### Offline validation treats fixture paths as baseline expectations

The validator assumes filesystem checks against fixture paths should pass before student work. Lesson 20 intentionally starts with incorrect fixture content and expects corrected content later, so full materialized validation reports a false baseline failure. `--no-materialize` validates the contract but not that pedagogical transition.

**Close with:** mark checks as `initial`, `studentOutcome`, or both, then validate initial assertions and expected outcome assertions separately. Add an optional reference solution replay in CI.

### Latest-command checks are brittle and incomplete

`command_properties` records the shell wrapper and exact command string, while a task may be solved through an equivalent command. It also sees only the latest command. `pipeline_output` proves output, not how it was produced.

**Close with:** normalized command AST observations, command history predicates, permitted-equivalence rules, explicit pipeline/redirection checks, stderr and exit-status assertions, and task-level evidence aggregation.

### Shell script behavior is mostly inferred from files

The current verifier can inspect script content, mode, and generated files, but cannot directly assert that a named script execution produced those files, remained within allowed paths, or behaved identically across reset/retry.

**Close with:** a `script_execution` check carrying executable, arguments, exit code, stdout/stderr, write-set boundaries, timeout, and generated artifact digests.

### JSON Schema and Go contract can drift

The course schema rejects unknown fields, while Go's JSON unmarshalling accepts them. Check parameter shapes remain open in JSON Schema and are enforced only by Go.

**Close with:** generate JSON Schema from the Go contract or test conformance both ways in CI, reject unknown JSON fields in the loader, and add conditional schemas for every check kind.

## Content and standards gaps

### Standards are too coarse for the course

The current seed has navigation, file organization/inspection, and typing standards only. Pipes, redirection, text processing, operating-system structure, permissions, processes, and Bash scripting are temporarily mapped to the nearest file/navigation standards. That would overstate mastery and weaken reporting.

**Close with:** add narrow standards for OS/filesystem concepts, permissions, search/text pipelines, processes/environment/exit status, and safe introductory scripting. Re-map lessons 4 and 10–20 before production mastery records are accepted.

### Missing commands in the declared learning environment

The outline discusses `file`, `whoami`, `id`, and `ps`, but they are not all in the profile vocabulary or deployed closure. Activities avoid requiring unavailable commands, so those objectives remain conceptual.

**Close with:** add a versioned `linux-fundamentals` runtime capability set containing safe read-only identity, file identification, and process inspection tools, or narrow the formal objectives to the installed set.

### One activity can become oversized

Some generated lessons contain many deterministic checks and long task instructions. The three-pane TUI has not been usability-tested with this amount of content, wrapping, hint volume, and fixture complexity.

**Close with:** physical UAT for all representative activity shapes, scrolling/focus tests, concise task presentation with expandable teaching blocks, and authoring guidance for task/check limits.

## Operational gaps

### The API loader cannot create standards or resolve subject IDs

The LMS parent API can create activities, publish revisions, and assign them, but publication rejects unknown standard codes. The database publisher can seed standards, but it requires direct database access. The loader also relies on activity creation without a subject ID because no subject lookup is needed for assignment, leaving subject linkage dependent on server behavior.

**Close with:** a guarded curriculum-import API that validates complete activity documents, resolves subject codes, reconciles authorized custom standards, publishes transactionally, and returns per-document idempotency results. Until then, seed standards with the deployment-side publisher before using `load.py`.

### Bulk load is not transactional

If a remote request fails halfway through, earlier activities remain published/assigned. Re-running is mostly safe because revisions are content-addressed and open assignments are reused, but operators receive no machine-readable checkpoint or rollback plan.

**Close with:** server-side batch import, dry-run diff, atomic publication where practical, resumable result manifest, and explicit partial-success reporting.

### Student work synchronization lacks complete reconciliation

The client fetches work but does not persist/use a durable cursor or receive cancellation tombstones/full-snapshot semantics. A long course increases the impact of stale assignments.

**Close with:** cursor persistence, tombstones, periodic full reconciliation, and acceptance tests for assignment cancellation, revision replacement, and extended offline operation.

## Recommended UAT slice

Before broad use, publish the course but assign only lessons 1, 6, 13, 16, and 19 to a test student, one at a time. These cover command/output observation, filesystem mutation, redirection, script creation, and capstone-scale content. Verify reset, resume, hints, tutor behavior, completion evidence, mastery attribution, offline restart, and parent-visible results before assigning the full sequence.
