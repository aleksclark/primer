# Spike: MAF Go feasibility (Primer short-term agent runtime)

Throwaway standalone module answering whether
[Microsoft Agent Framework for Go](https://github.com/microsoft/agent-framework-go)
can serve as Primer's short-term agent SDK before Ultracore.

**Not production code.** Do not import from `server/` or workstation packages.

## Pin

```
github.com/microsoft/agent-framework-go v0.0.0-20260813082112-00ffc8c3648c
```

Upstream commit: `00ffc8c3648c547997eae3a3f2a3b00c28daea09` (main, 2026-08-13).
MIT, public preview, no release tags, `go 1.25.0`.

## Layout

| Path | Role |
|------|------|
| `primer/` | Minimal Primer-facing adapter (AgentSpec, Runner+PrepareRun, StreamingChildTool, SSEBridge, fail-closed MCP filter) |
| `fakes/` | Scripted OpenAI-compatible HTTP server + in-process MCP (`allow_me` / `deny_me`) |
| `primer/feasibility_test.go` | F1–F8 behavioral proofs (27 tests) |
| `cmd/demo/` | Tiny offline demo (scripted provider, no network) |
| `RUN_REPORT.md` | Gates, verdicts, Fantasy comparison, review trail |

## Run

```bash
cd spikes/maf-go
export GOTOOLCHAIN=auto   # pulls Go 1.25+ toolchain if needed
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
go run ./cmd/demo
```

No real LLM credentials. OpenAI tests use `httptest` + dummy `«redacted:sk-…»`.

## Feasibility summary (see RUN_REPORT.md)

| # | Question | Result |
|---|----------|--------|
| F1 | Tailored subagent | **VALIDATED** — identity, instructions/tools hard asserts, depth/budgets/run IDs |
| F2 | MCP fail-closed scope | **VALIDATED** |
| F3 | Nested streaming | **PARTIAL** — stock `agenttool` Collects; public streaming adapter works |
| F4 | Wire streaming | **PARTIAL** — Primer SSE + bridge: attribution + barrier incremental; AG-UI alone no nested attribution |
| F5 | Cancel / backpressure | **VALIDATED** — MCP cancel hard; async SSE bridge vs slow writer |
| F6 | Optional sessions | **VALIDATED** |
| F7 | OpenAI-compatible base URL | **VALIDATED** |
| F8 | In-process SDK boundary | **VALIDATED** |

**Overall: CONDITIONAL GO** — MAF reduces provider/MCP/AG-UI work vs greenfield, but Primer still owns nested streaming attribution, child budgets/depth/authority intersection, run lineage, and async SSE bridge. Does not replace Fantasy for free; not a distributed control plane (Ultracore still required long-term).

### Capability split

- **Stock MAF:** agent stream, Session JSON, OpenAI base URL, MCP list/call, AG-UI single-agent SSE, stock agenttool Collect.
- **Primer public-API adapter (proven here):** Runner depth/budgets/PrepareRun lineage, StreamingChildTool, fail-closed filter, SSEBridge + Primer SSE envelope.
- **Production remaining:** package integration, multi-turn root policy, Ultracore durability, broader tool-autocall matrix.

## Constraints honored

- Public MAF APIs only (no `internal/` imports, no fork/vendor patch)
- Standalone module (not woven into Primer server `go.mod`)
- Fail-closed tool scope; student policy `max_children=0`
- Orchestrator-controlled depth (never caller-supplied)
- Fresh run IDs ≠ agent IDs; root/parent attribution
- Best-effort bounded async event sinks (slow consumer must not block runner)

## Review

Fresh independent review @ code tip `1d73c2d`: **APPROVED** (0 Critical / 0 Important).
