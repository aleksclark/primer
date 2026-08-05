# Phase 0: Truthful evidence and mastery policy

## Purpose

Prevent the workstation from reporting more than it has actually observed while physical UAT continues. This phase deliberately reduces automation before adding features. A student's ability to reproduce a canned command or produce a filesystem state is useful evidence, but it is not automatically proof of conceptual understanding or formal mastery.

## Current need

The server currently accepts required check observations and increases confidence for every standard linked to the activity revision. The real PTY path does not yet produce trustworthy command completion data: it waits after a newline, labels the observation `pty-shell`, assumes exit code zero, and uses the visible screen as stdout. Conceptual activities also have no persisted short-answer or parent-attestation surface, so some course activities substitute exact `echo` text for explanation.

This creates two integrity risks:

- command-sensitive checks may fail unpredictably or pass for the wrong reason;
- procedural completion may increase mastery confidence for conceptual standards without the required evidence mix.

Both conflict with Primer's emphasis on honesty, precision, parent judgment, and continuous evidence as one assessment layer rather than the whole assessment system.

## Scope

### 1. Classify evidence and mastery effects

Introduce server-owned evidence classes:

- `procedural_continuous`: deterministic outcomes from a paired managed workstation;
- `conceptual_response`: a future persisted student explanation, reserved in the taxonomy but implemented in Phase 3;
- `parent_attestation`: a future educator decision tied to explicit rubric criteria, implemented in Phase 3;
- `formal_assessment`: evidence produced by Primer's assessment subsystem;
- `portfolio`: retained project evidence reviewed in context.

Activity-standard links gain an evidence policy describing which classes are required before a status transition. Activity completion may still record evidence and advance assignment state even when mastery is unchanged.

### 2. Stop unconditional mastery bumps

Change completion handling so it:

1. validates and stores the completion atomically;
2. records the evidence class and provenance;
3. evaluates the standard's evidence policy;
4. updates confidence/status only when the policy permits;
5. returns separate `assignmentCompletion` and `masteryTransitions` results.

Existing v1 terminal activities default to `procedural_continuous`. They may move a not-introduced standard into in-progress when appropriate, but must not independently establish conceptual or formal mastery.

### 3. Mark unreliable command checks during UAT

Until Phase 2, add a server/workstation capability indicating whether structured command evidence is supported. A required check whose kind needs structured command evidence must not pass on a runner lacking that capability. For UAT, re-author representative revisions so required completion relies on trustworthy filesystem outcomes, or mark the unsupported task as non-mastery practice. If every required completion path needs the capability, reject the revision before session start. The development fallback remains explicitly labeled and is never production student evidence.

Do not silently reinterpret screen text as completed-command evidence or weaken checks dynamically without an immutable revised activity policy.

### 4. Correct the Linux standards map

Add narrow custom standards before production course evidence is accepted:

- operating-system and filesystem organization;
- permissions and identity;
- file/content search;
- pipelines and redirection;
- text processing;
- process, environment, and exit-status concepts;
- safe introductory Bash scripting;
- project verification and explanation.

Re-map lessons 4 and 10–20. Preserve primary versus reinforcement roles and keep typing standards isolated from terminal-command mastery.

### 5. Add parent-visible evidence status

The learning overview should distinguish activity completion, procedural evidence accepted, additional evidence required, and formal mastery achieved. Phase 0 does not collect new conceptual responses; it reports that the current evidence policy is unsatisfied and leaves the standard unchanged. Phase 3 adds response review and parent attestation states.

The parent should see the accepted evidence and missing evidence classes, not a black-box score.

## Data and API changes

- Add evidence class and provenance fields to mastery evidence.
- Add a versioned evidence policy to revision-standard links or an associated policy table.
- Reserve policy vocabulary for future review evidence without adding response storage yet.
- Extend completion responses without breaking idempotency.
- Expose parent learning-overview fields for accepted and missing evidence classes.
- Expose a minimum runner-capability flag so incompatible required checks fail before or during a session according to immutable revision policy; full device capability persistence arrives in Phase 5.

Migrations must preserve existing evidence as `procedural_continuous` with a migration note that historic confidence may have been computed under the earlier policy. Do not silently rewrite historic mastery statuses; surface them for review.

## Implementation slices

1. Add evidence taxonomy and persistence.
2. Separate assignment completion from mastery transition in the mastery service.
3. Add conservative default policies and migrate existing records.
4. Add parent evidence-status surfaces.
5. Add minimum runner capability gating for command-sensitive checks.
6. Re-author the Phase 0 UAT revisions around trustworthy filesystem outcomes where possible.
7. Seed and map the Linux standards.
8. Update acceptance scripts to assert truthful statuses.

## Tests

- Completion remains idempotent while assignment and mastery outcomes differ.
- Procedural evidence alone cannot satisfy a conceptual policy.
- Missing conceptual/formal evidence leaves the mastery policy unsatisfied without losing procedural evidence.
- Historic evidence remains visible and can be superseded by later evidence under a newer policy.
- Cancelled assignments never create mastery evidence.
- A runner without structured-command capability cannot start an incompatible revision.
- Typing completion affects only typing standards.
- Existing evidence migration is deterministic and does not silently inflate mastery.

## UAT

Create Phase 0 UAT revisions derived from lessons 1, 6, 13, 16, and 19, removing command-sensitive checks from required completion where trustworthy filesystem outcomes suffice. Assign them one at a time and confirm that:

- filesystem outcomes can complete applicable tasks;
- unsupported required command-sensitive paths block precisely;
- conceptual claims appear as additional-evidence-required rather than mastered;
- the parent sees accepted procedural evidence and missing evidence classes;
- the student sees a precise status rather than a misleading success message.

## Exit criteria

- No student-client completion automatically bumps every linked standard.
- Evidence class, provenance, and missing policy requirements are visible in parent operations.
- Command-sensitive revisions cannot run against the synthetic PTY observation path in production.
- Linux course standards no longer overload broad navigation/file standards for scripting and operating-system concepts.
- Physical UAT demonstrates assignment completion without false mastery claims.

## Out of scope

- Structured shell instrumentation, implemented in Phase 2.
- Rich response authoring and TUI input, implemented in Phase 3.
- Course sequencing, implemented in Phase 4.
- Replacing parent judgment with LLM scoring.
