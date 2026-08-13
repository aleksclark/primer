# MAF Go Feasibility Spike — Implementation Plan

**Worktree:** `/home/aleks/work/projects/primer/worktrees/maf-go-feasibility`
**Branch:** `spike/maf-go-feasibility`
**Base:** `origin/master` @ `8772d98004a9bb6f86431c3f30cf10848dcc86b9`
**Upstream MAF pin target:** `github.com/microsoft/agent-framework-go@00ffc8c3648c547997eae3a3f2a3b00c28daea09` (main tip 2026-08-13; MIT; public preview; no release tags; `go 1.25.0`)
**Artifact root:** `spikes/maf-go/` (standalone throwaway module)
**Non-goals:** production Primer integration, Ultracore durability, session service, forking/vendoring MAF, real LLM credentials, push/PR.

## Context

Primer currently targets Fantasy (Charm) as the agent framework and will eventually use Ultracore. Short-term need: a Go agent runner that supports tailored subagents, MCP tool scopes, optional sessions, non-durable runs, and live streaming to clients. This spike answers whether Microsoft Agent Framework for Go (MAF Go) reduces work versus a thin Fantasy wrapper.

## Upstream facts (pre-code, to verify in tests)

| Fact | Source | Spike implication |
|------|--------|-------------------|
| `agent.Agent` wraps `ProviderConfig.Run` → `iter.Seq2[*ResponseUpdate,error]` | `agent/agent.go` | Streaming is first-class |
| Optional `Session` + history/context middleware | `agent/session.go`, history | Sessions optional |
| OpenAI/Anthropic/Gemini/Foundry providers | `provider/*` | OpenAI-compatible base URL via `openai.NewClient(option.WithBaseURL(...))` |
| MCP via official Go MCP SDK | `tool/mcptool` | Real MCP proof required |
| AG-UI SSE hosting | `provider/aguiprovider` | Wire streaming candidate |
| Stock `agenttool.New` calls `RunText(...).Collect()` | `tool/agenttool/agenttool.go:81` | Child stream updates hidden (F3 killer risk) |
| Workflows are in-process graphs/checkpoints | `workflow/` | Not Ultracore distributed durability (F8) |

## Feasibility questions (risk order)

### F3 — Nested streaming (KILLER)

**Given** a parent MAF agent invokes a child via stock `agenttool`
**When** the child streams text/tool events
**Then** parent stream consumers see only the collected final string (no child deltas)

**Given** a public-API-only Primer adapter (`StartChild` / streaming child tool)
**When** parent invokes child
**Then** parent event sink receives child start/text/tool/end with attribution
**OR** requirement is INVALIDATED with exact public-API blocker

Gates:

- Negative: stock `agenttool` stream invisibility test
- Positive (if possible): streaming child adapter surfaces attributed events without `internal/` imports

### F1 — Tailored subagent

**Given** parent + distinct child agent specs
**When** parent invokes child
**Then** child runs with its own identity (`Name`/`ID`/`Instructions`), model facade, and reduced tool set; parent tools do not automatically appear on child

Gates:

- Child config identity observable on run
- Child tool list is intersection of grants (not union/broadening)

### F2 — MCP scope (fail-closed)

**Given** real MCP server (in-process or httptest) exposing ≥2 tools
**When** child is granted only tool A
**Then** A is invokable via MAF `mcptool`; B is absent/uninvokable (negative test)

Gates:

- Official `modelcontextprotocol/go-sdk` + MAF `mcptool.ListTools`
- Denied tool not present in child's tool list / cannot be called through granted surface

### F5 — Cancellation / backpressure

**Given** parent → child → MCP/tool in flight
**When** client/request context cancels
**Then** cancellation propagates; no permanent block / leaked runner goroutine after disconnect

**Given** slow/disconnected stream consumer
**When** run continues
**Then** event sink is best-effort/bounded; subscriber loss does not fail the run; final correctness independent of client

### F4 — Wire streaming

**Given** remote HTTP client
**When** parent+child run streams
**Then** client receives incremental parent and child-attributed events

Prefer AG-UI SSE if public APIs carry child attribution. Else smallest Primer-shaped SSE adapter over public MAF streams. Do **not** claim AG-UI pass if only parent text is visible.

### F7 — Provider compatibility

**Given** local scripted OpenAI-compatible HTTP server (no real credentials)
**When** MAF OpenAI provider targets custom base URL
**Then** agent run succeeds against fake server using real MAF agent/tool/stream paths

### F6 — Optional sessions

**Given** no session option
**When** single-shot `Run`/`RunText`
**Then** run works

Also characterize serializable `Session`/history (JSON marshal) without building a session service.

### F8 — Operational boundary

**Given** MAF Go packages in-process
**When** documenting architecture
**Then** spike demonstrates SDK + protocol handlers only; no DBOS/NATS/durable worker; workflows ≠ distributed control plane

## Implementation shape

