// Command agent-protocol-gen emits the offline WebSocket contract from the
// Go tagged-union source. Outputs are build artifacts and are never tracked.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"primer-tasks/internal/api"
)

func main() {
	out := flag.String("out", "build/agent-protocol.schema.json", "output path")
	types := flag.String("typescript", "clients/typescript/generated/agent-protocol.ts", "TypeScript output path")
	flag.Parse()
	data, err := api.AgentSocketSchema()
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(filepath.Dir(*types), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*types, []byte(api.AgentSocketTypeScript()), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("generated %s and %s\n", *out, *types)
}
