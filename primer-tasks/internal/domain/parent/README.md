# Parent tools/auth/confirmation slice

This package is the Phase 3 parent command/inspect seam. `Tools` depends only on
phase-2 domain-service interfaces; it does not accept a repository or database
handle. The eventual server adapter constructs `parent.Context` from the
validated parent session (`TenantID`, `ActorID`) and a server-issued
idempotency key. Model input has no authority fields.

`NewToolSet` is intentionally fail-closed: an empty or unknown active tool
allowlist is invalid. Student-name resolution requires one exact tenant-scoped
match; zero or multiple matches returns `ErrClarification` before a mutation.

Destructive actions use `ConfirmationStore`. The in-memory implementation is
for deterministic tests only. Production must use `SQLConfirmationStore` after
the migration owner installs the table described by `ConfirmationTableSQL`.
The durable row binds the opaque handle to tenant, actor, normalized action
digest, expiry, and a consumed timestamp. Consume locks the row and performs a
single-use compare-and-set. Confirmation dispatches the action stored in the
row, never the caller's altered action. A phase-2 service remains responsible
for its own tenant predicate, optimistic version check, transaction, and
idempotency behavior.

Provider configuration is explicit and server-side:

- `disabled` is a real unavailable state; it is not a fake agent response, and
  callers continue using the ordinary phase-2 manual controls.
- `scripted` is test/development-only and is rejected in production.
- live mode is configured for Bedrock primary with optional OpenRouter fallback
  (`TASKS_AGENT_PRIMARY`, `TASKS_AGENT_FALLBACK`). Credentials are not copied
  into tool context or operational records.

This package does not register Huma routes, create migrations, construct a
Fantasy agent, or wire runtime workers. Those boundaries belong to the Phase 3
orchestrator.
