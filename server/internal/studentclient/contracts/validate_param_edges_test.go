package contracts_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestTypingMetricsParamCoercionAndPipelineArgs(t *testing.T) {
	t.Parallel()

	// int64 / float32 / json.Number / string coercions for typing metrics + command exitCode.
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion, Slug: "type-coerce", Title: "T",
		Kind: contracts.KindTyping, SubjectCode: "digital-literacy",
		Standards: []contracts.StandardRef{{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary}},
		Content: contracts.ActivityContent{
			Objective: "o", Instructions: "i",
			Typing: &contracts.TypingContent{PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}},
			Tasks:  []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}},
			Checks: []contracts.Check{{
				ID: "m", Kind: contracts.CheckTypingMetrics,
				Params: map[string]any{
					"minWpm":      json.Number("12.5"),
					"minAccuracy": float32(0.9),
				},
			}},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))

	// Negative wpm rejected after coercion.
	doc.Content.Checks[0].Params = map[string]any{"min_wpm": -1.0, "min_accuracy": 0.5}
	require.Error(t, contracts.ValidateDocument(doc))

	// Command args as []string + exitCode string.
	term := sampleTerminal()
	term.Content.Checks = append(term.Content.Checks, contracts.Check{
		ID: "cmd-str", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": []string{"-l"}, "exitCode": "0"},
	})
	require.NoError(t, contracts.ValidateDocument(term))

	// Command args non-string element.
	term = sampleTerminal()
	term.Content.Checks = append(term.Content.Checks, contracts.Check{
		ID: "cmd-bad", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": []any{1}},
	})
	require.Error(t, contracts.ValidateDocument(term))

	// Pipeline with only contains; content_not_exists path kind.
	term = sampleTerminal()
	term.Content.Checks = append(term.Content.Checks,
		contracts.Check{ID: "po", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "x"}},
		contracts.Check{ID: "ne", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "home/gone.txt"}},
		contracts.Check{ID: "cc", Kind: contracts.CheckContentContains, Params: map[string]any{"path": "home/welcome.txt", "value": "hi"}},
	)
	require.NoError(t, contracts.ValidateDocument(term))

	// Nil params on a check that requires path still fails (nil becomes empty map).
	term = sampleTerminal()
	term.Content.Checks = append(term.Content.Checks, contracts.Check{ID: "np", Kind: contracts.CheckFileExists, Params: nil})
	require.Error(t, contracts.ValidateDocument(term))

	// Public TypingMin helpers.
	_, err := contracts.TypingMinWPM(nil)
	require.Error(t, err)
	_, err = contracts.TypingMinAccuracy(nil)
	require.Error(t, err)
	w, err := contracts.TypingMinWPM(map[string]any{"minWpm": 3})
	require.NoError(t, err)
	assert.Equal(t, 3.0, w)
	a, err := contracts.TypingMinAccuracy(map[string]any{"minAccuracy": "0.8"})
	require.NoError(t, err)
	assert.InDelta(t, 0.8, a, 1e-9)

	// Bad float types.
	_, err = contracts.TypingMinWPM(map[string]any{"min_wpm": true})
	require.Error(t, err)
}

func TestCheckTreeNestedAllAny(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	// sampleTerminal already defines welcome-exists + in-docs.
	doc.Content.Tasks[0].Completion = contracts.CheckTree{
		All: []contracts.CheckTree{
			{CheckID: "welcome-exists"},
			{Any: []contracts.CheckTree{{CheckID: "in-docs"}, {CheckID: "welcome-exists"}}},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))
}
