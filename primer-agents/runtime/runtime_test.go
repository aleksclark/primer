package agentruntime_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "github.com/aleksclark/primer/agents/runtime"
	"github.com/aleksclark/primer/agents/runtime/testfakes"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	maftool "github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/functool"
)

// ---- helpers ----------------------------------------------------------------

func textUpdate(s string) *mafagent.ResponseUpdate {
	return &mafagent.ResponseUpdate{
		Role:     message.RoleAssistant,
		Contents: message.Contents{&message.TextContent{Text: s}},
	}
}

func scriptedAgent(t *testing.T, name, id string, updates ...*mafagent.ResponseUpdate) *mafagent.Agent {
	t.Helper()
	prov := &agentruntime.ScriptedProvider{Name: "scripted", Updates: updates}
	return agentruntime.NewScriptedAgent(mafagent.Config{
		ID:          id,
		Name:        name,
		Description: name + " agent",
	}, prov)
}

func hasKind(events []agentruntime.RunEvent, kind string) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func textsOf(events []agentruntime.RunEvent, kind string) []string {
	var out []string
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e.Text)
		}
	}
	return out
}

func mustFuncTool(t *testing.T, name string) maftool.Tool {
	t.Helper()
	tl, err := functool.New(functool.Config{Name: name, Description: name},
		func(ctx context.Context, in struct{}) (string, error) { return "x", nil })
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

// eventFuncSink wraps a bare function as an EventSink.
type eventFuncSink func(ctx context.Context, e agentruntime.RunEvent)

func (f eventFuncSink) Emit(ctx context.Context, e agentruntime.RunEvent) { f(ctx, e) }

// blockingWriter is an http.ResponseWriter that blocks on the Nth Write.
type blockingWriter struct {
	mu      sync.Mutex
	writes  int
	blockAt int
	blockCh chan struct{}
	entered chan struct{}
	buf     strings.Builder
	flushes int
}

func newBlockingWriter(blockAt int) *blockingWriter {
	return &blockingWriter{
		blockAt: blockAt,
		blockCh: make(chan struct{}),
		entered: make(chan struct{}),
	}
}

func (b *blockingWriter) Header() http.Header { return make(http.Header) }
func (b *blockingWriter) WriteHeader(int)     {}
func (b *blockingWriter) Flush()              { b.flushes++ }
func (b *blockingWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.writes++
	n := b.writes
	b.mu.Unlock()
	if n == b.blockAt {
		close(b.entered)
		<-b.blockCh
	}
	b.mu.Lock()
	b.buf.Write(p)
	b.mu.Unlock()
	return len(p), nil
}
func (b *blockingWriter) release() { close(b.blockCh) }
func (b *blockingWriter) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ---- Phase 1: public MAF import / version -----------------------------------

func TestPhase1_MAFModuleVersion(t *testing.T) {
	// Compile-time evidence: if the wrong version is pinned this file won't
	// compile (agentruntime.NewScriptedAgent uses mafagent.Config / ProviderConfig).
	// Runtime: exercise real MAF agent.New path.
	prov := &agentruntime.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{textUpdate("ok")}}
	a := agentruntime.NewScriptedAgent(mafagent.Config{ID: "v-check", Name: "V"}, prov)
	if a.ID() != "v-check" {
		t.Fatalf("agent id = %q", a.ID())
	}
}

func TestPhase1_NoInternalImports(t *testing.T) {
	// This test is intentionally trivial; the real check is the import graph.
	// If any production file imported agent-framework-go/internal/… it would
	// fail to compile (internal package rules). This test documents the intent.
	_ = agentruntime.NewRunner(agentruntime.AgentSpec{}, nil, nil)
}

// ---- F1: tailored subagent identity & tools ----------------------------------

