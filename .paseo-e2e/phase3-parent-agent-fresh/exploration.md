# Phase 3 parent-agent — fresh Terra re-drive

## Call #1 — 951ed6c, real Compose/Stacklane stack

- Started a fresh `primer-tasks-tasks-p3-terra` Compose project from this worktree with `TASKS_MODEL_PROVIDER=scripted`; Stacklane was degraded/unresolved, so the documented Docker loopback origin `http://127.0.0.1:38126` was used.
- **Required first command PASS:** From a fresh unauthenticated public page, drove `CONTINUE WITH PARENT SIGN-IN` → test issuer `Parent A` → `Parent agent`; submitted exactly one agent command, `List my tasks.`. The terminal UI showed generic `Thinking`, allowlisted `List students` progress, safe assistant text, `CURSOR 11`, and run state `completed`. See `01-preclear.snapshot.txt` through `06-list-terminal-or-blocked.snapshot.txt`.
- Wire/progress evidence: durable read-only PostgreSQL record in `08-db-readonly.txt` has ordered event sequences 1–11 ending in `terminal` / `completed`, with only generic thinking and allowlisted tool labels; no raw arguments or reasoning delta were stored. It also records a succeeded scripted run and done job with released lease. The browser snapshot shows the same cursor and terminal state. DevTools resource entries exposed only the conversation creation requests (WebSocket frames are not surfaced by this MCP); UI/progress plus durable event rows evidence the stream.
- Browser storage after login was empty `localStorage` and `sessionStorage`; it had only the expected CSRF cookie (value not recorded). No provider/auth tokens appeared in browser storage. API logs were startup-only and had no secret/reasoning hit (`07-api.log`). Artifact text scan found only intentional product copy/field names, not raw reasoning/credentials.
- **Create/publish/schedule PASS:** Created `Terra Fresh Student` in the ordinary public Student dialog, then used the public parent-agent command `Create and schedule a task for my student.`. It reached terminal `completed`, cursor 26, and visibly streamed Draft task → Publish task → List students → Create schedule. Public Tasks and Schedules snapshots are `13-tasks-agent-effect.snapshot.txt` and `14-schedules-agent-effect.snapshot.txt`.
- **Ambiguity/no mutation UI branch PASS (terminal clarification):** `Ambiguous Alex: retire a task.` streamed a list lookup then completed with `Please clarify which student you mean; I will not guess.` (`15-agent-ambiguity-empty.snapshot.txt` plus DevTools terminal snapshot). A before/after mutation-count DB assertion remains to be captured.

## Call #2 — destructive confirmation re-drive

- Reproduced the original invalid `text_delta` confirmation event. The parent supplied focused fix `6c19ece`, which instead emits schema-valid `tool_progress`.
- Fresh post-fix public `Retire this task.` reached preview with no mutation and explicit `CONFIRM PREVIEW`; clicking confirmation **exactly once** consumed the server preview (read-only `parent_confirmation_previews.consumed_at` changed), but no follow-up durable event was recorded and the UI remained on the active confirmation. There were no browser console errors. `19-confirm-redrive-result.snapshot.txt` and the terminal DB inspection show the failure.
- This is a concrete destructive-flow blocker: the state-changing confirmation completes server-side without a durable/published acknowledgement, leaving the public UI stale and offering a replayable-looking confirmation control. Do not treat confirmation/stale/replay as passing.

## Calls #13–14 — paced cancellation and restart

- `c9b292a` corrected cancellation propagation. With API scripted pacing at 5000ms, the public Parent A command visibly reached cursor 1 / `RUN STATE running` / `CANCEL RUN`; normal click then rendered `CANCELED`, cursor 2, `RUN STATE cancelled`, and no tool mutation (`75-cancel-fix-result.snapshot.txt`).
- A separately paced active Create-and-schedule run was restarted at API level after cursor 1. The mounted browser ultimately rendered the ordered four-tool sequence, cursor 26, and one completed terminal (`76-call14-active-restart.snapshot.txt`, `77-call14-postrestart.snapshot.txt`). This is a restart-survival observation, **not a remount/cursor-replay PASS**: the page's remount creates a new conversation, so it cannot subscribe to the prior run via normal UI.
- Restricted-tool attempt (`TASKS_AGENT_ACTIVE_TOOLS=list_students`) with public Create-and-schedule did not produce a failed/unavailable terminal inside 10 seconds (`78-call14-tool-restricted.snapshot.txt`). Do not count it as tool-failure coverage. API is restored to scripted standard tools and delay 0.

