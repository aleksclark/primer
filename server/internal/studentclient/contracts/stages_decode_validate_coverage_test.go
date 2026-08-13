package contracts_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestIsEvidenceBearingAndBoolPtr(t *testing.T) {
	t.Parallel()
	// Default (nil) is evidence-bearing.
	assert.True(t, contracts.IsEvidenceBearing(contracts.Check{ID: "a", Kind: contracts.CheckFileExists}))
	assert.True(t, contracts.IsEvidenceBearing(contracts.Check{
		ID: "b", Kind: contracts.CheckFileExists, EvidenceBearing: contracts.BoolPtr(true),
	}))
	assert.False(t, contracts.IsEvidenceBearing(contracts.Check{
		ID: "c", Kind: contracts.CheckFileExists, EvidenceBearing: contracts.BoolPtr(false),
	}))

	p := contracts.BoolPtr(false)
	require.NotNil(t, p)
	assert.False(t, *p)
	p2 := contracts.BoolPtr(true)
	require.NotNil(t, p2)
	assert.True(t, *p2)
	assert.NotSame(t, p, p2)
}

func TestFormatJSONPathAndDecodeErrorUnwrap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "content.tasks[0].id", contracts.FormatJSONPath("content", "tasks[0]", "id"))
	assert.Equal(t, "a.b", contracts.FormatJSONPath("a", "", "b"))
	assert.Equal(t, "root[0].leaf", contracts.FormatJSONPath("root", "[0]", "leaf"))
	assert.Equal(t, "", contracts.FormatJSONPath("", "", ""))

	inner := fmt.Errorf("boom")
	err := &contracts.DecodeError{Path: "act.json", Field: "schemaVersion", Message: "bad", Err: inner}
	assert.ErrorIs(t, err, inner)
	assert.Equal(t, inner, errors.Unwrap(err))
	assert.Contains(t, err.Error(), "act.json")
	assert.Contains(t, err.Error(), "schemaVersion")

	// No path/field still surfaces message + unwrap.
	err2 := &contracts.DecodeError{Message: "only-msg", Err: inner}
	assert.ErrorIs(t, err2, inner)
	assert.Contains(t, err2.Error(), "only-msg")
	assert.Contains(t, err2.Error(), "boom")
}

func TestDecodeActivityEdges(t *testing.T) {
	t.Parallel()
	// Trailing junk after a valid JSON value.
	_, err := contracts.DecodeActivityJSON([]byte(`{"schemaVersion":"1"} {"x":1}`), "trail.json")
	require.Error(t, err)
	var de *contracts.DecodeError
	require.ErrorAs(t, err, &de)
	assert.Contains(t, de.Error(), "trail.json")

	// Duplicate keys rejected.
	_, err = contracts.DecodeActivityJSON([]byte(`{"schemaVersion":"1","schemaVersion":"1"}`), "dup.json")
	require.Error(t, err)
	require.ErrorAs(t, err, &de)
	assert.Contains(t, strings.ToLower(de.Error()), "duplicate")

	// Missing schema version.
	_, err = contracts.DecodeActivityJSON([]byte(`{"slug":"x"}`), "nosv.json")
	require.Error(t, err)
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "schemaVersion", de.Field)

	// Unsupported schema version.
	_, err = contracts.DecodeActivityJSON([]byte(`{"schemaVersion":"99"}`), "badsv.json")
	require.Error(t, err)
	require.ErrorAs(t, err, &de)
	assert.Contains(t, de.Message, "unsupported")

	// Nested duplicate key inside array object.
	_, err = contracts.DecodeActivityJSON([]byte(`{"schemaVersion":"1","arr":[{"a":1,"a":2}]}`), "nested.json")
	require.Error(t, err)

	// Course decode edges.
	_, err = contracts.DecodeCourseJSON([]byte(`{"schemaVersion":"1"} junk`), "course.json")
	require.Error(t, err)
	_, err = contracts.DecodeCourseJSON([]byte(`{"schemaVersion":""}`), "course2.json")
	require.Error(t, err)
	_, err = contracts.DecodeCourseJSON([]byte(`{"schemaVersion":"9"}`), "course3.json")
	require.Error(t, err)
	require.ErrorAs(t, err, &de)
	assert.Contains(t, de.Message, "unsupported course")

	// YAML multi-document rejected.
	yamlMulti := "schemaVersion: \"1\"\n---\nschemaVersion: \"1\"\n"
	_, err = contracts.DecodeActivityYAML([]byte(yamlMulti), "multi.yaml")
	require.Error(t, err)

	// YAML duplicate keys.
	yamlDup := "schemaVersion: \"1\"\nschemaVersion: \"1\"\n"
	_, err = contracts.DecodeActivityYAML([]byte(yamlDup), "dup.yaml")
	require.Error(t, err)
}