func TestF1_TailoredSubagent_OwnIdentityAndReducedTools(t *testing.T) {
	parentTool, err := functool.New(functool.Config{Name: "parent_only", Description: "parent"},
		func(ctx context.Context, in struct{}) (string, error) { return "p", nil })
	if err != nil {
		t.Fatal(err)
	}
	sharedTool, err := functool.New(functool.Config{Name: "shared_calc", Description: "calc"},
		func(ctx context.Context, in struct{ X int }) (string, error) { return "42", nil })
	if err != nil {
		t.Fatal(err)
	}

	const childInstr = "You are the math tutor CHILD-ONLY instructions."
	const parentInstr = "You are the parent OVERSEER — must not leak to child."

	childProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				yield(textUpdate("child-ok"), nil)
			}
		},
	}
	childMAF := agentruntime.NewScriptedAgent(mafagent.Config{
		ID:         "child-math",
		Name:       "MathTutor",
		Tools:      []maftool.Tool{sharedTool},
		RunOptions: []mafagent.Option{mafagent.WithInstructions(childInstr)},
	}, childProv)

	parentGrants := []maftool.Tool{parentTool, sharedTool}
	spec := agentruntime.AgentSpec{
		Type:             "overseer",
		Name:             "Overseer",
		Instructions:     parentInstr,
		Tools:            parentGrants,
		MaxChildren:      2,
		MaxDepth:         2,
		MaxTotalChildren: 5,
	}
	parentMAF := scriptedAgent(t, "Overseer", "parent-1", textUpdate("done"))
	sink := &agentruntime.CollectingSink{}
	r := agentruntime.NewRunner(spec, parentMAF, sink)

	childToolsFiltered := agentruntime.FilterToolsFailClosed(parentGrants, []string{"shared_calc"})
	if len(childToolsFiltered) != 1 || childToolsFiltered[0].Name() != "shared_calc" {
		t.Fatalf("filter = %v", agentruntime.ToolNames(childToolsFiltered))
	}
	if err := agentruntime.AssertNoAuthorityExpansion(parentGrants, childToolsFiltered); err != nil {
		t.Fatal(err)
	}
	if err := agentruntime.AssertNoAuthorityExpansion(parentGrants, []maftool.Tool{parentTool, sharedTool, mustFuncTool(t, "evil")}); err == nil {
		t.Fatal("expected authority expansion error")
	}

	if err := r.Run(context.Background(), "parent turn", mafagent.WithInstructions(parentInstr)); err != nil {
		t.Fatal(err)
	}
	parentRunID := r.LastRunID()
	if parentRunID == "" || parentRunID == parentMAF.ID() {
		t.Fatalf("parent run id must be fresh, distinct from agent id; run=%q agent=%q", parentRunID, parentMAF.ID())
	}

	ct, childRunner, err := r.StartChild(context.Background(), agentruntime.ChildSpec{
		Type:         "math_tutor",
		Name:         "MathTutor",
		Instructions: childInstr,
		AllowedTools: []string{"shared_calc"},
		Tools:        childToolsFiltered,
		MaxChildren:  0,
	}, childMAF)
	if err != nil {
		t.Fatalf("StartChild: %v", err)
	}
	if ct.Name() != "MathTutor" {
		t.Fatalf("tool name = %q", ct.Name())
	}
	if childRunner == nil || childRunner.Depth() != 1 {
		t.Fatalf("child depth = %v", childRunner)
	}

	if _, err = ct.Call(context.Background(), `{"query":"solve"}`); err != nil {
		t.Fatal(err)
	}
	if childMAF.Name() != "MathTutor" || childMAF.ID() != "child-math" {
		t.Fatalf("child identity name=%s id=%s", childMAF.Name(), childMAF.ID())
	}

	// HARD assert: child instructions reached provider via MAF options.
	optsVal := childProv.LastOptions.Load()
	if optsVal == nil {
		t.Fatal("child provider never saw options")
	}
	opts := optsVal.([]mafagent.Option)
	instr, ok := mafagent.GetOption(opts, mafagent.WithInstructions)
	if !ok || instr != childInstr {
		t.Fatalf("child instructions = %q ok=%v, want %q", instr, ok, childInstr)
	}
	if strings.Contains(instr, "OVERSEER") || strings.Contains(instr, parentInstr) {
		t.Fatalf("parent instructions leaked to child: %q", instr)
	}

	// HARD assert: child tool set is only shared_calc (not parent_only).
	var toolNames []string
	for tl := range mafagent.AllOptions(opts, mafagent.WithTool) {
		if tl != nil {
			toolNames = append(toolNames, tl.Name())
		}
	}
	foundShared, foundParentOnly := false, false
	for _, n := range toolNames {
		switch n {
		case "shared_calc":
			foundShared = true
		case "parent_only":
			foundParentOnly = true
		}
	}
	if foundParentOnly {
		t.Fatalf("child saw parent_only tool: %v", toolNames)
	}
	if !foundShared {
		t.Fatalf("child missing shared_calc on run options: %v", toolNames)
	}

	ev := sink.Snapshot()
	if !hasKind(ev, agentruntime.KindChildStart) {
		t.Fatalf("events: %+v", ev)
	}
	for _, e := range ev {
		if e.Kind == agentruntime.KindChildStart {
			if e.ParentRunID != parentRunID {
				t.Fatalf("child parent_run_id=%q want %q", e.ParentRunID, parentRunID)
			}
			if e.RootRunID != parentRunID {
				t.Fatalf("child root_run_id=%q want %q", e.RootRunID, parentRunID)
			}
			if e.AgentID != "child-math" {
				t.Fatalf("child agent_id=%q", e.AgentID)
			}
			if e.RunID == "" || e.RunID == e.AgentID {
				t.Fatalf("child run_id must be fresh, distinct from agent id; run=%q agent=%q", e.RunID, e.AgentID)
			}
			if e.Depth != 1 {
				t.Fatalf("child depth=%d", e.Depth)
			}
		}
	}
}

func TestF1_UntrustedMaxChildrenZero(t *testing.T) {
	parent := scriptedAgent(t, "Student", "s1", textUpdate("hi"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		Type:        "student",
		MaxChildren: 0,
		MaxDepth:    3,
	}, parent, agentruntime.NoopSink{})
	child := scriptedAgent(t, "X", "c1", textUpdate("x"))
	_, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child)
	if err == nil || !strings.Contains(err.Error(), "max_children=0") {
		t.Fatalf("err = %v", err)
	}
}

func TestF1_ChildBudgetExhausted(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren:      1,
		MaxDepth:         3,
		MaxTotalChildren: 1,
	}, parent, agentruntime.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	ct, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ct.Call(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child); err == nil {
		t.Fatal("expected budget error")
	}
	d, total, active := r.ChildBudgetSnapshot()
	if d != 1 || total != 1 {
		t.Fatalf("direct=%d total=%d", d, total)
	}
	if active != 0 {
		t.Fatalf("active=%d after completion, want 0", active)
	}
}

