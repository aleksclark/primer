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
	// Tutor is a standard interactive tutoring profile.
	// Single-agent, no children, no MCP tools by default.
	Tutor Name = "tutor"
	// Admin is a privileged interactive profile with bounded child delegation.
	Admin Name = "admin"
	// Student is the sandboxed student tutoring profile.
	// Invariants are code-level, never configurable:
	//   MaxChildren=0, MaxDepth=0, no child factory, empty tool grant.
	// Server always selects this; callers cannot request it directly.
	Student Name = "student"
	// Job is the machine/scheduled-job profile: bounded single-agent, no tools.
	Job Name = "job"
)

// Allowed lists the profiles that authenticated callers may request via the
// parent/admin routes. Student and Job are always server-selected.
var Allowed = map[Name]bool{
	Tutor: true,
	Admin: true,
	// Student and Job are NOT in the caller-requestable set; they are
	// selected unconditionally by their dedicated routes.
}

// Spec carries the server-owned agent spec and MAF agent config.
type Spec struct {
	AgentSpec agentruntime.AgentSpec
	MAFConfig mafagent.Config
}

// Build returns the frozen server-owned spec for n.
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
		// Hard-coded invariants — never changed by configuration or request:
		//   MaxChildren=0  : no child agents, ever
		//   MaxDepth=0     : no delegation depth
		//   MaxTotalChildren=0 : total tree budget also zero
		//   Tools=nil      : empty tool grant by default
		// Any code path that tries to override these must fail at Build time.
		return Spec{
			AgentSpec: agentruntime.AgentSpec{
				Type:             "student",
				Name:             "Primer Student",
				Instructions:     "You are Primer, a careful tutor for a student. Keep answers concise and age-appropriate.",
				MaxChildren:      0, // INVARIANT: immutable
				MaxDepth:         0, // INVARIANT: immutable
				MaxTotalChildren: 0, // INVARIANT: immutable
				Tools:            nil,
			},
			MAFConfig: mafagent.Config{
				Name:        "primer-student",
				Description: "Sandboxed student-facing agent (no child delegation, no tools)",
			},
		}, nil

	case Job:
		return Spec{
			AgentSpec: agentruntime.AgentSpec{
				Type:             "job",
				Name:             "Primer Job",
				Instructions:     "You are Primer Job. Execute the scheduled task precisely and completely.",
				MaxChildren:      0,
				MaxDepth:         0,
				MaxTotalChildren: 0,
			},
			MAFConfig: mafagent.Config{
				Name:        "primer-job",
				Description: "Machine/scheduled job agent",
			},
		}, nil

	default:
		return Spec{}, fmt.Errorf("profile: unknown profile %q", n)
	}
}

// BuildAgent returns a MAF agent for the given spec and provider config.
func BuildAgent(spec Spec, providerCfg mafagent.ProviderConfig) *mafagent.Agent {
	return mafagent.New(providerCfg, spec.MAFConfig)
}

// ValidateStudentSpec asserts that the student spec invariants are intact.
// Called at startup and before every student run; any violation is fatal.
func ValidateStudentSpec(spec Spec) error {
	as := spec.AgentSpec
	if as.MaxChildren != 0 {
		return fmt.Errorf("student invariant violation: MaxChildren=%d, must be 0", as.MaxChildren)
	}
	if as.MaxDepth != 0 {
		return fmt.Errorf("student invariant violation: MaxDepth=%d, must be 0", as.MaxDepth)
	}
	if as.MaxTotalChildren != 0 {
		return fmt.Errorf("student invariant violation: MaxTotalChildren=%d, must be 0", as.MaxTotalChildren)
	}
	if len(as.Tools) != 0 {
		return fmt.Errorf("student invariant violation: non-empty tool grant (len=%d)", len(as.Tools))
	}
	return nil
}

// init validates student spec at package load so misconfiguration fails early.
func init() {
	s, err := Build(Student)
	if err != nil {
		panic(fmt.Sprintf("profile: failed to build student spec: %v", err))
	}
	if err := ValidateStudentSpec(s); err != nil {
		panic(fmt.Sprintf("profile: student invariant check failed at init: %v", err))
	}
}
