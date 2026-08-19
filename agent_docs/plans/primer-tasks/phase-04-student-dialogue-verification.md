# Phase 4: Student dialogue verification

## Goal

Add the first student-facing agentic verifier. A parent can configure a reading
task with an `agent_dialogue` requirement such as “answer three questions about
chapter 4.” The student opens the occurrence in web or Android, participates in
a streamed requirement-scoped conversation, and receives completion only after
the verification engine has three durable accepted answer evaluations.

The dialogue agent may assess answers and coach briefly, but it cannot manage
other tasks, reveal answer keys, or mark an occurrence complete directly.

## BDD Success Criteria

#### Scenario: Three accepted answers complete the reading task

- **Given** a published reading task whose dialogue requirement snapshots the
  source/context, question count 3, rubric, and retry policy
- **When** the bound student starts verification and gives three substantively
  accepted answers to distinct questions
- **Then** questions, answers, and criterion evaluations are durable and
  attributed to one attempt
- **And** the requirement receives one accepted decision and the occurrence is
  checked off in student and parent views.

#### Scenario: Incorrect answer receives bounded follow-up

- **Given** an active dialogue attempt
- **When** the student gives an incorrect, irrelevant, or insufficient answer
- **Then** the agent records a rejected evaluation, gives a concise retry/follow-
  up prompt within policy, and does not increment accepted-question count
- **And** the task remains incomplete until policy is satisfied or attempts end.

#### Scenario: Reconnect resumes the same attempt

- **Given** two accepted questions and a disconnected browser/app
- **When** the same student reconnects and opens the occurrence
- **Then** the server restores the durable conversation and accepted count and
  asks only the remaining question
- **And** neither replayed messages nor provider retries double-count an answer.

#### Scenario: Student agent tools are strictly scoped

- **Given** a student prompt that requests task edits, schedules, another
  student's data, answer keys, or completion
- **When** the agent processes it
- **Then** only dialogue tools for the authenticated occurrence/attempt are
  available and forbidden tools are not invoked
- **And** the response redirects to the current verification without leaking
  protected context.

#### Scenario: Parent authored source remains authoritative

- **Given** task revision context and a student message containing prompt
  injection or contradictory “source” text
- **When** a question/evaluation step runs
- **Then** server-loaded revision context and rubric remain authoritative
- **And** student content is never promoted into system/tool policy.

#### Scenario: Concurrent devices cannot race completion

- **Given** the same student is paired on web and Android
- **When** both submit messages concurrently to one attempt
- **Then** message sequence/idempotency rules serialize or clearly conflict the
  turn, accepted count never exceeds policy, and exactly one terminal decision
  is committed.

#### Scenario: Parent can inspect and override transparently

- **Given** a completed or failed dialogue attempt
- **When** the parent opens the occurrence inspector
- **Then** the parent sees questions, student answers, accepted/rejected outcomes,
  safe rationale, model/policy versions, and usage without hidden reasoning
- **And** an explicit parent override creates a separate audited decision rather
  than rewriting model evidence.

#### Scenario: Provider outage does not invent success

- **Given** the model times out or returns malformed/no evaluation
- **When** the student sends an answer
- **Then** the run fails/retries within finite bounds, the answer remains durable,
  and the UI offers a safe retry/escalation state
- **And** no requirement or occurrence is accepted from fallback prose.

## Implementation Instructions

1. Register `agent_dialogue` as a versioned verification manifest. Configuration
   includes source/context reference or parent-authored bounded text, learning
   focus, required distinct accepted questions, rubric/acceptance criteria,
   allowed follow-ups, max attempts/turns, and retention policy. Validate at task
   publish and snapshot into occurrences.
2. Add dialogue-specific durable projections only where the generic attempt,
   submission/message, evaluation, and decision tables cannot express ordered
   turns and question identity. Preserve one generic engine path and stable
   requirement/attempt IDs.
3. Build a requirement-scoped Fantasy agent with only tools such as
   `get_dialogue_state`, `record_question`, and `record_answer_evaluation`.
   - Tool context fixes tenant, student, occurrence, requirement, attempt, and
     policy version.
   - `record_question` enforces distinct IDs/count and strips answer-key leakage.
   - `record_answer_evaluation` validates rubric fields, binds exactly one student
     message/question, and is idempotent.
   - The verification engine, not the tool/model, counts accepted evaluations
     and commits the final decision.
4. Use a custom `PrepareStep`/stop policy to restrict active tools by turn, cap
   steps/tokens/time, and prevent further tool calls after terminal state.
   Preserve the phase-3 no-raw-reasoning progress mapping.
5. Build model messages from server-owned revision context plus bounded durable
   dialogue history. Clearly delimit untrusted student text. Do not let clients
   provide system prompts, rubrics, question count, completion status, or source
   substitutions.
