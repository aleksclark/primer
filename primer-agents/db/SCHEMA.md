# primer-agents Database Schema

## Databases

| Name | Purpose |
|------|---------|
| `primer_agents` | Production |
| `primer_agents_test` | Integration tests |

All tables live in the `agents` schema. No cross-database foreign keys exist; caller resource references are stored as opaque text.

## Goose version table

`agents_goose_db_version` — never collides with LMS (`goose_db_version`), TV, Identity, or Studio bookkeeping tables.

## Tables

### `agents.service_info` (migration 00001)
Key/value ledger for service-level metadata. `schema_version` records the active migration level.

### `agents.sessions` (migration 00002)
Multi-turn agent sessions. Revision is an optimistic lock for phase 5 state updates.

| Column | Type | Notes |
|--------|------|-------|
| `id` | uuid PK | server-generated |
| `owner_namespace` | text | validated principal+client namespace; never a bare JWT |
| `caller_context` | text ≤ 4096 | opaque caller metadata; no credential material |
| `profile` | text ≤ 128 | execution profile name |
| `status` | text | `open` \| `closed` |
| `revision` | bigint | CAS version for session updates |
| `state_ref` | text | opaque state/history reference (Phase 5) |
| `created_at`, `updated_at`, `expires_at` | timestamptz | |

### `agents.runs` (migration 00003)
Durable run records. Idempotency is enforced per `(owner_namespace, idempotency_key)`.

**State machine** (frozen; workers must not bypass):

```
queued → running → succeeded | failed | interrupted
queued | running → cancel_requested → canceled
```

Terminal states: `succeeded`, `failed`, `canceled`, `interrupted`.
`cancel_requested` is non-terminal; workers observe it and transition to `canceled`.

**Fencing**: every status transition increments `state_version`. Workers supply the version they read as a CAS predicate; a stale version is rejected.

**Sequence counter**: `next_event_seq` is incremented atomically by each event append (`UPDATE … RETURNING next_event_seq - 1`). Terminal events and the terminal status update commit in the same transaction.

**Content policy**: `input_preview` stores a bounded diagnostic summary only; raw input, prompts, response text, and credentials are never persisted.

### `agents.run_events` (migration 00004)
Immutable, sequenced event log. `(run_id, sequence)` is unique.

Events are append-only; no row is deleted or updated after commit.
Sequence is allocated by the runs `next_event_seq` counter, not by timestamp.
Payload is bounded to 65 535 octets; raw model response text is excluded.

### `agents.schedules` / `agents.schedule_firings` (migration 00005, Phase 6 reserved)
Reserved for Phase 6 scheduled jobs. `(schedule_id, due_at)` uniqueness prevents duplicate firings. No Phase 1–5 code reads or writes these tables.

## Migration policy

- Migrations are forward-only in production. Down migrations exist only for local development.
- Migration must complete before the HTTP listener opens. The `readyz` endpoint verifies DB connectivity after migration.
- Do not share migrations with LMS, TV, Identity, or Studio databases. The `agents_goose_db_version` table is the only migration ledger for this service.

## Restart semantics

- Accepted run/session/event identity and status survive process restart.
- In-flight provider work (status `running`) whose lease expires becomes `interrupted` in Phase 4; it is not automatically resumed.
- Queued work can be safely reclaimed after a worker crash (Phase 4 lease reclaim).
- Cancellation (`cancel_requested`) is a committed database state; it survives the HTTP request that initiated it.

## Retention and content sensitivity

- `input_preview`, `payload`: bounded summaries; no raw prompt/response/credential material.
- `idempotency_key`, `idempotency_hash`: opaque request fingerprints; never bearer tokens.
- Retention policy (Phase 8): terminal runs and events are eligible for archival after a configurable TTL. Deletion cascade is `ON DELETE RESTRICT` to prevent accidental audit-evidence loss.
