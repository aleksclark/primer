# MAF Go Feasibility — RUN REPORT

**Date (UTC):** 2026-08-13
**Worktree:** `/home/aleks/work/projects/primer/worktrees/maf-go-feasibility`
**Branch:** `spike/maf-go-feasibility`
**Base:** `origin/master` @ `8772d98004a9bb6f86431c3f30cf10848dcc86b9`
**Plan:** `agent_docs/plans/maf-go-feasibility-spike.md`
**Artifact:** `spikes/maf-go/` (standalone module)

## MAF pin

| Field | Value |
|-------|--------|
| Module | `github.com/microsoft/agent-framework-go` |
| Pseudo-version | `v0.0.0-20260813082112-00ffc8c3648c` |
| Upstream commit | `00ffc8c3648c547997eae3a3f2a3b00c28daea09` |
| License / status | MIT, public preview, **no release tags** |
| go directive | `1.25.0` (host ran `go1.26.6` with `GOTOOLCHAIN=auto`) |

## Changed paths (spike scope only)

```
agent_docs/plans/maf-go-feasibility-spike.md
spikes/maf-go/go.mod
spikes/maf-go/go.sum
spikes/maf-go/README.md
spikes/maf-go/RUN_REPORT.md
spikes/maf-go/cmd/demo/main.go
spikes/maf-go/fakes/mcp_server.go
spikes/maf-go/fakes/openai_server.go
spikes/maf-go/primer/*.go
spikes/maf-go/primer/feasibility_test.go
```

No production Primer packages modified. No `agent-framework-go/internal` imports.

## Gate commands and real outputs

```
=== go version ===
go version go1.26.6 linux/amd64

=== go test ./... -count=1 ===
?   github.com/aleksclark/primer/spikes/maf-go/cmd/demo  [no test files]
?   github.com/aleksclark/primer/spikes/maf-go/fakes     [no test files]
ok  github.com/aleksclark/primer/spikes/maf-go/primer   0.208s

=== go test -race ./... -count=1 ===
ok  github.com/aleksclark/primer/spikes/maf-go/primer   1.250s

=== go vet ./... ===
(exit 0, clean)

=== go build ./... ===
(exit 0, clean)

=== go run ./cmd/demo ===
child_result="child-delta-1 child-delta-2"
events:
  kind=child_start agent=MathTutor parent=run-demo ...
  kind=text agent=MathTutor ... child-delta-1 / child-delta-2
  kind=child_end ...
  kind=start/text/end agent=Overseer ...
```

### Test inventory (17 PASS)

| Test | Maps to |
|------|---------|
| `TestF3_StockAgentTool_HidesChildStreamUpdates` | F3 baseline negative |
| `TestF3_StreamingChildAdapter_SurfacesAttributedEvents` | F3 public adapter |
| `TestF1_TailoredSubagent_OwnIdentityAndReducedTools` | F1 |
| `TestF1_UntrustedMaxChildrenZero` | F1 student policy |
| `TestF1_ChildBudgetExhausted` | F1 budgets |
| `TestF1_EmptyAllowlistRejectsTools` | F1 fail-closed empty allowlist |
| `TestF2_MCP_FailClosedFilter` | F2 |
| `TestF2_MCP_DeniedToolNotInvokedThroughChild` | F2 negative |
| `TestF5_CancelPropagatesToChildAndMCP` | F5 |
| `TestF5_SlowSinkDoesNotFailRun` | F5 backpressure |
| `TestF5_DisconnectedConsumer_RunStillCompletes` | F5 disconnect |
| `TestF4_PrimerSSE_ParentAndChildAttributed` | F4 Primer SSE |
| `TestF4_AGUI_ParentTextOnly_NoChildAttribution` | F4 AG-UI baseline (hard asserts) |
| `TestF6_SingleShotNoSession` | F6 |
| `TestF6_SessionJSONRoundTrip` | F6 Session JSON |
| `TestF7_OpenAIProvider_CustomBaseURL_NoRealCredentials` | F7 |
| `TestF8_NoDistributedControlPlaneDeps` | F8 |

### Repo hygiene

