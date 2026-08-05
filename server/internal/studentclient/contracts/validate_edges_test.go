package contracts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestValidateDocumentEdgeBranches(t *testing.T) {
	t.Parallel()

	require.Error(t, contracts.ValidateDocument(nil))
	require.Error(t, contracts.ValidateContent(contracts.KindTerminal, nil))

	doc := sampleTerminal()
	doc.SchemaVersion = "999"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Slug = "Bad_Slug"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Title = "   "
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Kind = "quiz"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.SubjectCode = ""
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Standards = nil
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Standards[0].Code = "not-a-code"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Standards = []contracts.StandardRef{
		{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary},
		{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary},
	}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Standards[0].Role = "bonus"
	require.Error(t, contracts.ValidateDocument(doc))

	// ValidateContent kind mismatch / empty fields
	c := sampleTerminal().Content
	c.Objective = ""
	require.Error(t, contracts.ValidateContent(contracts.KindTerminal, &c))
	c = sampleTerminal().Content
	c.Instructions = ""
	require.Error(t, contracts.ValidateContent(contracts.KindTerminal, &c))

	c = sampleTerminal().Content
	c.Terminal = nil
	require.Error(t, contracts.ValidateContent(contracts.KindTerminal, &c))
	c = sampleTerminal().Content
	c.Typing = &contracts.TypingContent{PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}}
	require.Error(t, contracts.ValidateContent(contracts.KindTerminal, &c))

	c = sampleTerminal().Content
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, &c)) // terminal set, typing nil
	c.Terminal = nil
	c.Typing = &contracts.TypingContent{PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}}
	// file_exists checks are invalid for typing activities in full ValidateDocument,
	// but ValidateContent only enforces typing block + generic check structure.
	// Replace checks with typing metrics so this path can succeed.
	c.Checks = []contracts.Check{{ID: "m", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 1.0, "min_accuracy": 0.9}}}
	c.Tasks = []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}}
	require.NoError(t, contracts.ValidateContent(contracts.KindTyping, &c))

	require.Error(t, contracts.ValidateContent("other", &sampleTerminal().Content))
}

func TestValidateTerminalAndTypingEdges(t *testing.T) {
	t.Parallel()

	// unknown runtime profile
	doc := sampleTerminal()
	doc.Content.Terminal.RuntimeProfile = "nope"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.RuntimeProfile = ""
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.InitialCwd = "../escape"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.Fixtures = append(doc.Content.Terminal.Fixtures, contracts.FixtureEntry{
		Path: "home", Type: contracts.FixtureDirectory, // duplicate
	})
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.Fixtures = []contracts.FixtureEntry{{Path: "../x", Type: contracts.FixtureFile}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.Fixtures = []contracts.FixtureEntry{{Path: "home", Type: "fifo"}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.Fixtures = []contracts.FixtureEntry{{Path: "home", Type: contracts.FixtureDirectory, Content: "nope"}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Terminal.Fixtures = []contracts.FixtureEntry{{Path: "home/a.txt", Type: contracts.FixtureFile, Mode: "99"}}
	require.Error(t, contracts.ValidateDocument(doc))

	// typing edges via ValidateContent
	tc := &contracts.ActivityContent{
		Objective: "o", Instructions: "i",
		Typing: &contracts.TypingContent{},
		Tasks:  []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}},
		Checks: []contracts.Check{{ID: "m", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 1.0, "min_accuracy": 0.9}}},
	}
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc)) // empty prompt_set_id

	tc.Typing.PromptSetID = "set"
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc)) // empty prompts

	tc.Typing.Prompts = []contracts.TypingPrompt{{ID: "1bad", Text: "hi"}}
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc))

	tc.Typing.Prompts = []contracts.TypingPrompt{{ID: "p1", Text: "hi"}, {ID: "p1", Text: "yo"}}
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc))

	tc.Typing.Prompts = []contracts.TypingPrompt{{ID: "p1", Text: "  "}}
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc))

	tc.Typing.Prompts = []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}
	tc.Typing.TimeLimitSec = -1
	require.Error(t, contracts.ValidateContent(contracts.KindTyping, tc))

	tc.Typing.TimeLimitSec = 0
	require.NoError(t, contracts.ValidateContent(contracts.KindTyping, tc))
}

