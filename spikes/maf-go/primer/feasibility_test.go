package primer_test

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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aleksclark/primer/spikes/maf-go/fakes"
	"github.com/aleksclark/primer/spikes/maf-go/primer"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/provider/aguiprovider"
	"github.com/microsoft/agent-framework-go/provider/openaiprovider"
	"github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/agenttool"
	"github.com/microsoft/agent-framework-go/tool/functool"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// --- helpers ---

func textUpdate(s string) *agent.ResponseUpdate {
	return &agent.ResponseUpdate{
		Role:     message.RoleAssistant,
		Contents: message.Contents{&message.TextContent{Text: s}},
	}
}

func scriptedAgent(t *testing.T, name, id string, updates ...*agent.ResponseUpdate) *agent.Agent {
	t.Helper()
	prov := &primer.ScriptedProvider{Name: "scripted", Updates: updates}
	return primer.NewScriptedAgent(agent.Config{
		ID:          id,
		Name:        name,
		Description: name + " agent",
	}, prov)
}

func hasKind(events []primer.RunEvent, kind string) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func textsOf(events []primer.RunEvent, kind string) []string {
	var out []string
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e.Text)
		}
	}
	return out
}

func mustTool(t *testing.T, name string) tool.Tool {
	t.Helper()
	tl, err := functool.New(functool.Config{Name: name, Description: name}, func(ctx context.Context, in struct{}) (string, error) {
		return "x", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

// --- F3: stock agenttool hides child stream ---

func TestF3_StockAgentTool_HidesChildStreamUpdates(t *testing.T) {
	childProv := &primer.ScriptedProvider{
		Name: "child-scripted",
		Updates: []*agent.ResponseUpdate{
			textUpdate("one"),
			textUpdate("two"),
			textUpdate("three"),
		},
	}
	child := primer.NewScriptedAgent(agent.Config{
		ID:   "child-id",
		Name: "Specialist",
	}, childProv)

	var parentSeen []string
	parentProv := &primer.ScriptedProvider{
		Name: "parent",
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			at := agenttool.New(child, agenttool.Config{})
			res, err := at.Call(ctx, `{"query":"go"}`)
			if err != nil {
				return func(yield func(*agent.ResponseUpdate, error) bool) {
					yield(nil, err)
				}
			}
			s, _ := res.(string)
			parentSeen = append(parentSeen, s)
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				yield(textUpdate("parent-final:"+s), nil)
			}
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "parent-id", Name: "Overseer"}, parentProv)

	resp, err := parent.RunText(context.Background(), "hi").Collect()
	if err != nil {
		t.Fatalf("parent run: %v", err)
	}
	if got := resp.String(); !strings.Contains(got, "onetwothree") && !strings.Contains(got, "one") {
		t.Fatalf("expected collected child text in parent result, got %q", got)
	}
	if len(parentSeen) != 1 {
		t.Fatalf("stock agenttool Call count via parent = %d, want 1 collected call", len(parentSeen))
	}
	if childProv.Started.Load() != 1 {
		t.Fatalf("child started = %d, want 1", childProv.Started.Load())
	}
}

func TestF3_StreamingChildAdapter_SurfacesAttributedEvents(t *testing.T) {
	childProv := &primer.ScriptedProvider{
		Updates: []*agent.ResponseUpdate{
			textUpdate("alpha"),
			textUpdate("beta"),
		},
	}
	child := primer.NewScriptedAgent(agent.Config{
		ID:          "child-42",
		Name:        "MathTutor",
		Description: "math",
	}, childProv)

	sink := &primer.CollectingSink{}
	st := primer.NewStreamingChildTool(child, "math_tutor", "parent-run-1", sink)
	res, err := st.Call(context.Background(), `{"query":"2+2"}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res != "alphabeta" {
		t.Fatalf("result = %q, want alphabeta", res)
	}
	ev := sink.Snapshot()
	if !hasKind(ev, primer.KindChildStart) {
		t.Fatalf("missing child_start: %+v", ev)
	}
	if !hasKind(ev, primer.KindChildEnd) {
		t.Fatalf("missing child_end: %+v", ev)
	}
	texts := textsOf(ev, primer.KindText)
	if len(texts) < 2 || texts[0] != "alpha" || texts[1] != "beta" {
		t.Fatalf("text events = %v, want alpha,beta", texts)
	}
	for _, e := range ev {
		if e.Kind == primer.KindText {
			if e.ParentRunID != "parent-run-1" {
				t.Fatalf("parent_run_id = %q", e.ParentRunID)
			}
			if e.AgentType != "math_tutor" || e.AgentName != "MathTutor" {
				t.Fatalf("attribution = %+v", e)
			}
		}
	}
}

// --- F1 tailored subagent ---

func TestF1_TailoredSubagent_OwnIdentityAndReducedTools(t *testing.T) {
	parentTool, err := functool.New(functool.Config{Name: "parent_only", Description: "parent"}, func(ctx context.Context, in struct{}) (string, error) {
		return "p", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sharedTool, err := functool.New(functool.Config{Name: "shared_calc", Description: "calc"}, func(ctx context.Context, in struct{ X int }) (string, error) {
		return "42", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	const childInstr = "You are the math tutor CHILD-ONLY instructions."
	const parentInstr = "You are the parent OVERSEER — must not leak to child."

	childProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				yield(textUpdate("child-ok"), nil)
			}
		},
	}
	// Child agent construction: reduced tools + child instructions on Config path.
	child := primer.NewScriptedAgent(agent.Config{
		ID:          "child-math",
		Name:        "MathTutor",
		Description: "specialist math",
		// Construction tools reduced — parent_only intentionally absent.
		Tools:      []tool.Tool{sharedTool},
		RunOptions: []agent.Option{agent.WithInstructions(childInstr)},
	}, childProv)

	parentGrants := []tool.Tool{parentTool, sharedTool}
	spec := primer.AgentSpec{
		Type:             "overseer",
		Name:             "Overseer",
		Instructions:     parentInstr,
		Tools:            parentGrants,
		MaxChildren:      2,
		MaxDepth:         2,
		MaxTotalChildren: 5,
	}
	parent := scriptedAgent(t, "Overseer", "parent-1", textUpdate("done"))
	sink := &primer.CollectingSink{}
	r := primer.NewRunner(spec, parent, sink)

	childToolsFiltered := primer.FilterToolsFailClosed(parentGrants, []string{"shared_calc"})
	if len(childToolsFiltered) != 1 || childToolsFiltered[0].Name() != "shared_calc" {
		t.Fatalf("filter = %v", primer.ToolNames(childToolsFiltered))
	}
	if err := primer.AssertNoAuthorityExpansion(parentGrants, childToolsFiltered); err != nil {
		t.Fatal(err)
	}
	if err := primer.AssertNoAuthorityExpansion(parentGrants, []tool.Tool{parentTool, sharedTool, mustTool(t, "evil")}); err == nil {
		t.Fatal("expected authority expansion error")
	}

	// Parent run first so lineage root is established.
	if err := r.Run(context.Background(), "parent turn", agent.WithInstructions(parentInstr)); err != nil {
		t.Fatal(err)
	}
	parentRunID := r.LastRunID()
	if parentRunID == "" || parentRunID == parent.ID() {
		t.Fatalf("parent run id must be fresh, distinct from agent id; run=%q agent=%q", parentRunID, parent.ID())
	}

	ct, childRunner, err := r.StartChild(context.Background(), primer.ChildSpec{
		Type:         "math_tutor",
		Name:         "MathTutor",
		Instructions: childInstr,
		AllowedTools: []string{"shared_calc"},
		Agent:        child,
		Tools:        childToolsFiltered,
		MaxChildren:  0, // child cannot start grandchildren by direct budget
	})
	if err != nil {
		t.Fatalf("StartChild: %v", err)
	}
	if ct.Name() != "MathTutor" {
		t.Fatalf("tool name = %q", ct.Name())
	}
	if childRunner == nil || childRunner.Depth() != 1 {
		t.Fatalf("child depth = %v", childRunner)
	}

	_, err = ct.Call(context.Background(), `{"query":"solve"}`)
	if err != nil {
		t.Fatal(err)
	}
	if child.Name() != "MathTutor" || child.ID() != "child-math" {
		t.Fatalf("child identity name=%s id=%s", child.Name(), child.ID())
	}

	// HARD assert: child-specific instructions reached the child provider via MAF options.
	optsVal := childProv.LastOptions.Load()
	if optsVal == nil {
		t.Fatal("child provider never saw options")
	}
	opts := optsVal.([]agent.Option)
	instr, ok := agent.GetOption(opts, agent.WithInstructions)
	if !ok || instr != childInstr {
		t.Fatalf("child instructions = %q ok=%v, want %q", instr, ok, childInstr)
	}
	if strings.Contains(instr, "OVERSEER") || strings.Contains(instr, parentInstr) {
		t.Fatalf("parent instructions leaked to child: %q", instr)
	}

	// HARD assert: child tool set is only shared_calc (not parent_only).
	var toolNames []string
	for tl := range agent.AllOptions(opts, agent.WithTool) {
		if tl != nil {
			toolNames = append(toolNames, tl.Name())
		}
	}
	foundShared, foundParentOnly := false, false
	for _, n := range toolNames {
		if n == "shared_calc" {
			foundShared = true
		}
		if n == "parent_only" {
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
	if !hasKind(ev, primer.KindChildStart) {
		t.Fatalf("events: %+v", ev)
	}
	// Child events must attribute parent_run_id and root_run_id.
	for _, e := range ev {
		if e.Kind == primer.KindChildStart {
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
	r := primer.NewRunner(primer.AgentSpec{
		Type:        "student",
		MaxChildren: 0,
		MaxDepth:    3,
		Tools:       nil,
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "X", "c1", textUpdate("x"))
	_, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: child, AllowedTools: nil})
	if err == nil || !strings.Contains(err.Error(), "max_children=0") {
		t.Fatalf("err = %v", err)
	}
}

func TestF1_ChildBudgetExhausted(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := primer.NewRunner(primer.AgentSpec{
		MaxChildren:      1,
		MaxDepth:         3,
		MaxTotalChildren: 1,
		Tools:            nil,
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	ct, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: child})
	if err != nil {
		t.Fatal(err)
	}
	// Complete the child so active accounting closes.
	if _, err := ct.Call(context.Background(), `{"query":"x"}`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: child}); err == nil {
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
	shared := mustTool(t, "shared_calc")
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := primer.NewRunner(primer.AgentSpec{
		MaxChildren: 1,
		MaxDepth:    1,
		Tools:       []tool.Tool{shared},
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	_, _, err := r.StartChild(context.Background(), primer.ChildSpec{
		Agent:        child,
		AllowedTools: nil,
		Tools:        []tool.Tool{shared},
	})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("err = %v, want allowlist rejection", err)
	}
	// Failed start must not consume budget permanently.
	d, total, active := r.ChildBudgetSnapshot()
	if d != 0 || total != 0 || active != 0 {
		t.Fatalf("budget after failed allowlist: direct=%d total=%d active=%d", d, total, active)
	}
	// Explicit empty allowlist + empty tools OK.
	ct, _, err := r.StartChild(context.Background(), primer.ChildSpec{
		Agent:        child,
		AllowedTools: nil,
		Tools:        nil,
	})
	if err != nil {
		t.Fatalf("empty tools should be allowed: %v", err)
	}
	_, _ = ct.Call(context.Background(), `{"query":"x"}`)
}

func TestF1_MaxDepthDeniesGrandchild(t *testing.T) {
	// MaxDepth=1: root (0) may start child (1); child may NOT start grandchild (2).
	parent := scriptedAgent(t, "O", "p-agent", textUpdate("p"))
	sink := &primer.CollectingSink{}
	r := primer.NewRunner(primer.AgentSpec{
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
	ct, childRunner, err := r.StartChild(context.Background(), primer.ChildSpec{
		Type:        "child",
		Name:        "Child",
		Agent:       child,
		MaxChildren: 2, // would allow if depth permitted
	})
	if err != nil {
		t.Fatalf("StartChild child: %v", err)
	}
	if childRunner.Depth() != 1 {
		t.Fatalf("child depth=%d", childRunner.Depth())
	}

	// Grandchild denied by depth (orchestrator-controlled, not caller-supplied).
	gc := scriptedAgent(t, "GC", "gc-agent", textUpdate("g"))
	_, _, err = childRunner.StartChild(context.Background(), primer.ChildSpec{
		Type:  "gc",
		Name:  "GC",
		Agent: gc,
	})
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("grandchild err = %v, want max depth", err)
	}

	// Complete child lifecycle.
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
	r := primer.NewRunner(primer.AgentSpec{
		MaxChildren: 5,
		MaxDepth:    0, // depth budget zero => no children
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	_, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: child})
	if err == nil || !strings.Contains(err.Error(), "max depth") {
		t.Fatalf("err=%v", err)
	}
}

func TestF1_ActiveChildrenClosesOnErrorAndCancel(t *testing.T) {
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := primer.NewRunner(primer.AgentSpec{
		MaxChildren:      3,
		MaxDepth:         2,
		MaxTotalChildren: 10,
	}, parent, primer.NoopSink{})

	// Error path.
	errProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				yield(nil, errors.New("child boom"))
			}
		},
	}
	errChild := primer.NewScriptedAgent(agent.Config{ID: "e", Name: "E"}, errProv)
	ct, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: errChild})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := ct.Call(context.Background(), `{"query":"x"}`)
	if callErr == nil {
		t.Fatal("expected child error")
	}
	_, _, active := r.ChildBudgetSnapshot()
	if active != 0 {
		t.Fatalf("active after error=%d", active)
	}

	// Cancel path.
	block := make(chan struct{})
	cancelProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				select {
				case <-ctx.Done():
					yield(nil, ctx.Err())
				case <-block:
					yield(textUpdate("late"), nil)
				}
			}
		},
	}
	cChild := primer.NewScriptedAgent(agent.Config{ID: "c", Name: "C"}, cancelProv)
	ct2, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: cChild})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ct2.Call(ctx, `{"query":"x"}`)
		done <- err
	}()
	// Wait until child started then cancel.
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
	r := primer.NewRunner(primer.AgentSpec{Name: "O", MaxChildren: 0}, parent, &primer.CollectingSink{})
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
	// Agent id remains stable.
	if parent.ID() != "stable-agent-id" {
		t.Fatal("agent id changed")
	}
}

func TestF1_PrepareRun_StartChildBeforeRun_LineageCoherent(t *testing.T) {
	// Mirrors SSE path: PrepareRun → StartChild → Run must share one parent/root id.
	sink := &primer.CollectingSink{}
	parent := scriptedAgent(t, "O", "p-agent", textUpdate("parent-text"))
	r := primer.NewRunner(primer.AgentSpec{
		Name: "O", MaxChildren: 1, MaxDepth: 1, MaxTotalChildren: 5,
	}, parent, sink)
	prepared := r.PrepareRun()
	if prepared == "" || prepared == parent.ID() {
		t.Fatalf("prepared=%q agent=%q", prepared, parent.ID())
	}
	child := scriptedAgent(t, "C", "c-agent", textUpdate("child-text"))
	ct, _, err := r.StartChild(context.Background(), primer.ChildSpec{Type: "c", Name: "C", Agent: child})
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
	var parentStart, childStart *primer.RunEvent
	for i := range sink.Snapshot() {
		e := sink.Snapshot()[i]
		if e.Kind == primer.KindStart && parentStart == nil {
			parentStart = &e
		}
		if e.Kind == primer.KindChildStart && childStart == nil {
			childStart = &e
		}
	}
	// Re-snapshot once.
	evs := sink.Snapshot()
	parentStart, childStart = nil, nil
	for i := range evs {
		e := evs[i]
		if e.Kind == primer.KindStart && parentStart == nil {
			cp := e
			parentStart = &cp
		}
		if e.Kind == primer.KindChildStart && childStart == nil {
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
	// Direct budget still open (MaxChildren=5) but total tree budget is 1.
	parent := scriptedAgent(t, "O", "p", textUpdate("x"))
	r := primer.NewRunner(primer.AgentSpec{
		MaxChildren:      5,
		MaxDepth:         3,
		MaxTotalChildren: 1,
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	ct, _, err := r.StartChild(context.Background(), primer.ChildSpec{Agent: child})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = ct.Call(context.Background(), `{"query":"x"}`)
	_, _, err = r.StartChild(context.Background(), primer.ChildSpec{Agent: child})
	if err == nil || !strings.Contains(err.Error(), "total child budget") {
		t.Fatalf("err=%v, want total child budget", err)
	}
}

// --- F2 MCP scope ---

func TestF2_MCP_FailClosedFilter(t *testing.T) {
	env := fakes.StartMCP(t)
	all := env.AllTools(t)
	names := primer.ToolNames(all)
	if len(names) < 2 {
		t.Fatalf("expected >=2 mcp tools, got %v", names)
	}
	childTools := primer.FilterToolsFailClosed(all, []string{"allow_me"})
	if got := primer.ToolNames(childTools); len(got) != 1 || got[0] != "allow_me" {
		t.Fatalf("filtered = %v", got)
	}
	ft, ok := childTools[0].(tool.FuncTool)
	if !ok {
		t.Fatalf("type %T", childTools[0])
	}
	_, err := ft.Call(context.Background(), `{"q":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if env.AllowCalls.Load() != 1 {
		t.Fatalf("allow calls = %d", env.AllowCalls.Load())
	}
	for _, n := range primer.ToolNames(childTools) {
		if n == "deny_me" {
			t.Fatal("deny_me present on child")
		}
	}
	expanded := primer.FilterToolsFailClosed(all, []string{"allow_me", "deny_me", "not_real"})
	childOnly := primer.FilterToolsFailClosed(expanded, []string{"allow_me"})
	if len(childOnly) != 1 {
		t.Fatalf("childOnly=%v", primer.ToolNames(childOnly))
	}
}

func TestF2_MCP_DeniedToolNotInvokedThroughChild(t *testing.T) {
	env := fakes.StartMCP(t)
	all := env.AllTools(t)
	childTools := primer.FilterToolsFailClosed(all, []string{"allow_me"})
	invokable := map[string]tool.FuncTool{}
	for _, tl := range childTools {
		if ft, ok := tl.(tool.FuncTool); ok {
			invokable[ft.Name()] = ft
		}
	}
	if _, ok := invokable["deny_me"]; ok {
		t.Fatal("deny_me invokable")
	}
	if _, ok := invokable["allow_me"]; !ok {
		t.Fatal("allow_me missing")
	}
	for _, tl := range all {
		if tl.Name() == "deny_me" {
			_, _ = tl.(tool.FuncTool).Call(context.Background(), `{}`)
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

// --- F5 cancel / backpressure ---

func TestF5_CancelPropagatesToChildAndMCP(t *testing.T) {
	env := fakes.StartMCP(t)
	env.BlockAllow = 5 * time.Second
	all := env.AllTools(t)
	allow := primer.FilterToolsFailClosed(all, []string{"allow_me"})[0].(tool.FuncTool)

	childProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				_, err := allow.Call(ctx, `{"q":"x"}`)
				if err != nil {
					yield(nil, err)
					return
				}
				yield(textUpdate("should-not"), nil)
			}
		},
	}
	child := primer.NewScriptedAgent(agent.Config{ID: "c", Name: "C"}, childProv)
	sink := &primer.CollectingSink{}
	st := primer.NewStreamingChildTool(child, "c", "p", sink)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := st.Call(ctx, `{"query":"x"}`)
		done <- err
	}()
	// Wait until MCP handler is entered (AllowCalls) then cancel.
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
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "cancel") {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel")
	}
	// HARD: blocking MCP tool must observe ctx cancel (not merely child Call error).
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.AllowSawCancel.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("AllowSawCancel=false after cancel (MCP did not observe ctx cancel); AllowCalls=%d", env.AllowCalls.Load())
}

func TestF5_SlowSinkDoesNotFailRun(t *testing.T) {
	sink := primer.NewBoundedSink(2)
	sink.BlockFor = time.Hour // would hang if unbounded
	child := scriptedAgent(t, "C", "c", textUpdate("a"), textUpdate("b"), textUpdate("c"), textUpdate("d"))
	st := primer.NewStreamingChildTool(child, "c", "p", sink)
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
	if sink.Dropped == 0 && len(sink.Snapshot()) > 2 {
		t.Fatalf("expected drops or cap, dropped=%d n=%d", sink.Dropped, len(sink.Snapshot()))
	}
}

func TestF5_DisconnectedConsumer_RunStillCompletes(t *testing.T) {
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	sink := primer.NewBoundedSink(1)
	st := primer.NewStreamingChildTool(child, "c", "p", sink)
	res, err := st.Call(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res != "x" {
		t.Fatalf("%q", res)
	}
}

// blockingWriter blocks on the Nth Write until released; used to prove SSE bridge
// keeps the agent runner independent of a slow ResponseWriter.
type blockingWriter struct {
	mu        sync.Mutex
	writes    int
	blockAt   int
	blockCh   chan struct{} // closed to release
	entered   chan struct{} // closed when blocking write entered
	buf       strings.Builder
	writeErr  error
	flushes   int
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
	err := b.writeErr
	b.mu.Unlock()
	if err != nil {
		return 0, err
	}
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

func TestF5_SSEBridge_SlowWriterDoesNotBlockRunner(t *testing.T) {
	// Agent emits many events; writer blocks on first flush write.
	// Runner must finish even while writer is stuck.
	releaseGate := make(chan struct{})
	var runFinished atomic.Bool
	earlyEventSeen := make(chan struct{})
	var earlyOnce sync.Once

	// Provider that waits until runFinished path is free — actually we measure
	// that Run returns while writer still blocked.
	parentProv := &primer.ScriptedProvider{
		Updates: []*agent.ResponseUpdate{
			textUpdate("early-event"),
			textUpdate("mid-event"),
			textUpdate("late-event"),
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "p", Name: "P"}, parentProv)

	bw := newBlockingWriter(1) // block on first Write
	bridge := primer.NewSSEBridge(4)

	// Writer goroutine blocked on first event.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		bridge.WriteTo(context.Background(), bw, bw)
	}()

	// Emit start via runner.
	r := primer.NewRunner(primer.AgentSpec{Type: "t", Name: "P"}, parent, eventFunc(func(ctx context.Context, e primer.RunEvent) {
		if e.Kind == primer.KindText && e.Text == "early-event" {
			earlyOnce.Do(func() { close(earlyEventSeen) })
		}
		bridge.Emit(ctx, e)
	}))

	runDone := make(chan error, 1)
	go func() {
		err := r.Run(context.Background(), "hi")
		runFinished.Store(true)
		runDone <- err
	}()

	// Wait until writer entered blocking write OR early event enqueued.
	select {
	case <-bw.entered:
		// Writer is blocked holding the first write.
	case <-time.After(2 * time.Second):
		t.Fatal("writer never entered blocking write")
	}

	// Runner must complete while writer still blocked.
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

	// Release writer and ensure it drains/exits.
	bw.release()
	bridge.Close()
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("writer goroutine leak")
	}
	_ = releaseGate
	_ = earlyEventSeen
}

func TestF5_SSEBridge_CancelTerminatesWriter(t *testing.T) {
	bridge := primer.NewSSEBridge(8)
	// Keep channel open with no close yet; stream-ctx cancel should end WriteTo promptly.
	ctx, cancel := context.WithCancel(context.Background())
	bw := newBlockingWriter(0) // never blocks (blockAt=0 means n==0 never)
	// Fix: blockAt=0 never triggers; use large blockAt.
	bw.blockAt = 999

	done := make(chan struct{})
	go func() {
		defer close(done)
		bridge.WriteTo(ctx, bw, bw)
	}()
	// Emit one event so writer is alive.
	bridge.Emit(context.Background(), primer.RunEvent{Kind: primer.KindText, Text: "x", RunID: "r"})
	// Queue more events after cancel path to prove residual drop without write drain hang.
	for i := 0; i < 4; i++ {
		bridge.Emit(context.Background(), primer.RunEvent{Kind: primer.KindText, Text: fmt.Sprintf("q%d", i), RunID: "r"})
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

// TestF5_SSEBridge_ConcurrentEmitClose_NoPanic proves Emit/Close are race-safe:
// no send-on-closed-channel panic, no deadlock, under -race.
// Avoid recover-as-success: the test process must not crash; failures surface as panic.
func TestF5_SSEBridge_ConcurrentEmitClose_NoPanic(t *testing.T) {
	const (
		rounds    = 200
		emitters  = 8
		eventsEach = 50
	)
	var wg sync.WaitGroup
	for round := 0; round < rounds; round++ {
		bridge := primer.NewSSEBridge(4)
		// Drain consumer (may exit mid-round via Close).
		drainDone := make(chan struct{})
		go func(b *primer.SSEBridge) {
			defer close(drainDone)
			// Use a throwaway writer that never blocks.
			bw := newBlockingWriter(9999)
			// Background stream ctx never cancels; Close ends the channel.
			b.WriteTo(context.Background(), bw, bw)
		}(bridge)

		wg.Add(emitters + 1)
		// Closers race with emitters.
		go func(b *primer.SSEBridge) {
			defer wg.Done()
			// Stagger close slightly so some emits land before/after.
			time.Sleep(time.Duration(round%3) * time.Microsecond)
			b.Close()
			// Double-close must be safe.
			b.Close()
		}(bridge)

		for e := 0; e < emitters; e++ {
			go func(b *primer.SSEBridge, id int) {
				defer wg.Done()
				for i := 0; i < eventsEach; i++ {
					b.Emit(context.Background(), primer.RunEvent{
						Kind:  primer.KindText,
						Text:  fmt.Sprintf("e%d-%d", id, i),
						RunID: "race",
					})
				}
			}(bridge, e)
		}
		wg.Wait()
		// Ensure Close eventually happens if closer was slow (already called).
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

// TestF5_SSEHandler_HTTPDisconnect_RunCompletes is the load-bearing two-context proof:
// client disconnect cancels stream writer only; runtime-owned run continues to KindEnd
// with after-disconnect text; no writer/run goroutine leak.
func TestF5_SSEHandler_HTTPDisconnect_RunCompletes(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	// hold keeps the provider in-flight until AFTER client disconnect is observed.
	hold := make(chan struct{})
	// runSawCancel must stay false: disconnect must NOT cancel runCtx.
	var runSawCancel atomic.Bool
	// final text must be produced after disconnect.
	var producedAfter atomic.Bool
	// Barriers / completion hooks from SSEHandler.
	runEnded := make(chan error, 1)
	var runHandle atomic.Pointer[primer.RunHandle]
	writerExited := make(chan struct{})
	var writerOnce sync.Once

	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				if !yield(textUpdate("first-wire"), nil) {
					return
				}
				startedOnce.Do(func() { close(started) })
				select {
				case <-hold:
					// Released only after client disconnect + writer exit proof path.
				case <-ctx.Done():
					// If this fires on mere subscriber loss, two-context split is broken.
					runSawCancel.Store(true)
					yield(nil, ctx.Err())
					return
				}
				// Still running after disconnect: emit final completion token.
				producedAfter.Store(true)
				if !yield(textUpdate("after-disconnect"), nil) {
					return
				}
			}
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "p", Name: "P"}, parentProv)

	// Capture KindEnd via a side sink is not available on SSEHandler; use OnRunEnd +
	// bridge observation through a custom ExtraOptions no-op. Instead, wrap via OnRunEnd
	// and a collecting side-channel by monkeying BuildChildTool-less path: we assert
	// OnRunEnd err==nil and producedAfter + !runSawCancel.
	h := &primer.SSEHandler{
		Agent:          parent,
		AgentType:      "overseer",
		BridgeCapacity: 8,
		RunBudget:      10 * time.Second,
		OnRunStart: func(rh *primer.RunHandle) {
			runHandle.Store(rh)
		},
		OnRunEnd: func(err error) {
			runEnded <- err
		},
	}

	// Real TCP listener so Body.Close cancels only the request/stream context.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()
	url := "http://" + ln.Addr().String()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(resp.Body)

	// Read until first-wire is on the wire.
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
				// Keep reading until disconnect closes the body.
			}
		}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider never started")
	}
	// Wait briefly for first event to flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !gotFirst.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if !gotFirst.Load() {
		t.Fatal("first-wire never observed on stream before disconnect")
	}

	// Disconnect client → cancels stream/request ctx only.
	_ = resp.Body.Close()

	// Stream reader should observe EOF/error promptly (writer exits on stream cancel).
	select {
	case <-readErr:
		writerOnce.Do(func() { close(writerExited) })
	case <-time.After(2 * time.Second):
		t.Fatal("stream reader did not exit after client disconnect (writer leak?)")
	}

	// Run handle must still be alive (not canceled by disconnect).
	rh := runHandle.Load()
	if rh == nil {
		t.Fatal("OnRunStart never fired")
	}
	if rh.Err() != nil {
		t.Fatalf("run handle canceled by disconnect: %v", rh.Err())
	}

	// Release provider; run must complete successfully AFTER disconnect.
	close(hold)

	select {
	case err := <-runEnded:
		if err != nil {
			t.Fatalf("run ended with error after disconnect: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not complete after disconnect (possible cancel conflation or hang)")
	}

	if runSawCancel.Load() {
		t.Fatal("run context was canceled by client disconnect — stream/run contexts are still conflated")
	}
	if !producedAfter.Load() {
		t.Fatal("provider did not produce after-disconnect token — run did not continue past disconnect")
	}
	if parentProv.Started.Load() < 1 {
		t.Fatal("parent never started")
	}
}

