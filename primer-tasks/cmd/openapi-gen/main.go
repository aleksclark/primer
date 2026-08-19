package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"primer-tasks/internal/api"
)

func main() {
	out := flag.String("out", "build/openapi.yaml", "offline contract output path")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*out), 0700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, []byte(api.OpenAPI()+"\n"), 0600); err != nil {
		panic(err)
	}
	if flag.NArg() == 0 {
		fmt.Print(api.OpenAPI() + "\n")
	}
}
