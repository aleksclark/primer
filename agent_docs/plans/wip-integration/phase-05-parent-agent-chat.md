# Phase 05: Integrate parent agent command chat

## Goal

Land donor Phase 3 snapshot (`dc8cedb0`) on the durable checklist service. An
authorized parent can use Fantasy-backed command chat over WebSocket to inspect
and propose task/schedule changes; durable jobs stream bounded events; explicit
confirmation gates effects; reconnect/restart preserves the conversation. This
phase remains credential-free with the scripted provider and makes no live-model
claim.

## BDD Success Criteria

### Scenario: Parent command streams a durable, confirmable proposal

- **Given** an authenticated parent with an active tenant and task
- **When** the parent asks through the public WebSocket to change a schedule
- **Then** a durable agent job streams schema-valid progress and a safe preview
- **And** no domain mutation occurs until that parent confirms it.

### Scenario: Confirmation is idempotent and authority-bounded

- **Given** a pending proposal, duplicate confirmations, a stale schedule CAS,
  or a tool outside the active allowlist
- **When** effects race or replay
- **Then** at most one authorized durable effect is committed
- **And** stale, cross-tenant, cancelled, or disallowed effects fail without
  widening tool authority.

### Scenario: Reconnect and process restart preserve safe state

- **Given** a slow/disconnected client or worker restart during a run
- **When** the same parent reconnects with the durable cursor
- **Then** committed events/conversation replay in order and work resumes or
  terminates according to its durable state
- **And** raw provider reasoning is absent from wire, DB, logs, and UI.

### Scenario: Backpressure does not compromise other clients

- **Given** one subscriber stops reading while another remains healthy
- **When** bounded buffers fill
- **Then** the slow subscriber closes with a diagnosable protocol outcome
- **And** workers and healthy tenants continue without goroutine leaks.

## Implementation Instructions

- Apply the non-plan donor delta `8274d217..dc8cedb0` after P2 merges. Preserve
  migrations, job/event/effect-step fencing, WebSocket protocol generation,
  Fantasy pin/import boundaries, and the scripted pacing fixture.
- Keep parent agent tools as adapters over the same task/schedule domain path;
  they must not write tables directly or bypass parent confirmation/CAS.
- Keep provider output untrusted. Only generated protocol event variants may
  reach clients, and no chain-of-thought/raw reasoning may be persisted.
- Reconcile dependency pins against current Agents/MAF work; do not replace or
  couple to the separate `primer-agents` service unless the donor contract
  explicitly requires it.
- Retain restart-safe DB leases and bounded slow-subscriber behavior. Avoid broad
  retries that conceal terminal errors.
- Open a dedicated P3 PR; live provider qualification remains out of scope.

## End-to-End Test Plan

- Run `make tasks-test tasks-cover tasks-agent-compat tasks-clients tasks-web`
  and `make tasks-e2e` with the scripted provider and real PostgreSQL.
- Run `cd primer-tasks && go test -race ./internal/agent/... ./internal/jobs/...
  ./internal/api/... -count=10`.
- In a managed headless browser connected to the real stack, request a schedule
  change, inspect the preview, reject once, accept a fresh proposal, reload,
  reconnect by cursor, and verify the persisted task/schedule through the public
  API/UI.
- Add cross-tenant, stale CAS, cancellation-window, duplicate acknowledgement,
  worker-kill/resume, backpressure, and tool-allowlist tests through the public
  WebSocket plus persisted-state assertions.
- Run Fantasy compatibility/pin/import audits and scan emitted evidence/logs for
  raw reasoning or secrets.
- Compare the integrated Tasks tree with `dc8cedb0`, documenting only required
  master adaptations.

## Anti-Cheating Audit

- Inspect tools for direct SQL/domain bypass and handlers for fake streamed
  success or test-only scripted branches active in production.
- Confirm event/job/conversation/effect state is durable PostgreSQL state, not an
  in-memory replay list.
- Confirm tests drive the public WebSocket and reconnect cursor rather than
  invoking worker internals only.
- Inspect backpressure closure and goroutine lifecycle; sleeps and oversized
  buffers are not a fix.
- Search protocol, logs, DB columns, screenshots, and fixtures for chain-of-
  thought/raw provider output.
- Verify confirmation authorization is enforced server-side for the current
  parent/tenant/tool/effect version.

## Completion Gate

- [ ] P3 BDD scenarios pass through real WebSocket/Fantasy/domain/PostgreSQL paths.
- [ ] Scripted-provider browser exploration and promoted Playwright pass.
- [ ] Compatibility/import/pin, race x10, restart, cancellation, slow-subscriber,
      and leak gates pass.
- [ ] No raw reasoning or unsafe provider content is persisted/emitted.
- [ ] Tasks 85% coverage and all Go/client/web/diff gates pass.
- [ ] Endpoint tree/adaptation and anti-cheating reviews have no blockers.
- [ ] P3 PR checks are green and the exact reviewed head is merged.
