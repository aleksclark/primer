// Package testfakes provides deterministic in-process test fixtures for the
// production agent package. These are test helpers only; they must not be
// imported by production code.
package testfakes

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	maftool "github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/mcptool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPEnv holds an in-process MCP server with two tools: allow_me and deny_me.
type MCPEnv struct {
	Server         *mcp.Server
	Client         *mcp.ClientSession
	AllowCalls     atomic.Int64
	DenyCalls      atomic.Int64
	AllowSawCancel atomic.Bool
	// BlockAllow makes allow_me block until ctx done or timeout.
	BlockAllow time.Duration
	mu         sync.Mutex
}

// StartMCP starts an in-memory MCP server with allow_me and deny_me tools.
func StartMCP(t *testing.T) *MCPEnv {
	t.Helper()
	env := &MCPEnv{}
	server := mcp.NewServer(&mcp.Implementation{Name: "primer-test-mcp", Version: "1.0.0"}, nil)

	server.AddTool(&mcp.Tool{
		Name:        "allow_me",
		Description: "Allowed tool for child scope tests",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"q": map[string]any{"type": "string"}},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		env.AllowCalls.Add(1)
		if env.BlockAllow > 0 {
			timer := time.NewTimer(env.BlockAllow)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				env.AllowSawCancel.Store(true)
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "allow-ok"}},
		}, nil
	})

	server.AddTool(&mcp.Tool{
		Name:        "deny_me",
		Description: "Denied tool for child scope tests",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"q": map[string]any{"type": "string"}},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		env.DenyCalls.Add(1)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "deny-ok"}},
		}, nil
	})

	env.Server = server
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("mcp server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	clientSession, err := mcptool.Connect(ctx, clientTransport)
	if err != nil {
		t.Fatalf("mcp client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	env.Client = clientSession
	return env
}

// AllTools lists MCP tools via MAF mcptool.
func (e *MCPEnv) AllTools(t *testing.T) []maftool.Tool {
	t.Helper()
	tools, err := mcptool.ListTools(context.Background(), e.Client)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return tools
}
