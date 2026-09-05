# Phase 5: Web media evidence and asynchronous rubric review

## Goal

Support artifact-based verification in the standalone web product. Students can
submit image, video, or audio files through the student SPA. The first
agent-powered non-chat driver, `agent_artifact_rubric`, evaluates an image
against a parent-authored, snapshotted rubric in a durable background job and
streams safe progress. The poem example completes only after the submitted image
satisfies every required rubric criterion.

Audio/video are fully supported as secure web submissions and may use parent or
external review; this phase does not pretend Fantasy or every provider can
natively understand every codec. Native camera, recorder, and device-file work is owned by a separate
continuation plan and does not block this phase.

## BDD Success Criteria

#### Scenario: Student submits a poem image from the web

- **Given** a poem task requiring one image and a rubric
- **When** the paired student selects/reviews an image in the web SPA and
  finalizes the upload
- **Then** bytes are stored under a server-generated tenant/student key in object
  storage, PostgreSQL stores metadata/digest/state only, and the attempt enters
  evaluating
- **And** the parent/student can view an authorized derivative, not an arbitrary
  object-store URL.

#### Scenario: Asynchronous rubric acceptance completes the task

- **Given** a finalized image and required rubric criteria
- **When** the Fantasy multimodal evaluation job runs
- **Then** the browser sees queued/loading/evaluating progress without opening a
  chat, each criterion receives structured evidence and safe rationale, and one
  accepted decision completes the occurrence
- **And** the result records artifact digest, rubric revision, provider/model,
  policy, and usage provenance.

#### Scenario: Failed rubric remains incomplete and reviewable

- **Given** an image that misses a required criterion or cannot be evaluated
- **When** evaluation finishes
- **Then** the requirement is rejected or moved to explicit parent review per
  configured policy, the task remains unchecked, and the student receives
  age-appropriate next steps
- **And** retrying with a new image creates a new submission/attempt without
  rewriting prior evidence.

#### Scenario: Browser accepts allowed media files

- **Given** requirements allowing image, video, and audio within declared limits
- **When** students use bounded web file inputs (or standards-based browser
  capture where available)
- **Then** accepted media upload/finalize with correct type, size, digest, and
  duration metadata
- **And** unsupported type, oversize, duration, corrupt/truncated, or digest-
  mismatch uploads are rejected before evaluation.

#### Scenario: Upload replay and interruption are safe

- **Given** a reserved multipart/presigned upload
- **When** the browser loses network, retries chunks/finalize, or the server
  restarts
- **Then** idempotency returns the same artifact/submission or resumes safely,
  exactly one finalized object is referenced, and orphaned partials expire
- **And** finalization cannot substitute a different tenant/object/digest.

#### Scenario: Artifact authorization and privacy are enforced

- **Given** tenant A artifact and tenant B parent/student credentials, an expired
  download URL, or a revoked student browser session
- **When** they request original/derivative bytes or evaluation state
- **Then** access is denied without an existence oracle
- **And** object keys, signed URLs, logs, and model requests do not expose another
  tenant or long-lived credentials.

#### Scenario: Progress is safe and disconnect-tolerant

- **Given** an evaluation job running without chat
- **When** the browser disconnects and reconnects
- **Then** the job continues, durable progress/final state replays, and one
  terminal result appears
- **And** raw model reasoning and raw tool inputs are never streamed.

#### Scenario: Controlled live image qualification is honest

- **Given** an approved configured multimodal provider and a controlled image
  fixture with no student data
- **When** the opt-in live qualification runs
- **Then** the production Fantasy/provider adapter transmits the image, receives
  structured rubric output, and passes required positive/negative fixtures
- **And** scripted-model gates remain labeled wiring evidence rather than image-
  understanding proof.

## Implementation Instructions

1. Add an object-store interface with filesystem implementation for narrow unit
   tests and S3-compatible implementation for Compose/deploy. Add MinIO to
   Compose with a named data volume and internal DNS; expose only the endpoint
   needed by browser direct upload through Stacklane.
2. Add artifacts, upload reservations/parts, submission links, derivatives,
   scan/validation status, and retention/tombstone records. PostgreSQL stores no
   artifact bytes. Object keys are server-generated from tenant-scoped opaque
   IDs, never filenames or client paths.
