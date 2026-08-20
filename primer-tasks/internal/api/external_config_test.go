package api

import "testing"

func TestExternalConfigRejectsMalformedAndIncompletePublicConfig(t *testing.T) {
	for _, raw := range []string{"{", `{}`, `{"verifierId":"v"}`, `{"verifierId":"v","capability":"response"}`, `{"verifierId":"v","capability":"response","schemaVersion":""}`} {
		if _, err := externalConfig([]byte(raw)); err == nil {
			t.Errorf("config accepted: %s", raw)
		}
	}
	config, err := externalConfig([]byte(`{"verifierId":"v","capability":"response","schemaVersion":"external_callback.v1","options":{"public":"ok"}}`))
	if err != nil || config.Options["public"] != "ok" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}

func TestContainsStringRequiresExactCatalogCapability(t *testing.T) {
	if !containsString([]string{"response", "image"}, "response") {
		t.Fatal("known capability not found")
	}
	if containsString([]string{"response", "image"}, "Response") || containsString(nil, "response") {
		t.Fatal("unsupported capability matched")
	}
}