func TestF1_EmptyAllowlistRejectsTools(t *testing.T) {
	shared := mustFuncTool(t, "shared_calc")
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren: 1,
		MaxDepth:    1,
		Tools:       []maftool.Tool{shared},
	}, parent, agentruntime.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))

	// Empty AllowedTools with non-empty Tools should be rejected.
	_, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{
		AllowedTools: nil,
		Tools:        []maftool.Tool{shared},
	}, child)
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("err = %v, want allowlist rejection", err)
	}
	// Budget must not be consumed on failure.
	d, total, active := r.ChildBudgetSnapshot()
	if d != 0 || total != 0 || active != 0 {
		t.Fatalf("budget after failed allowlist: direct=%d total=%d active=%d", d, total, active)
	}
	// Empty AllowedTools + empty Tools is OK.
	ct, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{
		AllowedTools: nil,
		Tools:        nil,
	}, child)
	if err != nil {
		t.Fatalf("empty tools should be allowed: %v", err)
	}
	_, _ = ct.Call(context.Background(), `{"query":"x"}`)
}

func TestF1_MaxDepthDeniesGrandchild(t *testing.T) {
	parent := scriptedAgent(t, "O", "p-agent", textUpdate("p"))
	sink := &agentruntime.CollectingSink{}
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		Type:             "overseer",
		Name:             "Overseer",
		MaxChildren:      2,
		MaxDepth:         1,
		MaxTotalChildren: 10,
	}, parent, sink)
	if err := r.Run(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	if r.Depth() != 0 {
		t.Fatalf("root depth=%d", r.Depth())
	}

	child := scriptedAgent(t, "Child", "c-agent", textUpdate("c"))
	ct, childRunner, err := r.StartChild(context.Background(), agentruntime.ChildSpec{
		Type:        "child",
		Name:        "Child",
		MaxChildren: 2,
	}, child)
	if err != nil {
		t.Fatalf("StartChild child: %v", err)
	}
	if childRunner.Depth() != 1 {
		t.Fatalf("child depth=%d", childRunner.Depth())
	}

	// Grandchild denied by depth (orchestrator-controlled, not caller-supplied).
	gc := scriptedAgent(t, "GC", "gc-agent", textUpdate("g"))
	_, _, err = childRunner.StartChild(context.Background(), agentruntime.ChildSpec{
		Type: "gc",
		Name: "GC",
	}, gc)
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("grandchild err = %v, want max depth", err)
	}

	if _, err := ct.Call(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatal(err)
	}
	_, _, active := r.ChildBudgetSnapshot()
	if active != 0 {
		t.Fatalf("active=%d after child complete", active)
	}
}

func TestF1_MaxDepthZeroDeniesAllChildren(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren: 5,
		MaxDepth:    0,
	}, parent, agentruntime.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	_, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child)
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("err=%v", err)
	}
}

func TestF1_ActiveChildrenClosesOnErrorAndCancel(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren:      3,
		MaxDepth:         2,
		MaxTotalChildren: 10,
	}, parent, agentruntime.NoopSink{})

	// Error path.
	errProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				yield(nil, errors.New("child boom"))
			}
		},
	}
	errChild := agentruntime.NewScriptedAgent(mafagent.Config{ID: "e", Name: "E"}, errProv)
	ct, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, errChild)
	if err != nil {
		t.Fatal(err)
	}
	if _, callErr := ct.Call(context.Background(), `{"query":"x"}`); callErr == nil {
		t.Fatal("expected child error")
	}
	_, _, active := r.ChildBudgetSnapshot()
	if active != 0 {
		t.Fatalf("active after error=%d", active)
	}

	// Cancel path.
	block := make(chan struct{})
	cancelProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				select {
				case <-ctx.Done():
					yield(nil, ctx.Err())
				case <-block:
					yield(textUpdate("late"), nil)
				}
			}
		},
	}
	cChild := agentruntime.NewScriptedAgent(mafagent.Config{ID: "c", Name: "C"}, cancelProv)
	ct2, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, cChild)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ct2.Call(ctx, `{"query":"x"}`)
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for cancelProv.Started.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancel error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	close(block)
	_, _, active = r.ChildBudgetSnapshot()
	if active != 0 {
		t.Fatalf("active after cancel=%d", active)
	}
}

