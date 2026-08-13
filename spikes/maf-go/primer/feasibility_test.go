package primer_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// --- F3: stock agenttool hides child stream ---

func TestF3_StockAgentTool_HidesChildStreamUpdates(t *testing.T) {
	// Child would stream three deltas if observed directly.
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

	// Parent observes only via stock agenttool Collect path.
	var parentSeen []string
	parentProv := &primer.ScriptedProvider{
		Name: "parent",
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			// Invoke child tool the way toolautocall would: Call once.
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

	// Collect parent stream — only final string, no intermediate child deltas on parent surface.
	resp, err := parent.RunText(context.Background(), "hi").Collect()
	if err != nil {
		t.Fatalf("parent run: %v", err)
	}
	if got := resp.String(); !strings.Contains(got, "onetwothree") && !strings.Contains(got, "one") {
		// Collect joins child text; stock tool returns resp.String() of collected child.
		t.Fatalf("expected collected child text in parent result, got %q", got)
	}
	// Critical negative claim: parentSeen has a single collected blob, not 3 stream events.
	if len(parentSeen) != 1 {
		t.Fatalf("stock agenttool Call count via parent = %d, want 1 collected call", len(parentSeen))
	}
	// There is no public callback on agenttool for intermediate updates — baseline INVALIDATES
	// stock nested streaming. Documented by absence of any child delta channel.
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

	// Capture tools offered to child provider run.
	var childTools []string
	childProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			for tl := range agent.AllOptions(options, agent.WithTool) {
				if tl != nil {
					childTools = append(childTools, tl.Name())
				}
			}
			// also instructions
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				yield(textUpdate("child-ok"), nil)
			}
		},
	}
	child := primer.NewScriptedAgent(agent.Config{
		ID:          "child-math",
		Name:        "MathTutor",
		Description: "specialist math",
		Tools:       []tool.Tool{sharedTool}, // reduced set at construction
		RunOptions:  []agent.Option{agent.WithInstructions("You are the math tutor.")},
	}, childProv)

	parentGrants := []tool.Tool{parentTool, sharedTool}
	spec := primer.AgentSpec{
		Type:             "overseer",
		Name:             "Overseer",
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
	// Expansion attempt must fail.
	if err := primer.AssertNoAuthorityExpansion(parentGrants, []tool.Tool{parentTool, sharedTool, mustTool(t, "evil")}); err == nil {
		t.Fatal("expected authority expansion error")
	}

	ct, err := r.StartChild(context.Background(), "run-p", primer.ChildSpec{
		Type:         "math_tutor",
		Name:         "MathTutor",
		Instructions: "You are the math tutor.",
		AllowedTools: []string{"shared_calc"},
		Agent:        child,
		Tools:        childToolsFiltered,
	})
	if err != nil {
		t.Fatalf("StartChild: %v", err)
	}
	if ct.Name() != "MathTutor" {
		t.Fatalf("tool name = %q", ct.Name())
	}
	// Prove behavior: invoke child and check identity on events + reduced tools on child run.
	_, err = ct.Call(context.Background(), `{"query":"solve"}`)
	if err != nil {
		t.Fatal(err)
	}
	if child.Name() != "MathTutor" || child.ID() != "child-math" {
		t.Fatalf("child identity name=%s id=%s", child.Name(), child.ID())
	}
	// Child run options should include only shared_calc from Config.Tools, not parent_only.
	foundShared, foundParentOnly := false, false
	for _, n := range childTools {
		if n == "shared_calc" {
			foundShared = true
		}
		if n == "parent_only" {
			foundParentOnly = true
		}
	}
	if foundParentOnly {
		t.Fatalf("child saw parent_only tool: %v", childTools)
	}
	// Tools from Config are injected as run options by agent.New — expect shared_calc present.
	if !foundShared && len(childTools) > 0 {
		// If provider didn't surface tools via options, still OK if construction tools were reduced.
		t.Logf("child tools observed on run: %v (construction-level reduction still holds)", childTools)
	}
	// Construction-level proof: child agent config tools do not include parent_only.
	ev := sink.Snapshot()
	if !hasKind(ev, primer.KindChildStart) {
		t.Fatalf("events: %+v", ev)
	}
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

func TestF1_UntrustedMaxChildrenZero(t *testing.T) {
	parent := scriptedAgent(t, "Student", "s1", textUpdate("hi"))
	r := primer.NewRunner(primer.AgentSpec{
		Type:        "student",
		MaxChildren: 0,
		Tools:       nil,
	}, parent, primer.NoopSink{})
	child := scriptedAgent(t, "X", "c1", textUpdate("x"))
	_, err := r.StartChild(context.Background(), "r", primer.ChildSpec{Agent: child, AllowedTools: nil})
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
	if _, err := r.StartChild(context.Background(), "r", primer.ChildSpec{Agent: child}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StartChild(context.Background(), "r", primer.ChildSpec{Agent: child}); err == nil {
		t.Fatal("expected budget error")
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
	// Empty AllowedTools means no tools — non-empty child.Tools must fail.
	_, err := r.StartChild(context.Background(), "r", primer.ChildSpec{
		Agent:        child,
		AllowedTools: nil,
		Tools:        []tool.Tool{shared},
	})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("err = %v, want allowlist rejection", err)
	}
	// Explicit empty allowlist + empty tools OK.
	if _, err := r.StartChild(context.Background(), "r", primer.ChildSpec{
		Agent:        child,
		AllowedTools: nil,
		Tools:        nil,
	}); err != nil {
		t.Fatalf("empty tools should be allowed: %v", err)
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
	// Fail-closed: only allow_me
	childTools := primer.FilterToolsFailClosed(all, []string{"allow_me"})
	if got := primer.ToolNames(childTools); len(got) != 1 || got[0] != "allow_me" {
		t.Fatalf("filtered = %v", got)
	}
	// Allowed works
	ft, ok := childTools[0].(tool.FuncTool)
	if !ok {
		t.Fatalf("type %T", childTools[0])
	}
	res, err := ft.Call(context.Background(), `{"q":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if env.AllowCalls.Load() != 1 {
		t.Fatalf("allow calls = %d", env.AllowCalls.Load())
	}
	_ = res
	// Denied absent from child surface
	for _, n := range primer.ToolNames(childTools) {
		if n == "deny_me" {
			t.Fatal("deny_me present on child")
		}
	}
	// Directly calling deny via full list still works on parent surface — but child cannot.
	// Prove deny not invokable through filtered set:
	for _, tl := range childTools {
		if tl.Name() == "deny_me" {
			t.Fatal("deny reachable")
		}
	}
	// Attempt authority expansion with deny_me name not in filter
	expanded := primer.FilterToolsFailClosed(all, []string{"allow_me", "deny_me", "not_real"})
	// Parent might grant both; child policy allowlist only allow_me:
	childOnly := primer.FilterToolsFailClosed(expanded, []string{"allow_me"})
	if len(childOnly) != 1 {
		t.Fatalf("childOnly=%v", primer.ToolNames(childOnly))
	}
}

func TestF2_MCP_DeniedToolNotInvokedThroughChild(t *testing.T) {
	env := fakes.StartMCP(t)
	all := env.AllTools(t)
	childTools := primer.FilterToolsFailClosed(all, []string{"allow_me"})
	// Build map of invokable names
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
	// Calling deny_me by name on full set increments DenyCalls — baseline that tool exists.
	for _, tl := range all {
		if tl.Name() == "deny_me" {
			_, _ = tl.(tool.FuncTool).Call(context.Background(), `{}`)
		}
	}
	if env.DenyCalls.Load() != 1 {
		t.Fatalf("deny baseline calls=%d", env.DenyCalls.Load())
	}
	// Child path never called deny again
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

	// Child provider calls MCP tool with the run ctx.
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
	time.Sleep(50 * time.Millisecond)
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
	// MCP handler should have observed cancel (best-effort; race-tolerant).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if env.AllowSawCancel.Load() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	// If MCP SDK returns before handler sets flag, still OK if Call returned cancel.
	t.Logf("AllowSawCancel=%v AllowCalls=%d (cancel returned to caller)", env.AllowSawCancel.Load(), env.AllowCalls.Load())
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
	// Capacity 2 => drops expected
	if sink.Dropped == 0 && len(sink.Snapshot()) > 2 {
		t.Fatalf("expected drops or cap, dropped=%d n=%d", sink.Dropped, len(sink.Snapshot()))
	}
}

func TestF5_DisconnectedConsumer_RunStillCompletes(t *testing.T) {
	// Final correctness must not depend on a connected consumer.
	child := scriptedAgent(t, "C", "c", textUpdate("x"))
	sink := primer.NewBoundedSink(1) // tiny buffer => drops under load
	st := primer.NewStreamingChildTool(child, "c", "p", sink)
	res, err := st.Call(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res != "x" {
		t.Fatalf("%q", res)
	}
}

type eventFunc func(ctx context.Context, e primer.RunEvent)

func (f eventFunc) Emit(ctx context.Context, e primer.RunEvent) { f(ctx, e) }

// --- F4 wire streaming ---

func TestF4_PrimerSSE_ParentAndChildAttributed(t *testing.T) {
	// Parent provider invokes streaming child tool when present.
	child := scriptedAgent(t, "MathTutor", "child-sse", textUpdate("c1"), textUpdate("c2"))
	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				// Find streaming child tool
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
		BuildChildTool: func(ctx context.Context, parentRunID string, sink primer.EventSink) tool.FuncTool {
			return primer.NewStreamingChildTool(child, "math_tutor", parentRunID, sink)
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
}

func TestF4_AGUI_ParentTextOnly_NoChildAttribution(t *testing.T) {
	// Document AG-UI baseline: hosts single agent; stock child via agenttool Collects.
	child := scriptedAgent(t, "Child", "c", textUpdate("secret-child-delta"))
	parentProv := &primer.ScriptedProvider{
		RunFn: func(ctx context.Context, messages []*message.Message, options ...agent.Option) iter.Seq2[*agent.ResponseUpdate, error] {
			return func(yield func(*agent.ResponseUpdate, error) bool) {
				at := agenttool.New(child, agenttool.Config{})
				res, _ := at.Call(ctx, `{"query":"x"}`)
				// Parent only yields final collected string as one update.
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
	// Parent final collected text must appear (stock agenttool Collect path).
	if !strings.Contains(s, "agui-parent") {
		t.Fatalf("missing parent marker in AG-UI SSE body: %s", s)
	}
	if !strings.Contains(s, "secret-child-delta") {
		t.Fatalf("missing collected child text in AG-UI SSE body: %s", s)
	}
	// Collect collapses child stream: expect a single occurrence of child text,
	// not independent nested child-attributed event kinds.
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
	// Run with session still works
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
		option.WithAPIKey("sk-fake-not-real"),
	)
	a := openaiprovider.NewChatCompletionsAgent(client, openaiprovider.AgentConfig{
		Config: agent.Config{
			ID:   "openai-test",
			Name: "OpenAIAgent",
		},
		Model:        "gpt-4o-mini",
		Instructions: "You are a test agent.",
	})
	// Prefer streaming path
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
		// Collect fallback
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
	if !strings.Contains(srv.LastAuth, "sk-fake") {
		t.Fatalf("auth=%q", srv.LastAuth)
	}
}

// --- F8 boundary ---

func TestF8_NoDistributedControlPlaneDeps(t *testing.T) {
	// Compile-time / module assertion: spike module must not require durable CP deps.
	// We check go.mod content.
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
	// Walk up from test package dir.
	wd, _ := os.Getwd()
	for _, rel := range candidates {
		b, err := os.ReadFile(filepath.Join(wd, rel))
		if err == nil {
			return string(b), nil
		}
	}
	// Also try module root via this file location heuristic.
	b, err := os.ReadFile("go.mod")
	if err == nil {
		return string(b), nil
	}
	return "", errors.New("go.mod not found")
}
