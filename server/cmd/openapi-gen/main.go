// Command openapi-gen writes the OpenAPI 3.1 spec generated from an API's
// handler type signatures to a file (or stdout).
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/api"
	tvapi "github.com/aleksclark/primer/server/internal/tv/api"
)

// builders maps a service name to its API constructor. Specs are generated
// without a database or media client; only handler signatures matter.
var builders = map[string]func() (huma.API, http.Handler){
	"lms": func() (huma.API, http.Handler) { return api.New(nil, api.Options{}) },
	"tv":  func() (huma.API, http.Handler) { return tvapi.New(nil, tvapi.Options{}) },
}

func main() {
	out := flag.String("out", "", "output file (default stdout)")
	service := flag.String("service", "lms", "service to generate the spec for (lms or tv)")
	flag.Parse()

	build, ok := builders[*service]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown service %q: want lms or tv\n", *service)
		os.Exit(2)
	}

	humaAPI, _ := build()
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