func TestF1_RunIDsDistinctAcrossInvocations(t *testing.T) {
	parent := scriptedAgent(t, "O", "stable-agent-id", textUpdate("a"), textUpdate("b"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{Name: "O", MaxChildren: 0}, parent, &agentruntime.CollectingSink{})
	if err := r.Run(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	id1 := r.LastRunID()
	if err := r.Run(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	id2 := r.LastRunID()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("run ids must be distinct: %q %q", id1, id2)
	}
	if id1 == parent.ID() || id2 == parent.ID() {
		t.Fatalf("run ids must not reuse agent id %q: %q %q", parent.ID(), id1, id2)
	}
	if parent.ID() != "stable-agent-id" {
		t.Fatal("agent id changed")
	}
}

func TestF1_PrepareRun_StartChildBeforeRun_LineageCoherent(t *testing.T) {
	sink := &agentruntime.CollectingSink{}
	parent := scriptedAgent(t, "O", "p-agent", textUpdate("parent-text"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		Name: "O", MaxChildren: 1, MaxDepth: 1, MaxTotalChildren: 5,
	}, parent, sink)
	prepared := r.PrepareRun()
	if prepared == "" || prepared == parent.ID() {
		t.Fatalf("prepared=%q agent=%q", prepared, parent.ID())
	}
	child := scriptedAgent(t, "C", "c-agent", textUpdate("child-text"))
	ct, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{Type: "c", Name: "C"}, child)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ct.Call(context.Background(), `{"query":"q"}`); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if r.LastRunID() != prepared {
		t.Fatalf("Run id=%q want prepared %q", r.LastRunID(), prepared)
	}
	evs := sink.Snapshot()
	var parentStart, childStart *agentruntime.RunEvent
	for i := range evs {
		e := evs[i]
		if e.Kind == agentruntime.KindStart && parentStart == nil {
			cp := e
			parentStart = &cp
		}
		if e.Kind == agentruntime.KindChildStart && childStart == nil {
			cp := e
			childStart = &cp
		}
	}
	if parentStart == nil || childStart == nil {
		t.Fatalf("missing events: %+v", evs)
	}
	if parentStart.RunID != prepared || parentStart.RootRunID != prepared {
		t.Fatalf("parent start run=%q root=%q want %q", parentStart.RunID, parentStart.RootRunID, prepared)
	}
	if childStart.ParentRunID != prepared || childStart.RootRunID != prepared {
		t.Fatalf("child parent=%q root=%q want %q", childStart.ParentRunID, childStart.RootRunID, prepared)
	}
}

func TestF1_MaxTotalChildrenIndependentOfDirect(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren:      5,
		MaxDepth:         3,
		MaxTotalChildren: 1,
	}, parent, agentruntime.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	ct, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ct.Call(context.Background(), `{"query":"x"}`)
	if _, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child); err == nil || !strings.Contains(err.Error(), "total child budget") {
		t.Fatalf("err=%v, want total child budget", err)
	}
}

// ---- F2: MCP fail-closed filter ---------------------------------------------

func TestF2_MCP_FailClosedFilter(t *testing.T) {
	env := testfakes.StartMCP(t)
	all := env.AllTools(t)
	names := agentruntime.ToolNames(all)
	if len(names) < 2 {
		t.Fatalf("expected >=2 mcp tools, got %v", names)
	}
	childTools := agentruntime.FilterToolsFailClosed(all, []string{"allow_me"})
	if got := agentruntime.ToolNames(childTools); len(got) != 1 || got[0] != "allow_me" {
		t.Fatalf("filtered = %v", got)
	}
	ft, ok := childTools[0].(maftool.FuncTool)
	if !ok {
		t.Fatalf("type %T", childTools[0])
	}
	if _, err := ft.Call(context.Background(), `{"q":"hi"}`); err != nil {
		t.Fatal(err)
	}
	if env.AllowCalls.Load() != 1 {
		t.Fatalf("allow calls = %d", env.AllowCalls.Load())
	}
	for _, n := range agentruntime.ToolNames(childTools) {
		if n == "deny_me" {
			t.Fatal("deny_me present on child")
		}
	}
}

func TestF2_MCP_DeniedToolNotInvokedThroughChild(t *testing.T) {
	env := testfakes.StartMCP(t)
	all := env.AllTools(t)
	childTools := agentruntime.FilterToolsFailClosed(all, []string{"allow_me"})
	invokable := map[string]maftool.FuncTool{}
	for _, tl := range childTools {
		if ft, ok := tl.(maftool.FuncTool); ok {
			invokable[ft.Name()] = ft
		}
	}
	if _, ok := invokable["deny_me"]; ok {
		t.Fatal("deny_me invokable after filter")
	}
	if _, ok := invokable["allow_me"]; !ok {
		t.Fatal("allow_me missing")
	}
	// Baseline: deny_me is reachable directly.
	for _, tl := range all {
		if tl.Name() == "deny_me" {
			_, _ = tl.(maftool.FuncTool).Call(context.Background(), `{}`)
		}
	}
	if env.DenyCalls.Load() != 1 {
		t.Fatalf("deny baseline calls=%d", env.DenyCalls.Load())
	}
	before := env.DenyCalls.Load()
	for _, ft := range invokable {
		_, _ = ft.Call(context.Background(), `{}`)
	}
	if env.DenyCalls.Load() != before {
		t.Fatal("deny invoked through child surface")
	}
}

// ---- F3: streaming child adapter surfacing attributed events ----------------

