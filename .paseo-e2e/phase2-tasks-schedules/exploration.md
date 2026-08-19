# Phase 2 tasks and schedules — exploration

## Status

**FAIL / remains in `explore` after call 2.** Authentication remediation works and major public UI paths were exercised in isolated Chrome contexts. The central schedule creation request is rejected by the service even though the UI supplied `timezone: "America/Chicago"`; therefore no occurrence exists and the required recurring, student execution/approval, skip/cancel, restart/DST, and occurrence-collection paths cannot honestly be passed. No Playwright file was created.

## Environment

- Original requested start URL: `http://web.primer-tasks-p2.primer-tasks.test/` (call 1: unreachable)
- Current, remediated base URL: `http://127.0.0.1:37940/`
- Call 2 parent-A context: `phase2-tasks-schedules-call2`
- Call 2 student context: `phase2-tasks-schedules-student-call2`
- Call 2 parent-B context: `phase2-tasks-schedules-parent-b-call2`
- Desktop evidence: `desktop-initial.png` (call 1), `call2-parent-b-desktop.png`
- Mobile evidence (390×844, DPR 2, mobile/touch): `mobile-initial.png` (call 1), `call2-student-mobile.png`
- All corresponding a11y snapshots are stored beside this file under `.paseo-e2e/phase2-tasks-schedules/`.

## Call 1 — previous authentication blocker

| # | Browser action / live URL | Selector observed or used | Result / evidence |
|---|---|---|---|
| 1 | Open requested FQDN | N/A | `net::ERR_CONNECTION_REFUSED`. |
| 2 | Open previous fallback `http://127.0.0.1:37933/` | N/A | App rendered `/parent/students`, but session was unauthenticated. |
| 3 | Snapshot sign-in screen | `button` **CONTINUE WITH PARENT SIGN-IN**, `uid=1_32` | Live unauthenticated selector recorded. |
| 4 | Click parent sign-in | `uid=1_32` | Browser redirected to unreachable `test-issuer.primer-tasks-p2.primer-tasks.test` and failed `net::ERR_CONNECTION_REFUSED`. |

The original FQDN/issuer routing failure is superseded by the successful call-2 direct auth route below.

## Call 2 — authenticated live procedure

