# Phase 6: Portfolio continuity and curriculum operations

## Purpose

Add deliberate project continuity and dependable curriculum deployment after the core teaching, evidence, sequencing, and offline loop is trustworthy. These capabilities support craftsmanship and parent curation without making every exercise permanent or giving import tools broad unchecked authority.

## Current need

Activity workspaces are isolated. Resume preserves work within one activity, but later activities cannot consume selected prior output. The artifact API currently records metadata; it does not upload, store, scan, approve, or re-materialize bytes. A clean fixture remains preferable for reproducible assessment, while selected authentic projects belong in Primer's portfolio and may need to continue across phases.

Curriculum publication from the database-side publisher resolves subjects and upserts standards. The HTTP loader creates/publishes/assigns documents one request at a time, omits subject resolution, and cannot reconcile an authorized standards bundle as one reviewable operation. Individual revision publication is transactional and content-addressed, but a long batch can stop halfway without a durable result manifest.

## Scope

## A. Portfolio and project continuity

### 1. Real bounded artifact storage

Implement artifact byte upload with:

- reservation and idempotency key;
- content digest and size verification;
- allowed media/type policy from the immutable activity revision;
- per-file, per-session, and per-student quotas;
- malware/content scanning appropriate to deployment;
- encrypted or access-controlled storage;
- retention state and deletion audit;
- parent-visible provenance.

The broker stages uploads from an explicit export directory only. It never uploads arbitrary workspace or host files.

### 2. Artifact review and promotion

Artifacts begin as session evidence. A parent may promote selected artifacts into:

- a portfolio item;
- an active project workspace input;
- a later assignment's approved fixture bundle.

Promotion records source student, session, activity revision, digest, parent decision, and destination. The later assignment references an immutable promoted bundle. Student modifications create new evidence rather than mutating the source artifact.

### 3. Deliberate continuity policy

Course membership declares one of:

- `fresh`: always materialize a clean authored fixture;
- `optional_previous`: offer a parent-approved prior artifact or use a clean fixture;
- `required_project`: require an approved promoted bundle;
- `portfolio_review`: inspect retained evidence without importing it into an executable workspace.

Default to `fresh`. Authentic continuity is used when it improves the project; it is not required merely because two lessons are adjacent.

### 4. Safe materialization and reset

Imported bundles are scanned, bounded, and materialized under a declared workspace path. Symlinks, device files, sockets, absolute paths, traversal, and unsupported modes are rejected. Reset restores the approved bundle, not an unreviewed local state. Parent withdrawal prevents future imports but does not erase historic evidence silently.

### 5. Portfolio surfaces

Parent and student views show:

- artifact preview/metadata;
- source and revision provenance;
- promotion/review status;
- project lineage across activities;
- retention and export controls;
- monthly portfolio review prompts.

Portfolio reflection remains a parent-student activity, not an automated grade.

## B. Guarded curriculum import

### 1. Document-level import contract

Add a parent/admin guarded import API accepting a strict bundle:

- custom standard seeds within authorized namespaces;
- activity documents;
- course documents;
- content digests and bundle version.

The server validates everything before writing:

- strict schemas and semantic references;
- subject code resolution;
- standard existence or authorized custom-standard reconciliation;
- activity/course graph validity;
- runtime capability names;
- evidence-policy compatibility;
- immutable digest/idempotency behavior.

Generic standards CRUD remains available for administration, but course loaders use the safer document-level path.

### 2. Plan and apply

Follow Primer's existing desired-state operational pattern:

- `plan` returns creates, reused digests, new revisions, conflicts, warnings, and unresolved references;
- `apply` uses the exact planned bundle digest and rejects drift;
- no LLM participates in operational reconciliation;
- parent/human review resolves ambiguous or policy-sensitive changes.

### 3. Transaction and resumability

Validate the whole bundle first. Apply all PostgreSQL standards, activities, and course revisions in one database transaction. Artifact object storage is not part of curriculum import and therefore does not weaken this guarantee. Keep student enrollment/assignment as a separate explicit operation so importing content does not surprise students.

Return a durable per-document result manifest with status, IDs, digests, and retryability. Reapplying the same bundle is idempotent. Once immutable revisions are referenced by evidence, later correction converges through new revisions rather than destructive rollback.

### 4. Authorization and audit

- Only parent/admin roles with curriculum-management permission can import.
- Custom standards are restricted to configured namespaces; official standards cannot be overwritten through this path.
- Every import records actor, bundle digest, source label, plan, apply result, and timestamps.
- Secrets and local filesystem paths never enter the bundle.
- Course publication does not automatically enroll or assign students.

### 5. Loader migration

Update `content/technology/basic_linux/load.py` or replace it with a supported client that:

1. validates locally;
2. submits an import plan;
3. displays machine-readable and human-readable diffs;
4. applies by plan digest;
5. optionally enrolls a specified student only through a separate explicit flag/API;
6. writes a local result manifest for audit and resume.

The default remains publish/import only, consistent with parent control and Phase 4 orchestration.

## Implementation slices

1. Choose artifact storage and implement reservation/upload/digest verification.
2. Enforce artifact policy and explicit export directories.
3. Add parent review, portfolio promotion, and lineage.
4. Add approved-bundle materialization and continuity policies.
5. Define strict curriculum import bundle and plan response.
6. Implement subject/standard/activity/course reconciliation.
7. Add apply-by-digest, audit, and durable result manifests.
8. Migrate the Linux loader and exercise optional capstone continuity.

## Tests

- Artifact retries never duplicate bytes or metadata.
- Size/type/quota violations fail before promotion.
- Traversal, symlinks, devices, sockets, and unsafe modes are rejected.
- Only parent-approved artifacts enter later workspaces.
- Fresh-fixture assessment remains available and reproducible.
- Import planning is read-only and deterministic.
- Unknown subjects, standards, runtime profiles, and graph references fail before writes.
- Unauthorized namespace updates and official-standard overwrites fail.
- Applying the same bundle digest is idempotent.
- Import does not assign or enroll students implicitly.
- Partial external failures produce a resumable result manifest without corrupting published revisions.

## UAT

- Complete Linux lesson 19, promote the field-report project, and start an optional-continuity variant of lesson 20.
- Compare it with the clean-fixture lesson 20 and confirm both provenance stories are clear.
- Reset the continued project and verify restoration from the approved digest.
- Import the full Linux standards/activity/course bundle through plan/apply.
- Reapply unchanged content, then change one activity and inspect the revision diff.
- Confirm no student receives work until a parent enrolls the course or pins an activity.
- Review artifacts and import audit records in the parent SPA.

## Exit criteria

- Artifact bytes are stored, bounded, verified, and parent-visible.
- Selected projects can continue through immutable, parent-approved promotion with lineage.
- Fresh reproducible fixtures remain the default.
- Curriculum bundles resolve subjects and authorized standards through a guarded plan/apply API.
- A forced failure at every database write stage leaves zero rows from the new bundle, while repeated successful imports are idempotent and produce byte-stable durable result manifests.
- Importing content never silently assigns it to a student.
- The Linux course can be deployed and optionally continued as a portfolio project without direct database access.

## Out of scope

- Syncing arbitrary student home directories.
- Making all practice permanent.
- Automatic public sharing of student artifacts.
- LLM approval of curriculum imports or portfolio promotion.
- Destructive rollback of immutable revisions already referenced by evidence.