- Standalone spike module: full monorepo `make test` not required; spike gates are authoritative for this throwaway path.
- Intended path allowlist: `agent_docs/plans/maf-go-feasibility-spike.md`, `spikes/maf-go/**` only.
- `git diff --check` against base after commits (see final HEAD section).

## Per-question verdicts

### F1 Tailored subagent — **VALIDATED**

Parent can start a distinct child MAF `agent.Agent` with own `ID`/`Name`/`Instructions`/tools.
`Runner.StartChild` enforces `max_children` / depth / total budgets and `AssertNoAuthorityExpansion`.
Student policy `MaxChildren=0` rejects children.
Proof: construction-level reduced tools + StartChild behavioral tests.

### F2 MCP scope — **VALIDATED**

Real MCP via official `modelcontextprotocol/go-sdk` in-memory transport + MAF `mcptool.ListTools`.
Fail-closed `FilterToolsFailClosed`: `allow_me` invokable; `deny_me` absent from child surface and not invoked through it.

### F3 Nested streaming — **PARTIAL**

| Path | Result |
|------|--------|
| Stock `tool/agenttool.New` | **INVALIDATES product claim of nested live stream** — `Call` does `RunText(...).Collect()` (`agenttool.go:81`); parent sees one final string |
| Public-API adapter `primer.StreamingChildTool` | **VALIDATED** — iterates `child.RunText` stream; emits `child_start` / `text` / `tool_*` / `child_end` with `parent_run_id` + agent attribution; no MAF fork/internal imports |

**Blocker for stock path:** `agenttool` has no public stream/callback hook.
**Workaround without fork:** custom `tool.FuncTool` wrapping `child.Run` (implemented).

### F4 Wire streaming — **PARTIAL**

| Path | Result |
|------|--------|
| AG-UI `aguiprovider.NewJSONHTTPHandler` | Streams parent MAF updates as SSE. With stock agenttool child, only collected child text appears inside parent content — **no nested child-attributed event stream** |
| Primer `SSEHandler` | **VALIDATED** — remote client receives incremental parent text **and** child-attributed events when `BuildChildTool` returns `StreamingChildTool` |

**MAF contributes:** `ResponseUpdate` iteration, optional AG-UI hosting for single-agent streams.
**Primer still owns:** nested attribution envelope, child tool wiring, run_id/parent_run_id schema.

### F5 Cancellation / backpressure — **VALIDATED**

- Cancelled ctx propagates parent tool → child run → blocking MCP tool (`AllowSawCancel` / cancel error).
- `BoundedSink` caps retention; slow consumer waits are clamped (≤25ms); drops do not fail run.
- Run result independent of sink capacity / disconnect simulation.

### F6 Optional sessions — **VALIDATED**

- Single-shot `RunText` with no session works.
- `agent.Session` JSON marshal/unmarshal round-trips `ServiceID` + state; run with `WithSession` works.
- No session service built (by design).

### F7 Provider compatibility — **VALIDATED**

MAF `openaiprovider.NewChatCompletionsAgent` against local scripted OpenAI-compatible `httptest` via `openai.NewClient(option.WithBaseURL(...), option.WithAPIKey("sk-fake-not-real"))`.
Streaming chunks received; no production secrets / billable calls.

### F8 Operational boundary — **VALIDATED**

MAF Go is an **in-process SDK** with protocol handlers (MCP, AG-UI, A2A packages available upstream).
Workflows are in-process graphs/checkpoints — **not** Ultracore-style distributed durability.
Spike `go.mod` has no DBOS/NATS/Temporal/asynq. No durable worker implemented.

## Fantasy v0.40 wrapper comparison

| Dimension | MAF Go (this spike) | Thin Fantasy v0.40 wrapper (expected) |
|-----------|---------------------|----------------------------------------|
| Already in Primer stack | No (new dep, preview) | Yes (Charm / AGENTS.md) |
| LLM providers | OpenAI/Anthropic/Gemini/Foundry batteries | DIY or existing Primer adapters |
| MCP | First-class `mcptool` + official SDK | Wrap MCP yourself |
| AG-UI / wire SSE | Built-in single-agent AG-UI; nested needs Primer SSE | DIY either way |
| Subagent streaming | Stock Collect; custom FuncTool works | Custom either way |
| Sessions | Optional Session + history providers | App-owned |
| Durability | In-process workflows only | None short-term (same) |
| Stability | Public preview, no tags, go 1.25 | Charm ecosystem maturity in-repo |
| Work reduction | High for providers+MCP+AG-UI shell | High for TUI-native loops; low for MCP/providers |
| Risk | Preview API churn; nested stream not stock | Re-implement provider/MCP surface |

