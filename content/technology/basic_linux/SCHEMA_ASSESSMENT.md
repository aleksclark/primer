# Primer activity schema assessment

## Decision

The existing Primer `ActivityDocument` v1 is suitable as the immediate storage and delivery format. The lesson JSON files therefore use its camelCase API representation directly, and the supplied JSON Schema makes that contract explicit and rejects unknown authoring fields.

This choice preserves the working path from a git-authored document to an immutable LMS activity revision, assignment, offline student cache, terminal runner, deterministic checks, event reporting, and standards-linked mastery.

## What fits now

- Stable activity identity, immutable revisions, subject, and standard references.
- Terminal fixtures with safe workspace-relative paths.
- Ordered tasks, prerequisites, deterministic completion trees, hints, tutor boundaries, resume/reset policy, and bounded artifacts.
- A sandbox runtime vocabulary that covers the core file and text tools used by most lessons.
- JSON input is already accepted by Primer's contract loader even though existing examples are YAML.

## Proposed contract changes

Proceed with the course assuming these changes will be made before full production delivery.

### 1. Add course and sequence manifests

Activity documents are intentionally independent, but Primer lacks a committed contract for a course, modules, ordering, prerequisites between activities, estimated content size, and capstone grouping. Add a versioned `CourseDocument` with activity slugs and prerequisite relationships. This repository includes `course.json` as a provisional loader manifest rather than embedding schedule state in each activity.

### 2. Separate instruction from executable tasks

`objective`, `instructions`, and task instructions are plain strings. They can carry concise teaching text but not a well-structured lesson with explanation, examples, vocabulary, reflection prompts, and parent notes. Add versioned instructional blocks such as `explanation`, `example`, `warning`, `question`, and `practice`, or attach a Markdown resource by content hash. The paired Markdown files in this course are currently parent-facing source material; command-teacher does not render them.

### 3. Add conceptual and oral evidence

The verifier can prove filesystem state and the most recent command/output, but cannot directly assess a student's explanation of paths, operating-system roles, permissions, process concepts, or script choices. Add response tasks with rubric-backed short answers and parent attestation. These should produce evidence without allowing an LLM judgment alone to grant mastery.

### 4. Strengthen command and shell verification

`command_properties` observes only the latest command and exact argument list. `pipeline_output` checks latest stdout without proving the requested pipeline produced it. Add command-history predicates, pipeline structure predicates, exit-status checks, stderr checks, and script execution checks. This is especially important for redirection, processes, and Bash lessons.

### 5. Add fixture and reset composition

Every activity carries its complete fixture set, and a reset returns to that isolated state. Add reusable fixture bundles and an explicit project workspace lifecycle so multi-activity capstones can preserve approved artifacts across assignments while ordinary practice remains disposable.

### 6. Align JSON Schema and Go validation

Primer's Go loader currently permits unknown JSON fields, while this JSON Schema rejects them. Generate one representation from the other, or run both in CI, to prevent drift. Add schema-level parameter definitions per check kind rather than leaving `params` open and relying only on Go validation.

### 7. Expand runtime profile semantics

The contract names `text-processing`, but the workstation packages only one combined coreutils profile through `PRIMER_RUNTIME_PROFILE_DIR`. Add independently packaged, versioned runtime profiles and include process inspection tools needed by the process lesson. Activities should declare capabilities, with deployment resolving them to a closure.

### 8. Model mastery sequencing explicitly

An activity's standards are enough to record evidence, but not enough to express prerequisite mastery, remediation branches, required review, or capstone gates. Add course-level mastery gates and let the overseer select the next eligible activity instead of bulk-assigning all nominal days.

## Compatibility notes

- JSON field names must remain camelCase; YAML uses snake_case tags.
- `schemaVersion` remains `"1"` until server and workstation support a new version.
- The activity directory name must match `slug` when using Primer's existing directory loader. The course loader stages each JSON file into that expected layout.
- Standard codes in this course use the current `PRIMER.DL.6.*` namespace and require corresponding LMS standard seeds before publication.