func TestF3_StreamingChildAdapter_SurfacesAttributedEvents(t *testing.T) {
	childProv := &agentruntime.ScriptedProvider{
		Updates: []*mafagent.ResponseUpdate{
			textUpdate("alpha"),
			textUpdate("beta"),
		},
	}
	child := agentruntime.NewScriptedAgent(mafagent.Config{
		ID:          "child-42",
		Name:        "MathTutor",
		Description: "math",
	}, childProv)

	sink := &agentruntime.CollectingSink{}
	st := agentruntime.NewStreamingChildTool(child, "math_tutor", "parent-run-1", sink)
	res, err := st.Call(context.Background(), `{"query":"2+2"}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res != "alphabeta" {
		t.Fatalf("result = %q, want alphabeta", res)
	}
	ev := sink.Snapshot()
	if !hasKind(ev, agentruntime.KindChildStart) {
		t.Fatalf("missing child_start: %+v", ev)
	}
	if !hasKind(ev, agentruntime.KindChildEnd) {
		t.Fatalf("missing child_end: %+v", ev)
	}
	texts := textsOf(ev, agentruntime.KindText)
	if len(texts) < 2 || texts[0] != "alpha" || texts[1] != "beta" {
		t.Fatalf("text events = %v, want alpha,beta", texts)
	}
	for _, e := range ev {
		if e.Kind == agentruntime.KindText {
			if e.ParentRunID != "parent-run-1" {
				t.Fatalf("parent_run_id = %q", e.ParentRunID)
			}
			if e.AgentType != "math_tutor" || e.AgentName != "MathTutor" {
				t.Fatalf("attribution = %+v", e)
			}
		}
	}
}

// ---- Phase 2: concurrent runs, cancellation, controller --------------------

func TestPhase2_ConcurrentPrepareRun_IsolatedIDs(t *testing.T) {
	const n = 50
	parent := scriptedAgent(t, "O", "p-concurrent", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{Name: "O", MaxChildren: 0}, parent, agentruntime.NoopSink{})

	ids := make([]string, n)
	var wg sync.WaitGroup
	// PrepareRun is idempotent on the SAME runner; use separate runners for concurrency.
	runners := make([]*agentruntime.Runner, n)
	for i := 0; i < n; i++ {
		runners[i] = agentruntime.NewRunner(agentruntime.AgentSpec{Name: "O", MaxChildren: 0},
			scriptedAgent(t, "O", fmt.Sprintf("p-%d", i), textUpdate("x")), agentruntime.NoopSink{})
	}
	_ = r // keep root runner alive
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = runners[idx].PrepareRun()
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			t.Fatal("empty run id")
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestPhase2_CancellationReachesNestedMCP(t *testing.T) {
	env := testfakes.StartMCP(t)
	env.BlockAllow = 5 * time.Second
	all := env.AllTools(t)
	allow := agentruntime.FilterToolsFailClosed(all, []string{"allow_me"})[0].(maftool.FuncTool)

	childProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				_, err := allow.Call(ctx, `{"q":"x"}`)
				if err != nil {
					yield(nil, err)
					return
				}
				yield(textUpdate("should-not"), nil)
			}
		},
	}
	child := agentruntime.NewScriptedAgent(mafagent.Config{ID: "c", Name: "C"}, childProv)
	sink := &agentruntime.CollectingSink{}
	st := agentruntime.NewStreamingChildTool(child, "c", "p", sink)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := st.Call(ctx, `{"query":"x"}`)
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for env.AllowCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if env.AllowCalls.Load() == 0 {
		t.Fatal("MCP allow_me never entered before cancel")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancel error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.AllowSawCancel.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("AllowSawCancel=false; MCP did not observe ctx cancel")
}

func TestPhase2_SubscriberLoss_RunStillCompletes(t *testing.T) {
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	sink := agentruntime.NewBoundedSink(1)
	st := agentruntime.NewStreamingChildTool(child, "c", "p", sink)
	res, err := st.Call(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res != "x" {
		t.Fatalf("%q", res)
	}
}

func TestPhase2_SlowSinkDoesNotBlockRun(t *testing.T) {
	sink := agentruntime.NewBoundedSink(2)
	sink.BlockFor = time.Hour
	child := scriptedAgent(t, "C", "c",
		textUpdate("a"), textUpdate("b"), textUpdate("c"), textUpdate("d"))
	st := agentruntime.NewStreamingChildTool(child, "c", "p", sink)
	start := time.Now()
	res, err := st.Call(context.Background(), `{"query":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("run blocked too long: %v", time.Since(start))
	}
	if res != "abcd" {
		t.Fatalf("res=%q", res)
	}
}

// ---- Phase 2: unused child reservation reclamation and multi-turn root policy --

func TestPhase2_UnusedChildReservation_Reclaimed(t *testing.T) {
	// Phase 2 BDD: a child prepared but never invoked must release
	// direct/total/active budget so a later child can use the reclaimed capacity.
	parent := scriptedAgent(t, "O", "p-unreserved", textUpdate("x"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{
		MaxChildren:      1,
		MaxDepth:         1,
		MaxTotalChildren: 1,
	}, parent, agentruntime.NoopSink{})

	child := scriptedAgent(t, "C", "c-unreserved", textUpdate("x"))
	st, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child)
	if err != nil {
		t.Fatal(err)
	}
	ct := st.(*agentruntime.StreamingChildTool)

	// Budget fully reserved after StartChild.
	d, total, active := r.ChildBudgetSnapshot()
	if d != 1 || total != 1 || active != 1 {
		t.Fatalf("after StartChild: direct=%d total=%d active=%d; want 1/1/1", d, total, active)
	}

	// Release without calling: must reclaim all three counters.
	if !ct.Release() {
		t.Fatal("Release should return true when Call has not been invoked")
	}
	d, total, active = r.ChildBudgetSnapshot()
	if d != 0 || total != 0 || active != 0 {
		t.Fatalf("after Release: direct=%d total=%d active=%d; want 0/0/0", d, total, active)
	}

	// Release is idempotent: second call returns false.
	if ct.Release() {
		t.Fatal("second Release must return false")
	}

	// Call on a released tool must return an error immediately.
	_, callErr := ct.Call(context.Background(), `{"query":"x"}`)
	if callErr == nil || !strings.Contains(callErr.Error(), "released") {
		t.Fatalf("call on released tool: err=%v, want 'released' error", callErr)
	}

	// Reclaimed capacity lets a replacement child be admitted.
	child2 := scriptedAgent(t, "C2", "c2-unreserved", textUpdate("x"))
	ct2, _, err := r.StartChild(context.Background(), agentruntime.ChildSpec{}, child2)
	if err != nil {
		t.Fatalf("replacement child denied after Release: %v", err)
	}
	if _, err := ct2.Call(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatalf("replacement child call: %v", err)
	}
}

func TestPhase2_MultiTurnRootPolicy_StickyAcrossTurns(t *testing.T) {
	// Phase 2 BDD: a runner reused across sequential turns keeps the root run
	// id from the first turn (sticky root). Run ids must be distinct.
	sink := &agentruntime.CollectingSink{}
	parent := scriptedAgent(t, "O", "p-multirun", textUpdate("done"))
	r := agentruntime.NewRunner(agentruntime.AgentSpec{Name: "O", MaxChildren: 0}, parent, sink)

	if err := r.Run(context.Background(), "turn-one"); err != nil {
		t.Fatal(err)
	}
	id1 := r.LastRunID()

	if err := r.Run(context.Background(), "turn-two"); err != nil {
		t.Fatal(err)
	}
	id2 := r.LastRunID()

	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("run ids must be distinct non-empty: %q %q", id1, id2)
	}

	// Find the two KindStart events in order.
	evs := sink.Snapshot()
	var starts []agentruntime.RunEvent
	for _, e := range evs {
		if e.Kind == agentruntime.KindStart {
			starts = append(starts, e)
		}
	}
	if len(starts) < 2 {
		t.Fatalf("expected >=2 start events, got %d: %+v", len(starts), evs)
	}
	turn1, turn2 := starts[0], starts[1]

	if turn1.RunID != id1 {
		t.Fatalf("turn1 run_id=%q want %q", turn1.RunID, id1)
	}
	if turn2.RunID != id2 {
		t.Fatalf("turn2 run_id=%q want %q", turn2.RunID, id2)
	}
	// Sticky root: first turn is the root; second turn must share it.
	if turn1.RootRunID != id1 {
		t.Fatalf("turn1 root_run_id=%q want %q (first run is its own root)", turn1.RootRunID, id1)
	}
	if turn2.RootRunID != id1 {
		t.Fatalf("turn2 root_run_id=%q want %q (sticky root from first turn)", turn2.RootRunID, id1)
	}
}

