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

	ct, err := r.StartChild(ctx, "run-demo", primer.ChildSpec{
		Type:  "math_tutor",
		Name:  "MathTutor",
		Agent: child,
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
	if err := r.Run(ctx, "session complete"); err != nil {
		fmt.Fprintf(os.Stderr, "parent run: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("child_result=%q\n", res)
	fmt.Println("events:")
	for _, e := range sink.Snapshot() {
		fmt.Printf("  kind=%s agent=%s parent=%s text=%q\n", e.Kind, e.AgentName, e.ParentRunID, e.Text)
	}
}
