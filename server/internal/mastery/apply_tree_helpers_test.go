package mastery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestEvalTreeValidateRequiredMore(t *testing.T) {
	t.Parallel()

	byID := map[string]contracts.Observation{
		"a": {CheckID: "a", Passed: true},
		"b": {CheckID: "b", Passed: false},
		"c": {CheckID: "c", Passed: true},
	}

	assert.True(t, evalTree(contracts.CheckTree{Optional: true, CheckID: "b"}, byID))
	assert.True(t, evalTree(contracts.CheckTree{CheckID: "a"}, byID))
	assert.False(t, evalTree(contracts.CheckTree{CheckID: "b"}, byID))
	assert.False(t, evalTree(contracts.CheckTree{CheckID: "missing"}, byID))

	assert.True(t, evalTree(contracts.CheckTree{All: []contracts.CheckTree{
		{CheckID: "a"}, {CheckID: "c"},
	}}, byID))
	assert.False(t, evalTree(contracts.CheckTree{All: []contracts.CheckTree{
		{CheckID: "a"}, {CheckID: "b"},
	}}, byID))

	assert.True(t, evalTree(contracts.CheckTree{Any: []contracts.CheckTree{
		{CheckID: "b"}, {CheckID: "a"},
	}}, byID))
	assert.False(t, evalTree(contracts.CheckTree{Any: []contracts.CheckTree{
		{CheckID: "b"}, {CheckID: "missing"},
	}}, byID))

	// Empty tree true.
	assert.True(t, evalTree(contracts.CheckTree{}, byID))

	content := contracts.ActivityContent{
		Checks: []contracts.Check{
			{ID: "a", Kind: contracts.CheckFileExists},
			{ID: "b", Kind: contracts.CheckFileExists, Optional: true},
			{ID: "c", Kind: contracts.CheckFileExists},
		},
		Tasks: []contracts.Task{
			{ID: "t1", Completion: contracts.CheckTree{CheckID: "a"}},
			{ID: "t2", Optional: true, Completion: contracts.CheckTree{CheckID: "b"}},
			{ID: "t3", Completion: contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "c"}}}},
		},
	}
	obs := []contracts.Observation{
		{CheckID: "a", Passed: true},
		{CheckID: "c", Passed: true},
	}
	require.NoError(t, validateRequiredChecks(content, obs))

	// Missing required check.
	err := validateRequiredChecks(content, []contracts.Observation{{CheckID: "a", Passed: true}})
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)
	assert.Contains(t, br.Msg, "required check")

	// Task tree fails while all required checks still pass (task references extra id).
	content.Tasks = append(content.Tasks, contracts.Task{
		ID: "t-extra", Completion: contracts.CheckTree{CheckID: "need-me"},
	})
	err = validateRequiredChecks(content, []contracts.Observation{
		{CheckID: "a", Passed: true},
		{CheckID: "c", Passed: true},
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &br)
	assert.Contains(t, br.Msg, "task completion")
}

func TestPrimaryCheckIDAndClampMore(t *testing.T) {
	t.Parallel()
	content := contracts.ActivityContent{Checks: []contracts.Check{
		{ID: "opt", Optional: true},
		{ID: "req"},
	}}
	assert.Equal(t, "req", primaryCheckID(content, nil))
	assert.Equal(t, "x", primaryCheckID(contracts.ActivityContent{}, []contracts.Observation{{CheckID: "x"}}))
	assert.Equal(t, "unknown", primaryCheckID(contracts.ActivityContent{}, nil))

	assert.Equal(t, 0.0, clamp01(-1))
	assert.Equal(t, 1.0, clamp01(2))
	assert.Equal(t, 0.5, clamp01(0.5))
}

func TestDecodeContentMore(t *testing.T) {
	t.Parallel()
	c, err := decodeContent(map[string]any{"objective": "learn", "instructions": "do"})
	require.NoError(t, err)
	assert.Equal(t, "learn", c.Objective)

	_, err = decodeContent(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
}