// ---- Phase 3: SSE bridge race-safety and bounded behaviour -----------------

func TestPhase3_SSEBridge_SlowWriterDoesNotBlockRunner(t *testing.T) {
	parentProv := &agentruntime.ScriptedProvider{
		Updates: []*mafagent.ResponseUpdate{
			textUpdate("early-event"),
			textUpdate("mid-event"),
			textUpdate("late-event"),
		},
	}
	parent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "p", Name: "P"}, parentProv)

	bw := newBlockingWriter(1) // block on first Write
	bridge := agentruntime.NewSSEBridge(4)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		bridge.WriteTo(context.Background(), bw, bw)
	}()

	var runFinished atomic.Bool
	r := agentruntime.NewRunner(agentruntime.AgentSpec{Type: "t", Name: "P"}, parent, eventFuncSink(func(ctx context.Context, e agentruntime.RunEvent) {
		bridge.Emit(ctx, e)
	}))

	runDone := make(chan error, 1)
	go func() {
		err := r.Run(context.Background(), "hi")
		runFinished.Store(true)
		runDone <- err
	}()

	select {
	case <-bw.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("writer never entered blocking write")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("run err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner blocked behind slow writer")
	}
	if !runFinished.Load() {
		t.Fatal("run not finished")
	}
	bw.release()
	bridge.Close()
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("writer goroutine leak")
	}
}

func TestPhase3_SSEBridge_CancelTerminatesWriter(t *testing.T) {
	bridge := agentruntime.NewSSEBridge(8)
	ctx, cancel := context.WithCancel(context.Background())
	bw := newBlockingWriter(9999) // never actually blocks

	done := make(chan struct{})
	go func() {
		defer close(done)
		bridge.WriteTo(ctx, bw, bw)
	}()
	bridge.Emit(context.Background(), agentruntime.RunEvent{Kind: agentruntime.KindText, Text: "x", RunID: "r"})
	for i := 0; i < 4; i++ {
		bridge.Emit(context.Background(), agentruntime.RunEvent{Kind: agentruntime.KindText, Text: fmt.Sprintf("q%d", i), RunID: "r"})
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not terminate on stream cancel")
	}
	bridge.Close()
	select {
	case <-bridge.WriteDone():
	case <-time.After(time.Second):
		t.Fatal("WriteDone not closed")
	}
}