| # | Browser action / live URL | Selector observed or used | Result / evidence |
|---|---|---|---|
| 1 | Open `http://127.0.0.1:37940/` | N/A | Rendered unauthenticated `/parent/students`; snapshot `call2-parent-login.snapshot.txt`. |
| 2 | Browser-navigate public test-auth route `GET /api/auth/login?principal=parent-a` | N/A | Success: returned to parent-A `/parent/students` in the isolated context. `call2-parent-home.snapshot.txt` shows parent navigation and zero students. |
| 3 | Add student | **ADD FIRST STUDENT** `uid=5_68`; modal **DISPLAY NAME** `uid=6_8`; **SAVE STUDENT** `uid=6_13` | Created `E2E Student Call Two`; roster shows **READY** in `call2-student-created.snapshot.txt`. Network: `POST /api/students` → `201`. |
| 4 | Navigate Tasks, create brushing task draft | Parent **Tasks** link `uid=5_21`; **Task title** `uid=8_10`; **CREATE DRAFT** `uid=8_13` | Created `E2E Brushing Call Two`, state **DRAFT**, `call2-task-draft.snapshot.txt`; network `POST /api/tasks` → `201`. UI explicitly says parent approval is the only enabled verification driver for this phase. |
| 5 | Publish task | **PUBLISH** `uid=9_12` | Task became **PUBLISHED** and available in **Task to schedule** options; `call2-task-published.snapshot.txt`; network `POST /api/tasks/{id}/publish` → `200`. |
| 6 | Select published task and student in schedule form | Combobox **Task to schedule** `uid=8_16` then observed option `uid=10_0`; **Student to schedule** `uid=8_19` then option `uid=8_22` | Both selections visibly persisted in `call2-datetime-picker.snapshot.txt`. |
| 7 | Attempt initial native date-time picker | **Schedule start** `uid=8_23`, picker button `uid=8_44` | Native picker exposed selected Aug 19 and hour/minute/AM-PM in snapshot, but its internal entries could not be clicked by Chrome MCP (`Element ... no longer exists`). Keyboard manipulation set an observed temporary start value. This was a harness interaction limitation, not treated as a product result. An accidental string was entered in RRULE while attempting keyboard focus and was cleared before the final attempt. |
| 8 | Continue after HMR replacement with real labeled text input | **Schedule start** `uid=8_23` (textbox) filled `2026-08-19T14:00`; **RRULE** `uid=8_45` cleared using real keyboard `Control+A`, `Backspace`; **SCHEDULE TASK** `uid=8_48` | The final visible form state is captured in `call2-rrule-cleared-keyboard.snapshot.txt`: published task, student, text start value, blank RRULE, enabled schedule button. |
| 9 | Submit one-off schedule three times (including the clean keyboard-cleared attempt) | **SCHEDULE TASK** `uid=8_48` | All requests failed `POST /api/schedules` → `400`; rendered live alert **UNABLE TO LOAD** / “The Tasks service did not return the requested record.” See `call2-one-off-result-keyboard-clear.snapshot.txt` and network evidence below. |
| 10 | Exercise task `q` URL state | **Search tasks** `uid=8_51`, filled `Brushing` | The controlled UI remounted per keystroke, leaving the final observed value `g`; URL became `http://127.0.0.1:37940/parent/tasks?q=g` (`call2-tasks-q-url.snapshot.txt`). This proves query state is written to the URL but does not pass full multi-character search. No filter/sort/page controls appeared in the public tasks UI snapshot, so those states were not exercised. |
| 11 | Retire task | **RETIRE** `uid=9_12` | Success: task row changed to **RETIRED** in `call2-task-retired.snapshot.txt`. This covers the task-retirement lifecycle control only. |
| 12 | Issue student pairing material in parent-A context | Student roster **OPEN** `uid=26_88`; **ISSUE PAIRING QR** `uid=27_34` | Public UI issued show-once short-lived pairing material and QR (`call2-pairing-code.snapshot.txt`). The transient code is intentionally not reproduced here. |
| 13 | Pair separate student browser | New isolated browser `/student/pair`; **PAIRING CODE** `uid=29_42`; **PAIR THIS BROWSER** `uid=29_46` | Success: public browser pairing navigated to `/student`. `call2-student-checklist-empty.snapshot.txt` identifies the bound student and says **Nothing assigned yet**. Mobile screenshot/snapshot were captured after pairing. Network: `GET /api/student/profile`, `/api/student/checklist`, `/api/student/today` all → `200`. |
| 14 | Parent-B cross-tenant negative: direct UI route to parent-A student | New isolated parent-B login using public `/api/auth/login?principal=parent-b`, then browser-navigate to the parent-A student URL observed in the parent UI | Public API requests `GET /api/students/{parent-A-student-id}` twice returned `404`; UI rendered **UNABLE TO LOAD**, not the parent-A record (`call2-parent-b-cross-tenant.snapshot.txt`, `call2-parent-b-desktop.png`). |
| 15 | Parent-B cross-tenant negative: collection isolation | Parent-B `/parent/tasks` | Empty task selector/list and **No tasks match** despite parent-A’s created task; `call2-parent-b-tasks-isolated.snapshot.txt`. This is a second tenant-isolation negative. |

## Schedule blocking evidence

The final browser-visible one-off form supplied a valid local start representation and blank RRULE. Its final public HTTP request (`reqid=72`, inspected through Chrome DevTools) was:

```json
{
  "studentId": "a40db714-ba59-4f45-93fd-191aa41a6563",
  "templateId": "1322c647-40ce-45a6-80f9-db5dd57fdcee",
  "revisionId": "cd36c09b-29ef-486a-8b5b-f94e69dd568c",
  "kind": "one_off",
  "timezone": "America/Chicago",
  "startAt": "2026-08-19T19:00:00.000Z",
  "dueOffsetMinutes": 0
}
```

The service response was HTTP `400` with:

```json
{
  "code": "invalid_request",
  "message": "explicit IANA timezone is required",
  "detail": "explicit IANA timezone is required"
}
```

Thus the exact explicit IANA timezone required by the acceptance is visibly sent by the public UI but rejected as absent. This is a product/API contract defect, not a missing selector, a test timing issue, or a reason to bypass the UI with a private API call.

