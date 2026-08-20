# Primer Tasks Phase 5 evidence

## Scope

Phase 5 web/server media evidence and asynchronous rubric review only. Android,
native, emulator, and live multimodal quality work are out of scope.

## Reviewed result

- Branch: `impl/tasks-p5-media`
- Plan revision: `24cd3abe`
- Reviewed tip: `5bf284631cceea9a071f35ca6db396565bbe9219` (final implementation tip; fresh independent anti-cheat review pending)
- Exploratory suites: `.paseo-e2e/p5-media-final/` and `.paseo-e2e/p5-media-final2/` (sanitized, local evidence)
- Terra final exploratory result: **PASS** (call #6)
- Promoted Playwright result: **PASS, 2/2** (call #7 plus fresh independent Chrome confirmation)

## Browser evidence

Real Stacklane Compose with PostgreSQL and MinIO verified:

- Parent authored/published/scheduled image rubric; student paired, uploaded through
  same-origin bounded `PUT /api/student/artifacts/{id}/upload`, saw queued/no-chat
  progress, and reached durable scripted-fixture completion.
- Malformed, pixel-bomb, oversize, and digest-mismatch cases failed closed;
  invalid reservations/submissions were reconciled and same-occurrence replacement
  succeeded. Duplicate finalize and interrupted upload/resume were replay-safe.
- Audio and video persisted securely, rendered local/authorized previews, and were
  visibly routed to parent review rather than silently passed.
- Foreign tenant and revoked student access returned generic denial/revocation
  states. Browser network/DOM scans found no MinIO host, presigned query,
  X-Amz credential, tenant object key, or object-store URL.
- Parent inspect showed snapshotted rubric, criterion evidence/feedback,
  scripted-fixture provenance, model, and policy. A read-only database/object/log
  review found no artifact bytes in PostgreSQL, no EXIF/profile data in the image
  derivative, and no raw reasoning/tool input/provider credential leakage.
- Responsive 390px, keyboard focus, and Lighthouse mobile accessibility/best
  practices checks passed.

The scripted multimodal fixture is wiring evidence only. Controlled live image
qualification remains explicitly **BLOCKED**; no model/image-understanding or
educational-quality claim is made.

## Promoted automation

- `primer-tasks/web/e2e/p5-media.spec.ts`
- `PRIMER_TASKS_BASE_URL=http://web.primer-tasks-p5.primer-tasks.test:5173 npm --prefix primer-tasks/web run browser:promoted -- e2e/p5-media.spec.ts`
- Result: **2 passed**
- Independent fresh Chrome redrive: **PASS**

The suite uses real public UI setup, generated/client façade behavior, real media
fixtures, and no private repository seeding, mocks, skips, or weakened assertions.

## Web/server gates

- `go -C primer-tasks test ./... -count=1` — PASS
- `make -C primer-tasks tasks-clients tasks-web` — PASS
- TypeScript client tests — PASS (14/14), including negative foreign-origin/signed-query/non-artifact binary-target tests
- Focused malformed-finalize/retry and S3 public-presign unit/integration checks — PASS
- Real PostgreSQL migration and security integration checks — PASS
- Compose `check` and real PostgreSQL/MinIO Stacklane health — PASS
- `make -C primer-tasks tasks-cover` — PASS at **85.0%** (mandatory 85% minimum)
- `git diff --check` — PASS

## Explicit limitations

- `TASKS_ARTIFACT_SCRIPTED_FIXTURE=1` is a controlled deterministic fixture and
  is not live multimodal qualification.
- `TASKS_LIVE_MODEL_TEST=1` remains **BLOCKED** pending approved non-sensitive
  provider credentials/fixtures and cost approval.
- Native/mobile/emulator gates belong to the Android continuation plan and are
  neither implemented nor a blocker for this phase.

## Final reconciliation and release decision

Final implementation tip: `5bf284631cceea9a071f35ca6db396565bbe9219`.
The final tip closes the previously reported blockers: all artifact worker
mutations are owner- and unexpired-lease fenced with stale-reclaim tests;
audio/video use the ffprobe-plus-decoder boundary with real valid/corrupt/
truncated fixtures and Compose `ffmpeg`; artifact JSON uses strict typed Huma
operations and generated client types; the public authenticated API→PostgreSQL→
MinIO integration proves isolation, replay, revocation, original/derivative
permissions, and retention cleanup; and the aggregate coverage gate is 85.0%.
The binary client façade additionally rejects foreign origins, signed queries,
non-artifact paths, and traversal-like targets.

Phase 5 web/server completion gate: **PASS**. Controlled live image
qualification remains explicitly **BLOCKED** pending approved provider
credentials/fixtures; scripted fixture evidence makes no image-understanding or
educational-quality claim. Native/mobile/emulator work remains out of scope and
paused. On the web/server gate, **Phase 6 is eligible to be dispatched**; the
live multimodal and native limitations must remain explicit and must not be
represented as completed evidence.
