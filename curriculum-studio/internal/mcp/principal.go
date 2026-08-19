package mcp

import "github.com/aleksclark/primer/curriculum-studio/internal/authn"

// Principal is the validated token identity passed to MCP tool handlers.
// It is a type alias for authn.AuthContext so *authn.Validator directly
// satisfies TokenValidator without a wrapper.
type Principal = authn.AuthContext

// Compile-time assertion: *authn.Validator satisfies TokenValidator.
var _ TokenValidator = (*authn.Validator)(nil)