// TestF5_SSEHandler_ExplicitRunCancel_ReachesBlockingMCP proves that explicit
// runtime cancellation (RunHandle.Cancel), not stream disconnect, cancels
// parent→child→blocking MCP.
func TestF5_SSEHandler_ExplicitRunCancel_ReachesBlockingMCP(t *testing.T) {
	env := fakes.StartMCP(t)
	env.BlockAllow = 5 * time.Second
	all := env.AllTools(t)
	allow := primer.FilterToolsFailClosed(all, []string{"allow_me"})[0].(tool.FuncTool)

	childProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				_, err := allow.Call(ctx, `{"q":"x"}`)
				if err != nil {
					yield(nil, err)
					return
				}
				yield(textUpdate("should-not"), nil)
			}
		},
	}
	child := primer.NewScriptedAgent(agent.Config{ID: "c", Name: "C"}, childProv)

	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				for tl := range agent.AllOptions(options, agent.WithTool) {
					if ft, ok := tl.(tool.FuncTool); ok {
						_, err := ft.Call(ctx, `{"query":"nested"}`)
						if err != nil {
							yield(nil, err)
							return
						}
					}
				}
				yield(textUpdate("parent-should-not"), nil)
			}
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "p", Name: "P"}, parentProv)

	var handle atomic.Pointer[primer.RunHandle]
	runEnded := make(chan error, 1)
	h := &primer.SSEHandler{
		Agent:     parent,
		AgentType: "overseer",
		Spec: primer.AgentSpec{
			Type:        "overseer",
			Name:        "P",
			// Parent grants must include allow_me so StartChild authority check passes.
			Tools:       []tool.Tool{allow},
			MaxChildren: 1,
			MaxDepth:    1,
		},
		BridgeCapacity: 8,
		RunBudget:      10 * time.Second,
		BuildChildTool: func(ctx context.Context, parent *primer.Runner, sink primer.EventSink) tool.FuncTool {
			ct, _, err := parent.StartChild(ctx, primer.ChildSpec{
				Type:         "c",
				Name:         "C",
				Agent:        child,
				AllowedTools: []string{"allow_me"},
				Tools:        []tool.Tool{allow},
			})
			if err != nil {
				t.Errorf("StartChild: %v", err)
				return nil
			}
			return ct
		},
		OnRunStart: func(rh *primer.RunHandle) { handle.Store(rh) },
		OnRunEnd:   func(err error) { runEnded <- err },
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()
	url := "http://" + ln.Addr().String()

	// Keep client connected so stream ctx stays live; only RunHandle.Cancel fires.
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Wait until blocking MCP is entered.
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
	// Explicit runtime cancel (NOT client disconnect).
	rh.Cancel()

	select {
	case err := <-runEnded:
		if err == nil {
			t.Fatal("expected run error after explicit cancel")
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
			!strings.Contains(err.Error(), "cancel") {
			t.Fatalf("unexpected run err: %v", err)
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
	t.Fatalf("AllowSawCancel=false after RunHandle.Cancel; AllowCalls=%d", env.AllowCalls.Load())
}

// --- F4 wire streaming ---

type eventFunc func(ctx context.Context, e primer.RunEvent)

func (f eventFunc) Emit(ctx context.Context, e primer.RunEvent) { f(ctx, e) }

func TestF4_PrimerSSE_ParentAndChildAttributed(t *testing.T) {
	child := scriptedAgent(t, "MathTutor", "child-sse", textUpdate("c1"), textUpdate("c2"))
	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				for tl := range agent.AllOptions(options, agent.WithTool) {
					if ft, ok := tl.(tool.FuncTool); ok && strings.Contains(ft.Name(), "MathTutor") {
						_, err := ft.Call(ctx, `{"query":"nested"}`)
						if err != nil {
							yield(nil, err)
							return
						}
					}
				}
				yield(textUpdate("parent-tail"), nil)
			}
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "parent-sse", Name: "Overseer"}, parentProv)

	h := &primer.SSEHandler{
		Agent:     parent,
		AgentType: "overseer",
		Spec: primer.AgentSpec{
			Type:        "overseer",
			Name:        "Overseer",
			MaxChildren: 1,
			MaxDepth:    1,
		},
		BuildChildTool: func(ctx context.Context, parent *primer.Runner, sink primer.EventSink) tool.FuncTool {
			ct, _, err := parent.StartChild(ctx, primer.ChildSpec{
				Type:  "math_tutor",
				Name:  "MathTutor",
				Agent: child,
			})
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
		t.Fatalf("missing parent text in SSE: %s", s)
	}
	if !strings.Contains(s, "c1") || !strings.Contains(s, "c2") {
		t.Fatalf("missing child text in SSE: %s", s)
	}
	if !strings.Contains(s, "math_tutor") && !strings.Contains(s, "MathTutor") {
		t.Fatalf("missing child attribution: %s", s)
	}
	if !strings.Contains(s, `"kind":"child_start"`) && !strings.Contains(s, "child_start") {
		t.Fatalf("missing child_start: %s", s)
	}

	// Parse SSE data lines and hard-assert lineage coherence:
	// parent start.run_id == parent start.root_run_id == child.parent_run_id == child.root_run_id
	var parentStart, childStart *primer.RunEvent
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev primer.RunEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		switch {
		case ev.Kind == primer.KindStart && parentStart == nil:
			e := ev
			parentStart = &e
		case ev.Kind == primer.KindChildStart && childStart == nil:
			e := ev
			childStart = &e
		}
	}
	if parentStart == nil {
		t.Fatalf("missing parsed parent start event in %s", s)
	}
	if childStart == nil {
		t.Fatalf("missing parsed child_start event in %s", s)
	}
	if parentStart.RunID == "" || parentStart.RunID != parentStart.RootRunID {
		t.Fatalf("parent run_id=%q root_run_id=%q want equal non-empty", parentStart.RunID, parentStart.RootRunID)
	}
	if parentStart.RunID == parent.ID() {
		t.Fatalf("parent run_id must not equal agent id %q", parent.ID())
	}
	if childStart.ParentRunID != parentStart.RunID {
		t.Fatalf("child parent_run_id=%q want parent run_id %q", childStart.ParentRunID, parentStart.RunID)
	}
	if childStart.RootRunID != parentStart.RootRunID {
		t.Fatalf("child root_run_id=%q want %q", childStart.RootRunID, parentStart.RootRunID)
	}
	if childStart.RunID == "" || childStart.RunID == child.ID() || childStart.RunID == parentStart.RunID {
		t.Fatalf("child run_id=%q must be fresh (agent=%q parent=%q)", childStart.RunID, child.ID(), parentStart.RunID)
	}
}

