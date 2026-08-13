package activities_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestTerminalRunnerMarkResponseSubmitted(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "home"), 0o755))

	r := activities.NewTerminal()
	err := r.Open(context.Background(), activities.OpenOpts{
		Content: contracts.ActivityContent{
			Objective: "o",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				InitialCwd:     "home",
			},
			Tasks: []contracts.Task{{
				ID: "reflect", Title: "R", Instructions: "write",
				Kind: contracts.TaskKindShortResponse,
				Response: &contracts.ResponseTaskSpec{
					Prompt: "what?", MaxChars: 100, ParentReviewRequired: true,
				},
				Completion: contracts.CheckTree{CheckID: "c-resp"},
			}},
			Checks: []contracts.Check{{
				ID: "c-resp", Kind: contracts.CheckResponseSubmitted,
				Params: map[string]any{"taskId": "reflect"},
			}},
		},
		Digest:          "d-resp",
		Workspace:       ws,
		SkipMaterialize: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// Empty before mark.
	assert.Empty(t, r.SubmittedTasks())
	// Empty task id is ignored.
	r.MarkResponseSubmitted("")
	assert.Empty(t, r.SubmittedTasks())

	r.MarkResponseSubmitted("reflect")
	got := r.SubmittedTasks()
	assert.True(t, got["reflect"])
	// Snapshot copy is independent.
	got["other"] = true
	_, hasOther := r.SubmittedTasks()["other"]
	assert.False(t, hasOther)

	require.NoError(t, r.Verify(context.Background()))
	assert.True(t, r.CompleteReady())
}
