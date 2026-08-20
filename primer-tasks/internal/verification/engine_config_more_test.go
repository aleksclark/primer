package verification

import (
	"testing"

	"primer-tasks/internal/domain"
)

func TestRegistryValidatesExternalAndRejectsTypedConfigMismatches(t *testing.T) {
	r := NewRegistry()
	validExternal := map[string]any{"verifierId": "verifier", "capability": "response", "schemaVersion": ExternalCallbackSchemaVersion}
	if err := r.ValidateConfig(ExternalCallbackKind, ExternalCallbackConfigVersion, validExternal); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateConfig(ExternalCallbackKind, ExternalCallbackConfigVersion+1, validExternal); err == nil {
		t.Fatal("wrong external schema accepted")
	}
	if err := r.ValidateConfig(ExternalCallbackKind, ExternalCallbackConfigVersion, map[string]any{"verifierId": "verifier"}); err == nil {
		t.Fatal("incomplete external config accepted")
	}
	if err := r.ValidateConfig(domain.AgentDialogueKind, domain.AgentDialogueConfigVersion, map[string]any{}); err == nil {
		t.Fatal("incomplete dialogue config accepted")
	}
}
