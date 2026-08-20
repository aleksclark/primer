package api

import "testing"

func TestSafeExternalProgressProjectsOnlySafeTypedMessages(t *testing.T) {
	cases := []struct {
		kind      string
		status    string
		message   string
		rationale string
		want      bool
	}{
		{"verification.requested", "queued", "Verification requested.", "", true},
		{"external.progress", "progress", "External verification is in progress.", "", true},
		{"external.accepted", "accepted", "The external verifier accepted the response.", "The external verifier accepted the response.", true},
		{"external.rejected", "rejected", "The external verifier rejected the response.", "The external verifier rejected the response.", true},
		{"external.retryable_error", "", "", "", false},
		{"unknown", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			item, rationale := safeExternalProgress(7, tc.kind, []byte(`{"rationale":"do not expose this"}`))
			if (item != nil) != tc.want {
				t.Fatalf("item=%v want present=%v", item, tc.want)
			}
			if !tc.want {
				return
			}
			if item["sequence"] != int64(7) || item["status"] != tc.status || item["message"] != tc.message {
				t.Fatalf("safe item=%v", item)
			}
			if rationale != tc.rationale {
				t.Fatalf("rationale=%q want %q", rationale, tc.rationale)
			}
			if item["rationale"] != nil {
				t.Fatal("raw rationale leaked into safe projection")
			}
		})
	}
}