## Selector record

Live selectors that worked in call 2:

- Parent test authentication: browser URL `GET /api/auth/login?principal=parent-a` / `parent-b`.
- Student creation: **ADD FIRST STUDENT** `5_68`; **DISPLAY NAME** `6_8`; **SAVE STUDENT** `6_13`.
- Task creation/publish: **Task title** `8_10`; **CREATE DRAFT** `8_13`; row **PUBLISH** `9_12`.
- Schedule form: **Task to schedule** `8_16`; task option `10_0`; **Student to schedule** `8_19`; student option `8_22`; **Schedule start** `8_23`; **RRULE** `8_45`; **SCHEDULE TASK** `8_48`.
- Task lifecycle/search: **Search tasks** `8_51`; **RETIRE** `9_12`.
- Student pairing: roster **OPEN** `26_88`; **ISSUE PAIRING QR** `27_34`; isolated student **PAIRING CODE** `29_42`; **PAIR THIS BROWSER** `29_46`.

UIDs are only valid for the exact listed snapshots; fresh snapshots are required before any re-drive.

## Console and network notes

- Auth remediation passed: the direct parent-A and parent-B browser routes both returned to authenticated parent UI on port 37940.
- Successful public API network evidence: `POST /api/students` → `201`, `POST /api/tasks` → `201`, `POST /api/tasks/{id}/publish` → `200`; student profile/checklist/today reads → `200` after pairing.
- Expected product failure: `POST /api/schedules` → `400` three times. The final response/body are documented above.
- Cross-tenant detail read: `GET /api/students/{parent-A-id}` → `404` twice in parent-B context.
- Console on parent schedule page: browser warning for the earlier invalid native datetime formatted attempt, HMR info, and `400 Bad Request` from schedule submission. Student mobile context had only Vite/React informational messages and no errors.

## Acceptance status

| Acceptance area | Result |
|---|---|
| Parent test-auth | **PASS** on remediated direct route |
| Create brushing task, parent-approval driver, publish | **PASS** (the UI states parent approval is the only enabled driver) |
| One-off schedule with explicit timezone | **FAIL** — browser sent `America/Chicago`, API rejected it as missing |
| Bounded daily/weekly recurrence | **BLOCKED** by the same schedule creation defect; no recurrence was privately seeded |
| Student today/detail/start/checked state | **PARTIAL** — real student browser paired and today/checklist API/UI loaded, but correctly empty because schedule creation failed |
| Parent reject/retry/approve | **BLOCKED** — no occurrence/check can exist |
| task/schedule/occurrence q/filter/sort/page URL state | **PARTIAL/FAIL** — task `q` URL state observed (`?q=g`); no full multi-character query due remount and no public filter/sort/page controls observed; schedule/occurrence collections blocked |
| retire/skip/cancel | **PARTIAL** — task retirement passed; occurrence skip and schedule cancel blocked because no schedule exists |
| restart/DST | **BLOCKED** — requires a successful schedule/occurrence |
| model-disabled flow | **PARTIAL** — UI visibly states parent approval is the only enabled verification driver; no separate public model-disabled control/state was available to exercise |
| two-tenant negatives | **PASS** — parent-B cannot fetch parent-A student (404/UI error) and parent-B sees no parent-A task in its collection |

## Promotion decision

Do **not** promote to `codify`: exploratory PASS requires the full flow to succeed once in Chrome. Correct the `/api/schedules` explicit-timezone validation/serialization mismatch, then re-run the one-off schedule and cover daily/weekly bounded RRULEs, occurrence collection URL state, student detail/start/check, reject/retry/approve, skip/cancel, and restart/DST. Do not use private database/repository seeding, mocks, or direct state edits to bridge the blocker.

## Call 3 — tzdata remediation re-exploration