## Call #11 — lifecycle/failure coverage (partial)

- Fresh authenticated Parent A agent page started at `55-call11-agent-ready.snapshot.txt`. An API-container restart was scheduled 250ms after sending scripted `Disable the schedule.`. The command nonetheless had already reached one completed terminal at cursor 17 before the restart; the API came back healthy and read-only DB inspection recorded one terminal run (`agent_runs` / event count), but this is **not evidence of a mid-run restart or cursor replay**. Scripted streaming has no public delay/barrier knob, and the normal UI has no user-visible active state long enough to click `Cancel run`; therefore reconnect/remount-mid-run, restart-mid-run release, and active cancel remain unproven rather than passed.
- **Disabled/manual PASS:** recreated only the Terra API with `TASKS_MODEL_PROVIDER=disabled`; fresh public parent command showed `AGENT UNAVAILABLE`, explicit ordinary Tasks/Schedules links, disabled composer, terminal `UNAVAILABLE`, and cursor 18 (`59-call11-disabled-terminal.snapshot.txt`). No response or mutation was simulated.
- **Provider-start failure PASS:** recreated only the Terra API with `TASKS_MODEL_PROVIDER=bedrock` without provider configuration. A public `List my tasks.` terminally failed safely at cursor 3 with only `The configured provider could not start.` (`63-call11-provider-failure-terminal.snapshot.txt`). Read-only DB check recorded the run as `failed`.
- **Tool-failure configuration attempt NOT PASS:** recreated with scripted mode and `TASKS_AGENT_ACTIVE_TOOLS=list_students`, then sent `Disable the schedule.`. The public run nevertheless completed and displayed its ordinary tool labels (`61-call11-tool-failure-terminal.snapshot.txt`), so this deployment configuration does not induce a tool failure. It must not be counted as failure coverage. The stack was restored to scripted mode with the standard active-tool set.

## Call #10 — stale schedule confirmation PASS

- Avoided the intermittent shared-profile preemption by clearing only processes with the exact `paseo-e2e-profile` user-data-dir before starting the browser, then keeping related browser actions in immediate serial MCP batches.
- Fresh public Parent A flow re-driven: access page → issuer `Parent A` → Parent agent → required scripted command `Disable the schedule.`. The server streamed only generic `Thinking` and allowlisted `List schedules` / `Prepare change`, ended completed at cursor 17, and visibly rendered `CONFIRM PREVIEW` (`41`–`44` artifacts).
- Kept that authenticated agent tab open, opened a second normal public Schedules tab, and used ordinary `CANCEL SCHEDULE`. The first active row disappeared after the server refresh (`52` and `53` artifacts); this was the same first enabled schedule selected by scripted `list_schedules`.
- Returned to the still-authenticated original Agent WebSocket and clicked its normal `CONFIRM PREVIEW` control. The server rejected the now-stale handle and the UI rendered `AGENT CONNECTION PROBLEM`, `confirmation_rejected`, and `CONFIRMATION_REJECTED` at cursor 36 (`54-call10-stale-confirm-result.snapshot.txt`). This passes the required stale handle control: the ordinary public cancellation won; no agent mutation was applied.

## Call #9 — Chrome profile contention (blocked)

- Required first action, `chrome_devtools_list_pages`, initially failed on the stale shared profile owner. After terminating the stale profile root (`201449`) and clearing only its Singleton entries, the second list succeeded with `about:blank`.
- Public Parent A login was re-driven on the active Terra stack (`http://127.0.0.1:38126`) and the ordinary public Parent-agent command `Disable the schedule.` was entered and sent.
- Chrome MCP was then preempted by a newly launched unrelated Slack Chrome (`PID 337580`, its parent `1`) using the same `paseo-e2e-profile`; the required preview/cancel/stale-confirm results could not be observed. Details: `40-call9-chrome-profile-blocker.md`.

## Not yet complete — remain explore

No Playwright files were written. Remaining mandatory exploratory cases: destructive preview/one confirmation/stale/foreign/replay, disconnect/reconnect cursor + durable restart, cancel, provider/tool failure, disabled/manual path, cross-tenant, origin/CSRF, slow subscriber, complete wire/DB/log/browser sanitation, and System C responsive/light/dark/axe. Therefore this suite is **not promotable**.