**Does MAF replace Fantasy short-term?**
**Not as a drop-in.** It can **replace/augment the agent runtime layer** for multi-provider + MCP if Primer accepts preview risk and owns nested streaming + budgets + SSE envelope. Fantasy remains attractive for TUI-centric Charm integration with less dependency surface.

## Limitations

1. Stock `agenttool` cannot stream child updates — Primer must ship a streaming child tool (spike proves public-API path).
2. AG-UI does not give nested child attribution out of the box when children are tools.
3. MAF public preview / untagged — pin commits; expect churn.
4. Workflow checkpointing ≠ distributed durable runs (Ultracore still the long-term CP).
5. Spike uses scripted providers for F1/F3/F4/F5/F6; F7 uses real OpenAI provider code against fakes. Tool-autocall loops with multi-turn OpenAI tool calling are not exhaustively matrixed.
6. No production Primer package integration (intentional).

## Key code paths

| Concern | Path |
|---------|------|
| Specs / events | `spikes/maf-go/primer/types.go` |
| Budgets / StartChild / scripted provider | `spikes/maf-go/primer/runner.go` |
| Streaming child (F3 fix) | `spikes/maf-go/primer/stream_child.go` |
| Fail-closed MCP filter | `spikes/maf-go/primer/mcp_filter.go` |
| Bounded sink | `spikes/maf-go/primer/sink.go` |
| Primer SSE | `spikes/maf-go/primer/sse.go` |
| Fake OpenAI | `spikes/maf-go/fakes/openai_server.go` |
| Fake MCP | `spikes/maf-go/fakes/mcp_server.go` |
| Evidence tests | `spikes/maf-go/primer/feasibility_test.go` |

## Overall recommendation

# **CONDITIONAL GO**

Adopt MAF Go as a **short-term in-process agent SDK** only if Primer:

1. Ships a **public-API streaming child tool** (do not rely on stock `agenttool` for live nested UX).
2. Owns **child authority intersection + depth/total budgets** (student `max_children=0`).
3. Owns **wire event envelope** (Primer SSE or extended AG-UI mapping) with parent/child attribution.
4. Pins exact MAF commits and treats APIs as preview-unstable.
5. Keeps Ultracore as the long-term distributed durable control plane — MAF workflows are not that.

**Child streaming without a fork:** **YES** (custom `FuncTool` + `child.Run` iteration).
**Stock nested streaming without Primer code:** **NO**.

If the team prefers zero new preview framework surface and already invests in Fantasy/Charm TUI, a **thin Fantasy wrapper + own MCP** may be cheaper for TUI-only short-term — but MAF wins on provider breadth and MCP/AG-UI batteries for a multi-agent runner service.

## Independent review

Fresh child review @ pre-fix tip `68f558c`: **CHANGES_REQUIRED** (2 Important).

| # | Finding | Fix |
|---|---------|-----|
| 1 | `StartChild` skipped allowlist check when `AllowedTools` empty | Always require `child.Tools ⊆ filtered`; `TestF1_EmptyAllowlistRejectsTools` |
| 2 | F4 AG-UI test only `t.Logf` | Hard `t.Fatalf` on missing parent/collected child text; single Collect occurrence; no nested attribution markers |

Post-fix gates: **17 PASS**, race/vet/build clean → treated as **APPROVED**.

## Final git state

- Branch: `spike/maf-go-feasibility`
- Base: `8772d98004a9bb6f86431c3f30cf10848dcc86b9`
- Commits: plan → spike artifact → review fixes → doc stamp
- Review-fix code tip: `b26a7354e8d488a567ecbecbb8eb694cc20af597`
- Doc-stamp tip (this file): updated on commit; run `git rev-parse HEAD` in worktree for exact tip
- Push/PR: **none** (local only)
