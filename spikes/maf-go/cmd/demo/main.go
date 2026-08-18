// Command demo runs a parent→streaming-child offline MAF path (no network).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aleksclark/primer/spikes/maf-go/primer"
	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
)

func main() {
	ctx := context.Background()
	sink := &primer.CollectingSink{}

	child := primer.NewScriptedAgent(agent.Config{
		ID:          "demo-child",
		Name:        "MathTutor",
		Description: "demo child",
	}, &primer.ScriptedProvider{
		Updates: []*agent.ResponseUpdate{
			{Role: message.RoleAssistant, Contents: message.Contents{&message.TextContent{Text: "child-delta-1 "}}},
			{Role: message.RoleAssistant, Contents: message.Contents{&message.TextContent{Text: "child-delta-2"}}},
		},
	})

	parent := primer.NewScriptedAgent(agent.Config{
		ID:   "demo-parent",
		Name: "Overseer",
	}, &primer.ScriptedProvider{
		Updates: []*agent.ResponseUpdate{
			{Role: message.RoleAssistant, Contents: message.Contents{&message.TextContent{Text: "parent-done"}}},
		},
	})

	r := primer.NewRunner(primer.AgentSpec{
		Type:             "overseer",
		Name:             "Overseer",
		MaxChildren:      1,
		MaxDepth:         1,
		MaxTotalChildren: 1,
	}, parent, sink)

	// Root run establishes fresh run_id + root_run_id (distinct from agent id).
	if err := r.Run(ctx, "session start"); err != nil {
		fmt.Fprintf(os.Stderr, "parent run: %v\n", err)
		os.Exit(1)
	}

	ct, childRunner, err := r.StartChild(ctx, primer.ChildSpec{
		Type:         "math_tutor",
		Name:         "MathTutor",
		Instructions: "You are MathTutor.",
		Agent:        child,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "StartChild: %v\n", err)
		os.Exit(1)
	}
	res, err := ct.Call(ctx, `{"query":"demo"}`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "child: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("child_result=%q\n", res)
	fmt.Printf("parent_run_id=%q agent_id=%q depth=%d\n", r.LastRunID(), parent.ID(), r.Depth())
	fmt.Printf("child_depth=%d\n", childRunner.Depth())
	fmt.Println("events:")
	for _, e := range sink.Snapshot() {
		fmt.Printf("  kind=%s agent=%s run=%s parent=%s root=%s depth=%d text=%q\n",
			e.Kind, e.AgentName, e.RunID, e.ParentRunID, e.RootRunID, e.Depth, e.Text)
	}
}