## Call #14 — fail-closed durable-cursor remount blocker

- After resetting to `de371bc72f6b7df5c0a71b94cd3b3054126d68b4` and recreating a fresh real `tasks-p3-terra` Compose/Postgres stack with scripted Fantasy plus a 5000ms scripted barrier, a fresh Parent A browser command reached real generic `Thinking`, `CURSOR 1`, and `RUN STATE running` (`84-call15-midrun-before-remount.snapshot.txt`).
- A true `/parent/agent` browser remount during the barrier showed the later terminal at cursor 11 (`85-call15-remount-replay-terminal.snapshot.txt`), but this is not durable replay: public network inspection recorded fresh `POST /api/agent/conversations` 201 responses on mount/remount, so the page created a new conversation and had no prior conversation/run/cursor to subscribe with.
- A second true remount after the terminal had already occurred rendered `Ready for a parent command`, `CURSOR —`, and `No active run`, not the durable terminal/history (`87-call15-postterminal-remount-no-replay.snapshot.txt`). The prior terminal was only a live tenant-wide fan-out observation, not a replay.
- This is a product blocker for the mandatory durable cursor replay after a browser remount/reconnect. Stop exploration per assignment; do not promote or write Playwright. Exact sanitized procedure/evidence: `88-call15-remount-cursor-replay-blocker.md`.

## Call #15 — 53634e4 replay fix PASS; restricted-tool authority blocker

- Reset to `53634e48f0837b82e2f3514de4a7518ad57f9b4e`, preserving suite evidence, then destroyed/recreated the real `tasks-p3-terra` Compose/Postgres/scripted-Fantasy stack with a 5000ms barrier. Stacklane daemon was reachable but its FQDN unresolved; used documented fresh loopback `http://127.0.0.1:38153`.
- **Mandatory true remount/reconnect PASS:** Parent A's running List command at cursor 1 was browser-remounted and then replayed the original progress/final/terminal to cursor 11 (`93`–`95`). Network inspection showed only the original 201 conversation creation request and no new conversation POST after either remount; scoped session storage held the durable conversation ID (`96`).
- **Restart at durable boundary PASS:** delayed second List command was API-restarted after cursor 12 / generic Thinking and recovered to exactly one completed terminal at cursor 22 (`97`–`99`).
- **Restricted active-tool failure BLOCKER:** recreated the live API with verified `TASKS_AGENT_ACTIVE_TOOLS=list_students` only. The public real-WebSocket `Create and schedule a task for my student.` nevertheless invoked and completed Draft task, Publish task, List students, and Create schedule, ending completed at cursor 48 (`102`). This is a server authority/allowlist bypass, not a tool-failure pass. Stop fail-closed; do not continue remaining tenant/security/backpressure/a11y gates or write Playwright. Sanitized detail: `103-call15-restricted-tool-fail-closed-blocker.md`.

## Call #16 — e212920 restricted authority fix PASS

- Reset to `e212920445c0e94e72d64bab2da4e8de489c03d8`, preserving evidence, and rebuilt a fresh real Compose/Postgres/scripted-Fantasy Stacklane-aware project. `scripts/dev check` passed; Stacklane FQDN remained unresolved, so the documented fresh loopback `http://127.0.0.1:38164` was used.
- **Restricted list_students PASS:** with the live API configured `TASKS_AGENT_ACTIVE_TOOLS=list_students`, fresh public Parent A Create-and-schedule reached a bounded failed terminal at cursor 4 after `Draft task` failed (`109`). Authenticated public before/after reads both had zero tasks and zero schedules (`108`, `110`).
- Standard-allowlist fresh tenant re-drive created a normal student/task/schedule for Parent A (`113`–`116`). Fresh Parent B ordinary/public reads were zero students/tasks/schedules and its tenant-A-name prompt returned no A name, count, record, or oracle (`119`, `120`, `122`).
- Current browser WebSocket missing-CSRF and sandboxed opaque-origin negative probes closed; expected 403 entries are the only console errors (`123`–`125`). Responsive desktop/mobile dark/light snapshots/screenshots pass; DevTools Lighthouse accessibility is 100 desktop and mobile (`126`–`137`).
- Remain explore and do not promote: a direct real slow-subscriber queue-overflow observation is not controllable through the public browser, and no axe-core package is installed (Lighthouse evidence is not labeled axe). No Playwright was written. Full sanitized evidence: `138-call16-e212920-exploration-summary.md`.

