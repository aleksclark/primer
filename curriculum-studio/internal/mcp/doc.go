// Package mcp mounts the Curriculum Studio Streamable HTTP MCP endpoint at /mcp.
//
// # Qualification evidence — S19 spike PROCEED
//
// SDK pin: github.com/modelcontextprotocol/go-sdk v1.7.0
// Protocol revision: 2026-07-28 (spec string "2026-07-28")
//
// Confirmed exact API names from module source:
//   - mcp.NewStreamableHTTPHandler(getServer func(*http.Request) *mcp.Server,
//     opts *mcp.StreamableHTTPOptions) *mcp.StreamableHTTPHandler
//   - mcp.StreamableHTTPOptions{Stateless bool, JSONResponse bool,
//     MaxRequestBodyBytes int64, PropagateRequestCancellation bool,
//     CrossOriginProtection *http.CrossOriginProtection}
//   - mcp.NewServer(impl *mcp.Implementation, options *mcp.ServerOptions) *mcp.Server
//   - mcp.AddTool[In, Out any](s *mcp.Server, t *mcp.Tool,
//     h mcp.ToolHandlerFor[In, Out])
//   - mcp.ToolHandlerFor[In, Out any] func(ctx context.Context,
//     req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error)
//   - mcp.CallToolResult{Content []mcp.Content, StructuredContent any, IsError bool}
//   - mcp.TextContent{Text string}
//   - mcp.Tool{Name, Description, InputSchema, OutputSchema any}
//   - mcp.Implementation{Name, Version string}
//   - mcp.ServerOptions{Capabilities *mcp.ServerCapabilities}
//   - mcp.ServerCapabilities{Tools *mcp.ToolCapabilities}
//   - protocolVersion20260728 = "2026-07-28" (internal const; latest default)
//   - Stateless mode: GET and DELETE return 405; each POST is an independent
//     session-free transaction matching the 2026-07-28 transport requirement.
//   - PropagateRequestCancellation: true propagates HTTP request context
//     (including values set by auth middleware) to tool handlers.
//
// PROCEED — Stateless mode confirmed available on v1.7.0 pin.
// All required server-side APIs present; no custom transport needed.
package mcp
