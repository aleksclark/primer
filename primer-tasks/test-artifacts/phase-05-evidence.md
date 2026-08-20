# Primer Tasks Phase 5 evidence

## Scope

Phase 5 web/server media evidence and asynchronous rubric review only. Android,
native, emulator, and live multimodal quality work are out of scope.

## Reviewed result

- Branch: `impl/tasks-p5-media`
- Plan revision: `24cd3abe`
- Reviewed tip: `ee81f119` (final implementation tip; anti-cheat findings remain open)
- Exploratory suite: `.paseo-e2e/p5-media/` (sanitized, local evidence)
- Terra exploratory result: **PASS** (call #35)
- Promoted Playwright result: **PASS, 2/2** (call #36 plus fresh independent rerun)

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
- TypeScript client tests — PASS (13/13)
- Focused malformed-finalize/retry and S3 public-presign unit/integration checks — PASS
- Real PostgreSQL migration and security integration checks — PASS
- Compose `check` and real PostgreSQL/MinIO Stacklane health — PASS
- `make -C primer-tasks tasks-cover` — **FAIL at 80.5%** (mandatory 85% minimum)
- `git diff --check` — PASS

## Explicit limitations

- `TASKS_ARTIFACT_SCRIPTED_FIXTURE=1` is a controlled deterministic fixture and
  is not live multimodal qualification.
- `TASKS_LIVE_MODEL_TEST=1` remains **BLOCKED** pending approved non-sensitive
  provider credentials/fixtures and cost approval.
- Native/mobile/emulator gates belong to the Android continuation plan and are
  neither implemented nor a blocker for this phase.

## Release decision

Phase 5 web/server completion gate: **NOT PASS**. Browser exploratory and
promoted UI flows pass as wiring evidence, but the fresh anti-cheat review found
open hard-coded scripted-evaluator, media decode/duration, cleanup/recovery,
CSP/client-contract, and coverage issues. Controlled live image quality remains
explicitly **BLOCKED**. **Phase 6 may not be dispatched.**
