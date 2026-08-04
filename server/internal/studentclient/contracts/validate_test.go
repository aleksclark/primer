package contracts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func sampleTerminal() *contracts.ActivityDocument {
	return &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "basic-navigation",
		Title:         "Basic Navigation",
		Summary:       "Learn ls, cd, pwd",
		Kind:          contracts.KindTerminal,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary, Weight: 1},
		},
		Content: contracts.ActivityContent{
			Objective:    "Navigate a small directory tree",
			Instructions: "Explore the workspace with ls, cd, and pwd.",
			Terminal: &contracts.TerminalContent{
				RuntimeProfile: contracts.RuntimeCoreutilsBasic,
				InitialCwd:     "home",
				Fixtures: []contracts.FixtureEntry{
					{Path: "home", Type: contracts.FixtureDirectory},
					{Path: "home/welcome.txt", Type: contracts.FixtureFile, Content: "hello\n"},
					{Path: "home/docs", Type: contracts.FixtureDirectory},
					{Path: "home/docs/guide.txt", Type: contracts.FixtureFile, Content: "read me\n", Mode: "0644"},
				},
			},
			Tasks: []contracts.Task{
				{
					ID:           "list-home",
					Title:        "List home",
					Instructions: "List the files in home.",
					Completion:   contracts.CheckTree{CheckID: "welcome-exists"},
				},
				{
					ID:            "enter-docs",
					Title:         "Enter docs",
					Instructions:  "Change into home/docs.",
					Prerequisites: []string{"list-home"},
					Completion: contracts.CheckTree{
						All: []contracts.CheckTree{
							{CheckID: "in-docs"},
							{CheckID: "guide-is-file", Optional: true},
						},
					},
				},
			},
			Checks: []contracts.Check{
				{
					ID:   "welcome-exists",
					Kind: contracts.CheckFileExists,
					Params: map[string]any{
						"path": "home/welcome.txt",
					},
				},
				{
					ID:   "in-docs",
					Kind: contracts.CheckCwd,
					Params: map[string]any{
						"path": "home/docs",
					},
				},
				{
					ID:   "guide-is-file",
					Kind: contracts.CheckPathType,
					Params: map[string]any{
						"path": "home/docs/guide.txt",
						"type": contracts.PathTypeFile,
					},
				},
			},
			Hints: []contracts.Hint{
				{ID: "hint-ls", Level: 1, Text: "Try the ls command."},
			},
		},
	}
}

func TestValidateDocumentOK(t *testing.T) {
	t.Parallel()
	require.NoError(t, contracts.ValidateDocument(sampleTerminal()))
}

func TestValidateRejectsUnsafeCheckPath(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Checks[0].Params["path"] = "../etc/passwd"
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe path")
}

func TestValidateRejectsAbsoluteFixturePath(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Terminal.Fixtures[0].Path = "/tmp/x"
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "relative")
}

func TestValidateRejectsUnknownCheckKind(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID:     "evil",
		Kind:   "shell_eval",
		Params: map[string]any{"script": "rm -rf /"},
	})
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestValidateRejectsBadCheckTree(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{CheckID: "missing"}
	require.Error(t, contracts.ValidateDocument(doc))
}

func TestValidateRejectsTaskCycle(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Tasks[0].Prerequisites = []string{"enter-docs"}
	doc.Content.Tasks[1].Prerequisites = []string{"list-home"}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestValidateTyping(t *testing.T) {
	t.Parallel()
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "typing-home-row",
		Title:         "Home Row",
		Kind:          contracts.KindTyping,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary},
		},
		Content: contracts.ActivityContent{
			Objective:    "Type accurately",
			Instructions: "Type the prompts.",
			Typing: &contracts.TypingContent{
				PromptSetID: "home-row-1",
				Prompts: []contracts.TypingPrompt{
					{ID: "p1", Text: "asdf jkl;"},
				},
			},
			Tasks: []contracts.Task{
				{
					ID:           "finish",
					Title:        "Finish",
					Instructions: "Complete prompts",
					Completion:   contracts.CheckTree{CheckID: "done"},
				},
			},
			Checks: []contracts.Check{
				{
					ID:   "done",
					Kind: contracts.CheckPipelineOutput,
					Params: map[string]any{
						"contains": "complete",
					},
				},
			},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))
}

func TestValidateRejectsWrongSchema(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.SchemaVersion = "999"
	require.Error(t, contracts.ValidateDocument(doc))
}
