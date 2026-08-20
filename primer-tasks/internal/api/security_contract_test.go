package api

import (
	"strings"
	"testing"
)

// The generated contract is a security boundary as well as a type boundary.
// This catches accidental promotion of provider internals into REST/WS schema
// before the TypeScript generator can make them available to pages.
func TestGeneratedOpenAPIDoesNotExposeProviderInternals(t *testing.T) {
	doc := strings.ToLower(OpenAPI())
	for _, forbidden := range []string{
		"reasoning_delta",
		"provider_metadata",
		"raw_prompt",
		"tool_input",
		"tool_arguments",
		"authorization",
		"x-amz-signature",
		"s3://",
		"minio://",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("OpenAPI exposes protected field %q", forbidden)
		}
	}
}
