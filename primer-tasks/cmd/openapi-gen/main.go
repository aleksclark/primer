package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"primer-tasks/internal/api"
)

func main() {
	out := flag.String("out", "build/openapi.yaml", "offline YAML contract output path")
	jsonOut := flag.String("json-out", "", "optional deterministic JSON contract output path (defaults beside -out)")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		panic(err)
	}
	yaml := api.OpenAPI()
	if err := os.WriteFile(*out, []byte(yaml+"\n"), 0600); err != nil {
		panic(err)
	}
	if *jsonOut == "" {
		*jsonOut = filepath.Join(filepath.Dir(*out), "openapi.json")
	}
	if err := os.MkdirAll(filepath.Dir(*jsonOut), 0700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*jsonOut, []byte(api.OpenAPIJSON()+"\n"), 0600); err != nil {
		panic(err)
	}
	if flag.NArg() == 0 {
		fmt.Print(yaml + "\n")
	}
}