func TestPhase3_SSEBridge_ConcurrentEmitClose_NoPanic(t *testing.T) {
	const (
		rounds     = 200
		emitters   = 8
		eventsEach = 50
	)
	var wg sync.WaitGroup
	for round := 0; round < rounds; round++ {
		bridge := agentruntime.NewSSEBridge(4)
		drainDone := make(chan struct{})
		go func(b *agentruntime.SSEBridge) {
			defer close(drainDone)
			bw := newBlockingWriter(9999)
			b.WriteTo(context.Background(), bw, bw)
		}(bridge)

		wg.Add(emitters + 1)
		go func(b *agentruntime.SSEBridge) {
			defer wg.Done()
			time.Sleep(time.Duration(round%3) * time.Microsecond)
			b.Close()
			b.Close() // double-close must be safe
		}(bridge)
		for e := 0; e < emitters; e++ {
			go func(b *agentruntime.SSEBridge, id int) {
				defer wg.Done()
				for i := 0; i < eventsEach; i++ {
					b.Emit(context.Background(), agentruntime.RunEvent{
						Kind: agentruntime.KindText, Text: fmt.Sprintf("e%d-%d", id, i), RunID: "race",
					})
				}
			}(bridge, e)
		}
		wg.Wait()
		bridge.Close()
		select {
		case <-drainDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: writer/drain deadlock", round)
		}
		select {
		case <-bridge.WriteDone():
		case <-time.After(time.Second):
			t.Fatalf("round %d: WriteDone not closed", round)
		}
	}
}

// ---- Phase 3: SSE handler two-context proofs --------------------------------

func TestPhase3_SSEHandler_HTTPDisconnect_RunCompletes(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	hold := make(chan struct{})
	var runSawCancel atomic.Bool
	var producedAfter atomic.Bool
	runEnded := make(chan error, 1)
	var runHandle atomic.Pointer[agentruntime.RunHandle]

	parentProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				if !yield(textUpdate("first-wire"), nil) {
					return
				}
				startedOnce.Do(func() { close(started) })
				select {
				case <-hold:
				case <-ctx.Done():
					runSawCancel.Store(true)
					yield(nil, ctx.Err())
					return
				}
				producedAfter.Store(true)
				yield(textUpdate("after-disconnect"), nil)
			}
		},
	}
	parent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "p", Name: "P"}, parentProv)
	h := &agentruntime.SSEHandler{
		Agent:          parent,
		AgentType:      "overseer",
		BridgeCapacity: 8,
		RunBudget:      10 * time.Second,
		OnRunStart:     func(rh *agentruntime.RunHandle) { runHandle.Store(rh) },
		OnRunEnd:       func(err error) { runEnded <- err },
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String(),
		strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(resp.Body)

	var gotFirst atomic.Bool
	readErr := make(chan error, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readErr <- err
				return
			}
			if strings.Contains(line, "first-wire") {
				gotFirst.Store(true)
			}
		}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider never started")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !gotFirst.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if !gotFirst.Load() {
		t.Fatal("first-wire never observed before disconnect")
	}

	_ = resp.Body.Close()

	select {
	case <-readErr:
	case <-time.After(2 * time.Second):
		t.Fatal("stream reader did not exit after disconnect (writer leak?)")
	}

	rh := runHandle.Load()
	if rh == nil {
		t.Fatal("OnRunStart never fired")
	}
	if rh.Err() != nil {
		t.Fatalf("run handle canceled by disconnect: %v", rh.Err())
	}

	close(hold)
	select {
	case err := <-runEnded:
		if err != nil {
			t.Fatalf("run ended with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not complete after disconnect")
	}

	if runSawCancel.Load() {
		t.Fatal("run context was canceled by client disconnect — contexts conflated")
	}
	if !producedAfter.Load() {
		t.Fatal("provider did not produce after-disconnect token")
	}
}

