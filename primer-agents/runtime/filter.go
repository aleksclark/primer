package agentruntime

import (
	"fmt"
	"slices"

	maftool "github.com/microsoft/agent-framework-go/tool"
)

// FilterToolsFailClosed returns tools whose Name() is in allowlist.
// Unknown allowlist entries are silently ignored. Empty allowlist returns nil.
// Never invents tools not present in available.
func FilterToolsFailClosed(available []maftool.Tool, allowlist []string) []maftool.Tool {
	if len(allowlist) == 0 || len(available) == 0 {
		return nil
	}
	allow := make(map[string]struct{}, len(allowlist))
	for _, n := range allowlist {
		if n != "" {
			allow[n] = struct{}{}
		}
	}
	out := make([]maftool.Tool, 0, len(allowlist))
	seen := make(map[string]struct{})
	for _, t := range available {
		if t == nil {
			continue
		}
		name := t.Name()
		if _, ok := allow[name]; !ok {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, t)
	}
	return out
}

// IntersectToolNames returns sorted unique names present in both parent grants
// and the request list.
func IntersectToolNames(parent []maftool.Tool, request []string) []string {
	parentSet := make(map[string]struct{}, len(parent))
	for _, t := range parent {
		if t != nil {
			parentSet[t.Name()] = struct{}{}
		}
	}
	out := make([]string, 0, len(request))
	seen := make(map[string]struct{})
	for _, n := range request {
		if _, ok := parentSet[n]; !ok {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// AssertNoAuthorityExpansion fails if childTools contains a name absent from
// parentTools. This is a compile-time safety net; runtime allowlist checks are
// the actual enforcement path.
func AssertNoAuthorityExpansion(parentTools, childTools []maftool.Tool) error {
	parent := make(map[string]struct{}, len(parentTools))
	for _, t := range parentTools {
		if t != nil {
			parent[t.Name()] = struct{}{}
		}
	}
	for _, t := range childTools {
		if t == nil {
			continue
		}
		if _, ok := parent[t.Name()]; !ok {
			return fmt.Errorf("authority expansion: child tool %q not in parent grants", t.Name())
		}
	}
	return nil
}

// ToolNames extracts sorted tool names from a slice.
func ToolNames(tools []maftool.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t != nil {
			out = append(out, t.Name())
		}
	}
	slices.Sort(out)
	return out
}
