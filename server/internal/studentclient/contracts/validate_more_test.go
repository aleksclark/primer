package contracts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestValidateAdditionalCheckKinds(t *testing.T) {
	t.Parallel()
	base := sampleTerminal()
	// content equals/contains/match + path mode + command properties + pipeline
	base.Content.Checks = []contracts.Check{
		{ID: "exists", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "home/welcome.txt"}},
		{ID: "missing", Kind: contracts.CheckFileNotExists, Params: map[string]any{"path": "home/nope.txt"}},
		{ID: "eq", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/welcome.txt", "value": "hello\n"}},
		{ID: "contains", Kind: contracts.CheckContentContains, Params: map[string]any{"path": "home/welcome.txt", "value": "he"}},
		{ID: "match", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/welcome.txt", "pattern": "^hello"}},
		{ID: "ptype", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home", "type": contracts.PathTypeDirectory}},
		{ID: "pmode", Kind: contracts.CheckPathMode, Params: map[string]any{"path": "home/welcome.txt", "mode": "0644"}},
		{ID: "cwd", Kind: contracts.CheckCwd, Params: map[string]any{"path": "home"}},
		{ID: "cmd", Kind: contracts.CheckCommandProperties, Params: map[string]any{
			"executable": "ls", "args": []any{"-la"}, "exitCode": 0,
		}},
		{ID: "pipe", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"contains": "x", "pattern": "x+"}},
	}
	base.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "Go",
		Completion: contracts.CheckTree{Any: []contracts.CheckTree{
			{CheckID: "exists"}, {CheckID: "eq"},
		}},
	}}
	require.NoError(t, contracts.ValidateDocument(base))

	// Reject bad command args type
	bad := sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badcmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "args": "not-a-list"},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Reject bad exitCode
	bad = sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badexit", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "exitCode": "nope"},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Reject invalid regex
	bad = sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badre", Kind: contracts.CheckContentMatch,
		Params: map[string]any{"path": "home/welcome.txt", "pattern": "("},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Reject path type unknown
	bad = sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badpt", Kind: contracts.CheckPathType,
		Params: map[string]any{"path": "home", "type": "socket"},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Reject bad mode
	bad = sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badmode", Kind: contracts.CheckPathMode,
		Params: map[string]any{"path": "home/welcome.txt", "mode": "999"},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Pipeline requires one of value/contains/pattern
	bad = sampleTerminal()
	bad.Content.Checks = append(bad.Content.Checks, contracts.Check{
		ID: "badpipe", Kind: contracts.CheckPipelineOutput, Params: map[string]any{},
	})
	require.Error(t, contracts.ValidateDocument(bad))

	// Typing metrics camelCase keys
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "typing-camel",
		Title:         "Camel",
		Kind:          contracts.KindTyping,
		SubjectCode:   "digital-literacy",
		Standards:     []contracts.StandardRef{{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary}},
		Content: contracts.ActivityContent{
			Objective: "o", Instructions: "i",
			Typing: &contracts.TypingContent{
				PromptSetID: "p",
				Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "hi"}},
			},
			Tasks: []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}},
			Checks: []contracts.Check{{
				ID: "m", Kind: contracts.CheckTypingMetrics,
				Params: map[string]any{"minWpm": 12.0, "minAccuracy": 0.9},
			}},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))

	// Negative wpm
	doc.Content.Checks[0].Params = map[string]any{"min_wpm": -1.0, "min_accuracy": 0.9}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_wpm")

	// ParseMode
	m, err := contracts.ParseMode("0644")
	require.NoError(t, err)
	assert.Equal(t, 0o644, int(m))
	_, err = contracts.ParseMode("xyz")
	require.Error(t, err)
}