func TestValidateResourceRefViaBlocks(t *testing.T) {
	t.Parallel()
	base := func() *contracts.ActivityDocument {
		return &contracts.ActivityDocument{
			SchemaVersion: contracts.SchemaVersion,
			Slug:          "resource-block-act",
			Title:         "Resource Block",
			Summary:       "s",
			Kind:          contracts.KindTerminal,
			SubjectCode:   "digital-literacy",
			Standards:     []contracts.StandardRef{{Code: "PRIMER.DL.6.NAV.1", Role: contracts.StandardRolePrimary}},
			Content: contracts.ActivityContent{
				Objective:    "o",
				Instructions: "i",
				Terminal: &contracts.TerminalContent{
					RuntimeProfile: contracts.RuntimeCoreutilsBasic,
					Fixtures:       []contracts.FixtureEntry{{Path: "home", Type: contracts.FixtureDirectory}},
				},
				Tasks: []contracts.Task{{
					ID: "t1", Title: "T", Instructions: "go",
					Completion: contracts.CheckTree{CheckID: "c1"},
				}},
				Checks: []contracts.Check{{
					ID: "c1", Kind: contracts.CheckFileExists,
					Params: map[string]any{"path": "home"},
				}},
			},
		}
	}

	goodSHA := strings.Repeat("ab", 32)
	doc := base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "Handout", MediaType: "application/pdf", ByteSize: 12,
		},
	}}
	require.NoError(t, contracts.ValidateDocument(doc))

	// Missing resource pointer.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{ID: "res1", Kind: contracts.BlockResource}}
	err := contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource is required")

	// Bad sha length.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: "deadbeef", Label: "L", MediaType: "text/plain", ByteSize: 1},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sha256")

	// Empty label.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "  ", MediaType: "text/plain", ByteSize: 1},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "label")

	// Empty media type.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{SHA256: goodSHA, Label: "L", MediaType: "", ByteSize: 1},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mediaType")

	// Remote URL-like label rejected.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "https://evil.example/x", MediaType: "text/plain", ByteSize: 1,
		},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote")

	// HTML media type rejected.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "page", MediaType: "text/html", ByteSize: 1,
		},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTML")

	// Negative byte size.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource,
		Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "L", MediaType: "text/plain", ByteSize: -1,
		},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "byteSize")

	// Unexpected fields on resource block.
	doc = base()
	doc.Content.Blocks = []contracts.InstructionBlock{{
		ID: "res1", Kind: contracts.BlockResource, Text: "nope",
		Resource: &contracts.ResourceRef{
			SHA256: goodSHA, Label: "L", MediaType: "text/plain", ByteSize: 1,
		},
	}}
	err = contracts.ValidateDocument(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected fields")
}

func TestRevisionRequiresStructuredCommandBranches(t *testing.T) {
	t.Parallel()
	// No tasks: only command checks → requires structured command.
	assert.True(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Checks: []contracts.Check{{ID: "c1", Kind: contracts.CheckCommandProperties}},
	}))
	// No tasks: filesystem check present → does not require.
	assert.False(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Checks: []contracts.Check{
			{ID: "c1", Kind: contracts.CheckCommandProperties},
			{ID: "c2", Kind: contracts.CheckFileExists},
		},
	}))
	// No tasks: all optional → false.
	assert.False(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Checks: []contracts.Check{{ID: "c1", Kind: contracts.CheckCommandProperties, Optional: true}},
	}))

	// Task with only command completion requires capability.
	assert.True(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks:  []contracts.Task{{ID: "t1", Completion: contracts.CheckTree{CheckID: "cmd"}}},
		Checks: []contracts.Check{{ID: "cmd", Kind: contracts.CheckCommandProperties}},
	}))
	// Optional task does not force requirement.
	assert.False(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks:  []contracts.Task{{ID: "t1", Optional: true, Completion: contracts.CheckTree{CheckID: "cmd"}}},
		Checks: []contracts.Check{{ID: "cmd", Kind: contracts.CheckCommandProperties}},
	}))
	// any: one filesystem alternative avoids requirement.
	assert.False(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks: []contracts.Task{{
			ID: "t1",
			Completion: contracts.CheckTree{Any: []contracts.CheckTree{
				{CheckID: "cmd"}, {CheckID: "fs"},
			}},
		}},
		Checks: []contracts.Check{
			{ID: "cmd", Kind: contracts.CheckCommandProperties},
			{ID: "fs", Kind: contracts.CheckFileExists},
		},
	}))
	// any: every alternative is command → requires.
	assert.True(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks: []contracts.Task{{
			ID: "t1",
			Completion: contracts.CheckTree{Any: []contracts.CheckTree{
				{CheckID: "cmd1"}, {CheckID: "cmd2"},
			}},
		}},
		Checks: []contracts.Check{
			{ID: "cmd1", Kind: contracts.CheckCommandProperties},
			{ID: "cmd2", Kind: contracts.CheckPipelineOutput},
		},
	}))
	// all: one command child forces requirement.
	assert.True(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks: []contracts.Task{{
			ID: "t1",
			Completion: contracts.CheckTree{All: []contracts.CheckTree{
				{CheckID: "fs"}, {CheckID: "cmd"},
			}},
		}},
		Checks: []contracts.Check{
			{ID: "fs", Kind: contracts.CheckFileExists},
			{ID: "cmd", Kind: contracts.CheckCommandProperties},
		},
	}))
	// Missing check id treated as requiring capability.
	assert.True(t, contracts.RevisionRequiresStructuredCommand(contracts.ActivityContent{
		Tasks: []contracts.Task{{ID: "t1", Completion: contracts.CheckTree{CheckID: "missing"}}},
	}))
}
