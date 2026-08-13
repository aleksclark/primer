package contracts_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestValidateReferenceSolutionEdges(t *testing.T) {
	t.Parallel()
	doc := sampleTerminal()
	doc.ReferenceSolution = &contracts.ReferenceSolution{Steps: nil}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.ReferenceSolution = &contracts.ReferenceSolution{Steps: []contracts.ReferenceStep{
		{Argv: nil},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.ReferenceSolution = &contracts.ReferenceSolution{Steps: []contracts.ReferenceStep{
		{Argv: []string{"ls", "  "}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.ReferenceSolution = &contracts.ReferenceSolution{Steps: []contracts.ReferenceStep{
		{Argv: []string{"ls"}, WorkDir: "../escape"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.ReferenceSolution = &contracts.ReferenceSolution{Steps: []contracts.ReferenceStep{
		{Argv: []string{"ls", "-la"}, WorkDir: "home"},
	}}
	require.NoError(t, contracts.ValidateDocument(doc))
}

func TestValidateInstructionBlockAndResponseEdges(t *testing.T) {
	t.Parallel()

	// Invalid block id (must start with a letter; no spaces).
	doc := sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "1bad", Kind: contracts.BlockProse, Text: "hi"}}
	require.Error(t, contracts.ValidateDocument(doc))
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "bad id", Kind: contracts.BlockProse, Text: "hi"}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Duplicate block id.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{
		{ID: "b1", Kind: contracts.BlockProse, Text: "one"},
		{ID: "b1", Kind: contracts.BlockWarning, Text: "two"},
	}
	require.Error(t, contracts.ValidateDocument(doc))

	// Prose empty text.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "b1", Kind: contracts.BlockProse, Text: "  "}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Prose too long.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "b1", Kind: contracts.BlockProse, Text: strings.Repeat("a", contracts.MaxInstructionBlockText+1),
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Prose unsafe link.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "b1", Kind: contracts.BlockProse, Text: "see https://evil.example"}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Prose with unexpected terms.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "b1", Kind: contracts.BlockProse, Text: "ok",
		Terms: []contracts.VocabularyTerm{{Term: "t", Definition: "d"}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Vocabulary missing terms.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "v1", Kind: contracts.BlockVocabulary}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Vocabulary empty term/definition.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "v1", Kind: contracts.BlockVocabulary,
		Terms: []contracts.VocabularyTerm{{Term: " ", Definition: "d"}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Vocabulary unsafe markup.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "v1", Kind: contracts.BlockVocabulary,
		Terms: []contracts.VocabularyTerm{{Term: "x", Definition: "<script>alert(1)</script>"}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Vocabulary with unexpected text.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "v1", Kind: contracts.BlockVocabulary, Text: "nope",
		Terms: []contracts.VocabularyTerm{{Term: "t", Definition: "d"}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Example empty.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "e1", Kind: contracts.BlockExample}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Example unsafe.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "e1", Kind: contracts.BlockExample, Input: "javascript:alert(1)",
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Example with terms.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "e1", Kind: contracts.BlockExample, Input: "ls",
		Terms: []contracts.VocabularyTerm{{Term: "t", Definition: "d"}},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Resource missing.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "r1", Kind: contracts.BlockResource}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Resource bad sha / label / media / html / size / unexpected text.
	goodSHA := strings.Repeat("ab", 32)
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: "short", Label: "L", MediaType: "text/plain"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "  ", MediaType: "text/plain"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "L", MediaType: ""},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "http://x", MediaType: "text/plain"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "L", MediaType: "text/html"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "L", MediaType: "text/plain", ByteSize: -1},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "r1", Kind: contracts.BlockResource, Text: "nope",
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "L", MediaType: "text/plain", ByteSize: 1},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// Happy resource + vocabulary + example.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{
		{ID: "p1", Kind: contracts.BlockProse, Text: "Read carefully."},
		{ID: "v1", Kind: contracts.BlockVocabulary, Terms: []contracts.VocabularyTerm{{Term: "cwd", Definition: "current working directory"}}},
		{ID: "e1", Kind: contracts.BlockExample, Input: "pwd", Output: "/home", Explanation: "prints cwd"},
		{ID: "r1", Kind: contracts.BlockResource, Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "sheet", MediaType: "application/pdf", ByteSize: 12,
		}},
		{ID: "n1", Kind: contracts.BlockParentNote, Text: "parent only"},
	}
	require.NoError(t, contracts.ValidateDocument(doc))

	// Unknown block kind.
	doc = sampleTerminal()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "x1", Kind: "video", Text: "nope"}}
	require.Error(t, contracts.ValidateDocument(doc))

	// short_response task edges
	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		// missing response
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	// Need exists check from sample
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response:   &contracts.ResponseTaskSpec{Prompt: "  ", Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "ok"}}},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "https://bad", Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "ok"}},
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "Why?", MaxChars: -1, Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "ok"}},
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "Why?", Rubric: nil,
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "Why?",
			Rubric: []contracts.RubricCriterion{{ID: "Bad", Description: "ok"}},
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "Why?",
			Rubric: []contracts.RubricCriterion{
				{ID: "c1", Description: "ok"},
				{ID: "c1", Description: "dup"},
			},
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Tasks = []contracts.Task{{
		ID: "t", Title: "T", Instructions: "G", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "Why?",
			Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "  "}},
		},
		Completion: contracts.CheckTree{CheckID: "exists"},
	}}
	require.Error(t, contracts.ValidateDocument(doc))

	// action task must not carry response
	doc = sampleTerminal()
	doc.Content.Tasks[0].Response = &contracts.ResponseTaskSpec{
		Prompt: "no", Rubric: []contracts.RubricCriterion{{ID: "c1", Description: "d"}},
	}
	require.Error(t, contracts.ValidateDocument(doc))

	// unknown task kind
	doc = sampleTerminal()
	doc.Content.Tasks[0].Kind = "essay"
	require.Error(t, contracts.ValidateDocument(doc))

	// Happy short_response + response_submitted check
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "rs", Kind: contracts.CheckResponseSubmitted, Params: map[string]any{"taskId": "tresp"},
	})
	doc.Content.Tasks = []contracts.Task{{
		ID: "tresp", Title: "Reflect", Instructions: "Write", Kind: contracts.TaskKindShortResponse,
		Response: &contracts.ResponseTaskSpec{
			Prompt: "What did you learn?", MaxChars: 200,
			Rubric: []contracts.RubricCriterion{{ID: "r1", Description: "mentions command"}},
		},
		Completion: contracts.CheckTree{CheckID: "rs"},
	}}
	require.NoError(t, contracts.ValidateDocument(doc))
}

