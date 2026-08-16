// Command openapi-gen writes the Identity IB1 OpenAPI 3.1 spec generated from
// handler type signatures. Generation is offline: no database, provider, or
// network is used.
//
// Usage:
//
//	go run ./cmd/openapi-gen                 # write to stdout
//	go run ./cmd/openapi-gen -out PATH       # write PATH with mode 0644
//
// To update the committed baseline:
//
//	go run ./cmd/openapi-gen -out openapi.yaml
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aleksclark/primer/identity/internal/api"
)

func main() {
	out := flag.String("out", "", "output file (default stdout); use -out openapi.yaml to update the committed IB1 baseline")
	flag.Parse()

	spec, err := api.GenerateOpenAPIYAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate spec: %v\n", err)
		os.Exit(1)
	}

	if *out == "" {
		fmt.Print(string(spec))
		return
	}
	if err := os.WriteFile(*out, spec, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write spec: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chmod(*out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "chmod spec: %v\n", err)
		os.Exit(1)
	}
}