func TestValidateChecksTasksHintsArtifactsEdges(t *testing.T) {
	t.Parallel()

	// empty checks
	doc := sampleTerminal()
	doc.Content.Checks = nil
	require.Error(t, contracts.ValidateDocument(doc))

	// invalid / duplicate check id
	doc = sampleTerminal()
	doc.Content.Checks[0].ID = "9bad"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, doc.Content.Checks[0])
	require.Error(t, contracts.ValidateDocument(doc))

	// unknown check kind
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{ID: "unk", Kind: "magic", Params: map[string]any{}})
	require.Error(t, contracts.ValidateDocument(doc))

	// missing path params
	for _, kind := range []string{
		contracts.CheckFileExists, contracts.CheckContentEquals, contracts.CheckContentMatch,
		contracts.CheckPathType, contracts.CheckPathMode, contracts.CheckCwd,
	} {
		doc = sampleTerminal()
		doc.Content.Checks = append(doc.Content.Checks, contracts.Check{ID: "x", Kind: kind, Params: map[string]any{}})
		require.Error(t, contracts.ValidateDocument(doc), kind)
	}

	// content equals missing value
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "eq", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/welcome.txt"},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// content match missing pattern
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "m", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/welcome.txt"},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// path type missing type
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "pt", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home"},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// symlink type allowed
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "sl", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home/welcome.txt", "type": contracts.PathTypeSymlink},
	})
	require.NoError(t, contracts.ValidateDocument(doc))

	// command properties missing executable
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "cp", Kind: contracts.CheckCommandProperties, Params: map[string]any{},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// pipeline value ok + bad pattern
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "po", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"value": "x", "pattern": "("},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// typing metrics missing numbers / accuracy range
	typing := func(params map[string]any) *contracts.ActivityDocument {
		return &contracts.ActivityDocument{
			SchemaVersion: contracts.SchemaVersion, Slug: "type-edge", Title: "T",
			Kind: contracts.KindTyping, SubjectCode: "digital-literacy",
			Standards: []contracts.StandardRef{{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary}},
			Content: contracts.ActivityContent{
				Objective: "o", Instructions: "i",
				Typing: &contracts.TypingContent{PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}},
				Tasks:  []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}},
				Checks: []contracts.Check{{ID: "m", Kind: contracts.CheckTypingMetrics, Params: params}},
			},
		}
	}
	require.Error(t, contracts.ValidateDocument(typing(map[string]any{})))
	require.Error(t, contracts.ValidateDocument(typing(map[string]any{"min_wpm": 1.0})))
	require.Error(t, contracts.ValidateDocument(typing(map[string]any{"min_wpm": 1.0, "min_accuracy": 1.5})))
	require.NoError(t, contracts.ValidateDocument(typing(map[string]any{"min_wpm": 1.0, "min_accuracy": 0.5})))

	// empty tasks
	doc = sampleTerminal()
	doc.Content.Tasks = nil
	require.Error(t, contracts.ValidateDocument(doc))

	// invalid/duplicate task ids, missing title/instructions
	doc = sampleTerminal()
	doc.Content.Tasks[0].ID = "0x"
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = append(doc.Content.Tasks, doc.Content.Tasks[0])
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Title = ""
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Instructions = ""
	require.Error(t, contracts.ValidateDocument(doc))

	// bad prerequisite id format / unknown / self
	doc = sampleTerminal()
	doc.Content.Tasks[0].Prerequisites = []string{"9bad"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Prerequisites = []string{"missing-task"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Prerequisites = []string{doc.Content.Tasks[0].ID}
	require.Error(t, contracts.ValidateDocument(doc))

	// cycle a->b->a
	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{
		{ID: "a", Title: "A", Instructions: "A", Prerequisites: []string{"b"}, Completion: contracts.CheckTree{CheckID: "welcome-exists"}},
		{ID: "b", Title: "B", Instructions: "B", Prerequisites: []string{"a"}, Completion: contracts.CheckTree{CheckID: "welcome-exists"}},
	}
	require.Error(t, contracts.ValidateDocument(doc))

	// hints: invalid id, duplicate, empty text, unknown hint on task
	doc = sampleTerminal()
	doc.Content.Hints = []contracts.Hint{{ID: "1h", Text: "x"}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Hints = []contracts.Hint{{ID: "h1", Text: "x"}, {ID: "h1", Text: "y"}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Hints = []contracts.Hint{{ID: "h1", Text: "  "}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Hints = []contracts.Hint{{ID: "h1", Text: "tip"}}
	doc.Content.Tasks[0].HintIDs = []string{"missing"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Hints = []contracts.Hint{{ID: "h1", Text: "tip"}}
	doc.Content.Tasks[0].HintIDs = []string{"h1"}
	require.NoError(t, contracts.ValidateDocument(doc))

	// check tree edges
	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{} // none
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{CheckID: "welcome-exists", All: []contracts.CheckTree{{CheckID: "in-docs"}}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{CheckID: "nope"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{All: []contracts.CheckTree{}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks[0].Completion = contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "welcome-exists"}, {CheckID: "nope"}}}
	require.Error(t, contracts.ValidateDocument(doc))

	// artifacts negative limits
	doc = sampleTerminal()
	doc.Content.Artifacts = &contracts.ArtifactPolicy{Enabled: true, MaxFiles: -1}
	require.Error(t, contracts.ValidateDocument(doc))
	doc.Content.Artifacts = &contracts.ArtifactPolicy{Enabled: true, MaxBytesEach: -1}
	require.Error(t, contracts.ValidateDocument(doc))
	doc.Content.Artifacts = &contracts.ArtifactPolicy{Enabled: true, MaxBytesTotal: -1}
	require.Error(t, contracts.ValidateDocument(doc))
	doc.Content.Artifacts = &contracts.ArtifactPolicy{Enabled: true, MaxFiles: 1, MaxBytesEach: 10, MaxBytesTotal: 10}
	require.NoError(t, contracts.ValidateDocument(doc))
}

func TestParamHelpersAndParseModeEdges(t *testing.T) {
	t.Parallel()
	// exercised indirectly via checks with typed numeric params
	doc := sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "cmdn", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "exitCode": float64(0), "args": []any{"-l"}},
	})
	require.NoError(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "cmdi", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "exitCode": 0},
	})
	require.NoError(t, contracts.ValidateDocument(doc))

	// unsafe path
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "badp", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "../etc/passwd"},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	m, err := contracts.ParseMode("644")
	require.NoError(t, err)
	assert.Equal(t, 0o644, int(m))
	_, err = contracts.ParseMode("")
	require.Error(t, err)
	_, err = contracts.ParseMode("0888")
	require.Error(t, err)
}