func TestValidateCheckStageAndParamEdges(t *testing.T) {
	t.Parallel()

	// unknown / duplicate stages
	doc := sampleTerminal()
	doc.Content.Checks[0].Stages = []string{"mid"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks[0].Stages = []string{contracts.StageFinal, contracts.StageFinal}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks[0].InvariantAt = []string{"whenever"}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks[0].InvariantAt = []string{contracts.InvariantAtFinal, contracts.InvariantAtFinal}
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks[0].Stages = []string{contracts.StageFixture, contracts.StageTask}
	doc.Content.Checks[0].InvariantAt = []string{contracts.InvariantAtAfterTask}
	require.NoError(t, contracts.ValidateDocument(doc))

	// content equals missing value type
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "eq", Kind: contracts.CheckContentEquals, Params: map[string]any{"path": "home/a.txt", "value": 12},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// content match missing pattern
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "m", Kind: contracts.CheckContentMatch, Params: map[string]any{"path": "home/a.txt", "pattern": ""},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// path type missing type key
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "pt", Kind: contracts.CheckPathType, Params: map[string]any{"path": "home", "type": 1},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// command properties extra param + bad stdout pattern
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "cmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "extra": "nope"},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "cmd", Kind: contracts.CheckCommandProperties,
		Params: map[string]any{"executable": "ls", "stdoutPattern": "("},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// pipeline extra param + bad pattern
	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "pipe", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"value": "x", "nope": 1},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	doc = sampleTerminal()
	doc.Content.Checks = append(doc.Content.Checks, contracts.Check{
		ID: "pipe", Kind: contracts.CheckPipelineOutput, Params: map[string]any{"pattern": "("},
	})
	require.Error(t, contracts.ValidateDocument(doc))

	// typing metrics accuracy out of range + missing numbers
	doc = &contracts.ActivityDocument{
		SchemaVersion: contracts.SchemaVersion,
		Slug:          "typing-edge",
		Title:         "T",
		Kind:          contracts.KindTyping,
		SubjectCode:   "digital-literacy",
		Standards:     []contracts.StandardRef{{Code: "PRIMER.DL.6.TYPE.1", Role: contracts.StandardRolePrimary}},
		Content: contracts.ActivityContent{
			Objective: "o", Instructions: "i",
			Typing: &contracts.TypingContent{PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "hi"}}},
			Tasks:  []contracts.Task{{ID: "t", Title: "T", Instructions: "G", Completion: contracts.CheckTree{CheckID: "m"}}},
			Checks: []contracts.Check{{
				ID: "m", Kind: contracts.CheckTypingMetrics,
				Params: map[string]any{"min_wpm": 10.0, "min_accuracy": 1.5},
			}},
		},
	}
	require.Error(t, contracts.ValidateDocument(doc))

	doc.Content.Checks[0].Params = map[string]any{"min_wpm": "no", "min_accuracy": "no"}
	require.Error(t, contracts.ValidateDocument(doc))

	// response_submitted bad task id / extra params / task_id alias
	docT := sampleTerminal()
	docT.Content.Checks = append(docT.Content.Checks, contracts.Check{
		ID: "rs", Kind: contracts.CheckResponseSubmitted, Params: map[string]any{"taskId": "Bad Id"},
	})
	require.Error(t, contracts.ValidateDocument(docT))

	docT = sampleTerminal()
	docT.Content.Checks = append(docT.Content.Checks, contracts.Check{
		ID: "rs", Kind: contracts.CheckResponseSubmitted, Params: map[string]any{"taskId": "t1", "extra": 1},
	})
	require.Error(t, contracts.ValidateDocument(docT))

	docT = sampleTerminal()
	docT.Content.Checks = append(docT.Content.Checks, contracts.Check{
		ID: "rs", Kind: contracts.CheckResponseSubmitted, Params: map[string]any{"task_id": "t1"},
	})
	// Need task id t1 - sample uses different id; still valid check params
	// But task completion may not reference it — just ensure check validates.
	// Sample tasks reference exists; adding optional check is fine.
	require.NoError(t, contracts.ValidateDocument(docT))

	// StudentBlocks filters parent_note
	blocks := []contracts.InstructionBlock{
		{ID: "a", Kind: contracts.BlockProse, Text: "a"},
		{ID: "p", Kind: contracts.BlockParentNote, Text: "secret"},
	}
	out := contracts.StudentBlocks(blocks)
	require.Len(t, out, 1)
	assert.Equal(t, "a", out[0].ID)
	assert.Nil(t, contracts.StudentBlocks(nil))
	assert.Nil(t, contracts.StudentBlocks([]contracts.InstructionBlock{{ID: "p", Kind: contracts.BlockParentNote, Text: "x"}}))

	assert.Equal(t, contracts.TaskKindAction, contracts.TaskKindOrDefault(contracts.Task{}))
	assert.Equal(t, contracts.TaskKindShortResponse, contracts.TaskKindOrDefault(contracts.Task{Kind: contracts.TaskKindShortResponse}))
}