func TestPhase3_SSEHandler_ExplicitRunCancel_ReachesBlockingMCP(t *testing.T) {
	env := testfakes.StartMCP(t)
	env.BlockAllow = 5 * time.Second
	all := env.AllTools(t)
	allow := agentruntime.FilterToolsFailClosed(all, []string{"allow_me"})[0].(maftool.FuncTool)

	childProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				_, err := allow.Call(ctx, `{"q":"x"}`)
				if err != nil {
					yield(nil, err)
					return
				}
				yield(textUpdate("should-not"), nil)
			}
		},
	}
	child := agentruntime.NewScriptedAgent(mafagent.Config{ID: "c", Name: "C"}, childProv)

	parentProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				for tl := range mafagent.AllOptions(options, mafagent.WithTool) {
					if ft, ok := tl.(maftool.FuncTool); ok {
						if _, err := ft.Call(ctx, `{"query":"nested"}`); err != nil {
							yield(nil, err)
							return
						}
					}
				}
				yield(textUpdate("parent-should-not"), nil)
			}
		},
	}
	parent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "p", Name: "P"}, parentProv)

	var handle atomic.Pointer[agentruntime.RunHandle]
	runEnded := make(chan error, 1)
	h := &agentruntime.SSEHandler{
		Agent:     parent,
		AgentType: "overseer",
		Spec: agentruntime.AgentSpec{
			Type:        "overseer",
			Name:        "P",
			Tools:       []maftool.Tool{allow},
			MaxChildren: 1,
			MaxDepth:    1,
		},
		BridgeCapacity: 8,
		RunBudget:      10 * time.Second,
		BuildChildTool: func(ctx context.Context, parentRunner *agentruntime.Runner, sink agentruntime.EventSink) maftool.FuncTool {
			ct, _, err := parentRunner.StartChild(ctx, agentruntime.ChildSpec{
				Type:         "c",
				Name:         "C",
				AllowedTools: []string{"allow_me"},
				Tools:        []maftool.Tool{allow},
			}, child)
			if err != nil {
				t.Errorf("StartChild: %v", err)
				return nil
			}
			return ct
		},
		OnRunStart: func(rh *agentruntime.RunHandle) { handle.Store(rh) },
		OnRunEnd:   func(err error) { runEnded <- err },
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String(),
		strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for env.AllowCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if env.AllowCalls.Load() == 0 {
		t.Fatal("MCP allow_me never entered")
	}

	rh := handle.Load()
	if rh == nil {
		t.Fatal("run handle missing")
	}
	rh.Cancel()

	select {
	case err := <-runEnded:
		if err == nil {
			t.Fatal("expected run error after explicit cancel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not end after explicit cancel")
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.AllowSawCancel.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("AllowSawCancel=false after RunHandle.Cancel")
}

// ---- Phase 3: SSE wire attribution ------------------------------------------

func TestPhase3_PrimerSSE_ParentAndChildAttributed(t *testing.T) {
	child := scriptedAgent(t, "MathTutor", "child-sse", textUpdate("c1"), textUpdate("c2"))
	parentProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				for tl := range mafagent.AllOptions(options, mafagent.WithTool) {
					if ft, ok := tl.(maftool.FuncTool); ok && strings.Contains(ft.Name(), "MathTutor") {
						if _, err := ft.Call(ctx, `{"query":"nested"}`); err != nil {
							yield(nil, err)
							return
						}
					}
				}
				yield(textUpdate("parent-tail"), nil)
			}
		},
	}
	parent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "parent-sse", Name: "Overseer"}, parentProv)
	h := &agentruntime.SSEHandler{
		Agent:     parent,
		AgentType: "overseer",
		Spec: agentruntime.AgentSpec{
			Type:        "overseer",
			Name:        "Overseer",
			MaxChildren: 1,
			MaxDepth:    1,
		},
		BuildChildTool: func(ctx context.Context, parentRunner *agentruntime.Runner, sink agentruntime.EventSink) maftool.FuncTool {
			ct, _, err := parentRunner.StartChild(ctx, agentruntime.ChildSpec{
				Type: "math_tutor",
				Name: "MathTutor",
			}, child)
			if err != nil {
				t.Errorf("StartChild: %v", err)
				return nil
			}
			return ct
		},
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "parent-tail") {
		t.Fatalf("missing parent text: %s", s)
	}
	if !strings.Contains(s, "c1") || !strings.Contains(s, "c2") {
		t.Fatalf("missing child text: %s", s)
	}
	if !strings.Contains(s, "child_start") {
		t.Fatalf("missing child_start: %s", s)
	}

	// Parse SSE and hard-assert lineage coherence.
	var parentStart, childStart *agentruntime.RunEvent
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev agentruntime.RunEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		switch {
		case ev.Kind == agentruntime.KindStart && parentStart == nil:
			e := ev
			parentStart = &e
		case ev.Kind == agentruntime.KindChildStart && childStart == nil:
			e := ev
			childStart = &e
		}
	}
	if parentStart == nil {
		t.Fatalf("missing parent start event")
	}
	if childStart == nil {
		t.Fatalf("missing child_start event")
	}
	if parentStart.RunID == "" || parentStart.RunID != parentStart.RootRunID {
		t.Fatalf("parent run_id=%q root_run_id=%q want equal non-empty", parentStart.RunID, parentStart.RootRunID)
	}
	if childStart.ParentRunID != parentStart.RunID {
		t.Fatalf("child parent_run_id=%q want %q", childStart.ParentRunID, parentStart.RunID)
	}
	if childStart.RootRunID != parentStart.RootRunID {
		t.Fatalf("child root_run_id=%q want %q", childStart.RootRunID, parentStart.RootRunID)
	}
}

func TestPhase3_PrimerSSE_IncrementalBeforeCompletion(t *testing.T) {
	releaseProvider := make(chan struct{})
	earlyOnWire := make(chan struct{})
	var earlyOnce sync.Once

	parentProv := &agentruntime.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
			return func(yield func(*mafagent.ResponseUpdate, error) bool) {
				if !yield(textUpdate("early-parent-delta"), nil) {
					return
				}
				select {
				case <-releaseProvider:
				case <-ctx.Done():
					yield(nil, ctx.Err())
					return
				}
				yield(textUpdate("final-parent-delta"), nil)
			}
		},
	}
	parent := agentruntime.NewScriptedAgent(mafagent.Config{ID: "inc-p", Name: "Overseer"}, parentProv)
	h := &agentruntime.SSEHandler{Agent: parent, AgentType: "overseer", BridgeCapacity: 16}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String(),
		strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%s", ct)
	}

	reader := bufio.NewReader(resp.Body)
	foundEarly := false
	readDone := make(chan error, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readDone <- err
				return
			}
			if strings.Contains(line, "early-parent-delta") {
				foundEarly = true
				earlyOnce.Do(func() { close(earlyOnWire) })
			}
			if strings.Contains(line, "final-parent-delta") {
				readDone <- nil
				return
			}
		}
	}()

	select {
	case <-earlyOnWire:
	case <-time.After(3 * time.Second):
		t.Fatal("early event not observed before provider completion")
	}
	if !foundEarly {
		t.Fatal("early flag false")
	}
	close(releaseProvider)
	select {
	case err := <-readDone:
		if err != nil && !errors.Is(err, io.EOF) && !foundEarly {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for stream end")
	}
}
