# Phase 4 student dialogue evidence (sanitized)

## Fresh Terra Chrome exploration — call 13

A clean local Phase 4 stack was started with its test-only scripted provider. No QR material, pairing codes, cookies, bearer tokens, raw WebSocket frames, raw inspect payloads, screenshots, or model reasoning are retained here.

### Passed live browser path

- Parent A created a student, configured the bounded chapter dialogue, published it, scheduled it, and issued a fresh browser pairing.
- The freshly paired student started the occurrence. Three distinct source-grounded answers completed the requirement and occurrence.
- An insufficient mortar answer stayed at **1 of 3** and displayed the bounded retry state. An injection/answer-key/completion request at **2 of 3** was rejected and did not complete the occurrence.
- After two accepted answers, a browser reload reconnected to the same durable attempt with **2 of 3** and only the rushing-the-work question remaining.
- Parent Inspect showed separate student answers, questions, accepted/rejected safe rationales, and policy version. An override was appended and immediately reconciled in the timeline without rewriting prior evidence.
- The first fresh path exposed a focused defect: a rejected follow-up preceding a later accepted evaluation kept the same question current. The product now treats a question as advanced when any durable evaluation for it is accepted; a regression unit test covers the rejected-then-accepted sequence.
- The same fresh path exposed missing visible evaluation provenance. The product now persists server-selected provider/model provenance with each evaluation and returns latest provider/policy in Inspect. A second freshly scheduled occurrence completed 3/3 and Inspect visibly displayed `scripted`, `primer-dialogue-fixture`, and `dialogue.v1`; no reasoning was displayed.
- A second tab connected to the second occurrence at 2/3 and received the terminal completion from the first tab before it could submit its prepared final answer. This confirms same-attempt delivery recovery, but is **not** sufficient evidence of the required real concurrent conflict path.
- Chrome console was clean for the final parent inspect navigation. Final live UI uses only generic thinking/evaluating states and safe rationale; no raw reasoning was observed in those surfaces.
- Mobile dark emulation on the parent inspect page returned Lighthouse accessibility 100. This is supplemental only; it does not satisfy the complete desktop/mobile dark/light matrix.

### Commands

- `cd primer-tasks && go test ./internal/api -run 'TestPostgresPairingReplayTenantAndArchiveBoundaries' -count=1` — PASS after the focused pairing-origin fix.
- `cd primer-tasks && go test ./internal/agent ./internal/api ./internal/verification -count=1` — PASS after the focused fixes.
- `cd primer-tasks && make tasks-test` — PASS on the final working tree.

## Fresh Terra Chrome exploration — call 15

- Fresh browser pairing issued from the parent UI and claimed through the student pairing page. A newly authored/published/scheduled `Terra Phase4 Call15 Dialogue` occurrence completed three server-evaluated answers.
- The student completed the first two answers, reloaded, and the durable state resumed at **2 of 3** with only the rushing question. A separate fresh `Terra Phase4 Concurrency` occurrence was driven in two actual same-session Chrome tabs.
- With the real scripted worker delay enabled only in the existing development stack, both tabs submitted the mortar answer. The second tab received the visible typed conflict, **"Another device is already answering this turn."** The first answer evaluated once, the conflict cleared upon the next durable question, and the recovered student completed the third answer. This exposed a focused client defect: same-cursor ephemeral conflict frames were discarded as replays. `clients/typescript/src/dialogue-client.ts` now preserves error frames at the current cursor and clears the transient error on a later durable progress/question/complete event.
- Parent created a distinct second student and scheduled a distinct occurrence. A new Chrome WebSocket, authenticated only by the first student browser session, subscribed to that foreign occurrence and received the generic `not_found` / `This task is unavailable.` denial. The sanitized result is `chrome-call15-foreign-subscribe.json`; no credentials or raw frames were retained.
- Desktop snapshot Lighthouse accessibility was 100. The post-restart console was **not clean**: it contains one expected-but-still-gating WebSocket-close warning caused by the intentional API recreation. It cannot support the clean-console gate.
### Call 15 commands