- New isolated parent-A context authenticated through `http://127.0.0.1:37943/api/auth/login?principal=parent-a` (`call3-parent-home.snapshot.txt`). Existing tenant-scoped student was visible.
- Created and published **E2E Brushing Call Three** entirely through parent UI: `Task title` `uid=37_4`, `CREATE DRAFT` `37_5`, `PUBLISH` `38_5`; published snapshot `call3-published.snapshot.txt`.
- Created a one-off at `2026-08-19T14:00`, using the parent-A student and published revision. The real UI displayed browser alert **“Schedule saved and occurrences materialized.”** (accepted with Chrome dialog). `call3-occurrences.snapshot.txt` shows its `PENDING` occurrence.
- Created bounded recurrence through the same public schedule form: daily `FREQ=DAILY;COUNT=2`, start `2026-08-20T14:00`, and weekly `FREQ=WEEKLY;COUNT=2;BYDAY=FR`, start `2026-08-21T14:00`. Both emitted the same successful materialization alert. `call3-recurrences.snapshot.txt` shows the expected bounded future dates Aug 20, Aug 21, and Aug 28.
- Issued pairing material in parent UI and paired a fresh isolated student browser (one-use code intentionally omitted); snapshots `call3-paircode.snapshot.txt`, `call3-student-pair.snapshot.txt`.
- Student checklist showed live one-off card; detail route rendered **START TASK** (`45_6`). After click, it changed to **AWAITING_VERIFICATION** (`call3-started.snapshot.txt`). Parent occurrence state changed accordingly (`call3-awaiting.snapshot.txt`).
- Parent **REJECT** (`47_34`) returned occurrence to `PENDING` and exposed **RETRY** (`48_0`); refreshed student detail again showed **START TASK** (`call3-student-after-reject.snapshot.txt`). Student started again; parent observed `AWAITING_VERIFICATION` (`call3-awaiting-again.snapshot.txt`) then clicked **APPROVE** (`50_33`). Final occurrence collection shows **COMPLETED** (`call3-recurrences.snapshot.txt`).

### Call 3 remaining acceptance gaps / failure reason

The tzdata blocker is fixed: no `400`, and public one-off/daily/weekly schedules materialized. However exploratory PASS is still not available:

1. The public parent UI has a task collection and occurrence collection but no schedule collection/link, no schedule cancellation control, and no occurrence skip control in the live snapshots. All action cells only expose Approve/Reject/Retry. Thus schedule collection q/filter/sort/page state, cancellation, and skip cannot be exercised without inventing a private API path.
2. Only task `q` URL state was observed in call 2; the public UI still exposes no filter/sort/page controls for task, occurrence, or a schedule collection.
3. Restart and DST behavior have no public UI trigger/observable control in this stack. No process/database manipulation was performed to fake them.
4. The UI states parent approval is the only enabled verification driver, but exposes no separate model-disabled behavior/control to test.

The above are product-coverage gaps, not gating relaxations. No Playwright has been written.

## Call 4 — collections and lifecycle re-exploration

- Fresh isolated parent-A login at `http://127.0.0.1:37943/api/auth/login?principal=parent-a` succeeded. `call4-home.snapshot.txt` shows the new **Schedules** navigation item.
- **Schedules** (`call4-schedules.snapshot.txt`) is a live server-owned collection with **Schedule status filter** and explicit **AMERICA/CHICAGO** rows for one-off, `FREQ=DAILY;COUNT=2`, and `FREQ=WEEKLY;COUNT=2;BYDAY=FR`. Selecting **All schedules** moved browser URL to `/parent/schedules?status=all` (`call4-schedules-all.snapshot.txt`). Clicked daily **CANCEL SCHEDULE** `uid=55_27`; refreshed row is version `V2` with no cancel action (`call4-schedule-cancelled.snapshot.txt`).
- **Occurrences** (`call4-occurrences.snapshot.txt`) has public **Occurrence status filter** and **Occurrence sort direction** controls. Selecting Pending yielded `/parent/occurrences?status=pending`; selecting Latest first yielded `/parent/occurrences?status=pending&dir=desc` (`call4-occurrences-pending-desc.snapshot.txt`). Network confirmed matching server requests with `status=pending` and `dir=desc` returning 200.
- Clicked a live **SKIP** control (`58_84`): server call `POST /api/occurrences/eafd9b7f-a8f6-47db-94e2-40118ec49c51/skip` → 200; all-status collection displays it as **EXCUSED** (`call4-all-after-skip.snapshot.txt`). Clicked a different live **CANCEL** control (`58_75`): server call `POST /api/occurrences/7b30375f-b7cb-4a5a-9be3-b867b823504e/cancel` → 200; evidence `call4-occurrence-cancelled.snapshot.txt`.
- No console messages occurred during the collection/lifecycle run. Browser network evidence is listed above and in the MCP transcript.