3. Implement reserve → upload → finalize with requirement-owned media limits,
   short-lived least-authority signed URLs or bounded streaming, ownership/size/
   content/digest/decode validation, stable idempotency keys, and orphan cleanup.
4. Generate authorized thumbnails/derivatives with stripped image metadata.
   Parent-only original access and student derivative access are separate
   capabilities with explicit retention rules.
5. Register `agent_artifact_rubric`. Configuration includes accepted artifact
   kinds, stable rubric criteria, required flags/pass rule, retry/review policy,
   and optional student feedback style. Snapshot it with each occurrence.
6. Build a no-chat Fantasy job that loads only authorized bytes/derivatives and
   rubric. Use `AgentCall.Files` only after provider-path qualification. The only
   mutation tool records structured criterion evaluations; the generic engine
   validates completeness and commits the decision.
7. Persist/preview video and audio submissions. Unsupported codecs or provider
   capabilities route visibly to parent/external review and never silently pass.
8. Enforce filename/content/decode/pixel/duration bounds, encryption/TLS
   assumptions, short URL TTLs, CSP, and explicit malware-scanner policy state.
9. Add parent rubric authoring/preview and artifact/evaluation inspector. Add
   student web upload, progress, criterion feedback, retry, and completion views
   using System C.
10. Extend generated REST/WS TypeScript clients. Binary PUT/GET may use one
    narrowly allowlisted helper inside the client façade; pages never construct
    arbitrary transport URLs.
11. Add a scripted multimodal model that asserts expected digest/media type and
    emits criterion tool calls. Add opt-in `TASKS_LIVE_MODEL_TEST=1` qualification
    with non-sensitive fixtures and explicit provider/model/cost evidence.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent authors/schedules the poem + image rubric. Student uploads a valid image,
  observes no-chat progress, and sees the accepted criteria/checkmark.
- Repeat with a failing image, retry, interrupted upload, duplicate finalize,
  malformed type, oversize/pixel-bomb fixture, expired URL, and foreign tenant.
- Upload allowed audio/video and verify secure stored/preview/parent-review state
  without false automatic-understanding claims.
- Inspect network/object store/DB/logs/EXIF/URLs for scope and secret leakage.

### Promoted automation

- After exploratory PASS, add Playwright specs for rubric authoring, web upload,
  non-chat progress, accept/reject/retry, download auth, malformed/oversize,
  reconnect, and two-tenant object isolation.
- Add S3-compatible process E2E for reserve/upload/finalize replay, checksum
  mismatch, orphan cleanup, derivatives, auth, worker restart, and exactly one
  decision.
- Add default scripted Fantasy artifact tests and separately run/record controlled
  live image qualification before making a real image-review claim.

Commands:

```bash
make tasks-test tasks-cover tasks-clients tasks-web tasks-e2e
TASKS_LIVE_MODEL_TEST=1 make tasks-live-image-e2e   # explicit credentials/approval only
```

## Anti-Cheating Audit

- Trace real browser-uploaded bytes through reservation/object store/finalize to
  the Fantasy adapter and criterion decision; reject metadata-only fake uploads
  or hard-coded rubric results.
- Verify bytes are absent from PostgreSQL and object keys/downloads are tenant-
  scoped server-side. Test direct-object and foreign signed-URL attacks.
- Inspect content validation, digest, pixel/duration bounds, derivative metadata
  stripping, partial cleanup, and revoked-session behavior.
- Trace rubric completion through criterion rows and generic policy; reject model
  prose or job status directly checking the task.
- Confirm evaluation jobs are durable/detached and reconnect is real.
- Confirm audio/video automatic-review claims are capability-gated.
- Keep scripted and live evidence labels distinct; scripted evidence cannot close
  controlled live image qualification.

## Completion Gate

- [ ] Image/video/audio web submission works with real object storage.
- [ ] Poem image rubric accept/reject/retry works through durable no-chat evaluation.
- [ ] Dedicated browser exploratory PASS precedes Playwright promotion.
- [ ] Playwright media suite is green.
- [ ] S3-compatible replay/restart/auth/cleanup process tests pass.
- [ ] Controlled live image qualification is PASS or explicitly BLOCKED with no overclaim.
- [ ] Privacy, derivative, retention, generated-client, Go/web/coverage/build/diff gates pass.
- [ ] Anti-cheating audit finds no fake bytes/evaluation, DB blob, object leak, or unsupported-media overclaim.
