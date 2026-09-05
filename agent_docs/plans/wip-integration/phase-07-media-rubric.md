# Phase 07: Integrate media rubric verification

## Goal

Land donor Phase 5 snapshot (`e196dd2c`) so student web clients submit
image/video/audio evidence to real S3-compatible storage and receive durable,
asynchronous, no-chat rubric evaluation. The phase includes authoritative media
probing, reservation/finalization fencing, retry, privacy, derivatives, and
retention while leaving native media capture to the Android continuation.

## BDD Success Criteria

### Scenario: Valid media is stored and evaluated authoritatively

- **Given** an artifact requirement and a paired student
- **When** the student reserves, uploads, and finalizes valid image, audio, or
  video through public generated-client boundaries
- **Then** object bytes reside in S3-compatible storage rather than PostgreSQL
- **And** authoritative server probing queues one durable rubric evaluation whose
  progress/result is visible after reload.

### Scenario: Poem image accept, reject, and retry preserve provenance

- **Given** the controlled poem-image rubric fixture
- **When** accepted, rejected, and retried submissions are evaluated
- **Then** each decision references the immutable artifact digest, criterion,
  evaluator configuration, and attempt
- **And** only a policy-satisfying authoritative result completes the occurrence.

### Scenario: Unsafe or stale artifacts fail closed

- **Given** corrupt/truncated/unsupported bytes, traversal-like object targets,
  expired reservations, mismatched digests/body IDs, cross-tenant access, or
  duplicate/concurrent finalize
- **When** public upload/finalize/read operations occur
- **Then** the service rejects or safely releases them without object disclosure
- **And** retries cannot produce duplicate terminal decisions.

### Scenario: Retention and previews protect private source bytes

- **Given** completed/expired artifacts and authorized preview needs
- **When** retention and derivative workers run/restart
- **Then** policy-selected objects are removed idempotently, authorized bounded
  derivatives remain as configured, and source object URLs/bytes do not leak to
  unauthorized clients or CSP traffic.

## Implementation Instructions

- Apply non-plan delta `89583ced..e196dd2c` after P4. Preserve additive media
  migrations, typed artifact API, object-store adapter, ffprobe decoder boundary,
  digest/body identity, lease/fence, evaluation provenance, and retention logic.
- Use MinIO/S3-compatible storage in integration tests. In-memory bytes or DB
  blobs cannot satisfy this phase.
- Treat browser MIME/metadata as untrusted. Validate size/type/duration/dimensions
  from server-observed bytes and allowlisted object targets.
- Keep evaluation asynchronous/durable and separate from chat. Scripted rubric
  fixtures are permitted for credential-free deterministic proof; they do not
  prove live image-model quality.
- Preserve privacy/CSP behavior: local previews must not cause unauthorized blob
  or object URL network traffic.
- Build but do not claim missing native camera/audio/video flow.
- Open one P5 PR and record any live-model qualification as BLOCKED unless
  explicitly authorized and actually run.

## End-to-End Test Plan

- Run `make tasks-test tasks-cover tasks-clients tasks-web tasks-e2e` with real
  PostgreSQL and MinIO/S3-compatible storage.
- Through managed headless browser on the real stack, upload valid poem image,
  MP3, and MP4 fixtures; observe asynchronous progress; reload; inspect accept,
  reject, and retry; verify no source URL leaks in browser network/console/CSP.
- Run process tests for worker kill/restart, duplicate finalize, concurrent
  reservations, cross-tenant reads, expired cleanup, derivative authorization,
  corrupt/truncated media, traversal targets, and retention replay.
- Execute the generated TypeScript client boundary under Node as well as browser.
- Run `TASKS_LIVE_MODEL_TEST=1 make tasks-live-image-e2e` only with explicit
  credentials/approval; otherwise record it BLOCKED without affecting
  credential-free completion.
- Compare integrated tree to `e196dd2c` and review all stored evidence for secret
  or private-media leakage.

## Anti-Cheating Audit

- Inspect DB schema/repositories to ensure only metadata/object refs, not media
  bytes, are persisted.
- Confirm tests upload/read real MinIO objects through public presigned/finalize
  paths; fixture digest assertions alone are insufficient.
- Check ffprobe/decoder invocation and production error propagation; filename or
  browser MIME checks are insufficient.
- Check worker leases/fences and exactly-once decision constraints under restart
  and concurrency.
- Check object key allowlists, tenant authorization, URL expiry, derivatives,
  retention, browser network logs, and CSP.
- Reject claims that scripted rubric acceptance proves a live model.

## Completion Gate

- [ ] Image/audio/video public submissions work with real object storage.
- [ ] Poem accept/reject/retry and durable no-chat progress pass after reload/restart.
- [ ] Security, corruption, traversal, concurrency, privacy, derivative, and
      retention negatives pass.
- [ ] Managed-headless exploration and promoted P5 browser suite pass.
- [ ] Generated clients, Go/web/build/diff, and Tasks 85% coverage gates pass.
- [ ] Live model is either actually qualified or explicitly BLOCKED.
- [ ] Endpoint/adaptation and anti-cheating reviews pass; green P5 PR merges.