### Call 4 conclusion

The newly exposed schedule collection, cancellation, occurrence status/sort URL state, skip, and occurrence cancellation all work through public UI/API boundaries. The overall exploratory suite still **does not achieve full PASS**: no user-visible public restart control/observable restart behavior, no user-visible DST-specific occurrence scenario/control, and no distinct model-disabled flow/control exists in the real UI. Existing text only says parent approval is the sole enabled verification driver; it is not a testable disabled-model branch. No fake restart, clock manipulation, model toggle, mock, or private seeding was used.

## Call 5 — model, restart, and DST remediation

- `make tasks-endpoints` initially confirmed web `http://127.0.0.1:37943/`, API `37942`. In a fresh isolated authenticated parent browser, Tasks visibly showed **Model provider: disabled** and server start **8/19/2026, 3:00:34 AM** (`call5-tasks-before.snapshot.txt`). The schedule form now exposes editable **IANA timezone** defaulting to `America/Chicago` as a real labeled textbox.
- Created two public America/New_York recurrence schedules with the form and observed **“Schedule saved and occurrences materialized.”**: weekly `FREQ=WEEKLY;COUNT=2;BYDAY=SA`, start `2027-03-13T09:00`, and a nearer fall DST-crossing daily `FREQ=DAILY;COUNT=3`, start `2026-10-31T09:00`. The schedules collection displays the former as `AMERICA/NEW_YORK` (`call5-dst-schedule.snapshot.txt`) and persisted it through restart (`call5-schedules-after-restart.snapshot.txt`).
- Per explicit caller request, restarted the real dev stack using `make tasks-down && make tasks-up && make tasks-endpoints`; the direct port changed to web `37946`, API `37945`. Re-authenticated via public browser route on the new web port. Tasks then showed **Model provider: disabled** and server start **8/19/2026, 3:02:52 AM** (`call5-tasks-after-restart.snapshot.txt`), proving server-derived health state changed after actual restart. Schedule collection persisted after restart and its **REFRESH SERVER STATE** control was clicked.
- Existing lifecycle/collection proof from calls 3–4 remained server-persisted after restart: completed/rejected/retried/approved one-off, schedules status URL, occurrence status/sort URL, schedule cancellation, occurrence skip/excused, and occurrence cancellation.

### Call 5 blocker

The UI accepted both New York DST schedules and retained timezone/cadence in the schedule collection, but the public occurrences collection only materialized/listed Aug 2026 records (`call5-dst-october.snapshot.txt`); it exposed no Oct 31/Nov 1/Nov 2 or Mar 2027 occurrences. Therefore a real, observable DST boundary result (date/time offset or generated boundary occurrence) cannot be verified through the public UI. No clock/database manipulation or private materialization call was used. Console had no error (only Vite connection and a form-id/name audit issue); occurrence collection API reads returned 200.

## Call 6 — 365-day horizon DST re-exploration

- Fresh isolated parent-A login at current web `http://127.0.0.1:37949/` succeeded. Tasks visibly still reports **Model provider: disabled** and server start timestamp `3:05:55 AM` (`call6-form.snapshot.txt`), preserving the model-disabled/restart observability established in call 5.
- Using only the public schedule form’s selected published task/student, **IANA timezone** textbox, and RRULE textbox, created bounded America/New_York daily recurrences: fall start `2026-10-31T09:00`, `FREQ=DAILY;COUNT=3`; spring start `2027-03-13T09:00`, `FREQ=DAILY;COUNT=3`. Both submissions displayed real browser alert **“Schedule saved and occurrences materialized.”**
- The public Occurrences UI now materializes both boundaries within the 365-day horizon: Oct 31 / Nov 1 / Nov 2 2026, and Mar 13 / Mar 14 / Mar 15 2027, all as PENDING (`call6-dst-results.snapshot.txt`). This proves public recurrence materialization across both DST date boundaries.

### Call 6 blocker / conclusion