```
spikes/maf-go/
  go.mod                 # standalone module; pin MAF pseudo-version
  README.md
  RUN_REPORT.md          # evidence + verdict (after gates)
  primer/                # minimal Primer-facing adapter (public MAF only)
    types.go             # AgentSpec, ChildSpec, RunEvent, budgets
    runner.go            # Run + StartChild + authority intersection
    stream_child.go      # streaming child tool adapter (F3)
    sink.go              # bounded best-effort event sink
    mcp_filter.go        # fail-closed MCP tool filter
    sse.go               # Primer-shaped SSE (fallback if AG-UI insufficient)
  fakes/
    openai_server.go     # scripted OpenAI chat/completions stream
    mcp_server.go        # in-process MCP with allow/deny tools
  tests/                 # or package tests next to code
    f1_subagent_test.go
    f2_mcp_test.go
    f3_nested_stream_test.go
    f4_wire_stream_test.go
    f5_cancel_test.go
    f6_session_test.go
    f7_provider_test.go
    f8_boundary_test.go  # compile/doc assertion: no durable deps
  cmd/demo/              # optional tiny runnable demo
```

### Primer-facing contracts (minimal)

```go
type AgentSpec struct {
    Type, Name, Instructions string
    Tools []tool.Tool
    MaxChildren int // student/default-untrusted: 0
    MaxDepth int
    MaxTotalChildren int
}

type ChildSpec struct {
    Type, Name, Instructions string
    AllowedTools []string // fail-closed allowlist; intersection with parent grants
}

type RunEvent struct {
    RunID, ParentRunID, AgentType, AgentName string
    Kind string // start|text|tool_start|tool_end|child_start|child_end|error|end
    Text string
    ToolName string
    Err error
}

// EventSink is best-effort and bounded; never blocks the runner forever.
type EventSink interface {
    Emit(ctx context.Context, e RunEvent)
}
```

Authority rule: **child capabilities = intersection(parent grants, child policy, request)**. Never broaden. Default untrusted: `MaxChildren=0`.

### Public-API constraints

- Import only public MAF packages (`agent`, `tool`, `tool/agenttool`, `tool/mcptool`, `provider/openaiprovider`, `provider/aguiprovider`, `message`)
- No `github.com/microsoft/agent-framework-go/internal/...`
- No fork/vendor/patch of MAF
- Pin exact revision in `go.mod`

## Risk-ordered build sequence (TDD)

1. Module scaffold + pin MAF + fake OpenAI server (enables F7 early)
2. **F3 baseline** stock agenttool Collect invisibility (negative, must fail product claim)
3. **F3 adapter** attempt streaming child via public `child.Run` + custom `tool.FuncTool` (not stock agenttool)
4. **F1** tailored child identity + reduced tools
5. **F2** real MCP + fail-closed filter
6. **F5** cancel propagation + slow sink
7. **F4** AG-UI probe then Primer SSE if needed
8. **F6** no-session + Session JSON characterization
9. **F7** OpenAI base URL against fake server (may already be green from step 1)
10. **F8** boundary doc + dependency assertion
11. Gates + independent review + `RUN_REPORT.md`

## Exact gates

From `spikes/maf-go/`:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
```

From worktree root:

```bash
git diff --check 8772d98004a9bb6f86431c3f30cf10848dcc86b9
# confirm changed paths only under agent_docs/plans/ and spikes/maf-go/
```

Full `make test` / `make lint` only if cheap and unaffected; standalone spike gates are authoritative for this throwaway module. Never lower existing repo gates.

## Verdict criteria

| Result | Criteria |
|--------|----------|
| **VALIDATED** | F1,F2,F5,F6,F7,F8 pass; F3 either adapter works without fork **or** honestly INVALIDATED; F4 wire stream shows parent+child attribution (AG-UI or Primer SSE) |
| **PARTIAL** | Core runner usable but ≥1 killer requirement needs Primer-owned work beyond thin adapter (e.g. F3 needs custom tool; AG-UI lacks child attribution) |
| **INVALIDATED** | Cannot run real MAF agent paths without fork/internal imports, or MCP/provider/cancel fundamentally broken for Primer needs |

### Recommendation labels (RUN_REPORT)

- **GO** — adopt MAF short-term; list Primer-owned surface
- **CONDITIONAL GO** — adopt only with named gaps owned by Primer (e.g. streaming child tool, SSE envelope)
- **NO-GO** — Fantasy thin wrapper cheaper / MAF blocks too hard

## Fantasy comparison dimensions (for RUN_REPORT)

| Dimension | MAF Go | Fantasy v0.40 wrapper (expected) |
|-----------|--------|-----------------------------------|
| Subagent as tool | stock Collect; custom FuncTool possible | likely custom either way |
| MCP | first-class mcptool | wrap MCP yourself |
| Streaming | ResponseUpdate iter + AG-UI | Charm stream patterns |
| Sessions | optional Session JSON | app-owned |
| Durability | in-process workflows only | none (same short-term) |
| Preview/stability | public preview, no tags | Charm ecosystem maturity |
| Work reduction | providers+MCP+AG-UI vs custom | thinner, already in Primer stack |

## Commit plan

1. `docs: add MAF Go feasibility spike plan` (this file only)
2. `spike(maf-go): implement F1–F8 feasibility artifact` (module + tests + README)
3. `spike(maf-go): RUN_REPORT and gate evidence` (after real runs + review fixes)

No push. Local commits only.