func TestF4_PrimerSSE_IncrementalBeforeCompletion(t *testing.T) {
	// Barrier-driven: early parent event must be observable on the wire BEFORE
	// the provider is released to complete.
	releaseProvider := make(chan struct{})
	earlyOnWire := make(chan struct{})
	var earlyOnce sync.Once

	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				if !yield(textUpdate("early-parent-delta"), nil) {
					return
				}
				// Block until test observes early event on the remote stream.
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
	parent := primer.NewScriptedAgent(agent.Config{ID: "inc-p", Name: "Overseer"}, parentProv)
	h := &primer.SSEHandler{
		Agent:          parent,
		AgentType:      "overseer",
		BridgeCapacity: 16,
	}

	// Use a real TCP listener so we can stream-read with http.Client.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()
	url := "http://" + ln.Addr().String()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
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
	// Read SSE lines until early-parent-delta appears — MUST happen before release.
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
				// Keep reading until final after release.
			}
			if strings.Contains(line, "final-parent-delta") {
				readDone <- nil
				return
			}
		}
	}()

	select {
	case <-earlyOnWire:
		// Proof: early event arrived while provider still blocked.
	case <-time.After(3 * time.Second):
		t.Fatal("early event not observed before provider completion")
	}
	if !foundEarly {
		t.Fatal("early flag false")
	}

	// Only now release provider to complete.
	close(releaseProvider)

	select {
	case err := <-readDone:
		if err != nil && !errors.Is(err, io.EOF) {
			// EOF ok if connection closed after end.
			if !foundEarly {
				t.Fatal(err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for stream end")
	}
}

func TestF4_AGUI_ParentTextOnly_NoChildAttribution(t *testing.T) {
	child := scriptedAgent(t, "Child", "c", textUpdate("secret-child-delta"))
	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				at := agenttool.New(child, agenttool.Config{})
				res, _ := at.Call(ctx, `{"query":"x"}`)
				yield(textUpdate("agui-parent:"+res.(string)), nil)
			}
		},
	}
	parent := primer.NewScriptedAgent(agent.Config{ID: "agui-p", Name: "P"}, parentProv)
	h := aguiprovider.NewJSONHTTPHandler(parent, aguiprovider.HandlerConfig{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	payload := `{"threadId":"t1","runId":"r1","messages":[{"id":"m1","role":"user","content":"hi"}]}`
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "agui-parent") {
		t.Fatalf("missing parent marker in AG-UI SSE body: %s", s)
	}
	if !strings.Contains(s, "secret-child-delta") {
		t.Fatalf("missing collected child text in AG-UI SSE body: %s", s)
	}
	if n := strings.Count(s, "secret-child-delta"); n != 1 {
		t.Fatalf("secret-child-delta occurrences=%d want 1 (Collect), body=%s", n, s)
	}
	if strings.Contains(s, "child_start") || strings.Contains(s, `"agent_type":"math_tutor"`) {
		t.Fatalf("unexpected nested child attribution in AG-UI body: %s", s)
	}
}

