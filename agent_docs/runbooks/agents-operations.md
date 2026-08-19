# primer-agents Operational Runbook

## Recovery contract (frozen)

| Run state at crash | Provider started? | Recovery action |
|-------------------|------------------|-----------------|
| `queued` + lease expired | No | `Reconcile` re-queues (safe retry) |
| `running` + lease expired | No | `Reconcile` re-queues (safe retry) |
| `running` + lease expired | Yes | `Reconcile` → `interrupted` + event |
| `cancel_requested` + no lease | — | `Reconcile` → `canceled` |

**Command**: `worker.Reconcile(ctx)` runs automatically at process start. A second process joining an existing DB will reconcile without operator intervention.

## Redaction policy

Logs must never contain:
- DSN passwords (`postgres://user:PASSWORD@…`) — scrubbed by `logging.RedactingHandler`
- JWT bearer tokens — never logged; discarded after validation
- Provider API keys — never stored; never logged
- Raw prompt / tool argument text — excluded from `run_events.payload`
- Student opaque references beyond approved correlation hashes

**Verify**: `grep -rn 'password\|secret\|bearer\|api_key' primer-agents/internal/ --include='*.go' | grep 'slog\|fmt.Print\|log\.'` should return empty for production paths.

## Database operations

### Migration

```bash
# Apply migrations (auto-runs at startup; manual override for ops)
PRIMER_AGENTS_DATABASE_URL=postgres://agents:...@db/primer_agents \
  ./primer-agents  # migrates then serves

# Check current version
psql $PRIMER_AGENTS_DATABASE_URL -c "SELECT * FROM agents_goose_db_version ORDER BY id DESC LIMIT 5;"
```

### Backup/restore

```bash
pg_dump $PRIMER_AGENTS_DATABASE_URL > agents-backup-$(date +%Y%m%d).sql
psql $PRIMER_AGENTS_NEW_DATABASE_URL < agents-backup.sql
```

### Rollback

Goose down migrations exist for each migration file. Destructive down migrations are refused in production unless `AGENTS_MIGRATE_BREAK_GLASS_DOWN=true` (not yet implemented; add before Phase 8 completion).

### Retention / pruning

Event rows are append-only and bounded by `octet_length(payload) <= 65535`. Terminal run rows (succeeded/failed/canceled/interrupted) are eligible for archival after a configurable TTL (Phase 8 TODO: add retention job).

## JWKS outage

When `PRIMER_AGENTS_IDENTITY_JWKS_URL` is unreachable:
- Cached keys (30 s TTL) remain usable
- Stale keys from a rotation are accepted for 15 minutes (`jwksRotationGrace`)
- After grace expires, all requests return 401
- `/healthz` still returns 200 (liveness); `/readyz` still returns 200 if DB is reachable

**Recovery**: JWKS endpoint restores → validator auto-refreshes on next request.

## Worker drain / graceful shutdown

On SIGTERM:
1. HTTP server stops accepting new connections (ShutdownTimeout, default 10 s)
2. In-flight HTTP requests complete
3. Worker goroutine observes `ctx.Done()` after current run's execute loop exits
4. Runs in state `running` with active leases are NOT forcibly terminated; their leases will expire and be reconciled as `interrupted` by the next process start

**Planned improvement**: dedicated worker drain signal and lease heartbeat stop before shutdown.

## Lease recovery / interrupted run handling

An `interrupted` run means the process died after the provider started. The run is NOT automatically retried because provider/tool side effects are not checkpointed. The caller should:
1. Read `run.error_class = "lease_expired"` and `status = "interrupted"`
2. Decide whether to create a new run (new idempotency key) or accept the interruption
3. Review events up to the interruption point via `GET /agents/v1/runs/{id}/events`

## Cancellation

1. `POST /agents/v1/runs/{id}/cancel` commits `cancel_requested` to DB
2. The worker's cancel poller (every 30 ms) observes the state and cancels the run-owned context
3. MAF/MCP/child work stops; run transitions to `canceled` with a terminal event
4. The HTTP request that initiated cancel may close before step 3 — the committed state survives

## SSE replay

On reconnect, supply `Last-Event-ID: N` or `?afterSeq=N`. The stream replays all committed events with sequence > N in strict order, then continues with live events. No events are lost; duplicates around the replay/live boundary are identifiable by sequence.

## Feature flags / rollout

See `agent_docs/runbooks/phase7-cutover.md`.

## Explicitly blocked

- **Live billable LLM**: no default/CI target reads ambient provider credentials. Production provider config is explicit.
- **Live Stytch**: no Stytch SDK imported. Identity JWT only.
- **Studio S19 / MCP**: not mounted. See `agent_docs/runbooks/studio-handoff.md`.
- **MAF distributed control plane**: not claimed. Phase 8 uses the bounded in-process worker model.
