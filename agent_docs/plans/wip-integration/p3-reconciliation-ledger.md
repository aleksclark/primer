# Original P3 recovery file ledger

Current base: `34a4f5c2aaddd4c37b95c3bbca5bb2dcdeef7be8`. Read-only recovered dirty donor: `primer-wave00-p3-p13` at `e173e6cc777ffc0829f75217041f2ce651999f29`. Original P3 endpoint: `dc8cedb0` (delta `8274d217..dc8cedb0`). This ledger records source adaptation, not completion of phase05 or phase13.

| Donor path (under primer-tasks/) | Disposition on current source |
|---|---|
| `Dockerfile.dev` | Current released file retained; stale donor replacement rejected |
| `Makefile` | Recovered dirty donor bytes |
| `clients/typescript/package.json` | Recovered dirty donor bytes |
| `clients/typescript/scripts/check-agent-boundary.mjs` | Adapted to current released source |
| `clients/typescript/src/agent-client.ts` | Adapted to current released source |
| `clients/typescript/src/agent-protocol.ts` | Adapted to current released source |
| `clients/typescript/src/index.ts` | Adapted to current released source |
| `cmd/agent-protocol-gen/main.go` | Adapted to current released source |
| `cmd/tasks-server/main.go` | Adapted to current released source |
| `compose.yaml` | Current released file retained; stale donor replacement rejected |
| `go.mod` | Adapted to current released source |
| `go.sum` | Adapted to current released source |
| `internal/agent/README.md` | Adapted to current released source |
| `internal/agent/compat_test.go` | Recovered dirty donor bytes |
| `internal/agent/events.go` | Recovered dirty donor bytes |
| `internal/agent/events_more_test.go` | Recovered dirty donor bytes |
| `internal/agent/events_test.go` | Recovered dirty donor bytes |
| `internal/agent/fantasy_qualification_test.go` | Recovered dirty donor bytes |
| `internal/agent/model.go` | Recovered dirty donor bytes |
| `internal/agent/model_test.go` | Recovered dirty donor bytes |
| `internal/agent/protocol/protocol.go` | Recovered dirty donor bytes |
| `internal/agent/protocol/protocol_more_test.go` | Recovered dirty donor bytes |
| `internal/agent/protocol/protocol_test.go` | Recovered dirty donor bytes |
| `internal/agent/repository.go` | Recovered dirty donor bytes |
| `internal/agent/repository_integration_test.go` | Recovered dirty donor bytes |
| `internal/agent/runtime.go` | Adapted to current released source |
| `internal/agent/runtime_more_test.go` | Recovered dirty donor bytes |
| `internal/agent/schema.sql` | Recovered dirty donor bytes |
| `internal/api/agent_command_branches_test.go` | Recovered dirty donor bytes |
| `internal/api/agent_execution_more_integration_test.go` | Recovered dirty donor bytes |
| `internal/api/agent_full_run_integration_test.go` | Adapted to current released source |
| `internal/api/agent_ws.go` | Adapted to current released source |
| `internal/api/agent_ws_integration_test.go` | Adapted to current released source |
| `internal/api/agent_ws_more_integration_test.go` | Recovered dirty donor bytes |
| `internal/api/agent_ws_more_test.go` | Recovered dirty donor bytes |
| `internal/api/agent_ws_test.go` | Recovered dirty donor bytes |
| `internal/api/openapi.go` | Adapted to current released source |
| `internal/api/openapi_contract_test.go` | Adapted to current released source |
| `internal/api/phase2.go` | Adapted to current released source |
| `internal/api/phase3_runtime_integration_test.go` | Recovered dirty donor bytes |
| `internal/api/phase3_services.go` | Adapted to current released source |
| `internal/api/phase3_services_more_integration_test.go` | Recovered dirty donor bytes |
| `internal/api/phase3_services_more_test.go` | Recovered dirty donor bytes |
| `internal/api/server.go` | Adapted to current released source |
| `internal/config/config.go` | Adapted to current released source |
| `internal/config/config_test.go` | Adapted to current released source |
| `internal/db/db_test.go` | Adapted to current released source |
| `internal/db/migrations/00005_parent_agent.sql` | Recovered dirty donor bytes (incoming renumber only) |
| `internal/db/migrations/00006_agent_tool_effects.sql` | Recovered dirty donor bytes (incoming renumber only) |
| `internal/db/migrations/00007_agent_tool_effect_steps.sql` | Recovered dirty donor bytes (incoming renumber only) |
| `internal/domain/parent/README.md` | Recovered dirty donor bytes |
| `internal/domain/parent/confirmation.go` | Adapted to current released source |
| `internal/domain/parent/confirmation_more_test.go` | Recovered dirty donor bytes |
| `internal/domain/parent/confirmation_sql.go` | Recovered dirty donor bytes |
| `internal/domain/parent/confirmation_sql_test.go` | Recovered dirty donor bytes |
| `internal/domain/parent/context.go` | Recovered dirty donor bytes |
| `internal/domain/parent/parent_test.go` | Recovered dirty donor bytes |
| `internal/domain/parent/phase3_more_test.go` | Recovered dirty donor bytes |
| `internal/domain/parent/provider.go` | Recovered dirty donor bytes |
| `internal/domain/parent/provider_more_test.go` | Recovered dirty donor bytes |
| `internal/domain/parent/services.go` | Adapted to current released source |
| `internal/domain/parent/tools.go` | Recovered dirty donor bytes |
| `internal/domain/parent/tools_more_test.go` | Recovered dirty donor bytes |
| `internal/jobs/worker.go` | Recovered dirty donor bytes |
| `internal/jobs/worker_test.go` | Recovered dirty donor bytes |
| `scripts/host-stack` | Recovered dirty donor bytes |
| `web/e2e/phase3-parent-agent.spec.ts` | Adapted to current released source |
| `web/scripts/check-client-boundary.mjs` | Recovered dirty donor bytes |
| `web/src/AgentCommandPage.tsx` | Adapted to current released source |
| `web/src/App.tsx` | Adapted to current released source |
| `web/src/index.css` | Adapted to current released source |
| `internal/api/agent_authority.go` | Adapted to current released source |
| `internal/api/agent_authority_ws_test.go` | Adapted to current released source |
| `internal/api/agent_backpressure_ws_test.go` | Adapted to current released source |
| `internal/api/agent_completion.go` | Adapted to current released source |
| `internal/api/agent_mutations_integration_test.go` | Adapted to current released source |
| `internal/api/agent_rejection_integration_test.go` | Adapted to current released source |
| `internal/api/agent_scripted_preview.go` | Recovered dirty donor bytes |
| `internal/api/agent_socket.go` | Adapted to current released source |
| `internal/api/agent_test_helpers_test.go` | Recovered dirty donor bytes |
| `internal/api/agent_tool_inputs.go` | Recovered dirty donor bytes |
| `internal/db/database.go` | Recovered dirty donor bytes |
| `internal/db/migrations/00007b_parent_agent_authority.sql` | Adapted to current released source; incoming `00009_parent_agent_authority.sql` |
| `internal/domain/parent/memory_confirmation_test.go` | Recovered dirty donor bytes |