The public Occurrences UI still renders only the calendar date and status for those records; it exposes **no local time, UTC time, offset, timezone, EDT, or EST text** in the live a11y snapshot (verified by searching the saved snapshot). Therefore the caller's required confirmation that records *expose local times/offsets in the public Occurrences UI* cannot be made honestly. No direct/private API inspection, database access, mocked time, or seeding was used to infer the offsets. Existing public lifecycle, collection URL state, tenant-negative, model-disabled, and restart observations remain recorded in calls 2–5.

## Call 7 — public DST offset confirmation and exploratory promotion

- Fresh isolated parent-A browser re-opened the server-owned occurrences collection at `http://127.0.0.1:37949/parent/occurrences?dir=asc`.
- The public UI now exposes each New York local nominal time, timezone, and short DST abbreviation. Fall: **Oct 31, 2026, 10:00 AM EDT · AMERICA/NEW_YORK**; **Nov 1, 2026, 10:00 AM EST · AMERICA/NEW_YORK**; **Nov 2, 2026, 10:00 AM EST · AMERICA/NEW_YORK**. Spring: **Mar 13, 2027, 10:00 AM EST · AMERICA/NEW_YORK**; **Mar 14, 2027, 10:00 AM EDT · AMERICA/NEW_YORK**; **Mar 15, 2027, 10:00 AM EDT · AMERICA/NEW_YORK**. Evidence: `call7-dst-offsets.snapshot.txt`, `call7-dst-offsets-desktop.png`.
- Browser network collection requests returned 200. Console had Vite connection info and a browser form id/name audit **issue**, not a console error.

### Exploratory acceptance result: PASS

All requested acceptance criteria have now been observed across the real public browser flow in calls 2–7:

1. Test-auth parent created/published parent-approval brushing task; one-off scheduled with explicit IANA timezone.
2. Bounded daily and weekly recurrence schedules materialized with explicit timezone; public DST daily records show the EST↔EDT transitions above.
3. Separate paired student browser showed today checklist/detail, started task, and entered awaiting-parent verification; parent reject/retry/approve completed it.
4. Task query URL, schedule status URL, and occurrence status/sort URL states were observed; all collections made real server-owned requests.
5. Parent retired a task; cancelled a schedule; skipped/excused and cancelled occurrences.
6. Real compose restart changed server-derived StartedAt in Tasks; model provider was visibly `disabled`.
7. Parent-B collection/detail access remained tenant-isolated (empty parent-B collection and 404 for parent-A student record).

The flow was never privately seeded or mocked. Promotion is therefore allowed to `codify`; this exploration call still writes no Playwright.

## Call 8 — fresh Android pairing evidence setup (no Playwright)

- Reused existing isolated, authenticated parent-A Chrome context and navigated only through the public Students UI. Opened the real student detail with **OPEN** `uid=76_39`, then clicked **ISSUE PAIRING QR** `uid=77_15`.
- The parent UI rendered fresh, one-use QR material. Exact rendered evidence: `android-fresh-parent-qr.png`; snapshot: `android-fresh-parent-qr.snapshot.txt`; short selector/network note: `android-fresh-parent-qr.md`.
- Browser network confirms the public action `POST /api/students/{student-id}/pairing` → 200; console had no messages. Pairing secret is intentionally not transcribed outside the rendered QR artifact. No manual code, private API call, mock, or seeding was used.
- Despite phase `codify`, caller explicitly required this evidence setup before Playwright promotion; no Playwright was written.

## Call 9 — immediate fresh Android QR

- In existing authenticated parent-A Chrome, clicked the public **ISSUE A NEW CODE** selector `uid=77_15`.
- Fresh exact QR artifact: `android-fresh-parent-qr-current.png`; rendered expiry date and capture timestamp are recorded without transcribing the secret in `android-fresh-parent-qr.md`.
- Public pairing request returned 200 (reqid 85); no manual code or private API action was used. No Playwright was written.

## Call 10 — final Android coordination QR

- Existing authenticated parent-A Chrome context issued a fresh parent QR via **ISSUE A NEW CODE** `uid=77_15`; public pairing request reqid 86 returned 200.
- Exact artifact: `android-qr-final.png`; timestamp/expiry and selector note: `android-fresh-parent-qr.md`.
- Parent context intentionally remains open at the rendered QR; awaiting user confirmation that Android has paired and started an occurrence. No Playwright and no manual-code/seeding action occurred.

