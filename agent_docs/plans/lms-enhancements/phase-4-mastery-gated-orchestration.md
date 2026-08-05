# Phase 4: Mastery-gated course orchestration

## Purpose

Turn an ordered collection of activities into a parent-approved, mastery-gated course while extending Primer's existing overseer rather than creating a second scheduler. The four-week Linux outline remains a content-volume reference; no student advances because a date arrived.

## Current need

Primer already stores curricula and ordered curriculum-standard relationships, and the student-client overseer already prefers reinforcement-due standards before selecting an uncompleted published activity. However, its fallback selection is global published activity order, not membership in a chosen course. The git course manifest is validated but not persisted. The loader compensates by assigning all lessons, which can expose later work before prerequisites or review requirements are satisfied.

The missing capability is not sequencing in general. It is explicit course membership, prerequisite eligibility, enrollment-level policy, and parent control over the overseer's choices.

## Scope

### 1. Course persistence

Extend the existing `curricula`, `curriculum_standards`, and `enrollments` model; it remains authoritative. Add immutable curriculum revisions and activity membership rather than creating a parallel learning-course/enrollment aggregate. Existing mutable curriculum rows become stable identities, current enrollments are migrated to an explicit revision selected by the parent or a documented default, and existing curriculum-standard sequencing remains part of that revision.

Add:

- stable course slug and immutable published revisions;
- title, subject, description, and parent-curated status;
- ordered activity revision references or activity slugs resolved at publication;
- prerequisite graph;
- module and capstone markers;
- evidence/mastery gates for entry and completion;
- remediation and reinforcement branches;
- estimated effort as descriptive metadata only, never an automatic date gate.

Course publication resolves immutable activity revisions so later source edits cannot silently change an enrolled student's course.

### 2. Enrollment and parent controls

Extend the existing student curriculum enrollment to store:

- selected course revision;
- active/paused/completed state;
- parent priority and optional pin;
- allowed pace constraints such as daily workload ceilings, not fixed advancement dates;
- current eligible set and blocking reasons;
- parent overrides with reason and audit trail.

The parent can enroll, pause, resume, pin a next activity, approve a remediation branch, or override a prerequisite. Overrides are explicit because the parent is the primary educator, but Primer shows what evidence or prerequisite is being bypassed.

### 3. Eligibility engine

An activity becomes eligible when:

- it belongs to an active enrolled course revision;
- prerequisite activities and standard policies are satisfied;
- required parent reviews are complete;
- no incompatible open assignment exists;
- runner capabilities satisfy the revision;
- capstone entry conditions are met.

Eligibility may include multiple valid activities. The overseer chooses among them using parent pins, active project context, reinforcement due state, workload balance, and deterministic tie-breaking.

### 4. Extend the existing overseer

Refactor `AssignNext` to accept course/enrollment context. Selection order:

1. explicit authorized parent pin or slug;
2. required remediation created by returned evidence;
3. due reinforcement that is eligible within the course or parent-approved cross-course integration;
4. next eligible course activity;
5. no assignment, with a clear blocking reason.

Do not select globally by slug when a student has active course enrollments. Preserve a controlled fallback only for explicitly unsequenced practice libraries.

### 5. Assignment lifecycle

Assignments retain the exact course revision, membership entry, and selection reason. Completion updates enrollment progress but does not itself imply mastery. If evidence is returned, the assignment or a remediation activity can reopen according to policy.

Course revision updates never mutate active enrollments silently. A parent sees a diff and chooses whether to keep the current revision or migrate unstarted membership entries.

### 6. Reinforcement and spiral integration

Course progression must preserve Primer's active reinforcement:

- mastered standards remain candidates for later reinforcement roles;
- integration in a new context is preferred over isolated drills;
- due reinforcement may interrupt nominal order when prerequisites remain satisfied;
- repeated failure lowers confidence and can open remediation;
- completed course status does not stop future reinforcement evidence.

### 7. Parent and student surfaces

Parent SPA:

- course catalog and revision diff;
- enrollment controls;
- course map showing eligible, blocked, assigned, completed, review-needed, and mastered states;
- blocking reasons and override actions;
- reinforcement and remediation rationale.

Student TUI:

- only eligible assigned work;
- concise reason for the next activity;
- no calendar countdown implying automatic progression;
- clear waiting state when parent review or synchronization is required.

## Data and API changes

- Course revision and course-activity membership tables.
- Prerequisite/gate representation with referential integrity.
- Student course enrollments and auditable overrides.
- Assignment provenance linking course membership and selection reason.
- Minimal course revision publication APIs for already validated/resolved activities, initially parent/admin guarded. Phase 6 owns bundle plan/apply, subject/standard reconciliation, and resumable import operations.
- Eligibility preview endpoint for parent UI and tests.
- Overseer result including candidates considered and deterministic reason codes.

Reuse existing curricula where semantics align; avoid duplicating subject, standard, or enrollment data merely to fit the first Linux course.

## Implementation slices

1. Map the Phase 1 course contract onto existing curriculum, curriculum-standard, and enrollment tables; decide and test the authoritative migration before adding tables.
2. Persist immutable course revisions and membership.
3. Add enrollment and override APIs.
4. Implement eligibility queries with blocking reasons.
5. Extend `AssignNext` and retain reinforcement behavior.
6. Add course progress and parent controls.
7. Change course loader default to publish-only and enroll/assign through LMS APIs.
8. Import the Linux course and run migration/revision tests.

## Tests

- Course graphs reject cycles, missing activities, and duplicate order.
- Nominal effort metadata never unlocks an activity by date.
- Unmet procedural, conceptual, review, or capability gates block assignment with the right reason.
- Parent override is authorized, auditable, and visible.
- Reinforcement can be selected without violating prerequisites.
- Existing open assignments are reused rather than duplicated.
- Course revision updates do not mutate active enrollments, and every pre-existing enrollment migrates deterministically to one explicit curriculum revision.
- Returned evidence creates remediation according to policy.
- Two students in the same course can progress differently.
- Global unsequenced fallback cannot leak unrelated activities into an enrolled course.

## UAT

Enroll a test student in the Linux course. Confirm:

1. only lesson 1 is initially eligible;
2. completing procedural work without required conceptual review does not unlock a gated successor;
3. parent acceptance unlocks the next eligible activity;
4. a returned explanation opens remediation;
5. reinforcement can appear between nominal lessons;
6. pausing the course prevents new assignments but preserves evidence;
7. parent pin/override shows its rationale;
8. the student never sees all 20 lessons merely because they were published.

## Exit criteria

- Git course order and prerequisites are represented by an immutable LMS course revision.
- The existing overseer selects only eligible work for enrolled students.
- Parent controls and overrides are explicit and auditable.
- Progress is driven by evidence and review, not dates or bulk assignment.
- Reinforcement remains integrated with course progression.
- The Linux course can be enrolled once and advance one eligible activity at a time.

## Out of scope

- Fully autonomous yearly planning without parent direction.
- Calendar-driven lockstep pacing.
- Replacing standards mastery with course completion percentages.
- Letting students self-enroll in unapproved content.
