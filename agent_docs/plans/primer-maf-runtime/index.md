# Primer MAF Go Runtime — Wave 1 Production Plan

## Outcome

Deliver a production-shaped, in-process MAF Go runtime under `server/internal/agent` and a Primer-owned HTTP streaming/cancellation boundary. A parent run can invoke authorized child agents with live attributed events, bounded execution, and cancellation that reaches nested MCP work without making subscriber disconnect cancel the run. The runtime is explicitly non-durable and remains compatible with the existing Fantasy/TUI path.

This plan translates the feasibility spike merged in PR #27 (`a7f3229`) into Primer code. The spike's reviewed artifact is evidence, not production code. Its exact MAF pin is `github.com/microsoft/agent-framework-go@v0.0.0-20260813082112-00ffc8c3648c` (upstream commit `00ffc8c3648c547997eae3a3f2a3b00c28daea09`). MAF is public preview with no release tags; this plan treats its APIs as preview-unstable and requires the pin to remain exact.

## Current-state summary

- `spikes/maf-go/` is a standalone module and contains the only MAF implementation. It proves public-API child streaming through a custom `tool.FuncTool`, fail-closed MCP filtering, depth/direct/total child budgets, attributed Primer SSE, a bounded asynchronous bridge, and separate stream/run contexts.
- The stock MAF `agenttool` collects child output and cannot provide nested live updates. Production must retain the spike's custom streaming child tool; it must not substitute stock `agenttool` for the required UX.
- The spike's `RunHandle` is intentionally minimal. It does not provide a production run registry/controller, authorization-aware cancel surface, safe concurrent preparation, sticky multi-turn root policy, or detached worker lifecycle.
- `server/` has an existing `internal/tutor` service and Huma/chi API construction in `server/internal/api` and `server/cmd/primer-server`; it has no `server/internal/agent` package and no MAF dependency in `server/go.mod`.
- The root `go.work` lists `curriculum-studio`, `primer-identity`, and `server`; adding MAF to `server` must not promote or rewrite the standalone spike module.

Evidence: `spikes/maf-go/RUN_REPORT.md`, spike code at `spikes/maf-go/primer/`, server wiring at `server/internal/api/api.go` and `server/cmd/primer-server/main.go`, and `server/go.mod`.

## Scope boundaries

### In scope

- A real `server/internal/agent` package importing only public MAF packages.
- Exact MAF pseudo-version pin and a small Primer-owned runtime contract.
- Child authority intersection, default untrusted `max_children=0`, depth and total budgets, and correct reservation release.
- A production run controller with explicit cancel, run status/error ownership, concurrent-safe run preparation, and a policy for multi-turn `root_run_id`.
- Primer SSE or an extended AG-UI-compatible envelope carrying root/parent/child attribution, incremental events, bounded async delivery, and stream/run context separation.
- HTTP/server integration with authentication and authorization appropriate to the existing server boundary, plus integration tests through a public HTTP handler.
- Operational logging/metrics hooks sufficient to diagnose dropped subscribers, cancelled runs, budget denials, and MAF version drift.

### Out of scope for wave 1

- Ultracore or any durable distributed control plane, queue, checkpoint store, or restart-resumable run.
- Live billable LLM acceptance tests or a multi-turn OpenAI tool-autocall matrix beyond cheap deterministic coverage.
- Replacing Fantasy or rewriting Charm/Bubble Tea workstation loops.
- Forking, vendoring, patching, or importing `agent-framework-go/internal`.
- Student-facing product claims of durable tutoring or autonomous authority; the student policy remains `max_children=0`.

## Global constraints

1. Production code lives under `server/internal/agent` unless a phase records a justified module-boundary exception. The spike remains read-only reference/evidence.
2. Import only MAF public packages and pin the exact upstream commit above. Any API adaptation belongs in Primer code.
3. A child receives the intersection of parent grants, child policy, and request scope. Empty/unknown allowlists fail closed; no caller-supplied depth can broaden authority.
4. Stream cancellation controls only SSE delivery. Runtime cancellation is explicit and owned by the run controller; it propagates through parent, child, tool, and MCP contexts.
5. Bounds are finite and observable. No unbounded goroutine, event queue, child count, or request-body-driven retention.
6. Existing `make test`, `make cover`, and `make build` gates must not be lowered. Generated OpenAPI/client output is refreshed only if the public API changes.
7. This is an in-process, non-durable runtime. Documentation and tests must not call workflow checkpoints a durable control plane.
8. Preserve current tutor behavior and default fake-provider behavior unless a separate integration explicitly opts into the new runtime.

## Phase overview

| Phase | Goal | Depends on |
|---|---|---|
| [Phase 1: Production foundation](./phase-01-production-foundation.md) | Move the proven public-API adapter into `server/internal/agent` with exact pin, contracts, and deterministic provider/MCP test seams. | None |
| [Phase 2: Controlled runs](./phase-02-controlled-runs.md) | Replace spike-only handles with concurrency-safe run control, cancellation, sticky lineage policy, and complete budget lifecycle. | Phase 1 |
| [Phase 3: Attributed streaming](./phase-03-attributed-streaming.md) | Ship the public streaming child tool and robust bounded Primer event/SSE bridge with independent stream/run contexts. | Phase 1, Phase 2 |
| [Phase 4: Server boundary](./phase-04-server-boundary.md) | Wire the runtime into the server behind explicit configuration and authorized start/status/cancel/stream HTTP boundaries. | Phase 2, Phase 3 |
| [Phase 5: Wave-1 hardening](./phase-05-hardening-and-rollout.md) | Prove race/reconnect behavior, document preview pin and non-durability, and leave a reversible, observable rollout. | Phase 4 |

## Completion rule

The plan is complete only when every phase's BDD scenarios pass through its stated public boundary, including negative authority/cancellation cases; server tests pass with real production wiring and permitted deterministic local provider/MCP fixtures; `go test -race ./...`, `go vet ./...`, `go build ./...`, `make test`, and `make cover` pass without gate changes; `git diff --check` is clean; the MAF pin and public-import audit pass; and a reviewer confirms no spike code was promoted by rewriting `go.mod`, no fake success path bypasses the runtime, and no durability claim is made.
