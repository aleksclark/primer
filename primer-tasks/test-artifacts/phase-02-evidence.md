# Primer Tasks Phase 2 — final integration evidence

Branch: `impl/tasks-p2-checklist`
Base reviewed tip: `6bf51be7755ec05fd7b17f81d94dc72f24e8947c`

## Final result

**PASS.** Phase 2 is complete. Phase 3 may be dispatched only after this final
reviewed tip is accepted; no Phase 3 work was started here.

## Occurrence immutability

- Forward migration `00004_occurrence_immutability.sql` backfills legacy rows
  and gives every occurrence a historical snapshot containing title,
  instructions, task revision/version, timezone, due offset, due semantics,
  and schedule version.
- API request materialization and the durable worker write the same complete
  snapshot. Parent, browser-student, and Android/device occurrence projections
  read title, instructions, timezone, and versioned due fields from the
  snapshot; they do not join mutable task revisions or schedules for display.
- Real PostgreSQL integration coverage edits the task revision and schedule
  after issue, proves old parent/student/device projections remain unchanged,
  and proves future materialization uses the new schedule version and due
  offset. Focused API/schedule/database tests and the full race suite pass.

## Exploratory acceptance and promotion order

- Browser exploratory PASS was recorded first in
  `.paseo-e2e/phase2-tasks-schedules/exploration.md`, call 7: public task
  publish/revision, one-off/daily/weekly schedules, DST offsets, student
  start and parent reject/retry/approve, skip/cancel, collection URL state,
  restart, model-disabled mode, and Parent-B isolation.
- Fresh Terra Android exploratory PASS is recorded in
  `test-artifacts/android-phase2-acceptance-final-2/acceptance-report.md`:
  one paired Parent-A student; own and Parent-B occurrence IDs sourced from
  public UI; `adb am start` on both `primertasks://occurrences/{id}` links;
  own detail; generic `TASK UNAVAILABLE` for foreign without title/ID leak;
  and retained pairing/LazyColumn/start/reject-retry-approve/checked,
  skip-cancel, and force-stop evidence.
- Promotion followed exploratory PASS. Playwright
  `web/e2e/phase2-tasks-schedules.spec.ts` passed against the real server and
  covers task publish, schedule materialization, collection URL state,
  student start, parent reject/retry/approve, checked state, and tenant
  isolation. The connected Android promotion test
  `OccurrenceDeepLinkConnectedTest` passed own completed detail and foreign
  generic unavailable/no-leak against the real configured API using public
  IDs and the generated Kotlin façade. The existing Photo Picker connected
  test also passes when run in its required clean/unpaired emulator class
  invocation; the deep-link class is run after public pairing with the
  documented install-preserving instrumentation command.

Historical failed runs remain concise and truthful: the initial Terra leaf is
retained at `test-artifacts/android-phase2-acceptance-final/acceptance-report.md`
(the old product fell back to the checklist for foreign links), and the
pairing/environment blocker is retained at
`test-artifacts/phase2-promotion-final/promotion-report.md`. Neither is used
as final evidence.

## Gates

- `cd primer-tasks && go test ./... -count=1` — PASS.
- `cd primer-tasks && go test -race ./... -count=1` — PASS (real PostgreSQL
  concurrency, DST, restart, lease, decision replay, and migration coverage).
- `go vet ./...` and command builds — PASS.
- `PRIMER_TASKS_COVERAGE_GATE=1 ./scripts/enforce-module-cover.sh primer-tasks 85 tasks` — PASS at 85.2%.
- Deterministic Huma OpenAPI emission twice plus generated TypeScript/Kotlin
  clients — PASS; generated outputs remain ignored.
- `make tasks-check`, `make tasks-web`, `make tasks-android`, and
  `make tasks-proof` — PASS.
- `PRIMER_TASKS_BASE_URL=http://127.0.0.1:37952
  PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1 make tasks-e2e` — PASS: Phase 1 and
  Phase 2 Playwright tests, 2 passed.
- Model-disabled mode was observed in the public browser flow and the
  promoted manual verification flow has no model dependency.

No generated client/spec/build outputs were added to the worktree. The
remaining untracked files are retained acceptance evidence only and will be
selectively cleaned before final review.
