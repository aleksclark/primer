# Phase 5: Media evidence and asynchronous rubric review

## Goal

Support artifact-based verification and the Android capture workflow. Students
can submit image, video, or audio from camera/recorder/device files through web
or Android. The first agent-powered non-chat driver,
`agent_artifact_rubric`, evaluates an image against a parent-authored,
snapshotted rubric in a durable background job and streams safe progress. The
poem example completes only after the submitted photo satisfies every required
rubric criterion.

Audio/video are fully supported as secure submissions and may use parent or
external review; this phase does not pretend Fantasy or every provider can
natively understand every codec.

## BDD Success Criteria

#### Scenario: Android captures and submits a poem image

- **Given** a poem task requiring one image and a rubric
- **When** the paired student captures a photo in Android, reviews it, uploads it,
  and finalizes the submission
- **Then** bytes are stored under a server-generated tenant/student key in object
  storage, PostgreSQL stores metadata/digest/state only, and the attempt enters
  evaluating
- **And** the parent/student can view an authorized derivative, not an arbitrary
  object-store URL.

#### Scenario: Asynchronous rubric acceptance completes the task

- **Given** a finalized image and required rubric criteria
- **When** the Fantasy multimodal evaluation job runs
- **Then** the client sees queued/loading/evaluating progress without opening a
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

#### Scenario: Web and Android accept allowed media sources

- **Given** requirements allowing image, video, and audio within declared limits
- **When** students use web file input, Android camera, Android video capture,
  Android audio recording, or Android system file picker
- **Then** accepted media upload/finalize with correct type, size, digest, and
  duration metadata
- **And** unsupported type, oversize, duration, corrupt/truncated, or digest-
  mismatch uploads are rejected before evaluation.

#### Scenario: Upload replay and interruption are safe

- **Given** a reserved multipart/presigned upload
- **When** the app loses network, retries chunks/finalize, or the server restarts
- **Then** idempotency returns the same artifact/submission or resumes safely,
  exactly one finalized object is referenced, and orphaned partials expire
- **And** finalization cannot substitute a different tenant/object/digest.

#### Scenario: Artifact authorization and privacy are enforced

- **Given** tenant A artifact and tenant B parent/student credentials, an expired
  download URL, or a revoked device
- **When** they request original/derivative bytes or evaluation state
- **Then** access is denied without an existence oracle
- **And** object keys, signed URLs, logs, and model requests do not expose another
  tenant or long-lived credentials.

#### Scenario: Progress is safe and disconnect-tolerant

- **Given** an evaluation job running without chat
- **When** web/Android disconnects and reconnects
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
   Compose with named data volume and internal DNS; expose only the endpoint
   needed by browser/device direct upload through Stacklane.
2. Add `artifacts`, upload reservations/parts (as needed), submission links,
   derived artifacts, scan/validation status, and retention/tombstone records.
   PostgreSQL stores no artifact bytes. Object keys are server-generated from
   tenant-scoped opaque IDs, never filenames or client paths.
3. Implement reserve → upload → finalize:
   - requirement controls allowed media types, max bytes/count/duration;
   - use short-lived least-authority signed URLs or bounded streaming endpoints;
   - finalization verifies object ownership, size, content sniff, cryptographic
     digest, and image/media decode metadata;
   - stable idempotency keys make retries safe;
   - a janitor removes expired partials/orphans after a documented grace period.
4. Generate authorized display/download derivatives (thumbnail, stripped image
   metadata) in a durable job. Remove EXIF/GPS from derivatives and define whether
   originals are retained. Parent-only original access and student derivative
   access are separate capabilities.
5. Register `agent_artifact_rubric`. Configuration includes accepted artifact
   kinds, rubric criteria with stable IDs/descriptions/required flags, pass rule,
   retry/review policy, and optional student feedback style. Snapshot it with the
   occurrence.
6. Build a no-chat Fantasy job that loads only the authorized artifact bytes/
   derivative and rubric. Use `AgentCall.Files`/provider adapter after a phase
   qualification fixture proves the selected Bedrock model path. The only
   mutation tool records structured criterion evaluations; the generic engine
   validates criterion completeness and commits decision/completion.
7. For video/audio, persist and preview submissions now. Do not pass unsupported
   codecs blindly to Fantasy. Require a compatible driver capability or route to
   `parent_approval`/phase-6 external verifier. Record this explicitly in task
   form capability checks.
8. Add media safety: filename sanitization, content sniffing, decode bounds,
   image pixel bomb limits, video/audio duration limits, object encryption/TLS
   assumptions, short URL TTLs, CSP, and optional malware scanner hook. Scanner
   unavailable must be an explicit policy state, not silent clean.
