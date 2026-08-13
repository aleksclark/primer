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
spikes/maf-go/**
```

No production Primer packages modified. No `agent-framework-go/internal` imports.

## Gate commands and real outputs (post-hardening tip)

```
=== go version ===
go version go1.26.6 linux/amd64

=== go test ./... -count=1 ===
?   github.com/aleksclark/primer/spikes/maf-go/cmd/demo  [no test files]
?   github.com/aleksclark/primer/spikes/maf-go/fakes     [no test files]
ok  github.com/aleksclark/primer/spikes/maf-go/primer   0.281s

=== go test -race ./... -count=1 ===
ok  github.com/aleksclark/primer/spikes/maf-go/primer   1.300s

=== go vet ./... ===
(exit 0, clean)

=== go build ./... ===
(exit 0, clean)

=== go run ./cmd/demo ===
child_result="child-delta-1 child-delta-2"
parent_run_id="<uuid>" agent_id="demo-parent" depth=0
child_depth=1
events: start/text/end (parent) + child_start/text/child_end with parent=root=parent_run_id

=== git diff --check base..HEAD ===
(exit 0, clean)
```

### Test inventory (27 PASS)

| Test | Maps to |
|------|---------|
| `TestF3_StockAgentTool_HidesChildStreamUpdates` | F3 baseline negative |
| `TestF3_StreamingChildAdapter_SurfacesAttributedEvents` | F3 public adapter |
| `TestF1_TailoredSubagent_OwnIdentityAndReducedTools` | F1 identity + hard MAF instructions/tools |
| `TestF1_UntrustedMaxChildrenZero` | F1 student policy |
| `TestF1_ChildBudgetExhausted` | F1 direct budget + active close |
| `TestF1_EmptyAllowlistRejectsTools` | F1 fail-closed empty allowlist + budget rollback |
| `TestF1_MaxDepthDeniesGrandchild` | F1 orchestrator depth denial |
| `TestF1_MaxDepthZeroDeniesAllChildren` | F1 MaxDepth=0 |
| `TestF1_ActiveChildrenClosesOnErrorAndCancel` | F1 lifecycle accounting |
| `TestF1_RunIDsDistinctAcrossInvocations` | F1 fresh run IDs ≠ agent ID |
| `TestF1_PrepareRun_StartChildBeforeRun_LineageCoherent` | F1 SSE-path lineage |
| `TestF1_MaxTotalChildrenIndependentOfDirect` | F1 total budget semantics |
| `TestF2_MCP_FailClosedFilter` | F2 |
| `TestF2_MCP_DeniedToolNotInvokedThroughChild` | F2 negative |
| `TestF5_CancelPropagatesToChildAndMCP` | F5 cancel → MCP AllowSawCancel hard |
| `TestF5_SlowSinkDoesNotFailRun` | F5 BoundedSink |
| `TestF5_DisconnectedConsumer_RunStillCompletes` | F5 drop ≠ fail run |
| `TestF5_SSEBridge_SlowWriterDoesNotBlockRunner` | F5 async bridge vs blocking writer |
| `TestF5_SSEBridge_CancelTerminatesWriter` | F5 writer shutdown |
| `TestF5_SSEHandler_HTTPDisconnect_RunCompletes` | F5 HTTP disconnect (partial: start proven) |
| `TestF4_PrimerSSE_ParentAndChildAttributed` | F4 attribution + lineage parse |
| `TestF4_PrimerSSE_IncrementalBeforeCompletion` | F4 barrier-driven incremental |
| `TestF4_AGUI_ParentTextOnly_NoChildAttribution` | F4 AG-UI negative baseline |
| `TestF6_SingleShotNoSession` | F6 |
| `TestF6_SessionJSONRoundTrip` | F6 Session JSON |
| `TestF7_OpenAIProvider_CustomBaseURL_NoRealCredentials` | F7 |
| `TestF8_NoDistributedControlPlaneDeps` | F8 |

## Capability layers (honest split)

| Layer | What is proven |
|-------|----------------|
| **Stock MAF** | `agent.Agent` stream iter, optional Session JSON, OpenAI provider + custom base URL, MCP via `mcptool` + official SDK, AG-UI single-agent SSE, stock `agenttool` Collects child streams |
| **Public-API Primer adapter (this spike)** | `Runner` budgets/depth/lineage, `StreamingChildTool`, fail-closed tool filter, `SSEBridge` + Primer SSE envelope with parent/child/root attribution |
| **Production work remaining** | Wire into Primer server/workstation packages; multi-turn root_run sticky policy; durable CP (Ultracore); multi-turn tool-autocall matrix; concurrent PrepareRun safety; stronger HTTP-disconnect completion proof; production backpressure tuning |

## Per-question verdicts

### F1 Tailored subagent — **VALIDATED**

- Distinct child MAF `agent.Agent` with own `ID`/`Name`.
- **Hard asserts:** child `WithInstructions` reaches provider options; parent instructions do not leak; child tool options are only allowlisted set (`shared_calc`), not `parent_only`.
- `Runner.StartChild` returns child `*Runner` at orchestrator-controlled `depth+1` (never caller-supplied depth).
- Budgets: `MaxChildren=0`, direct exhaustion, total exhaustion (independent of direct), `MaxDepth` grandchild denial, `MaxDepth=0`.
- `activeChildren` increments on StartChild reservation and decrements on Call completion/error/cancel; failed allowlist rolls back budgets.
- Fresh `run_id` per Run/child Call, distinct from stable `agent_id`; `root_run_id` + `parent_run_id` coherent on PrepareRun→StartChild→Run and SSE path.

### F2 MCP scope — **VALIDATED**

Real MCP via official `modelcontextprotocol/go-sdk` in-memory transport + MAF `mcptool.ListTools`.
Fail-closed `FilterToolsFailClosed`: `allow_me` invokable; `deny_me` absent from child surface and not invoked through it.

### F3 Nested streaming — **PARTIAL**

| Path | Result |
|------|--------|
| Stock `tool/agenttool.New` | **INVALIDATES product claim of nested live stream** — `Call` does `RunText(...).Collect()`; parent sees one final string |
| Public-API adapter `primer.StreamingChildTool` | **VALIDATED** — iterates `child.RunText`; emits `child_start` / `text` / `tool_*` / `child_end` with parent/root/depth attribution; no MAF fork/internal imports |

**Blocker for stock path:** `agenttool` has no public stream/callback hook.
**Workaround without fork:** custom `tool.FuncTool` wrapping `child.Run` (implemented).

### F4 Wire streaming — **PARTIAL**

| Path | Result |
|------|--------|
| AG-UI `aguiprovider.NewJSONHTTPHandler` | Parent SSE only; stock child Collect — **no nested child-attributed event stream** (hard negative asserts) |
| Primer `SSEHandler` + `SSEBridge` | **VALIDATED** for attribution + incremental arrival: barrier test proves `early-parent-delta` is on the wire **before** provider is released; F4 parse asserts `parent.run_id == parent.root_run_id == child.parent_run_id == child.root_run_id` |

**MAF contributes:** `ResponseUpdate` iteration, optional AG-UI hosting for single-agent streams.
**Primer still owns:** nested attribution envelope, child tool wiring, run/root/parent ids, async bridge.

### F5 Cancellation / backpressure — **VALIDATED** (with noted residual)

- Cancelled ctx propagates parent tool → child run → blocking MCP tool; **hard** `AllowSawCancel` assert.
- `BoundedSink` caps retention; drops do not fail run.
- **`SSEBridge`:** async bounded queue; slow/blocking `ResponseWriter` does **not** block runner (`TestF5_SSEBridge_SlowWriterDoesNotBlockRunner`); cancel terminates writer goroutine; stream loss does not fail run.
- Residual: `TestF5_SSEHandler_HTTPDisconnect_RunCompletes` proves provider start + disconnect path more weakly than the bridge unit tests (review suggestion; not a failing claim).

### F6 Optional sessions — **VALIDATED**

- Single-shot `RunText` with no session works.
- `agent.Session` JSON marshal/unmarshal round-trips `ServiceID` + state; run with `WithSession` works.
- No session service built (by design).

### F7 Provider compatibility — **VALIDATED**

MAF `openaiprovider.NewChatCompletionsAgent` against local scripted OpenAI-compatible `httptest` via `openai.NewClient(option.WithBaseURL(...), option.WithAPIKey("«redacted:sk-…»"))`.
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

## Limitations (post-hardening)

1. Stock `agenttool` cannot stream child updates — Primer must ship a streaming child tool.
2. AG-UI does not give nested child attribution out of the box when children are tools.
3. MAF public preview / untagged — pin commits; expect churn.
4. Workflow checkpointing ≠ distributed durable runs (Ultracore still the long-term CP).
5. Scripted providers for most F1/F3/F4/F5/F6; F7 uses real OpenAI provider against fakes. Multi-turn OpenAI tool-autocall not exhaustively matrixed.
6. `lineage.rootID` is sticky for a Runner lifetime (fine for per-request SSE `NewRunner`; multi-turn root reuse needs an explicit policy).
7. `activeChildren` is reserved at `StartChild` and released from `StreamingChildTool.Call`; a pre-wired never-invoked child leaks the active slot until process end (spike-acceptable; production should release on drop).
8. No production Primer package integration (intentional).

## Key code paths

| Concern | Path |
|---------|------|
| Specs / events / orchestration | `spikes/maf-go/primer/types.go` |
| Budgets / PrepareRun / StartChild / scripted provider | `spikes/maf-go/primer/runner.go` |
| Streaming child (F3) | `spikes/maf-go/primer/stream_child.go` |
| Fail-closed MCP filter | `spikes/maf-go/primer/mcp_filter.go` |
| Bounded sink | `spikes/maf-go/primer/sink.go` |
| Primer SSE + async SSEBridge | `spikes/maf-go/primer/sse.go` |
| Fake OpenAI | `spikes/maf-go/fakes/openai_server.go` |
| Fake MCP | `spikes/maf-go/fakes/mcp_server.go` |
| Evidence tests | `spikes/maf-go/primer/feasibility_test.go` |

## Overall recommendation

# **CONDITIONAL GO** (still stands)

Adopt MAF Go as a **short-term in-process agent SDK** only if Primer:

1. Ships a **public-API streaming child tool** (do not rely on stock `agenttool` for live nested UX).
2. Owns **child authority intersection + depth/total budgets** (student `max_children=0`).
3. Owns **wire event envelope** (Primer SSE or extended AG-UI mapping) with parent/child/root attribution and an **async bounded bridge**.
4. Pins exact MAF commits and treats APIs as preview-unstable.
5. Keeps Ultracore as the long-term distributed durable control plane — MAF workflows are not that.

**Child streaming without a fork:** **YES** (custom `FuncTool` + `child.Run` iteration).
**Stock nested streaming without Primer code:** **NO**.

If the team prefers zero new preview framework surface and already invests in Fantasy/Charm TUI, a **thin Fantasy wrapper + own MCP** may be cheaper for TUI-only short-term — but MAF wins on provider breadth and MCP/AG-UI batteries for a multi-agent runner service.

## Independent review trail

| Tip | Verdict | Notes |
|-----|---------|-------|
| `68f558c` | CHANGES_REQUIRED | 2 Important: empty allowlist skip; soft AG-UI asserts |
| `b26a735` | (no fresh post-fix review; previously overclaimed APPROVED) | allowlist + AG-UI hard asserts |
| `c874670` | CHANGES_REQUIRED | 2 Important: StartChild-before-Run lineage drift; F5 MCP cancel soft assert |
| **`1d73c2d`** | **APPROVED** | Fresh independent review; 0 Critical / 0 Important; suggestions only (sticky rootID, unused-child active slot, HTTP-disconnect residual, PrepareRun concurrency) |

## Final git state

- Branch: `spike/maf-go-feasibility`
- Base: `8772d98004a9bb6f86431c3f30cf10848dcc86b9`
- **Reviewed code SHA:** `1d73c2d6935c24c39ba482ac6bd9995320e3f6e1` (APPROVED)
- **Honest report content commit:** `20147d640a1e49e2d2b546b4a99f07af86457e82`
- **Final HEAD:** tip of `spike/maf-go-feasibility` after docs-only stamps (descendant of reviewed code SHA; `git rev-parse HEAD`)
- Push/PR: **none** (local only)
- **CONDITIONAL GO still stands**
