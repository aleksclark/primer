# Student / P4 reconciliation onto current master

## Scope and donor audit

Initial isolated base: `ccee6c8b114c73d52876021a3d93c98d70244050`.
This is a narrow source reconciliation, not full P4/native acceptance.

- Student tip `d4d56f74` itself changes A16 documentation. Audited its ancestry
  from shared `eeb04728`: ported the final eight-file Student feature delta
  (capability gating, unavailable restore result, back-navigation, pairing
  fallbacks and tests). The intermediate missing-capability fallback in
  `a6c556f8` was not adopted; final `c2142368` fails closed.
- Producer `51cad1c0` is already represented by master `218f749f`.
  `StudentRequirement`, `requirements` and `studentCapability` remain intact.
- P4 `89f3a593`, `fd34a7bd`, `825bef5c`: ported only product-local web,
  TypeScript facade/probe and Go API/test hunks. The seven web files matched
  the pre-P4 donor state on master. Did not import donor workflows, old producer
  files wholesale, migrations or historical PASS records.
- Educator donor feature/app files equal master: no port needed.
- Android integration `7eb4c4bc` context and documentation clarification
  `6cb46f6e` are represented by the explicitly historical A16 record in
  `test-artifacts/android-student/student-tasks-baseline-a16.md`. No hardware,
  signing, public rollout or native Control acceptance is inferred here.

## Reconciled contracts and local authority

- Browser manual start/submit now bind to the issued requirement and use a
  dedicated typed `StudentManualAction2` result, not donor `map[string]any` and
  not a response adapter that discards binding fields. Modern web always sends
  `requirementId`; omission is accepted only for exactly one supported manual
  requirement, resolved under the occurrence transaction.
- Every cookie manual mutation requires Origin/CSRF, including that legacy
  omission. Malformed raw queries fail before interpretation, rather than
  `URL.Query()` silently dropping a malformed explicit/duplicate selector.
- Native remains bearer `/device` only with unchanged typed `OccurrenceAction2`.
  No native query selection, cookie fallback or dialogue/WS support is added.
  Both native start and submit validate the entire single-manual envelope before
  mutation/idempotent success. Student/device rows stay locked through the
  occurrence transaction in archive-compatible order. Browser session lifetime
  checks and strict socket/inspect decoding remain separate and unchanged.
- Capability metadata and P4 attempt-state `verification` are additive, not
  alternatives. All-requirements completion, per-requirement attempts and retained
  inspection remain authoritative. Management migrations `00012`–`00015`,
  canonical `00001`–`00011`, identity policy and generated-client constraints
  are untouched by this port.
- Native unsupported states have neutral labels, not invented parent approval.
  Captured detail requests and a view generation prevent late results reopening
  a detail after Back/select B. Stale restore results cannot restore a revoked
  or replaced binding; session storage publication is serialized and checked,
  with a UI restore generation guarding already-returned stale results.
- Browser design authority remains **Primer System C**,
  `design-system/README.md`, `reference/system-c.html` and generated tokens.
  Student dialogue is Decide/Learn; retained parent history is Command/Inspect;
  task editing is Configure. No house palette/token replacement. Collections
  remain bounded and server-operated through the generated client.

## Review and checkpoint evidence (not final qualification)

L2 read-only reviewer `d9570a37-7dd0-41bb-8aa9-addc4d194b49` produced
`/tmp/student-p4-review-initial.md` and `...-followup.md`. Findings drove the
custody, typed-response, label, navigation and malformed-query corrections.
Reviewer withdrew mandatory selection for the safe legacy single-manual call
once unconditional CSRF and transactional binding were established. Final
source review and combined-source test receipts are recorded below when ready.

Local logs: `/tmp/student-p4-current-gates/`.

- `focused-go-corrected`: focused Phase2, public mixed-manual, capability and
  native compatibility tests passed (API 45.904s), before later query/restore
  hardening. Earlier RED preserved: native start-after-submit semantics,
  permissive missing-requirement test precondition, and an invalid-version
  fixture attempted through a producer that correctly rejects it.
- `client-web`: fresh TypeScript generation/typecheck/boundary/runtime tests,
  generation mutation tests and real generated-client public dialogue/manual
  probe passed; Kotlin generation and generator tests passed; web lint,
  typecheck, 26 unit tests and build passed. These are pre-merge/source-scoped,
  not final combined-head qualification. Vite reported its existing native
  config-loader and >500kB chunk warnings; no thresholds changed.
- `full-tasks`: FAILED solely at the old Clerk browser manual fixture, which
  supplied a cookie but no CSRF. The test now uses the separately asserted
  issued CSRF cookie/header and configured Origin; no identity guard was changed.
- `full-tasks-corrected` and `coverage`: interrupted before aggregate receipt;
  no exit file, no PASS. Fresh combined-master reruns remain required.

## Remaining at implementation checkpoint

Merge parent-authorized current master after this checkpoint, then rerun full
Tasks tests/85% coverage, clients, web and feasible real-stack browser exploration.
Kotlin/native compilation must use Gradle workers=1 and isolated output. Parent
may own the final all-Android suite/lint/assembly after integration. Full race10,
CI, live provider, native Control loop, camera/device, release and deployment
acceptance are not claimed; no floor/budget is relaxed. The separate management
replacement double-epoch defect remains outside this work.
