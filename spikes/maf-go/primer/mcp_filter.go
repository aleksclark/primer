package primer

import (
	"fmt"
	"slices"

	"github.com/microsoft/agent-framework-go/tool"
)

// FilterToolsFailClosed returns tools whose Name() is in allowlist.
// Unknown allowlist entries are ignored. Empty allowlist => no tools.
// Never invents tools outside available.
func FilterToolsFailClosed(available []tool.Tool, allowlist []string) []tool.Tool {
	if len(allowlist) == 0 || len(available) == 0 {
		return nil
	}
	allow := make(map[string]struct{}, len(allowlist))
	for _, n := range allowlist {
		if n == "" {
			continue
		}
		allow[n] = struct{}{}
	}
	out := make([]tool.Tool, 0, len(allowlist))
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

// IntersectToolNames returns sorted unique names present in both parent grants and request.
func IntersectToolNames(parent []tool.Tool, request []string) []string {
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

// AssertNoAuthorityExpansion fails if childTools contains a name absent from parentTools.
func AssertNoAuthorityExpansion(parentTools, childTools []tool.Tool) error {
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

// ToolNames extracts names from tools.
func ToolNames(tools []tool.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t != nil {
			out = append(out, t.Name())
		}
	}
	slices.Sort(out)
	return out
}
