package typing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/studentclient/typing"
)

func sampleContent() *contracts.TypingContent {
	return &contracts.TypingContent{
		PromptSetID:     "test-set",
		SuccessWPM:      10,
		SuccessAccuracy: 0.9,
		Prompts: []contracts.TypingPrompt{
			{ID: "p1", Text: "ls"},
			{ID: "p2", Text: "pwd"},
			{ID: "p3", Text: "cd .."},
		},
	}
}

func TestTypePerfectPromptsMeetsThresholds(t *testing.T) {
	t.Parallel()
	s, err := typing.NewSession(sampleContent(), []contracts.Check{{
		ID:   "metrics-ok",
		Kind: contracts.CheckTypingMetrics,
		Params: map[string]any{
			"min_wpm":      10.0,
			"min_accuracy": 0.9,
		},
	}})
	require.NoError(t, err)

	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// ~12 correct chars in 6 seconds => (12/5)/(6/60) = 24 wpm
	now := start
	s.SetClock(func() time.Time { return now })

	s.TypeString("ls")
	now = start.Add(2 * time.Second)
	s.TypeString("pwd")
	now = start.Add(4 * time.Second)
	s.TypeString("cd ..")
	now = start.Add(6 * time.Second)

	require.True(t, s.Done())
	m := s.Metrics()
	assert.Equal(t, 3, m.CompletedPrompts)
	assert.Equal(t, 0, m.IncorrectChars)
	assert.InDelta(t, 1.0, m.Accuracy, 1e-9)
	assert.GreaterOrEqual(t, m.WPM, 10.0)
	assert.True(t, m.ThresholdsMet)

	obs := s.Observations()
	require.Len(t, obs, 1)
	assert.Equal(t, "metrics-ok", obs[0].CheckID)
	assert.True(t, obs[0].Passed)
	assert.Equal(t, contracts.CheckTypingMetrics, obs[0].Kind)
}

func TestIncorrectKeysLowerAccuracy(t *testing.T) {
	t.Parallel()
	s, err := typing.NewSession(sampleContent(), nil)
	require.NoError(t, err)
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	now := start
	s.SetClock(func() time.Time { return now })

	// Type wrong then right for "ls"
	s.TypeKey('x')
	s.TypeKey('l')
	s.TypeKey('s')
	now = start.Add(time.Second)
	s.TypeString("pwd")
	now = start.Add(2 * time.Second)
	s.TypeString("cd ..")
	now = start.Add(3 * time.Second)

	m := s.Metrics()
	assert.True(t, m.Done)
	assert.Equal(t, 1, m.IncorrectChars)
	assert.Less(t, m.Accuracy, 1.0)
	// "ls"+"pwd"+"cd .." = 9 correct chars + 1 incorrect => 9/10 = 0.9
	// (TypeKey only counts correct advances; wrong key does not consume.)
	// Actual correct count is 9 if no extra; with wrong-then-right on ls:
	// correct l,s + p,w,d + c,d, ,.,. = 9? Wait — ls is 2, pwd 3, cd .. is 5 = 10 correct + 1 incorrect.
	assert.InDelta(t, 10.0/11.0, m.Accuracy, 1e-9)
	assert.GreaterOrEqual(t, m.Accuracy, 0.9)
	assert.True(t, m.ThresholdsMet)
}

func TestBackspaceCorrectsWithoutCredit(t *testing.T) {
	t.Parallel()
	s, err := typing.NewSession(&contracts.TypingContent{
		PromptSetID:     "one",
		SuccessWPM:      1,
		SuccessAccuracy: 1,
		Prompts:         []contracts.TypingPrompt{{ID: "p", Text: "ab"}},
	}, nil)
	require.NoError(t, err)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	s.SetClock(func() time.Time { return now })

	s.TypeKey('a')
	assert.Equal(t, "a", s.CurrentInput())
	s.Backspace()
	assert.Equal(t, "", s.CurrentInput())
	s.TypeString("ab")
	now = start.Add(2 * time.Second)

	m := s.Metrics()
	assert.True(t, m.Done)
	// correct: a, then after backspace correct--, then a,b => net 2 correct, 0 incorrect
	assert.Equal(t, 2, m.CorrectChars)
	assert.Equal(t, 0, m.IncorrectChars)
	assert.InDelta(t, 1.0, m.Accuracy, 1e-9)
}

func TestThresholdsFailWhenTooSlow(t *testing.T) {
	t.Parallel()
	s, err := typing.NewSession(&contracts.TypingContent{
		PromptSetID:     "slow",
		SuccessWPM:      100, // intentionally high
		SuccessAccuracy: 0.5,
		Prompts:         []contracts.TypingPrompt{{ID: "p", Text: "hi"}},
	}, []contracts.Check{{
		ID:   "m",
		Kind: contracts.CheckTypingMetrics,
		Params: map[string]any{
			"min_wpm":      100.0,
			"min_accuracy": 0.5,
		},
	}})
	require.NoError(t, err)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	s.SetClock(func() time.Time { return now })
	s.TypeString("hi")
	now = start.Add(60 * time.Second) // 2 chars in 60s => 0.4 wpm

	m := s.Metrics()
	assert.True(t, m.Done)
	assert.False(t, m.ThresholdsMet)
	obs := s.Observations()
	require.Len(t, obs, 1)
	assert.False(t, obs[0].Passed)
}

func TestIncompleteNotDone(t *testing.T) {
	t.Parallel()
	s, err := typing.NewSession(sampleContent(), nil)
	require.NoError(t, err)
	s.TypeString("ls")
	assert.False(t, s.Done())
	assert.Equal(t, 2, s.RemainingPrompts())
	id, text := s.CurrentPrompt()
	assert.Equal(t, "p2", id)
	assert.Equal(t, "pwd", text)
	assert.False(t, s.MeetsThresholds())
}
