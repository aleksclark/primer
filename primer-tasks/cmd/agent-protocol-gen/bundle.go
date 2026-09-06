package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"primer-tasks/internal/api"
)

const bundleFormat = "primer-tasks/client-contract-v2"
const contractNormalization = "json-recursive-object-key-order-v1"

type bundleManifest struct {
	Format         string            `json:"format"`
	Files          map[string]string `json:"files"`
	ContractDigest string            `json:"contractDigest"`
	Normalization  string            `json:"normalization"`
}

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// All contracts are produced by the SAME compiled production API boundaries.
// This transport manifest defines no DTOs. Docker's source -> spec -> web-inputs
// COPY chain binds the bundle to current Go sources, not generated host files.
// Hashes detect damaged/mixed members (including a damaged manifest); they are
// integrity metadata, not a signature authenticating arbitrary external bundles.
func emitBundle(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Invalidate BEFORE deriving or writing any member, not after a partial
	// generation has already failed. No existing-file fallback is supported.
	if err := os.Remove(filepath.Join(dir, "manifest.sha256")); err != nil && !os.IsNotExist(err) {
		return err
	}
	schema, err := api.AgentSocketSchema()
	if err != nil {
		return err
	}
	student, err := api.StudentSocketSchema()
	if err != nil {
		return err
	}
	config, err := api.DialogueConfigSchema()
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"openapi.yaml":                 []byte(api.OpenAPI() + "\n"),
		"openapi.json":                 []byte(api.OpenAPIJSON() + "\n"),
		"agent-protocol.schema.json":   append(schema, '\n'),
		"agent-protocol.ts":            []byte(api.AgentSocketTypeScript()),
		"student-dialogue.schema.json": append(student, '\n'),
		"student-dialogue.ts":          []byte(api.StudentSocketTypeScript()),
		"dialogue-config.schema.json":  append(config, '\n'),
		"dialogue-config.ts":           []byte(api.DialogueConfigTypeScript()),
	}
	normalized, err := normalizedContracts(files)
	if err != nil {
		return err
	}
	manifest := bundleManifest{Format: bundleFormat, Files: map[string]string{}, ContractDigest: digest(normalized), Normalization: contractNormalization}
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

// Normalize only JSON object-key order/whitespace. Preserve array order,
// required/optional/null semantics, enum members, constraints and all metadata.
// Map marshaling recursively sorts keys; all four contracts come from Go code.
func normalizedContracts(files map[string][]byte) ([]byte, error) {
	values := map[string]any{}
	for _, name := range []string{"openapi.json", "agent-protocol.schema.json", "student-dialogue.schema.json", "dialogue-config.schema.json"} {
		var value any
		if err := json.Unmarshal(files[name], &value); err != nil {
			return nil, err
		}
		values[name] = value
	}
	return json.Marshal(values)
}
