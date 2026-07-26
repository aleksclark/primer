// Command openapi-gen writes the OpenAPI 3.1 spec generated from the API's
// handler type signatures to a file (or stdout).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/aleksclark/primer/server/internal/api"
)

func main() {
	out := flag.String("out", "", "output file (default stdout)")
	flag.Parse()

	humaAPI, _ := api.New(nil, api.Options{})
	spec, err := humaAPI.OpenAPI().YAML()
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
	fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
}
