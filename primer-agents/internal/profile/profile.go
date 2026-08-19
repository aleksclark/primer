// Package profile defines server-owned execution profiles for primer-agents.
// Callers may name a profile in a create-run request but cannot supply agent
// depth, tool grants, child budgets, system prompt, provider config, or
// credentials. The service selects the spec; the request only names the mode.
package profile

import (
	"fmt"

	mafagent "github.com/microsoft/agent-framework-go/agent"

	agentruntime "github.com/aleksclark/primer/agents/runtime"
)

// Name enumerates allowed execution profile names.
type Name string

const (
	// Tutor is a standard tutoring profile.
	// Single-agent, no children, no MCP tools by default.
	Tutor Name = "tutor"
	// Admin is a privileged interactive profile with bounded child delegation.
	Admin Name = "admin"
	// Student is a sandboxed profile: max_children=0, no tools.
	// This invariant cannot be changed by any request field.
	Student Name = "student"
)

// Allowed lists the profiles callers may request. Unlisted names are rejected.
var Allowed = map[Name]bool{
	Tutor:   true,
	Admin:   true,
	Student: true,
}

// Spec carries the server-owned agent spec and the MAF agent config for a
// profile. The spec is frozen at startup; it is never influenced by request
// fields beyond the profile name itself.
type Spec struct {
	AgentSpec agentruntime.AgentSpec
	MAFConfig mafagent.Config
}

// Build returns the frozen server-owned spec for n, or an error if n is not
// a known allowed profile.
func Build(n Name) (Spec, error) {
	switch n {
	case Tutor:
		return Spec{
			AgentSpec: agentruntime.AgentSpec{
				Type:             "tutor",
				Name:             "Primer Tutor",
				Instructions:     "You are Primer, a patient and rigorous middle-school tutor.",
				MaxChildren:      0,
				MaxDepth:         0,
				MaxTotalChildren: 0,
			},
			MAFConfig: mafagent.Config{
				Name:        "primer-tutor",
				Description: "Primer tutoring agent",
			},
		}, nil

	case Admin:
		return Spec{
			AgentSpec: agentruntime.AgentSpec{
				Type:             "admin",
				Name:             "Primer Admin",
				Instructions:     "You are Primer Admin. You may delegate to specialist child agents.",
				MaxChildren:      3,
				MaxDepth:         1,
				MaxTotalChildren: 5,
			},
			MAFConfig: mafagent.Config{
				Name:        "primer-admin",
				Description: "Primer admin agent with bounded child delegation",
			},
		}, nil

	case Student:
		// Hard-coded invariant: max_children=0. This cannot be overridden by
		// any request field, HTTP header, or configuration value.
		return Spec{
			AgentSpec: agentruntime.AgentSpec{
				Type:             "student",
				Name:             "Primer Student",
				Instructions:     "You are Primer, a careful tutor for a student. Keep answers concise.",
				MaxChildren:      0,
				MaxDepth:         0,
				MaxTotalChildren: 0,
			},
			MAFConfig: mafagent.Config{
				Name:        "primer-student",
				Description: "Sandboxed student-facing agent (no child delegation)",
			},
		}, nil

	default:
		return Spec{}, fmt.Errorf("profile: unknown profile %q", n)
	}
}

// BuildAgent returns a new MAF agent for the given spec wired to the provider
// config. The provider is supplied by the worker; this package does not
// select providers or hold credentials.
func BuildAgent(spec Spec, providerCfg mafagent.ProviderConfig) *mafagent.Agent {
	cfg := spec.MAFConfig
	return mafagent.New(providerCfg, cfg)
}
