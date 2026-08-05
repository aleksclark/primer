# Phase 3: Instruction and human-reviewed responses

## Purpose

Deliver the curated teaching material that surrounds practice, and collect conceptual evidence without pretending deterministic output or an LLM can replace student articulation and parent judgment.

## Current need

Activity revisions currently carry one objective, one instruction string, task instructions, hints, and tutor context. The Linux course's Markdown briefs contain explanations, vocabulary, examples, warnings, misconceptions, guided practice, and oral evidence expectations that never reach the student client. Long task strings are a poor substitute and are difficult to navigate in the three-pane TUI.

Terminal checks can prove that a student created the requested state. They cannot prove that the student understands why `/etc` differs from `/var`, what execute permission means on a directory, or why a pipeline is ordered. Exact `echo` responses reward reproduction rather than explanation.

## Scope

### 1. Typed instructional blocks

Extend immutable activity content with ordered, typed blocks:

- `prose` for concise explanation;
- `vocabulary` for term-definition pairs;
- `example` with input, output, and explanation fields;
- `warning` for safety or destructive-command boundaries;
- `question` for prediction or reflection before action;
- `practice` for ungraded guided work;
- `parent_note` excluded from the student view;
- `resource` for a content-addressed, approved local attachment when truly necessary.

Use a small rendering model rather than treating arbitrary Markdown/HTML as executable presentation. Limited Markdown inside prose may be supported, but the serialized contract remains typed and sanitizable.

### 2. Student TUI reading experience

Add a dedicated lesson view or layered instruction pane with:

- keyboard-only navigation and visible focus;
- scrolling, section index, and return-to-current-task behavior;
- progress through instructional blocks without calendar timers;
- examples and warnings that remain readable at workstation terminal sizes;
- offline availability from the immutable cached revision;
- no browser launch for ordinary instruction.

Do not turn the TUI into a visually busy web lesson. Text remains primary because it develops abstraction and typing discipline.

### 3. Response tasks

Add activity task kinds beyond terminal and typing actions:

- short constructed response;
- structured claim/evidence/reasoning response;
- prediction followed by observation comparison;
- oral response requiring parent attestation;
- project defense linked to retained evidence.

Responses are persisted locally before sync, versioned, bounded, and tied to activity/task IDs. Editing history may be retained as evidence of revision, within privacy and size limits.

### 4. Rubrics and review workflow

A response task carries explicit rubric criteria authored with the revision. The parent can:

- accept evidence against named criteria;
- return it with a concise reason;
- request another attempt;
- attest to an oral explanation;
- mark a criterion not applicable with an auditable explanation.

The parent SPA presents the student's response, relevant deterministic evidence, and the rubric together. It must not encourage approval without reviewing the work.

### 5. Tutor role

The tutor may:

- ask Socratic follow-ups;
- point to an instructional block or graduated hint;
- identify missing parts of the rubric;
- help the student revise before submission.

The tutor may not:

- write the final response;
- treat text it generated for the student as student evidence;
- expose hidden parent notes or reference solutions.

Tutor and guard evaluation remains an important continuous-assessment layer. Repeated, auditable tutor/guard observations may contribute `conceptual_response` evidence when the server loads the authoritative rubric, retains the student's original response, records the model/policy version, and requires consistency across attempts. Evidence policy decides whether that is sufficient for a transition. Parent attestation remains mandatory for activities explicitly marked oral, safety-critical, project-defense, or parent-review-required; it is not required for every conceptual response.

A deterministic fallback remains available when the configured model is unavailable, but it may request revision or parent review rather than inventing a score.

### 6. Evidence and mastery integration

Response submission creates `conceptual_response` evidence. Depending on the immutable evidence policy, it enters tutor/guard evaluation, parent-review-needed state, or both. Parent acceptance creates or links `parent_attestation` evidence. Phase 0 evidence policies decide which repeated continuous observations and human decisions are sufficient for a mastery transition.

Procedural checks and conceptual responses remain separate in parent reporting. Completing one does not fabricate the other.

### 7. Authoring migration

Provide a tool to convert the Linux course's lesson briefs into reviewable typed block drafts. Human review is mandatory before publication. Do not parse arbitrary headings at runtime or make the platform depend on separate unversioned Markdown files.

## Data and API changes

- Add instructional blocks and response-task definitions to the activity contract.
- Add response drafts/submissions with immutable submitted versions.
- Add rubric criteria and review decisions.
- Extend student sync/outbox for offline response submission.
- Add parent review list/detail/decision endpoints.
- Add parent and student status summaries without exposing hidden content.

Responses should be stored as evidence, not mixed into generic chat transcripts.

## Implementation slices

1. Define typed blocks and strict schemas.
2. Add TUI lesson navigation and accessibility behavior.
3. Add response task runner and durable local drafts.
4. Add submission API and idempotent sync.
5. Add rubric persistence and parent review APIs.
6. Add parent SPA review workflow.
7. Integrate tutor constraints and evidence policy.
8. Migrate and review representative Linux lessons.

## Tests

- Blocks survive publication, API delivery, cache, and offline rendering unchanged.
- Unsafe links/markup and oversized resources are rejected.
- TUI scrolling/focus works at 80x24 and 120x30, with every block and control reachable by keyboard and no clipped required content.
- Response drafts survive process termination and reboot.
- Duplicate submission and review requests are idempotent.
- A tutor cannot submit or approve a response.
- Parent decisions are authorized and auditable.
- Returned work remains available for revision without overwriting prior evidence.
- Conceptual mastery cannot occur without the configured review evidence.

## UAT

Use lessons 1, 4, 10, 13, 15, and 20. Confirm that the student can:

- read explanation and vocabulary without leaving the TUI;
- predict, perform, observe, and explain;
- submit a response offline and see `awaiting sync`;
- receive a parent return reason and revise;
- complete an oral explanation with parent attestation;
- distinguish procedural completion from conceptual review status.

## Exit criteria

- Curated instruction reaches the student as part of the immutable revision.
- The TUI handles course-sized teaching content without oversized task strings.
- Conceptual responses and oral attestations are durable, reviewable evidence.
- Parent review is required where evidence policy says it is required.
- Tutor/guard evaluation contributes only auditable continuous evidence under an explicit policy; generated coaching text cannot be submitted as student evidence.
- Representative Linux lessons no longer use canned `echo` text as the conceptual assessment.

## Out of scope

- Automatic essay scoring as the final authority.
- Replacing periodic formal assessments.
- General-purpose HTML/web lesson rendering.
- Student-selected curriculum outside parent-approved courses.
