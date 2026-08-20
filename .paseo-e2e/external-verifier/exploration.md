# External verifier — manual browser exploration (FAIL)

- **Suite / phase:** `external-verifier` / `explore`, call 1
- **Start URL:** `[configured Tasks SPA origin]`
- **Browser:** isolated headless Chrome contexts `external-verifier-parent` and `external-verifier-student`
- **Fixture:** the real active catalog option presented by the SPA, named **Stacklane fixture verifier**. No endpoint, secret, verifier ID, pairing code, cookies, request body, or callback body is retained in this record.
- **Outcome:** FAIL. The real external callback delivery reached durable `dead` rather than signed progress / final completion. Per the exploration contract, stopped here; no Playwright files were created and no retry/cancel/fallback or synthetic fixture-control request was exercised.

## Manual procedure and actual observations

1. Navigated to `/parent/students`; authenticated in the supplied test identity screen as Parent A.
   - Snapshot showed the one pre-seeded student, **Stacklane Student** (`Parent students` snapshot uid `3_40` / `14_40`).
2. Opened `/parent/tasks` via sidebar `Tasks` uid `3_10`.
   - The `External verifier configuration` region was present (`4_115`). Its select (`4_122`) already showed **Stacklane fixture verifier** (`4_124`), capability `response` (`4_126`), and schema `external_callback.v1` (`4_128`).
   - The rendered safety text stated: “Endpoints, credentials, and signatures stay server-side” and “no endpoint or secret is exposed here.”
3. Created a uniquely labelled external task through the real form: entered a test-only title in `4_120`, clicked `Create external task` (`4_132`).
   - Post-action snapshot showed the new revision as `draft` (`6_4`) with `Publish` (`6_5`).
4. Clicked `Publish` (`6_5`).
   - Post-action snapshot showed `published` and exposed the revision in the schedule chooser as an option (`7_0`).
5. Selected that published task (`4_137`, `ArrowDown`, `Enter`), selected **Stacklane Student** (`4_139`, `ArrowDown`, `Enter`), entered the visible local schedule inputs (`Schedule start` `4_142`, `RRULE` `4_144`), and clicked `Schedule task` (`4_145`).
   - Browser alert: “Schedule saved and occurrences materialized.” Accepted it. The student checklist subsequently contained the created occurrence.
6. Parent opened the student record and used `Issue pairing QR` (`15_15`). A separately isolated student browser opened `/student/pair`, entered the one-use code, and reached `/student`.
   - Code value is intentionally not recorded.
   - Student checklist snapshot showed the created task with `pending` (`18_4`–`18_7`).
7. Student opened the occurrence (`18_4`) and clicked `Start task` (`19_5`).
   - External UI rendered **Stacklane fixture verifier**, heading `awaiting verification`, status `awaiting_verification`, a `Your response` field (`20_7`), and disabled-until-input submit (`20_8`).
8. Student entered a benign short response and clicked `Submit for external verification` (`20_8`).
   - Immediate snapshot: heading/status `queued`, with safe progress `queued`; submission control disabled.
   - After the SPA’s normal polling interval, `wait_for([completed, accepted], 15s)` timed out. The next snapshot showed heading/status **`dead`**, retained safe progress `queued`, and only `Retry verification` (`23_0`) plus refresh. There was no safe acceptance/rejection/completed event.

## Network and console evidence (sanitized)

- `POST /api/student/occurrences/<occurrence>/external/submit` returned **202 Accepted**. Its response status was `queued`; sensitive request body, cookies, and opaque IDs are intentionally omitted here.
- Repeated `GET /api/student/occurrences/<occurrence>/external` calls returned **200**. Final sanitized state: `status: dead`, `progress: [{status: queued}]`, `canRetry: true`, `canCancel: true`, `fallbackAvailable: false`. The public state named the expected schema and capability but contained no endpoint, credential, callback signature, callback raw body, or model reasoning.
- All recorded browser-facing SPA API requests were same-origin. Chrome console: **no messages found**.
- The failure is downstream of submission acceptance: SPA submit was accepted, then the server-side external delivery transitioned durably to `dead` without exposing internal error/reasoning to the student UI.

## Privacy / non-exposure check (limited by failed terminal flow)

- **Observed PASS:** parent authoring UI exposed only catalog name/capability/schema and explicit server-side safety language; no endpoint or secret was rendered.
- **Observed PASS:** student external page exposed only verifier display name, state/progress, and ordinary response input; dead state did not expose raw callback data, endpoint, credentials, signing material, or reasoning.
- **Not completed:** final accepted/completed views for both student and parent were unreachable, so final-state non-exposure and cross-role durability cannot be promoted as passing.

## Negative-path status

