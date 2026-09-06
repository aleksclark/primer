# Phase 06: Integrate student dialogue verification

## Goal

Land donor Phase 4 snapshot (`89583ced`) so the student web client can satisfy a
reading requirement through a streamed, requirement-scoped three-question
dialogue. The authoritative server evaluation, durable transcript, retries,
parent inspection, and completion decision use the same verification engine.
The donor's partial native dialogue code may compile, but Android continuation
acceptance remains explicitly incomplete.

## BDD Success Criteria

### Scenario: Curated three-question dialogue completes once

- **Given** a student occurrence configured with the curated chapter dialogue
  source and rubric
- **When** the paired student uses the public web dialogue flow and answers the
  current three questions
- **Then** only the current question advances, the durable rubric decision is
  recorded, and the occurrence completes at most once
- **And** a fresh parent session can inspect the immutable transcript/provenance.

### Scenario: Invalid or concurrent dialogue actions fail safely

- **Given** duplicate clients, stale question IDs, unauthorized tenant/student,
  premature completion, malformed frames, or a worker restart
- **When** frames race/replay through the student WebSocket
- **Then** CAS/idempotency permits one authoritative progression and decision
- **And** invalid clients receive bounded errors without seeing another
  student's transcript.

### Scenario: Retry and override preserve evidence

- **Given** an evaluation retry/failure or authorized parent override
- **When** the workflow resumes or override is applied
- **Then** prior answers/evaluation provenance remain durable and immutable
- **And** the resulting occurrence state is explainable through parent inspect.

### Scenario: Native continuation is not overclaimed

- **Given** partial dialogue UI/client files exist under Tasks Android
- **When** this phase is reported
- **Then** web P4 may be complete after its gates
- **And** Android dialogue/media/external/release acceptance remains open in the
  Android continuation plan.

## Implementation Instructions

- Apply the non-plan donor delta `dc8cedb0..89583ced` after P3. Preserve dialogue
  migrations, repository CAS, generated REST/WS facades, worker state, curated
  source/rubric configuration, and parent inspect path.
- Enforce question count/order and completion in authoritative server/domain
  code. The browser cannot assert policy completion directly.
- Keep student tools narrowly requirement-scoped; do not expose parent tools.
- Persist safe answers, rubric outcomes, and provenance, never hidden/raw model
  reasoning. Preserve immutable evidence across retries/override.
- Build partial Android sources for compatibility but do not manufacture
  emulator acceptance or mark Android Phase 1 complete.
- Open one web-authoritative P4 PR.

## End-to-End Test Plan

- Run `make tasks-test tasks-cover tasks-agent-compat tasks-clients tasks-web`
  and `make tasks-e2e`.
- Run `cd primer-tasks && go test -race ./internal/verification/...
  ./internal/agent/... ./internal/api/... -count=10`.
- Use managed headless browser against the real stack to start the configured
  reading occurrence, answer exactly three current questions, reconnect mid-
  dialogue, reject a stale/duplicate frame, complete, and inspect as parent.
- Kill/restart the dialogue worker between answers and assert public replay plus
  one persisted completion event/decision.
- Run two-student/two-tenant transcript IDOR negatives and authorized override
  evidence checks.
- Build Tasks Android JVM/APK gates and record connected/native dialogue as not
  claimed unless actually run.
- Compare integrated tree with `89583ced` using an explicit adaptation list.

## Anti-Cheating Audit

- Check that a model response or browser message cannot directly set occurrence
  completion.
- Check transcript/evaluation/decision durability in PostgreSQL and public
  reconnect, not an in-memory scripted conversation.
- Check public WebSocket tests for real handshake/auth and no handler/internal
  bypass.
- Search DB/log/protocol/UI for raw reasoning or client-supplied rubric policy.
- Check tenant/student scoping on transcript inspect and override server-side.
- Verify docs distinguish web acceptance from unfinished Android continuation.

## Completion Gate

- [ ] Curated dialogue, retry, restart, reconnect, concurrency, IDOR, and override
      BDD scenarios pass.
- [ ] Managed-headless exploration and promoted P4 browser suite pass.
- [ ] Race x10 and exactly-once persisted outcome tests pass.
- [ ] Generated clients, Go/build/vet, web, Tasks 85% coverage, and diff gates pass.
- [ ] Raw reasoning/client policy is absent from production boundaries.
- [ ] Android continuation remains truthful and unclaimed.
- [ ] P4 endpoint/adaptation audit passes and the green reviewed PR merges.