## Call #17 — mandatory axe evidence; public slow-subscriber close blocker

- Installed `axe-core@4.13.0` temporarily as a browser audit asset, injected it against the real authenticated System C page at mobile/desktop dark/light, then removed every temporary dependency/SUT asset. Mobile dark/light had zero violations and no incomplete checks (`140`, `142`). Desktop light/dark had zero violations with only axe `color-contrast` incomplete, retained as incomplete rather than relabeled pass (`143`, `145`).
- A real public authenticated protocol probe opened a 1 KiB receive-buffer slow subscriber on Parent B's durable conversation at cursor 22 and deliberately held its reader. A separate public authenticated WebSocket then submitted twenty read-only List commands. No task/schedule effects occurred (`154`).
- The worker completed durable commands and a fresh public reconnect with cursor 450 recovered completed terminal cursor 451 (`153`), but the evicted slow subscriber delivered only four events and then timed out; it was not actually closed (`152`). This violates the required bounded-subscriber behavior: eviction must close the WebSocket with a resumable cursor, not merely stop forwarding.
- **Product blocker.** Stop fail-closed and do not promote/write Playwright. Sanitized protocol + axe evidence: `155-call17-backpressure-and-axe-summary.md`.

## Call #18 — 8182524 actual slow-subscriber socket close PASS

- Preserved evidence, reset to `8182524b528afed8a5dd4a6f9b7db663314523a8`, and recreated fresh real Stacklane-aware Compose/Postgres/scripted Fantasy. `scripts/dev check` passed; Stacklane FQDN unresolved so documented loopback `http://127.0.0.1:38177` was used.
- Fresh Parent B List command established durable cursor 11 (`157`–`160`). A temporary, deleted public authenticated WebSocket slow subscriber used the browser BFF cookie/CSRF protocol with a 1 KiB receive buffer and held reads while an independent public authenticated WebSocket submitted 20 read-only List commands.
- The delayed slow reader observed a real closed network connection after the burst (`161`), fixing the previous open-but-silent subscriber eviction. A fresh authenticated public cursor-220 reconnect replayed ordered durable progress to completed terminal cursor 231 (`162`). Parent B public task/schedule reads remained empty (`163`).
- Axe evidence remains explicit: mobile dark/light zero violations/no incomplete; desktop dark/light zero violations with only `color-contrast` incomplete, retained as incomplete (not relabeled full axe) and paired with Lighthouse accessibility 100. Evidence and temporary audit/probe artifacts were sanitized/removed. Full record: `164-call18-backpressure-fix-pass.md`.
- Exploratory acceptance PASS. No Playwright was written; promote suite state to `codify` for the next call.

## Promotion call #19 — Playwright blocked

- Added the smallest real-stack Playwright Phase 3 spec using observed role/label selectors and no mocks, skips, or private DB mutation. Its first focused command is retained verbatim in `last-run.log`.
- The focused runner exited 1 after the controlled restricted-tool API reconfiguration recreated the real Vite service onto a new loopback port. The spec discovered the port, but the public Parent-A sign-in readiness view was not yet being served by the rebuilt proxy; it failed its ordinary public readiness assertion rather than weakening it.
- No green Playwright result means no independent Chrome re-drive, phase `run`, commit, or promotion. Temporary Playwright output was removed. Exact sanitized failure record: `promotion-call19-blocker.md`.

## Promotion call #20 — focused Playwright + Chrome PASS

- Replaced Vite-root-only readiness with bounded condition polling of the observed real BFF-proxied `/api/auth/login?return_to=%2Fparent%2Fstudents` status (200/302/303); no sign-in assertion was weakened and no arbitrary fixed test wait was added.
- Focused real-stack promoted command passed 1/1 in 1.8 minutes. It covers core observed replay/restart, restricted-tool/no-effects, tenant/public-negative, and responsive user-visible flows. Full command/output: `last-run.log`.
- Independent Chrome re-drive against fresh `http://127.0.0.1:38250` drove the authenticated Parent agent List command to safe allowlisted progress, cursor 11, and completed terminal (`165`–`166`).
- Slow-subscriber/axe remain explicit sanitized exploratory evidence, not mocked browser tests. Promotion requirements now hold; suite advances to `run`. Detail: `promotion-call20-pass.md`.