- **Observed real failure / retry affordance:** `dead` state made `Retry verification` available. It was **not clicked** because the required happy path had already failed and this call must stop in explore rather than manufacture a pass.
- **Not exercised:** disabled verifier, schema mismatch, cancel, fallback, or fixture fault controls. No gate was weakened and no fixture control endpoint was called.

## Reproducible selectors / locators for a later healthy re-exploration

- Parent: sidebar link `Tasks`; form `External verifier task configuration`; labels `Task title`, `Administrator verifier`, `Verifier capability`, `Verifier schema version`; buttons `Create external task`, `Publish`, `Schedule task`.
- Student pairing: `/student/pair`, label `Pairing code`, button `Pair this browser`.
- Student: checklist task link by title; `Start task`; external region text `External verification`; label `Your response`; button `Submit for external verification`; state text and `Refresh state` / `Retry verification`.

## Flake / environment risks

- The Stacklane server clock and schedule date were presented by the running service. A fresh run must select an occurrence visible in the student checklist rather than hard-code time/ID values.
- This call created one test-labelled published/scheduled task and a one-use pairing session in the real Stacklane environment. Later runs need new unique task titles and pairing material.
- The blocking signal is not a browser console error: server-side egress/fixture callback configuration or fixture process availability must be diagnosed before re-exploration.

---

# Re-exploration after callback request-ID binding fix (PASS)

- **Suite / phase:** `external-verifier` / explicit `explore again`, call 2
- **Outcome:** PASS. A fresh real task, schedule, occurrence, pairing code, parent browser context, and student browser context reached durable external verification completion.
- Raw call-2 snapshots were reviewed for acceptance and privacy evidence, then removed; the one pairing-code value was redacted before removal. No retained artifact records a pairing code, cookie, submitted response, endpoint, credential, signature, callback raw body, or opaque delivery identifier.

## Fresh live procedure and assertions

1. In a fresh parent Chrome context, authenticated as Parent A, opened **Tasks**, and used the live `External verifier task configuration` form.
   - `Administrator verifier` showed the real **Stacklane fixture verifier** (`27_122` / `27_124`), with `response` capability (`27_126`) and `external_callback.v1` schema (`27_128`). The live UI again stated that endpoints, credentials, and signatures remain server-side and that no endpoint/secret is exposed.
2. Created a new uniquely labelled external task (`27_120` → `27_132`), saw its `draft` revision (`28_4` / `28_5`), published it, and saw it become a schedule option (`29_0`) and `published` (`28_4`).
3. Selected the new published task and **Stacklane Student** in the live schedule form (`27_137`, `27_140`), supplied displayed schedule values, and clicked `Schedule task` (`27_146`). The browser confirmed “Schedule saved and occurrences materialized.”
4. Issued fresh pairing material in the parent student record (`33_15`). In a separately isolated new student context, used `/student/pair` (`35_17`, `35_19`) and reached the server-owned student checklist.
   - The fresh task appeared as `pending` at `36_8`–`36_11`; the prior failed task was a distinct checklist record and was not reused.
5. Student opened the new occurrence, clicked `Start task` (`37_5`), and received the external verifier UI (`38_0`): fixture name, `awaiting_verification`, required `Your response` input, and disabled-until-input submit control.
6. Student submitted one benign short response with `Submit for external verification` (`38_8`). The immediate saved snapshot shows durable `queued` status/progress (`38_3` / `39_0`).
7. The normal SPA poll `wait_for(["completed", "accepted"], 15s)` succeeded. The resulting student snapshot shows `completed` (`38_3` / `38_4`) and three generic safe `Verification update` entries (`40_0`–`40_2`), not callback payloads or internal details. Clicking `Refresh state` and resnapshotting retained `completed`.
8. Independently in the parent context, a fresh `/parent/occurrences` navigation showed the new occurrence as `completed` (`43_57`–`43_62`). Parent `Inspect` (`43_62`) showed the external verifier source, `completed` delivery status, retained queued/update entries (`44_4`–`44_14`), and the occurrence summary status `completed` (`44_20`–`44_21`).

## Sanitized live network / console evidence

- Student: start returned **200**; `POST /api/student/occurrences/<fresh-occurrence>/external/submit` returned **202**; all subsequent same-origin external-state polls returned **200** and reached sanitized `completed` state. The final state showed three ordered safe updates and terminal acceptance; `canRetry`/`canCancel` were false. No browser request went to the verifier endpoint directly.
- Parent: occurrence inspect and external inspect APIs returned **200** and the parent rendered the same completed durable projection. The real flow completed after the server accepted the callback sequence, which is the browser-observable result of the repaired server-side signed-callback binding; signature material itself correctly never entered the SPA.
- Student console: **no messages found**.
- Parent console: a residual two-count `404` resource error was recorded for the unrelated `/artifacts/inspect` request while opening the general occurrence inspector. The external inspect request was **200** and the external callback flow was unaffected. This is recorded as residual risk, not hidden or treated as a green console run.

