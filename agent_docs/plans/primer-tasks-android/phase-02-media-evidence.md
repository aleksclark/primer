# Android Phase 2: Media evidence and rubric

## Goal

Add native image, video, and audio evidence submission after the reviewed main
Phase 5 API is stable. Students capture or select bounded media, review/delete it
before upload, safely resume interrupted transfers, and observe durable no-chat
rubric progress/results without giving the client evaluation authority.

## BDD Success Criteria

#### Scenario: Capture and submit poem image

- **Given** an image-rubric occurrence
- **When** the student captures a photo, reviews it, uploads/finalizes it, and the
  server evaluator accepts the rubric
- **Then** the app shows durable progress/criteria/completion
- **And** no object credential, token, GPS/EXIF, or arbitrary storage URL leaks.

#### Scenario: Select image/video/audio from files

- **Given** media requirements and allowed limits
- **When** the student uses the system picker for an allowed file
- **Then** type/size/digest/duration metadata finalize correctly
- **And** unsupported, oversized, truncated, mismatched, or pixel-bomb inputs are
  rejected before evaluation.

#### Scenario: Record video and audio safely

- **Given** granted camera/microphone permissions
- **When** the student records within configured bounds
- **Then** recording stops at the bound, offers review/delete, and uploads only
  after explicit confirmation
- **And** denial/revocation/partial recording has a recoverable UI with no upload.

#### Scenario: Upload interruption and process death recover

- **Given** an active upload
- **When** network drops or the process is killed
- **Then** the app resumes/retries idempotently from durable reservation state,
  never references duplicate finalized objects, and cleans abandoned local media.

#### Scenario: Failed rubric supports retry/review

- **Given** a rejected or provider-unavailable image evaluation
- **When** the result arrives after reconnect
- **Then** the task remains incomplete, safe criterion feedback/parent-review state
  is visible, and a new submission does not rewrite prior evidence.

#### Scenario: Revocation blocks media operations

- **Given** an upload/evaluation in progress
- **When** the device credential is revoked
- **Then** further reserve/upload/finalize/download/state requests fail closed,
  local sensitive cache is removed, and parent-visible server evidence remains.

## Implementation Instructions

1. Consume reviewed artifact reservation/finalize/submission/evaluation contracts
   through generated Kotlin façades. Keep binary upload/download in one bounded
   transport helper; screens never construct object URLs.
2. Add CameraX photo/video, MediaRecorder (or reviewed equivalent) audio, and
   system picker flows with permission rationale/denial/retry and explicit review
   before upload.
3. Enforce local preflight limits for bytes/count/duration/pixels/content sniffing
   as UX; server validation remains authoritative. Never trust filename extension.
4. Use server-generated opaque names; strip/avoid location metadata for derived
   previews and never embed bearer/task/student identity into media metadata.
5. Persist bounded upload reservation/part/finalize idempotency state and use a
   supervised background worker. Process/background loss never implies success.
6. Add progress/cancel/retry, scan/evaluation waiting, criterion feedback,
   parent-review, rejected, and completed System C screens. Raw reasoning/tool
   inputs never render.
7. Encrypt/limit local temporary media, prevent backups/export where required,
   and remove successful/expired/canceled media and stale worker state.
8. Add unit/Compose tests for permission, bounds, metadata, reducer/state,
   interrupted/resumed idempotency, revocation, cleanup, and unsupported-media
   capability routing.

## End-to-End Test Plan

- Against real Stacklane/PostgreSQL/object storage, exercise camera photo, video,
  audio, and system file picker on a fresh emulator.
- Author/schedule the poem rubric in parent web; capture/upload a valid image;
  observe no-chat evaluation and completion; parent views authorized derivative.
- Repeat failing image/retry, permission denial/recovery, malformed/oversize,
  interrupted upload, process death/reopen, duplicate finalize, and expired URL.
- Upload allowed audio/video and verify stored/preview/parent-review state without
  automatic-understanding overclaim.
- Revoke during upload/evaluation and inspect app storage, logcat, backup, shared
  files, and media metadata for leakage/cleanup.
- After exploratory PASS, promote to connected UI plus black-box camera/file/
  network/process automation using real server/object boundaries.

Commands:

```bash
cd primer-tasks/android
./gradlew testDebugUnitTest assembleDebug connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Trace real emulator bytes through generated reservation/binary/finalize paths to
  object storage and server evaluation; reject metadata-only fake uploads.
- Verify no media bytes in PostgreSQL, no arbitrary object URL, and tenant-scoped
  download/state authorization.
- Inspect extension-independent validation, digest/pixel/duration bounds, EXIF/
  GPS handling, partial cleanup, and revoked credential behavior.
- Ensure job/prose state cannot locally check off a task; only server decision can.
- Confirm camera/file tests use real platform surfaces and real server; local
  doubles are supplemental only.

## Completion Gate

- [ ] Image/video/audio capture or selection and real object-store submission pass.
- [ ] Poem rubric accept/reject/retry and durable progress pass.
- [ ] Permission, interruption, process-death, duplicate-finalize, revocation, and cleanup pass.
- [ ] Dedicated exploratory PASS precedes emulator automation.
- [ ] Promoted native suites and privacy/metadata scans are green.
- [ ] Fresh anti-cheat review approves real-byte and server-decision boundaries.