## Call 11 — Android parent decision coordination (blocked before mutation)

- Reused authenticated parent-A context and opened public occurrence URL `/parent/occurrences?status=awaiting_verification&dir=asc`.
- The live UI contains **two** indistinguishable human-readable `E2E Brushing Call Three` occurrences in `AWAITING_VERIFICATION`: one due **Aug 20, 2026, 2:00 PM CDT** (UI record id shown as `22f44c2c-70db-4e28-aee5-aee3ef5282f3`) and one due **Aug 21, 2026, 2:00 PM CDT** (shown id `30f929a3-e6d8-4038-a149-926cae3cb4f5`). Both expose Approve/Reject/Retry/Skip/Cancel. Evidence: `android-parent-awaiting.snapshot.txt`.
- The Android coordination report names only the duplicated task title; it does not identify which due time/occurrence Android started. To avoid rejecting the wrong real occurrence, no parent decision was clicked. Await a matching due time or occurrence identifier from the Android operator, then reject that exact row and continue the manual retry/approve sequence.

## Call 12 — Android target reject and parent retry

- Latest Android mapping identified target `30f929a3-e6d8-4038-a149-926cae3cb4f5`, due **Aug 21, 2026, 2:00 PM CDT · AMERICA/CHICAGO**. The parent awaiting-verification snapshot uniquely matched that ID, due, title, and **REJECT** selector `uid=83_57`.
- Clicked that exact public UI **REJECT**. Network: `POST /api/occurrences/30f929a3-e6d8-4038-a149-926cae3cb4f5/decision` → 200 (reqid 113). It vanished under the `awaiting_verification` filter; the public Pending filter showed the same ID as **PENDING** with parent **RETRY** `uid=86_106`.
- Per Android coordination, clicked that exact public **RETRY**. Network: `POST /api/occurrences/30f929a3-e6d8-4038-a149-926cae3cb4f5/retry` → 200 (reqid 141). On all-status refresh the same ID is now **AWAITING_VERIFICATION** and has public **APPROVE** `uid=88_152`, indicating it is ready for parent approval after retry.
- Evidence: `android-parent-awaiting-current.snapshot.txt`, `android-parent-target-rejected.snapshot.txt`, `android-parent-target-pending-after-reject.snapshot.txt`, `android-parent-target-after-retry-all.snapshot.txt`. No Playwright was written.

## Call 13 — parent approval for Android-mapped retry

- In the same authenticated parent-A context, revalidated exact target row `30f929a3-e6d8-4038-a149-926cae3cb4f5` at **Aug 21, 2026, 2:00 PM CDT · AMERICA/CHICAGO**, state AWAITING_VERIFICATION, then clicked its row-local **APPROVE** `uid=88_152`.
- Public request `POST /api/occurrences/30f929a3-e6d8-4038-a149-926cae3cb4f5/decision` returned 200 (reqid 169). The same exact ID now renders **COMPLETED** in `android-parent-target-approved.snapshot.txt`.
- Android rerun handoff: parent approval is complete. Refresh Android and verify checked/completed state, force-stop persistence, and foreign denial. No Playwright was written.

## Call 14 — real Parent-B foreign occurrence for Android negative

- Cancelled an unsubmitted Parent-A add-student dialog and opened a separate isolated Parent-B session via public `/api/auth/login?principal=parent-b`.
- Parent-B public roster flow created student **Android Foreign Parent B Student**, identity `3e2a2310-90c1-484e-953f-3ebda5101ffd`; no pairing material was issued.
- Parent-B public Tasks UI created/published **Android Foreign Parent B Task** and scheduled it for that student with one-off start `2026-08-20T14:00` and default explicit `America/Chicago` timezone. Browser showed schedule materialization confirmation.
- Parent-B public Occurrences UI shows the resulting foreign record: **occurrence UUID `62b79178-af55-4ca7-8cb8-8f4000252d2b`**, Parent-B student **Android Foreign Parent B Student** (`3e2a2310-90c1-484e-953f-3ebda5101ffd`), due **Aug 20, 2026, 2:00 PM CDT · America/Chicago**, status PENDING. Evidence: `android-foreign-parent-b-occurrences.snapshot.txt`.
- Network occurrence collection reads returned 200; no private API, seed, pairing, or Playwright was used.
