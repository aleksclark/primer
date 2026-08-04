package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestGoldenActivityJSON(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	// Drop hints from golden surface to keep stable optional field set.
	raw, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	raw = append(raw, '\n')

	path := filepath.Join("testdata", "activity_basic_navigation.v1.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, raw, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden; run with UPDATE_GOLDEN=1")

	var gotObj, wantObj any
	require.NoError(t, json.Unmarshal(raw, &gotObj))
	require.NoError(t, json.Unmarshal(want, &wantObj))
	assert.Equal(t, wantObj, gotObj)

	// Round-trip must still validate.
	var loaded contracts.ActivityDocument
	require.NoError(t, json.Unmarshal(want, &loaded))
	require.NoError(t, contracts.ValidateDocument(&loaded))
}

func TestGoldenObservationEventCompletion(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	obs := contracts.Observation{
		SchemaVersion: contracts.ObservationSchemaVersion,
		CheckID:       "welcome-exists",
		Kind:          contracts.CheckFileExists,
		Passed:        true,
		Message:       "exists",
		ObservedAt:    fixed,
		Details:       map[string]any{"path": "home/welcome.txt"},
	}
	ev := contracts.SessionEvent{
		SchemaVersion: contracts.EventSchemaVersion,
		EventID:       "11111111-1111-1111-1111-111111111111",
		Type:          contracts.EventCheckEvaluated,
		Sequence:      3,
		ClientTime:    fixed,
		Payload: map[string]any{
			"activityDigest":  "sha256:abc",
			"verifierVersion": "1",
			"observation":     obs,
		},
	}
	comp := contracts.CompletionRequest{
		SchemaVersion: contracts.CompletionSchemaVersion,
		CompletionID:  "22222222-2222-2222-2222-222222222222",
		RequestDigest: "sha256:def",
		Observations:  []contracts.Observation{obs},
		ClientTime:    fixed,
	}
	art := contracts.ArtifactMeta{
		SchemaVersion: contracts.ArtifactSchemaVersion,
		ArtifactID:    "33333333-3333-3333-3333-333333333333",
		Filename:      "notes.txt",
		MediaType:     "text/plain",
		ByteSize:      12,
		SHA256:        "sha256:aaa",
		CreatedAt:     fixed,
	}

	cases := []struct {
		name string
		file string
		v    any
	}{
		{"observation", "observation.v1.json", obs},
		{"event", "session_event.v1.json", ev},
		{"completion", "completion_request.v1.json", comp},
		{"artifact", "artifact_meta.v1.json", art},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.MarshalIndent(tc.v, "", "  ")
			require.NoError(t, err)
			raw = append(raw, '\n')
			path := filepath.Join("testdata", tc.file)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, raw, 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			var gotObj, wantObj any
			require.NoError(t, json.Unmarshal(raw, &gotObj))
			require.NoError(t, json.Unmarshal(want, &wantObj))
			assert.Equal(t, wantObj, gotObj)
		})
	}
}