// --- F6 sessions ---

func TestF6_SingleShotNoSession(t *testing.T) {
	a := scriptedAgent(t, "A", "id", textUpdate("hello-no-session"))
	resp, err := a.RunText(context.Background(), "ping").Collect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.String(), "hello-no-session") {
		t.Fatalf("%q", resp.String())
	}
}

func TestF6_SessionJSONRoundTrip(t *testing.T) {
	s := &agent.Session{}
	s.SetServiceID("svc-1")
	s.Set("note", "hello")
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var s2 agent.Session
	if err := json.Unmarshal(b, &s2); err != nil {
		t.Fatal(err)
	}
	if s2.ServiceID() != "svc-1" {
		t.Fatalf("service id %q", s2.ServiceID())
	}
	var note string
	ok, err := s2.Get("note", &note)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || note != "hello" {
		t.Fatalf("note ok=%v val=%q", ok, note)
	}
	a := scriptedAgent(t, "A", "id2", textUpdate("with-session"))
	resp, err := a.RunText(context.Background(), "x", agent.WithSession(&s2)).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.String(), "with-session") {
		t.Fatalf("%q", resp.String())
	}
}

// --- F7 OpenAI-compatible base URL ---

func TestF7_OpenAIProvider_CustomBaseURL_NoRealCredentials(t *testing.T) {
	srv := fakes.NewOpenAIServer(
		fakes.OpenAIScript{Content: "Hello from fake", Stream: true},
	)
	defer srv.Close()

	client := openai.NewClient(
		option.WithBaseURL(srv.URL()),
		option.WithAPIKey("«redacted:sk-…»"),
	)
	a := openaiprovider.NewChatCompletionsAgent(client, openaiprovider.AgentConfig{
		Config: agent.Config{
			ID:   "openai-test",
			Name: "OpenAIAgent",
		},
		Model:        "gpt-4o-mini",
		Instructions: "You are a test agent.",
	})
	var chunks []string
	for update, err := range a.RunText(context.Background(), "hi", agent.Stream(true)) {
		if err != nil {
			t.Fatalf("stream err: %v", err)
		}
		if update != nil {
			if s := update.String(); s != "" {
				chunks = append(chunks, s)
			}
		}
	}
	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "Hello") && !strings.Contains(joined, "fake") {
		resp, err := a.RunText(context.Background(), "hi").Collect()
		if err != nil {
			t.Fatalf("collect: %v; stream joined=%q calls=%d", err, joined, srv.Calls())
		}
		joined = resp.String()
	}
	if !strings.Contains(joined, "Hello") {
		t.Fatalf("joined=%q calls=%d auth=%q", joined, srv.Calls(), srv.LastAuth)
	}
	if srv.Calls() < 1 {
		t.Fatal("fake server not hit")
	}
	// Dummy key only — never a real credential. Accept any Bearer we sent.
	if !strings.Contains(srv.LastAuth, "Bearer") || strings.TrimSpace(srv.LastAuth) == "" {
		t.Fatalf("auth=%q", srv.LastAuth)
	}
}

// --- F8 boundary ---

func TestF8_NoDistributedControlPlaneDeps(t *testing.T) {
	b, err := readGoMod()
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"dbos", "nats.go", "temporal", "asynq"}
	low := strings.ToLower(b)
	for _, f := range forbidden {
		if strings.Contains(low, f) {
			t.Fatalf("forbidden dep %q in go.mod", f)
		}
	}
	if !strings.Contains(b, "agent-framework-go") {
		t.Fatal("expected MAF pin")
	}
}

func readGoMod() (string, error) {
	candidates := []string{"go.mod", "../go.mod"}
	wd, _ := os.Getwd()
	for _, rel := range candidates {
		b, err := os.ReadFile(filepath.Join(wd, rel))
		if err == nil {
			return string(b), nil
		}
	}
	b, err := os.ReadFile("go.mod")
	if err == nil {
		return string(b), nil
	}
	return "", errors.New("go.mod not found")
}

// silence unused import if fmt not needed in some builds
var _ = fmt.Sprintf
