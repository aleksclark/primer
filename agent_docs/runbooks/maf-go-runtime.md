# MAF Go runtime operations

The Primer MAF runtime is a **preview, in-process, non-durable** execution path. It is not Ultracore, a durable queue, a workflow recovery system, or a learning-efficacy claim. Fantasy remains the workstation/TUI loop.

## Dependency and upgrade policy

Production imports only public MAF packages and pins:

```text
github.com/microsoft/agent-framework-go
v0.0.0-20260813082112-00ffc8c3648c
upstream: 00ffc8c3648c547997eae3a3f2a3b00c28daea09
```

Run `make agent-runtime-check` before and after any dependency change. An upgrade requires a deliberate `server/go.mod` change, the full agent race suite, provider/MCP cancellation tests, OpenAPI/build checks, and a reviewed PR. Do not import `agent-framework-go/internal`, fork MAF, or update the standalone `spikes/maf-go` module as a production shortcut.

## Enablement

The server keeps the runtime disabled unless all of these are configured:

- `AGENT_RUNTIME_ENABLED=true`
- `AGENT_RUNTIME_BASE_URL` — an OpenAI-compatible endpoint
- `AGENT_RUNTIME_API_KEY`
- `AGENT_RUNTIME_MODEL`
- optional `AGENT_RUNTIME_RUN_BUDGET` (default `2m`)

When enabled, `primer-server` constructs a pinned MAF OpenAI-compatible root agent and an in-process controller. The student device routes are not connected to this controller; the root policy remains `max_children=0` until a separately authorized child factory is shipped.

## HTTP behavior

Parent-authenticated routes are:

- `POST /agent/runs` — start a detached run and return its server-generated `runId`.
- `GET /agent/runs/{id}` — inspect process-local status.
- `POST /agent/runs/{id}/cancel` — explicitly cancel; repeated cancellation is safe.
- `GET /agent/runs/{id}/events` — Primer SSE event stream with root/parent/child attribution.

The runtime context is owned by the controller, not `r.Context()`. A disconnected SSE subscriber stops delivery only; it does not cancel the run. An explicit cancel propagates through MAF, child tools, and MCP contexts. The controller bounds run duration, event buffering, and retained status records.

Run IDs, status, and event history are lost on process restart. Do not advertise restart recovery or durable replay. A graceful server shutdown cancels active runs and waits within the server shutdown deadline.

## Multi-turn lineage policy

A `Runner` has a sticky process-local root: the first turn establishes `root_run_id`, and subsequent turns on that same runner receive fresh `run_id` values while retaining that root. The controller creates a new runner for each independent HTTP run, so each controller start is a new root. Session continuity across requests or restarts is not implemented; callers must not infer it from agent IDs.

A child reservation is released after invocation and can be explicitly reclaimed with `StreamingChildTool.Release()` when a prepared child is discarded before `Call`. `Release` is idempotent and prevents unused `activeChildren` slots from leaking.

## Rollback and diagnosis

To roll back the preview runtime, set `AGENT_RUNTIME_ENABLED=false` and restart `primer-server`. The existing tutor route and database schema are unaffected; no migration or cleanup is required. If a run fails, inspect the parent-visible `state`, `errorClass`, and safe `error` fields. Provider details, prompts, tool arguments, and credentials are not returned in the status response.

Useful checks:

```bash
make agent-runtime-check
cd server && go test -race ./internal/agent/... ./internal/api -count=1
make build
make openapi
make client
```

The complete repository `make test` gate must remain unchanged. If local studentclient file-mode tests fail because the host reports `0600` where those existing tests expect `0640`, record that environment failure separately; do not weaken the gate or claim the runtime caused it.