## Preservation and required adaptations

- Applied `00005_clerk_parents.sql` SHA256 `c1b3b82cc4b66a85215dd5781a29871e61293702b31201562703bb3be751d720` is unchanged and guarded by a test. Incoming parent-agent/effect/effect-step/authority migrations are 00006/00007/00008/00009. Real-PG tests cover fresh installation and upgrade from the exact released five migrations without changing applied history or local household/parent/student/revision rows.
- `TaskForms.tsx`, schedule presets, public mount and opaque paired-student boundary remain. Current `phase2.go` was not replaced: carry only transactional publication and schedule affected-row validation, then use its existing create/revise/publish/schedule/materialize handlers inside agent effect transactions.
- Recovered cursor-page socket tail, atomic message/run/job/ack admission, run/actor/tool/CAS/confirmation fencing, worker leases, and authority/backpressure tests are retained. New auth tests use public WebSockets and cryptographically signed credentials through the published Authstack verifier.
- Clerk upgrades take fresh bearer input in a subprotocol, never query strings; only the fixed application protocol is negotiated. Authstack, not Tasks JWT parsing, checks expiry repeatedly at input, output, and effect boundaries. Normalized issuer/subject/session provenance is durable; the verifier closure is ephemeral. Credential loss after process restart terminates queued work in both Clerk and supported development BFF lanes with a durable reauthorization-required result; a pending preview remains inert until a fresh same-session parent explicitly confirms. Current local membership/admin/identity/session revocations remain authoritative.
- Added original-P3 `update_task` through current append-revision validation/CAS, preserving published and issued work. Collection tools expose bounded offsets instead of hiding rows after their first page.
- Replaced handwritten donor TypeScript wire DTOs with offline generation from actual Go socket boundary structs and runtime-used variant requirements. The client facade retains validation/redaction/reconnect; outputs are ignored build artifacts. Full final text is durably replayable as an authoritative text-end replacement, not duplicated token deltas.
- Current Go 1.26.6 and Authstack dependency/replacement remain; only Tasks Fantasy v0.41.1/WebSocket dependencies and their required module graph are added. No other module source/pins are changed. Root updates are limited to Tasks compatibility/CI gates and `go.work.sum` checksums generated by ordinary workspace-mode Tasks test/codegen (new Fantasy transitive graph; removed duplicate checksums are retained in module sums; no `go work sync` or other-module manifest changes); no fleet/release manifest/image changes.
- Donor Dockerfile.dev/Compose version replacements are stale: preserve current Go image and current opt-in infrastructure. Host-stack restart support is recovered for credential-free process proof; host Make remains default.

## Tail-delivery revocation follow-up

Independent review blocked `aa7baac186b129190d3885d80985b7274ff5536d`: a plain authorization read could precede a revocation commit while the subsequent private WebSocket write still delivered data. The follow-up holds shared membership, identity/BFF-session and conversation locks through **one** bounded frame write (including queued private/control frames), not a 32-event replay page. A per-frame READ COMMITTED transaction acquires all locks first, then executes fresh authorization statements and Authstack verification after any waits. In particular, Clerk session-revocation `NOT EXISTS` is a separate post-lock statement, never part of the potentially stale identity-locking SELECT. Provider verification does not reenter the DB pool while holding these locks.