- `cd primer-tasks && go test ./internal/api ./internal/agent ./internal/verification -count=1` — PASS.
- `cd primer-tasks && make tasks-test` — PASS.
- `cd primer-tasks && npm --prefix clients/typescript test` — PASS (6 tests).
- `cd primer-tasks && npm --prefix web run typecheck && npm --prefix web run lint` — PASS; lint retains two pre-existing warnings in Phase 2/3 specs.
- Chrome DevTools: new-page/snapshots, UI authoring/pairing/dialogue, two-tab race, fresh socket foreign-subscribe, desktop snapshot Lighthouse, console/network inspection.

## Fresh Terra web completion — call 17

- Reviewed revised web-only plan at `24cd3abe`. A fresh direct local web/API stack was exercised with the development-only scripted Fantasy provider; no pairing material, browser credentials, raw WebSocket frames, raw inspect payloads, screenshots, or model reasoning are retained in this note.
- **Malformed and timeout:** each fault retained the student answer at 0/3 or 1/3, showed the generic durable *Verifier unavailable* retry state, and never completed the occurrence. After clearing the fault, the new narrow client `retry()` command re-enqueued the already durable answer and advanced it exactly once. The focused fix is in `clients/typescript/src/dialogue-client.ts` and `web/src/StudentDialoguePage.tsx`; a refresh-only Retry control had left a failed answer stranded.
- **Authorization and audit:** a student-session WebSocket attempting to subscribe to a separate student's occurrence received only `not_found` / `This task is unavailable.`. Parent Inspect showed separate student text, question/evaluation records, safe rationale, scripted provider/model and `dialogue.v1`; an override immediately appeared in the audited-override region without changing existing timeline entries.
- **Browser acceptance / promotion:** the fresh Playwright suite at `primer-tasks/web/e2e/phase4-student-dialogue.spec.ts` passed 2/2 against the real stack: three-answer completion including insufficient and injection rejection, reconnect at 2/3, parent inspect/append-only override, and foreign socket denial. A separate Chrome DevTools redrive independently authenticated and reloaded Inspect evidence. Earlier sanitized call-15 same-session two-tab typed-conflict/recovery remains retained as the concurrent-browser evidence.
- **Privacy/accessibility/clean browser:** source/DB-count/log scan is `privacy-scan-call17.txt`; it found only client allowlist names, no `complete_task`, no raw-reasoning payload/log result, and only safe durable counts. Fresh Inspect navigation had no console warn/error messages and its 35 network requests were successful page/static/API requests. Desktop dark and mobile light Inspect snapshots both scored Lighthouse accessibility 100; mobile navigation exposed the usable Menu control and light-theme toggle.

### Web/server gate result

- `make tasks-test` — PASS.
- `go test -race ./internal/verification/... ./internal/agent/... ./internal/api/... -count=10` — PASS.
- `npm --prefix clients/typescript test`, `npm --prefix web run typecheck`, and `npm --prefix web run lint` — PASS (lint retains only two pre-existing Phase 2/3 warnings).
- `make tasks-cover` initially measured **84.9%**, below the mandatory 85% minimum.

## Focused web/server coverage completion — call 18

- Added only deterministic coverage to the existing real-Postgres dialogue integration path and fixture worker unit tests. The integration assertion proves a failed provider job projects `status:error` with the generic durable-answer explanation before retry; it also asserts parent Inspect returns the server-selected provider and policy provenance. Worker tests cover unsupported source fail-closed selection and the scripted question-only follow-up path. No acceptance state is injected or mocked.
- `make tasks-cover` — **PASS**, `85.0% >= 85%`.
- Full rerun passed: `make tasks-test tasks-agent-compat tasks-clients tasks-web`; TypeScript client tests (6/6); web typecheck/lint (only two existing unrelated warnings); and `go test -race ./internal/verification/... ./internal/agent/... ./internal/api/... -count=10`.
- Promoted `phase4-student-dialogue.spec.ts` reran **PASS 2/2** against the real local stack. A fresh Chrome DevTools context independently authenticated and enumerated durable completed occurrences after the coverage-only test change.
- The final browser matrix covered desktop and mobile viewports in both light and dark themes. Lighthouse accessibility was 100 for the recorded desktop and mobile checks; responsive navigation remained usable, and the fresh final Inspect navigation had no console warnings/errors and successful page/static/API network requests.
- Final anti-cheat source/diff scan is `privacy-scan-call18.txt`. It contains only explicit client unsafe-field denylist terms; no completion tool, raw-reasoning transport, client policy field, or diff whitespace defect was found.
