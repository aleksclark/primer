// Package agent is a temporary LMS compatibility seam for the canonical
// standalone Primer Agents agentruntime. New runtime consumers must import
// github.com/aleksclark/primer/agents/runtime directly.
package agent

import (
	agentruntime "github.com/aleksclark/primer/agents/runtime"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	maftool "github.com/microsoft/agent-framework-go/tool"
)

type AgentSpec = agentruntime.AgentSpec
type ChildSpec = agentruntime.ChildSpec
type Orchestration = agentruntime.Orchestration
type RunEvent = agentruntime.RunEvent
type EventSink = agentruntime.EventSink
type NoopSink = agentruntime.NoopSink
type StreamingChildTool = agentruntime.StreamingChildTool
type RunHandle = agentruntime.RunHandle
type SSEHandler = agentruntime.SSEHandler
type SSEBridge = agentruntime.SSEBridge
type BoundedSink = agentruntime.BoundedSink
type CollectingSink = agentruntime.CollectingSink
type Runner = agentruntime.Runner
type ScriptedProvider = agentruntime.ScriptedProvider
type RunState = agentruntime.RunState
type RunSnapshot = agentruntime.RunSnapshot
type Controller = agentruntime.Controller
type ControllerConfig = agentruntime.ControllerConfig

const (
	KindStart                = agentruntime.KindStart
	KindText                 = agentruntime.KindText
	KindToolStart            = agentruntime.KindToolStart
	KindToolEnd              = agentruntime.KindToolEnd
	KindChildStart           = agentruntime.KindChildStart
	KindChildEnd             = agentruntime.KindChildEnd
	KindError                = agentruntime.KindError
	KindEnd                  = agentruntime.KindEnd
	RunStarting              = agentruntime.RunStarting
	RunRunning               = agentruntime.RunRunning
	RunSucceeded             = agentruntime.RunSucceeded
	RunFailed                = agentruntime.RunFailed
	RunCanceled              = agentruntime.RunCanceled
	DefaultSSEBridgeCapacity = agentruntime.DefaultSSEBridgeCapacity
	DefaultRunBudget         = agentruntime.DefaultRunBudget
)

var ErrRunNotFound = agentruntime.ErrRunNotFound

func NewStreamingChildTool(child *mafagent.Agent, childType, parentRunID string, sink EventSink, runOpts ...mafagent.Option) *StreamingChildTool {
	return agentruntime.NewStreamingChildTool(child, childType, parentRunID, sink, runOpts...)
}
func NewSSEBridge(capacity int) *SSEBridge           { return agentruntime.NewSSEBridge(capacity) }
func NewBoundedSink(capacity int) *BoundedSink       { return agentruntime.NewBoundedSink(capacity) }
func NewController(cfg ControllerConfig) *Controller { return agentruntime.NewController(cfg) }
func NewRunner(spec AgentSpec, a *mafagent.Agent, sink EventSink) *Runner {
	return agentruntime.NewRunner(spec, a, sink)
}
func NewScriptedAgent(cfg mafagent.Config, prov *ScriptedProvider) *mafagent.Agent {
	return agentruntime.NewScriptedAgent(cfg, prov)
}
func SetRunIDGenerator(fn func() string) { agentruntime.SetRunIDGenerator(fn) }
func FilterToolsFailClosed(available []maftool.Tool, allowlist []string) []maftool.Tool {
	return agentruntime.FilterToolsFailClosed(available, allowlist)
}
func IntersectToolNames(parent []maftool.Tool, request []string) []string {
	return agentruntime.IntersectToolNames(parent, request)
}
func AssertNoAuthorityExpansion(parentTools, childTools []maftool.Tool) error {
	return agentruntime.AssertNoAuthorityExpansion(parentTools, childTools)
}
func ToolNames(tools []maftool.Tool) []string { return agentruntime.ToolNames(tools) }
