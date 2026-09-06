package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"primer-tasks/internal/api"
)

const bundleFormat = "primer-tasks/client-contract-v1"

type bundleManifest struct {
	Format string            `json:"format"`
	Files  map[string]string `json:"files"`
}

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// All contracts are produced by the SAME compiled production API boundaries.
// This transport manifest defines no DTOs. Docker's source -> spec -> web-inputs
// COPY chain binds the bundle to current Go sources, not generated host files.
// Hashes detect damaged/mixed members (including a damaged manifest); they are
// integrity metadata, not a signature authenticating arbitrary external bundles.
func emitBundle(dir string) error {
	schema, err := api.AgentSocketSchema()
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"openapi.yaml":               []byte(api.OpenAPI() + "\n"),
		"agent-protocol.schema.json": append(schema, '\n'),
		"agent-protocol.ts":          []byte(api.AgentSocketTypeScript()),
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Remove the completion marker first: a failed emission is never consumable
	// as a previous generation merely because some old files still exist.
	if err = os.Remove(filepath.Join(dir, "manifest.sha256")); err != nil && !os.IsNotExist(err) {
		return err
	}
	manifest := bundleManifest{Format: bundleFormat, Files: map[string]string{}}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			return err
		}
		manifest.Files[name] = digest(data)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.sha256"), []byte(digest(data)+"\n"), 0644)
}