6. Add student WebSocket authorization using Android bearer headers or student
   browser cookie; never query-string tokens. The same protocol gains
   attempt-subscribe, student-message, and verification state events. Enforce
   student binding at upgrade, subscribe, replay, and every command.
7. Make user messages durable before enqueueing the agent run. Use client message
   idempotency keys and expected sequence/version. Concurrent turns either
   serialize deterministically or return a typed conflict with resumable state.
8. Add parent task-form support for dialogue configuration with schema-driven
   validation and preview. Do not expose hidden prompts; show parent-owned source,
   rubric, required question count, and retry policy.
9. Add student web and Android System C **Decide/Learn** transcript screens:
   current question, ruled history, typing state, generic thinking/evaluating
   progress, retry/error/offline explanation, and completion summary. No bubbles,
   raw score gamification, or raw model reasoning.
10. Add parent **Inspect** timeline with student-authored text clearly separated
    from agent/evaluation records, safe rationale, provenance, usage, overrides,
    and retention controls. Parent override uses an explicit domain operation and
    cannot alter immutable prior evidence.
11. Extend generated REST/WS clients and Android socket façade. Backgrounding the
    app disconnects delivery but not the run; reopening fetches/replays durable
    state.
12. Include a curated deterministic chapter fixture and scripted model that
    requires three distinct correct concepts, rejects an insufficient answer,
    and emits tool calls. This proves wiring/policy, not educational model quality.

## End-to-End Test Plan

### Browser exploratory acceptance

- Parent creates a chapter-reading task requiring three questions and schedules
  it. Student browser answers two correctly, one incorrectly, follows up, and
  completes. Parent inspects the complete transcript/evidence.
- Disconnect after two accepted answers, reconnect, and verify only one remains.
- Attempt prompt injection, answer-key request, cross-task/cross-tenant subscribe,
  message replay, concurrent tabs, malformed provider output, timeout, and parent
  override.
- Inspect wire/DB/logs for no reasoning deltas and no client-supplied policy.

### Android emulator acceptance

- On the paired emulator, open the reading task, stream questions/progress,
  background/kill after two answers, reopen and finish, then observe checked state.
- Run concurrent web/app submission and verify typed conflict/recovery without
  duplicate evaluation.
- Revoke device mid-attempt and verify subsequent subscribe/message denial while
  durable parent audit remains.

### Promoted automation

- After exploratory PASS, add Playwright specs for three-question success,
  incorrect/follow-up, reconnect, prompt injection, foreign subscription,
  concurrent turns, provider failure, parent inspect, and override.
- Add emulator-backed dialogue/restart/revocation tests using the real WebSocket
  and scripted Fantasy provider.
- Add real-Postgres process tests for message/evaluation idempotency, two-device
  races, exactly one decision/completion, and agent tool allowlist negatives.
- Run repeated stream/cancel/reconnect tests under race detector.

Commands:

```bash
make tasks-test tasks-cover tasks-agent-compat tasks-clients tasks-web tasks-android
make tasks-e2e
cd primer-tasks/android && ./gradlew connectedDebugAndroidTest
cd primer-tasks && go test -race ./internal/verification/... ./internal/agent/... ./internal/api/... -count=10
```

## Anti-Cheating Audit

- Trace completion from three bound accepted evaluation rows through the generic
  policy service; reject model prose, a `complete_task` tool, hard-coded success,
  or a client state toggle.
- Inspect agent active tools and context construction; reject parent tools,
  repository handles, client-supplied source/rubric/system text, or cross-
  occurrence lookups.
- Verify question distinctness/count and evaluation idempotency are server rules,
  not prompt instructions only.
- Verify WebSocket authorization on replay/subscribe/command, not just upgrade.
- Inspect concurrent-turn tests for real DB contention and exactly-one effects.
- Confirm parent override appends auditable evidence and does not mutate/delete
  model/student records.
- Search wire/DB/log/UI for raw reasoning and answer-key leakage.
- E2E must run the real Fantasy tool loop with a scripted model; tests that inject
  accepted DB decisions or call private evaluation methods do not satisfy the
  scenario.

## Completion Gate

- [ ] All dialogue BDD scenarios pass with the curated three-question fixture.
- [ ] Dedicated browser and Android exploratory agents PASS before promotion.
- [ ] Playwright and emulator dialogue suites are green.
- [ ] Two-device/idempotency/exactly-once process and race tests pass.
- [ ] Parent inspect/override evidence is immutable and tenant-scoped.
- [ ] No raw reasoning or client policy reaches wire, DB, logs, or UI.
- [ ] Generated REST/WS clients, Go coverage/build/vet, web gates, Android gates, and diff checks pass.
- [ ] Anti-cheating audit finds no model-direct completion, broad student tools, fake conversation, or tenancy leak.