## Final privacy / non-exposure result

- **PASS in both roles:** neither the authoring, student completed, nor parent completed UI rendered an endpoint, secret/credential, signature, callback raw body, or model/private reasoning. The status UI displayed only fixture source, status, and safe generic progress labels.
- The browser-facing responses were same-origin and did not require the student browser to contact the verifier. Opaque protocol metadata seen only in developer network inspection is deliberately not copied to this record; it was not rendered by either UI.

## Promotion notes

- Happy-path exploration is now sufficiently reproducible for codification; no Playwright files were written during this explore call.
- Preserve the real fixture and real parent/student flow in the later suite. Do not replace the external delivery with a mock, stub, or direct fixture-control call.
- Residual: investigate the unrelated inspector `/artifacts/inspect` 404 before asserting a globally clean parent console in the future suite. It is outside the completed external callback acceptance path but must not be silently ignored.

---

# Codification and independent Chrome re-drive (PASS)

- **Suite / phase:** `external-verifier` / `codify`, call 3
- **Focused suite:** `primer-tasks/web/e2e/external-verifier-progress.spec.ts`
- **Runner evidence:** `.paseo-e2e/external-verifier/last-run.log`
- **Result:** one focused Playwright test passed (`1 passed`, 7.4 s). The suite runs the real Stacklane origin through `browser-test` with the recorded exploratory-pass guard; it uses neither network mocking nor fixture-control calls.

## Automated acceptance coverage

- Fresh parent context authenticates, verifies the real **Stacklane fixture verifier** and its `response` / `external_callback.v1` capability/schema, creates/publishes/schedules a unique external task for the pre-seeded student, and issues fresh pairing material.
- A separately isolated student context pairs, starts the new occurrence, submits through the real SPA (`202` submit response), observes `queued` then `completed`, sees the three safe generic verifier updates, and reloads to prove durable completion.
- Parent independently opens the occurrence list and external inspect view, asserts completed delivery, source name, and the four durable list entries (queued plus three safe updates), and waits for the real external-inspect API to return `200`.
- Both rendered UI assertions reject callback/protocol/private-reasoning fields (`callbackId`, `requestDigest`, signature header name, chain-of-thought/internal-reasoning phrases). The test does not claim globally clean console because the known unrelated artifact-inspect 404 remains.
- The first two local codification attempts failed for test-locator alignment only (the endpoint-safety copy lives in the outer region, and the parent delivery list requires list-item rather than exact-text counting); those assertions were corrected to the actual explored DOM. The final behavioral assertions and product flow were not removed or weakened.

## Independent DevTools Chrome re-drive

- Fresh isolated contexts `external-verifier-codify-parent` and `external-verifier-codify-student` repeated the whole real parent/student flow with a new task and one-use pairing code. Raw call-3 snapshots were reviewed for evidence and removed; no one-use code was retained.
- Parent authoring snapshot again showed only the active fixture, capability/schema, and explicit server-side endpoint/credential/signature protection. The new task reached published/scheduled state.
- Student checklist showed the fresh occurrence `pending`; after Start and Submit it showed `awaiting_verification`, then `queued`, then `completed` with three generic `Verification update` entries. A subsequent state refresh retained completion.
- A fresh parent Occurrences load listed that exact new occurrence as `completed`; Parent Inspect showed source **Stacklane fixture verifier**, completed status, queued plus three safe updates, and no endpoint/secret/raw-body/reasoning UI.
- Chrome network evidence: parent external inspect requests returned `200`; the known unrelated artifact inspect requests returned `404` twice. Student console had no messages. Parent console repeated the two-count 404; it remains a recorded residual, not a green-console claim.

## Promotion result

All codify gates now hold: focused Playwright exit `0`, independent Chrome re-drive observed the documented acceptance criteria, and no test/product gate was weakened. Promote this suite to `run`.

---

# Final-tip regression run (FAIL closed)

- **Suite / phase:** `external-verifier` / `run`, call 4
- **Playwright:** the unchanged promoted suite exited **1**. It did complete the real external flow, but its existing assertion for exactly three generic `Verification update` labels found zero after the final tip.
- **Why not changed:** final-tip safe-progress projection now renders distinct safe text, so changing/removing the established generic-label assertion would be an assertion change. Per the requested run phase and no-weakening gate, the suite was left unchanged and the result is FAIL.
- **Independent final-tip Chrome re-drive:** fresh parent/student contexts created a new task, published/scheduled it, paired a fresh browser, and completed it. Student safe projection was `completed` with: `Verification requested.`, two `External verification is in progress.`, and two `The external verifier accepted the response.` entries. Parent Inspect independently showed the same completed source/status and same five safe messages. No endpoint, secret, callback raw body, or private reasoning was rendered.
- **Network / console:** parent inspect and external inspect were `200`; the known unrelated artifact inspect call remained `404` twice and produced the corresponding two-count console resource error. Raw call-4 snapshots were reviewed for evidence and removed; no pairing code was retained. Full Playwright failure: `.paseo-e2e/external-verifier/last-run.log`.

