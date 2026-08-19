# Phase 7 Cutover Runbook: LMS → primer-agents

## Status

**Flags default: OFF.** All legacy Fantasy/LMS tutor/local-controller paths remain
active. Remote integration is inert until explicitly enabled per cohort below.

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `PRIMER_AGENTS_ENABLED` | `false` | Enables the remote service adapter in the LMS server. All legacy paths unchanged when `false`. |
| `PRIMER_AGENTS_BASE_URL` | _(empty)_ | HTTPS URL of the primer-agents service. Required when enabled. |
| `PRIMER_AGENTS_TIMEOUT` | `30s` | HTTP client timeout for agents requests. |
| `PRIMER_AGENTS_TOKEN_ENV_VAR` | _(empty)_ | Name of the env var holding the short-lived Identity JWT (`aud=primer-agents`). Never a static secret. |
| `AGENT_RUNTIME_ENABLED` | `false` | **Unchanged.** Controls the existing process-local MAF preview only. Independent of the remote flag. |

## Prerequisites

Before enabling `PRIMER_AGENTS_ENABLED=true`:

1. `primer-agents` service is deployed, migrated, and readyz-green.
2. Identity has issued a service credential for the LMS with `aud=primer-agents`
   and approved scopes. See [identity prerequisites](#identity-prerequisites).
3. Separate PostgreSQL databases confirmed (`primer_agents` ≠ `primer`).
4. `PRIMER_AGENTS_BASE_URL` is an HTTPS URL in production.
5. The LMS has access to a short-lived bearer token rotated externally;
   **never** use a static shared secret.

## Identity prerequisites

| Caller | Scope needed | Status |
|--------|-------------|--------|
| LMS admin BFF | `agents:runs:write agents:runs:read agents:runs:cancel agents:sessions:write agents:sessions:read` | **Blocked**: reviewed Identity client grant not yet issued |
| Workstation student | `agents:student:session` | **Blocked**: reviewed student credential not yet issued — keep Fantasy default |

Until Identity issuance is reviewed and granted, remote mode remains off.
The existing Fantasy workstation path and LMS tutor remain the active paths.

## Rollout cohorts

| Cohort | Gate | When |
|--------|------|------|
| 0 – Off | `PRIMER_AGENTS_ENABLED=false` | All deployments today |
| 1 – Internal test | Enable on test stack with test Identity credentials | After Identity issuance |
| 2 – Admin opt-in | Single parent/admin per environment | After cohort 1 E2E pass |
| 3 – General admin | Full admin surface | After cohort 2 soak |
| 4 – Student | Workstation student flag | Separate acceptance gate |

## Fallback/rollback

**Before acceptance** (before `primer-agents` returns a run ID): set
`PRIMER_AGENTS_ENABLED=false` and redeploy. Legacy path resumes immediately.

**After acceptance** (after a remote run ID is issued to a caller): the adapter
holds the run ID in memory and reconnects retries to that run. If the service is
unavailable after acceptance, the LMS reports the truthful error rather than
silently starting a duplicate legacy run.

To explicitly roll back after remote runs exist: disable the flag AND document
the in-flight run IDs; they remain in the agents DB and can be queried directly.

## Browser UI

Admin parent/admin chat UI using the remote service is **deferred** per
phase-07 scope. When implemented it must follow the `frontend-house-design-system`
skill and use the generated TypeScript client (`client/ts/client.ts`) — no
hand-written DTOs, raw `fetch`, or copied SSE event parsers outside the
generated-client package.

## Import gates

- LMS Go code: import only `github.com/aleksclark/primer/agents/client/go`
  (generated). No handwritten transport.
- LMS TypeScript: import only `@primer-agents/client` types (generated).
  No raw fetch to `/agents/v1/`.
- CI checks: `go vet ./internal/remoteagent/...` and
  `cd primer-agents && go test ./...` pass together.

## Authorization boundary

LMS product authorization **must** occur before any call to the adapter:

```go
// CORRECT
if !lms.IsAuthorized(user, resource) {
    return ErrForbidden
}
run, err := remoteAdapter.StartRun(ctx, key, profile, preview, true)

// WRONG — never bypass
run, err := remoteAdapter.StartRun(ctx, key, profile, preview, false) // denied locally
```

The adapter enforces `locallyAuthorized=true`; `false` returns
`ErrLocalAuthorizationRequired` without making any remote call.