9. Add parent rubric authoring/preview and artifact/evaluation inspector. Add
   student web upload/capture (where browser APIs allow), progress, criterion
   feedback, retry, and complete states using System C.
10. Add Android CameraX image/video capture, audio recording, Storage Access
    Framework picker, permission rationale/denial states, review/delete before
    upload, progress/cancel/retry, background-safe upload, and cache cleanup.
    Never place tokens in media metadata or exported filenames.
11. Extend REST/WS generated clients for typed artifact reservations,
    finalization, submissions, evaluation events, and errors. Binary PUT/GET may
    use a narrowly allowlisted transport helper in the client façade; pages and
    screens may not construct arbitrary URLs/fetches.
12. Add a scripted multimodal Fantasy model that asserts it received the expected
    file digest/media type and emits criterion tool calls. Add an opt-in
    `TASKS_LIVE_MODEL_TEST=1` provider qualification using non-sensitive fixtures;
    record provider/model/cost and never run it accidentally in default CI.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent authors/schedules the poem + image rubric. Student web uploads a valid
  image; watch no-chat progress; inspect accepted criteria and checked state.
- Repeat with failing image, retry, interrupted upload, duplicate finalize,
  malformed type, oversize/pixel bomb fixture, expired URL, and foreign tenant.
- Upload allowed audio/video and verify secure stored/preview/parent-review state
  without false automatic understanding claims.
- Inspect network/object store/DB/logs/EXIF/URLs for scope and secret leakage.

### Android emulator acceptance

- On a fresh emulator state, exercise camera photo, video capture, audio record,
  and system file picker against the real stack. Verify permission denial and
  recovery, review/delete, upload progress, network interruption/retry, app
  process death, cache cleanup, and final parent-visible artifact.
- Complete the poem image rubric and observe checked state. Revoke the device
  during upload/evaluation and verify no further unauthorized operations.

### Promoted automation

- After exploratory PASS, add Playwright specs for rubric authoring, web upload,
  non-chat progress, accept/reject/retry, download auth, malformed/oversize,
  reconnect, and two-tenant object isolation.
- Promote Android flows to emulator-backed UI tests; use emulator virtual camera
  fixtures and media files, not a mocked camera screen. Include process restart
  and network shaping where harness support permits.
- Add S3-compatible process E2E for reserve/upload/finalize replay, checksum
  mismatch, orphan cleanup, derivatives, object auth, worker restart, and exactly
  one decision.
- Add default scripted Fantasy artifact tests and separately run/record controlled
  live image qualification before making a real image-review claim.

Commands:

```bash
make tasks-test tasks-cover tasks-clients tasks-web tasks-android tasks-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
TASKS_LIVE_MODEL_TEST=1 make tasks-live-image-e2e   # explicit credentials/approval only
```

## Anti-Cheating Audit

- Trace real bytes from browser/emulator through reservation/object store/finalize
  to the Fantasy provider adapter and criterion decision; reject metadata-only
  fake uploads or hard-coded rubric results.
- Verify bytes are absent from PostgreSQL and object keys/downloads are tenant-
  scoped server-side. Test direct object URL and foreign signed URL attacks.
- Inspect content validation, digest, pixel/duration bounds, derivative metadata
  stripping, partial cleanup, and revoke behavior; reject trusting extension or
  client MIME alone.
- Trace rubric completion through criterion rows and generic policy; reject model
  prose or job status directly checking the task.
- Confirm evaluation job is durable/detached and reconnect is real. Reject an
  in-request goroutine or socket-bound task.
- Inspect Android tests for real camera/file system boundaries and real server;
  MockWebServer/unit tests are supplemental only.
- Confirm audio/video automatic-review claims are capability-gated and unsupported
  media cannot silently pass.
- Keep scripted and live evidence labels distinct; a scripted model cannot close
  the controlled live image qualification.

## Completion Gate

- [ ] Image/video/audio submission works from required web/Android sources with real object storage.
- [ ] Poem image rubric accept/reject/retry works through durable no-chat Fantasy evaluation.
- [ ] Dedicated browser and Android exploratory agents PASS before promotion.
- [ ] Playwright and emulator media suites are green.
- [ ] S3-compatible replay/restart/auth/cleanup process tests pass.
- [ ] Controlled live image qualification is PASS or explicitly BLOCKED; no unqualified quality claim is made.
- [ ] Privacy, derivative, retention, generated-client, Go/web/Android/coverage/build/diff gates pass.
- [ ] Anti-cheating audit finds no fake bytes/evaluation, DB blob, object leak, or unsupported-media overclaim.