# Final assertion-aligned regression (PASS)

- User-authorized final workspace assertion aligns the parent first durable entry with the observed safe projection: `Verification requested.`. The focused promoted command passed: **1 passed (7.3 s)**; full command/output in `last-run.log`.
- Independent Chrome re-drive reloaded the same separately-created final-tip student and parent occurrence on the real origin. Both retained `completed`, fixture source, and only safe projection text (requested, in-progress, and accepted-response entries); raw call-5 snapshots were reviewed and removed.
- No further assertion changes were made. The known unrelated parent artifact-inspect 404 residual remains recorded.

---

# Fresh final-tip exploratory browser acceptance (PASS)

- **Suite / phase:** `external-verifier` / explicit fresh final-tip exploration, call 6
- **Result:** **PASS**. This run used new isolated parent and student Chrome contexts, a freshly authored/published/scheduled task, fresh one-use pairing material, and the seeded **Stacklane fixture verifier**. It did not reuse an existing occurrence, prior browser session, fixture control request, or in-process substitute.
- **Sanitization:** no pairing value, submitted response, opaque identifier, cookie, endpoint URL, request/callback body, header, signature, credential, or reasoning is retained in this record or `last-run.log`.

## Live browser procedure and observed acceptance

1. Parent authenticated through the test identity page, opened **Tasks**, and observed the active administrator catalog choice **Stacklane fixture verifier**, capability `response`, schema `external_callback.v1`, and the explicit copy that endpoints, credentials, and signatures remain server-side.
2. Parent authored a new external task, observed it as `draft`, published it, selected it in the live schedule form for **Stacklane Student**, and received the normal schedule-materialized confirmation.
3. Parent issued fresh pairing material; a separately isolated student context paired and showed the newly scheduled occurrence as `pending`.
4. Student opened the occurrence and started it. The live external region showed only the fixture display name, `awaiting verification`, and ordinary response input. After submitting a benign response, the region immediately showed `queued` and the safe entry **“Verification requested.”**
5. **Reconnect probe:** while delivery was in flight, the student page was reloaded. The reconnected page retained the server-owned record and reached `completed`, with the ordered safe projection: request, two in-progress updates, and two accepted-response updates.
6. Parent independently opened **Occurrences** and the matching **Inspect** view. It showed the same fixture source, `completed` delivery status, and the same five safe ordered entries. The occurrence summary was also `completed`.

## Redaction and operational observations

- **PASS:** parent authoring exposed catalog name/capability/schema only; the student and parent completed views exposed source, status, and safe progress only. None displayed endpoint/URL, secret/credential, raw callback body, signature/header, opaque protocol fields, or private reasoning.
- **PASS:** browser-facing requests remained same-origin; the student had no direct verifier request. Submit returned `202`; subsequent state and parent inspection calls returned `200`. Exact URLs, IDs, request bodies, and response bodies are intentionally omitted.
- **Residuals recorded, not hidden:** the student console reported one CSP inline-style violation/issue from the dev application. The parent inspector also made two unrelated artifact-inspector requests that returned `404`, while the external inspector returned `200`. Neither interrupted the real submit, durable progress, reload/reconnect, or completion. This suite’s stated acceptance criteria do not assert a globally clean console; these are not represented as a clean-console result.

## Separate-fixture and regression evidence

- The unchanged focused browser command passed one real Playwright test in 7.2 seconds (8 seconds wall clock). It creates/publishes/schedules, pairs a separate student context, verifies the queued/completed safe projection, reloads the student, and inspects the parent projection.
- `make tasks-external-e2e` passed in 2 seconds against the separate external-verifier fixture process. Its implementation checks the real process boundary and expected idempotent request/callback effect; output details and identifiers are not copied into this evidence.
- Full sanitized commands/results are in `last-run.log`. No gate, assertion, product source, test source, or fixture configuration was changed in this call.

## Final promoted browser run after fresh Terra PASS

- The promoted focused Playwright command was rerun after call 6 and passed: **1 passed (7.3 seconds)** against the real external process flow.
- Raw Playwright result artifacts were reviewed for failure/privacy evidence and removed; only this sanitized result and the process-E2E result are retained.
