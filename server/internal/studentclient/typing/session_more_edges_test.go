package typing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestObservationsSyntheticAndParamFallbacks(t *testing.T) {
	t.Parallel()
	// No checks → synthetic typing-metrics observation.
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "ab"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)
	s.SetClock(func() time.Time { return time.Unix(50, 0) })
	obs := s.Observations()
	require.Len(t, obs, 1)
	assert.Equal(t, "typing-metrics", obs[0].CheckID)
	assert.False(t, obs[0].Passed)
	assert.Contains(t, obs[0].Message, "incomplete")

	// Complete quickly with high WPM.
	s.TypeString("ab")
	s.SetClock(func() time.Time { return time.Unix(51, 0) })
	obs = s.Observations()
	require.Len(t, obs, 1)
	assert.True(t, obs[0].Passed)
	assert.True(t, s.MeetsThresholds())

	// Non-typing check kinds fail closed; missing params fall back to content thresholds.
	s2, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "x"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, []contracts.Check{
		{ID: "m", Kind: contracts.CheckTypingMetrics, Params: map[string]any{}}, // missing params
		{ID: "other", Kind: contracts.CheckFileExists, Params: map[string]any{"path": "a"}},
		{ID: "zero", Kind: contracts.CheckTypingMetrics, Params: map[string]any{"min_wpm": 0.0, "min_accuracy": 0.0}},
	})
	require.NoError(t, err)
	s2.SetClock(func() time.Time { return time.Unix(10, 0) })
	s2.TypeKey('x')
	s2.SetClock(func() time.Time { return time.Unix(11, 0) })
	obs = s2.Observations()
	require.Len(t, obs, 3)
	assert.True(t, obs[0].Passed, obs[0].Message) // fallback content thresholds
	assert.False(t, obs[1].Passed)
	assert.Contains(t, obs[1].Message, "not evaluated")
	assert.True(t, obs[2].Passed)
}

func TestRestoreStateValidationAndElapsedFreeze(t *testing.T) {
	t.Parallel()
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "hi"}, {ID: "p2", Text: "yo"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)

	require.NoError(t, s.RestoreState(nil))
	require.NoError(t, s.RestoreState([]byte{}))

	require.Error(t, s.RestoreState([]byte("{")))
	require.Error(t, s.RestoreState([]byte(`{"v":99}`)))

	// Clamp prompt idx above len and negative via crafted state.
	st := DurableState{Version: 1, PromptIdx: 99, Input: "x", Correct: 1, HasStarted: true, ElapsedMs: 1500}
	raw, err := json.Marshal(st)
	require.NoError(t, err)
	require.NoError(t, s.RestoreState(raw))
	assert.True(t, s.Done())
	assert.Equal(t, 0, s.RemainingPrompts())

	s2, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "hi"}},
	}, nil)
	require.NoError(t, err)
	base := time.Unix(1000, 0)
	s2.SetClock(func() time.Time { return base })
	st = DurableState{Version: 1, PromptIdx: -3, Input: "h", Correct: 1, Incorrect: 0, Keys: 1, HasStarted: true, ElapsedMs: 2000}
	raw, err = json.Marshal(st)
	require.NoError(t, err)
	require.NoError(t, s2.RestoreState(raw))
	assert.Equal(t, 0, s2.PromptIndex())
	m := s2.Metrics()
	assert.InDelta(t, 2.0, m.Elapsed.Seconds(), 0.05)

	// Encode with negative elapsed clamp (clock went backwards).
	s2.startedAt = base.Add(5 * time.Second)
	s2.SetClock(func() time.Time { return base })
	raw, err = s2.EncodeState()
	require.NoError(t, err)
	var got DurableState
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, int64(0), got.ElapsedMs)
}

func TestThresholdsMetDefaultsAndWPMEdge(t *testing.T) {
	t.Parallel()
	// SuccessAccuracy unset (0) defaults to 1; SuccessWPM negative → 0.
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID:     "p",
		Prompts:         []contracts.TypingPrompt{{ID: "p1", Text: "ok"}},
		SuccessWPM:      -5,
		SuccessAccuracy: 0,
	}, nil)
	require.NoError(t, err)
	// Instant complete: sub-second WPM path uses 1/60 minute floor.
	s.now = func() time.Time { return time.Unix(0, 0) }
	s.TypeString("ok")
	m := s.Metrics()
	assert.True(t, m.Done)
	assert.Greater(t, m.WPM, 0.0)
	assert.True(t, m.ThresholdsMet) // accuracy 1.0, wpm > 0

	// Incomplete never meets thresholds.
	s3, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "zz"}},
		SuccessWPM: 1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)
	assert.False(t, thresholdsMet(s3.Metrics(), s3.content))

	// Zero correct chars → wpm 0.
	assert.Equal(t, 0.0, wpm(0, time.Second))
	// Backspace clamps correct to >= 0.
	s4, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p", Prompts: []contracts.TypingPrompt{{ID: "p1", Text: "a"}},
	}, nil)
	require.NoError(t, err)
	s4.correct = 0
	s4.input = []rune("x")
	s4.Backspace()
	assert.Equal(t, 0, s4.correct)
}
