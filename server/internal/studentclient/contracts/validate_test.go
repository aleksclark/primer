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
					Kind: contracts.CheckTypingMetrics,
					Params: map[string]any{
						"min_wpm":      20.0,
						"min_accuracy": 0.95,
					},
				},
			},
		},
	}
	require.NoError(t, contracts.ValidateDocument(doc))
}

func TestValidateTypingMetricsRejectsBadAccuracy(t *testing.T) {
	t.Parallel()
	doc := &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "typing-bad",
		Title:         "Bad",
		Kind:          contracts.KindTyping,
		SubjectCode:   "digital-literacy",
		Standards: []contracts.StandardRef{
			{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary},
		},
		Content: contracts.ActivityContent{
			Objective:    "Type",
			Instructions: "Type",
			Typing: &contracts.TypingContent{
				PromptSetID: "x",
				Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "hi"}},
			},
			Tasks: []contracts.Task{{
				ID: "t", Title: "T", Instructions: "Go",
				Completion: contracts.CheckTree{CheckID: "m"},
			}},
			Checks: []contracts.Check{{
				ID:   "m",
				Kind: contracts.CheckTypingMetrics,
				Params: map[string]any{
					"min_wpm":      10.0,
					"min_accuracy": 1.5,
				},
			}},
		},
	}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min_accuracy")
}

func TestValidateRejectsWrongSchema(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.SchemaVersion = "999"
	require.Error(t, contracts.ValidateDocument(doc))
}

func TestValidateInstructionBlocksAndResponseTask(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{
		{ID: "intro", Kind: contracts.BlockProse, Text: "The terminal displays text."},
		{ID: "vocab", Kind: contracts.BlockVocabulary, Terms: []contracts.VocabularyTerm{
			{Term: "shell", Definition: "interprets command lines"},
		}},
		{ID: "ex", Kind: contracts.BlockExample, Input: "pwd", Output: "/home/student", Explanation: "path of cwd"},
		{ID: "warn", Kind: contracts.BlockWarning, Text: "Do not type the prompt symbol."},
		{ID: "parent", Kind: contracts.BlockParentNote, Text: "Ask for oral explanation of whoami vs pwd."},
	}
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID:     "response-done",
		Kind:   contracts.CheckResponseSubmitted,
		Params: map[string]any{"taskId": "explain-concepts"},
	})
	doc.Content.Tasks = append(doc.Content.Tasks, contracts.Task{
		ID:           "explain-concepts",
		Title:        "Explain concepts",
		Instructions: "Write a short explanation.",
		Kind:         contracts.TaskKindShortResponse,
		Prerequisites: []string{"enter-docs"},
		Completion:   contracts.CheckTree{CheckID: "response-done"},
		Response: &contracts.ResponseTaskSpec{
			Prompt:               "How do terminal and shell differ?",
			MaxChars:             500,
			ParentReviewRequired: true,
			Rubric: []contracts.RubricCriterion{
				{ID: "roles", Description: "Names distinct roles for terminal and shell", Required: true},
				{ID: "whoami", Description: "Contrasts whoami with pwd", Required: true},
			},
		},
	})
	require.NoError(t, contracts.ValidateDocument(doc))
	student := contracts.StudentBlocks(doc.Content.Blocks)
	require.Len(t, student, 4)
	for _, b := range student {
		assert.NotEqual(t, contracts.BlockParentNote, b.Kind)
	}
}

func TestValidateRejectsUnsafeBlockLink(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{
		{ID: "bad", Kind: contracts.BlockProse, Text: "See https://evil.example for more."},
	}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe")
}

func TestValidateRejectsResponseWithoutRubric(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.Content.Tasks[0].Kind = contracts.TaskKindShortResponse
	doc.Content.Tasks[0].Response = &contracts.ResponseTaskSpec{Prompt: "Why?"}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rubric")
}
