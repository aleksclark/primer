package mastery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestStatusForThresholdsAndRoles(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "mastered", statusFor("in_progress", MasteredThreshold, contracts.StandardRolePrimary))
	assert.Equal(t, "mastered", statusFor("approaching", 0.95, ""))
	assert.Equal(t, "approaching", statusFor("in_progress", 0.5, contracts.StandardRolePrimary))
	assert.Equal(t, "in_progress", statusFor("not_introduced", 0.1, contracts.StandardRolePrimary))
	assert.Equal(t, "in_progress", statusFor("", 0.2, ""))
	// Reinforcement keeps mastered when still above threshold path is separate;
	// below threshold from mastered falls to approaching.
	assert.Equal(t, "approaching", statusFor("mastered", 0.5, contracts.StandardRolePrimary))
	// Reinforcement role preserves mastered even mid confidence? only when conf < threshold from mastered -> approaching unless role reinforcement AND from mastered returns mastered at conf>=0.45 branch first.
	// conf 0.5 >= 0.45 => approaching before role check. Use conf just under 0.45.
	assert.Equal(t, "mastered", statusFor("mastered", 0.4, contracts.StandardRoleReinforcement))
	assert.Equal(t, "in_progress", statusFor("in_progress", 0.2, contracts.StandardRolePrimary))
}

func TestClamp01(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, clamp01(-1))
	assert.Equal(t, 1.0, clamp01(2))
	assert.Equal(t, 0.35, clamp01(0.35))
}

func TestEvalTreeAndValidateRequiredChecks(t *testing.T) {
	t.Parallel()
	obs := map[string]contracts.Observation{
		"a": {CheckID: "a", Passed: true},
		"b": {CheckID: "b", Passed: false},
		"c": {CheckID: "c", Passed: true},
	}
	assert.True(t, evalTree(contracts.CheckTree{Optional: true}, obs))
	assert.True(t, evalTree(contracts.CheckTree{CheckID: "a"}, obs))
	assert.False(t, evalTree(contracts.CheckTree{CheckID: "b"}, obs))
	assert.True(t, evalTree(contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "c"}}}, obs))
	assert.False(t, evalTree(contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "b"}}}, obs))
	assert.True(t, evalTree(contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "b"}, {CheckID: "c"}}}, obs))
	assert.False(t, evalTree(contracts.CheckTree{Any: []contracts.CheckTree{{CheckID: "b"}}}, obs))
	assert.True(t, evalTree(contracts.CheckTree{}, obs))

	content := contracts.ActivityContent{
		Checks: []contracts.Check{
			{ID: "a", Kind: "command"},
			{ID: "b", Kind: "command", Optional: true},
			{ID: "c", Kind: "command"},
		},
		Tasks: []contracts.Task{
			{ID: "t1", Completion: contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "c"}}}},
			{ID: "t2", Optional: true, Completion: contracts.CheckTree{CheckID: "b"}},
		},
	}
	require.NoError(t, validateRequiredChecks(content, []contracts.Observation{
		{CheckID: "a", Passed: true}, {CheckID: "c", Passed: true},
	}))
	err := validateRequiredChecks(content, []contracts.Observation{{CheckID: "a", Passed: true}})
	require.Error(t, err)
	var br repo.ErrBadRequest
	require.ErrorAs(t, err, &br)

	err = validateRequiredChecks(content, []contracts.Observation{
		{CheckID: "a", Passed: true}, {CheckID: "c", Passed: false},
	})
	require.Error(t, err)

	// Task tree failure with all checks present.
	content2 := contracts.ActivityContent{
		Checks: []contracts.Check{{ID: "a"}, {ID: "c"}},
		Tasks:  []contracts.Task{{ID: "t", Completion: contracts.CheckTree{All: []contracts.CheckTree{{CheckID: "a"}, {CheckID: "missing"}}}}},
	}
	err = validateRequiredChecks(content2, []contracts.Observation{
		{CheckID: "a", Passed: true}, {CheckID: "c", Passed: true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task completion")
}

func TestPrimaryCheckID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "req", primaryCheckID(contracts.ActivityContent{
		Checks: []contracts.Check{{ID: "opt", Optional: true}, {ID: "req"}},
	}, nil))
	assert.Equal(t, "from-obs", primaryCheckID(contracts.ActivityContent{}, []contracts.Observation{{CheckID: "from-obs"}}))
	assert.Equal(t, "unknown", primaryCheckID(contracts.ActivityContent{}, nil))
}

func TestDecodeContent(t *testing.T) {
	t.Parallel()
	c, err := decodeContent(map[string]any{
		"objective": "o",
		"checks":    []any{map[string]any{"id": "c1", "kind": "command"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "o", c.Objective)
	require.Len(t, c.Checks, 1)
}
