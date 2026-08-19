// Command openapi-gen emits the primer-agents OpenAPI 3.1 specification
// without binding a listener or connecting to PostgreSQL.
//
// Usage:
//
//	go run ./cmd/openapi-gen [-out openapi.yaml]
//
// The spec is derived from production handler signatures via Huma.
// Run via: make openapi
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aleksclark/primer/agents/internal/api"
)

func main() {
	out := flag.String("out", "", "output file path (default: stdout)")
	format := flag.String("format", "yaml", "output format: yaml (3.1) or yaml30 (3.0 downgrade for codegen tools)")
	flag.Parse()

	humaAPI := api.NewSpec(api.Options{})

	var spec []byte
	var err error
	if *format == "yaml30" {
		spec, err = humaAPI.OpenAPI().DowngradeYAML()
	} else {
		spec, err = humaAPI.OpenAPI().YAML()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: generate spec: %v\n", err)
		os.Exit(1)
	}
	if *out == "" {
		fmt.Print(string(spec))
		return
	}
	if err := os.WriteFile(*out, spec, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "openapi-gen: wrote %s (%d bytes)\n", *out, len(spec))
}