Real PostgreSQL/public-WebSocket tests use a gate below net/http on the actual TCP frame write and observe PostgreSQL's blocking graph. They prove revocation-first denial by an explicit policy close, delivery-first blocking of revocation until the real frame write returns, Clerk revocation insertion after the locking statement has begun, changed conversation actor/archive, and queued private control frames. Both BFF and Clerk delivery-first session tests call the real HTTP logout routes. A deliberately stalled real write is released only by the production deadline, which also unblocks durable Clerk logout. No time-based absence assertion, fake streamed effect, or alternate persistence path is used.

The pre-fix aa7 race session2897801 was stopped before edits with no completed exit receipt and is explicitly **interrupted/superseded, not PASS**, recorded at `/tmp/primer-p3-gates/aa7-race10-superseded.txt`; its log remains preserved. Focused compile, new concurrency suite and existing auth/replay/cancel/backpressure regressions passed before the focused follow-up freeze. Full Tasks/85%/client/web and one complete race x10 are required anew at that follow-up SHA. Applied005, released usability files, domain mutations and external modules are untouched by this fix.

## Browser-defect remediation

Independent Chrome CALL1 on aa7 found three functional failures, preserved read-only in the browser worktree's `.paseo-e2e/tasks-p3-reconciled/` exploration/JSON/screenshots: missing named completion text for confirmed changes, a malformed duplicate-Close exchange yielding1006, and overlapping error labels/still-actionable stale previews. e569's tail privacy fix was independently approved but did not address these surfaces. Its unfinished race session3225283 was explicitly stopped before browser-remediation edits and is superseded/NOT PASS; e569 full-gate85.5% evidence remains historical only.

- Confirmation now commits canonical domain-result receipt clauses, a system-authored final message and `source=domain` text_start/text_delta/text_end with the effects. Nothing streams before commit; semantic operation clauses are not simulated token pacing or a fabricated provider reply. Public-WS tests prove current human names, no unconfirmed effects, complete replay without duplication, duplicate-handle idempotency and no success receipt after rollback/cancel.
- The actual close cause was nhooyr1.8.17's unconditional echo of a received Close even after it sent Close. Its public Close CAS already makes repeated application Close calls no-ops, so the handler's old deferred1000 alone was not the cause. A real public RFC6455 probe reproduced a second opcode8 after one masked peer reply (`browser-fixes/close-rfc-before.exit1`). Node's native client was permissive, so it is not the sole oracle. Under the explicit L0 amendment, only Tasks' direct pin changes to coder/websocket1.8.14 and its eight existing import sites; standalone14 and the already-selected root-workspace15 both pass exactly-one-Close-then-EOF proof. No other module pin changed; root go.work.sum adds only the needed existing15 content checksum. Cleanup now uses CloseNow, with the maintained transport owning the handshake. Buffers/deadlines and e569 private-frame revocation guards remain.
- Version-stale domain CAS is distinct from expiry/auth failure. An authorized stale acknowledgement commits handle invalidation plus actionable confirmation_stale/failed terminal with no effect; credential-refreshable previews remain pending. UI labels use safe parent copy and wrap inside the original ruled layout. Obsolete confirmation controls disappear on the durable terminal; late old-handle errors cannot retarget a new proposal. Fifteen Node unit tests, including the four new chat-state/replay tests, run in the existing web build; no second browser/Playwright stack is introduced.

Focused receipts live under `/tmp/primer-p3-gates/browser-fixes/`. The initial raw-probe nonce-length fixture error in close-before.log is preserved separately and is not used as bug evidence. Current standalone14 close/authority regressions, normal-workspace receipt/CAS/raw-close regressions, clients/web, lint/vet and UI-state tests passed before the coherent follow-up freeze. All final full/85%/compat/client/web/race10 receipts must bind the follow-up SHA; independent Chrome re-exploration/promotion and security review remain L1-owned and mandatory. No new live browser stack, deployment, production data or provider-quality claim is made here.

## Evidence status

Frozen-candidate qualification is in progress, not accepted. `full3` passed Tasks tests, 85.5% coverage, compatibility, client generation, and web build before the final uniform BFF credential-loss tightening; it is not final-tip proof. The post-tightening fresh-schema/Clerk/BFF/revision focused suite passed. Fresh workspace-mode full gates and one race x10 run are in flight on the frozen source. The earlier superseded race binary was stopped and explicitly recorded as NOT PASS. Logs and actual command exit files are under `/tmp/primer-p3-gates/`; temporary handoff is `/tmp/primer-p3-reconciled-handoff.md`. No historical coverage/race/browser receipt is treated as current-tip acceptance. Independent browser exploration/promotion, exact-tip review/CI and merge belong to L1. Live model/provider/deployment testing is not claimed. Phase13 remains unfinished.
