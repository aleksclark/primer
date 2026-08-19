package mcp

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MCPConfig holds all MCP-specific runtime configuration.
// Fields are populated from the STUDIO_ environment prefix by the parent
// config.Config struct.
type MCPConfig struct {
	// Enabled controls whether the /mcp route is registered at startup.
	// Default: true in non-production environments; must be explicit in production.
	Enabled bool

	// OriginAllowlist is a comma-separated list of allowed Origin header values
	// for browser-initiated requests. Empty string disables browser-origin
	// checking (loopback-only environments). In production, a non-empty allowlist
	// is enforced by config validation.
	OriginAllowlist string

	// MaxBodyBytes caps the JSON-RPC request body size. 0 → SDK default (4 MiB).
	MaxBodyBytes int64

	// MaxConcurrent is the maximum number of concurrent in-flight MCP requests.
	// 0 → no explicit limit beyond Go runtime scheduling.
	MaxConcurrent int

	// RequestTimeout caps each tool invocation. 0 → no per-request timeout.
	RequestTimeout time.Duration

	// SupportedProtocolVersions is the set of MCP protocol revision strings
	// this server accepts. Must include "2026-07-28".
	// If empty the default {"2026-07-28"} is used.
	SupportedProtocolVersions string
}

// ProtocolVersion is the pinned MCP spec revision implemented by this package.
const ProtocolVersion = "2026-07-28"

// AllowedOrigins parses OriginAllowlist into a deduplicated, trimmed slice.
// An empty allowlist returns nil (no browser-origin restriction in dev/test).
func (c MCPConfig) AllowedOrigins() []string {
	if c.OriginAllowlist == "" {
		return nil
	}
	parts := strings.Split(c.OriginAllowlist, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !utf8.ValidString(p) {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// Validate checks that the MCPConfig is coherent. Production never permits an
// enabled endpoint with an unbounded body or an empty browser-origin policy.
func (c MCPConfig) Validate(env string) error {
	env = strings.ToLower(strings.TrimSpace(env))
	if c.MaxBodyBytes < 0 {
		return fmt.Errorf("mcp max body bytes cannot be negative")
	}
	if c.MaxConcurrent < 0 {
		return fmt.Errorf("mcp max concurrent cannot be negative")
	}
	if c.RequestTimeout < 0 {
		return fmt.Errorf("mcp request timeout cannot be negative")
	}
	if env == "production" && c.Enabled && len(c.AllowedOrigins()) == 0 {
		return fmt.Errorf("mcp origin allowlist is required in production")
	}
	return nil
}
