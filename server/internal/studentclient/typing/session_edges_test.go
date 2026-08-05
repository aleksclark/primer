package typing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestSubmitLineAndTypeKeyEdges(t *testing.T) {
	t.Parallel()
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "ab"}, {ID: "p2", Text: "cd"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)

	// SubmitLine no-op when incomplete
	assert.False(t, s.SubmitLine())

	// control chars ignored (except tab)
	s.TypeKey(1)
	assert.Equal(t, "", s.CurrentInput())

	s.TypeKey('a')
	s.TypeKey('b') // advances prompt
	assert.Equal(t, 1, s.PromptIndex())

	// wrong then correct on second prompt
	s.TypeKey('x')
	s.TypeKey('c')
	s.TypeKey('d')
	assert.True(t, s.Done())
	assert.False(t, s.SubmitLine()) // done

	// Backspace on empty / done
	s.Backspace()
	assert.True(t, s.Done())

	// NewSession errors
	_, err = NewSession(nil, nil)
	require.Error(t, err)
	_, err = NewSession(&contracts.TypingContent{Prompts: nil}, nil)
	require.Error(t, err)
	_, err = NewSession(&contracts.TypingContent{Prompts: []contracts.TypingPrompt{{ID: "p", Text: " "}}}, nil)
	require.Error(t, err)
}

func TestSubmitLineAdvancesExactMatchWithoutAutoAdvance(t *testing.T) {
	t.Parallel()
	// Craft session state where input matches but advance hasn't fired (restore-like).
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "hi"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)
	// Type without completing via final advance path: type h,i advances automatically.
	// To hit SubmitLine advance branch, restore mid-match then manually set input.
	s.input = []rune("hi")
	s.correct = 2
	// still on prompt 0 with full match
	assert.False(t, s.Done())
	assert.True(t, s.SubmitLine())
	assert.True(t, s.Done())
}

func TestTypeKeyExtraCharsAndTab(t *testing.T) {
	t.Parallel()
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "a\tb"}},
		SuccessWPM:  1, SuccessAccuracy: 0.5,
	}, nil)
	require.NoError(t, err)
	s.TypeKey('a')
	s.TypeKey('\t')
	s.TypeKey('b')
	assert.True(t, s.Done())

	// Done TypeKey is no-op
	s.TypeKey('z')

	// Extra char path: single char prompt, type correct then extra before advance?
	s2, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "z"}},
	}, nil)
	require.NoError(t, err)
	// force full input without Done by setting fields after match without advance
	s2.input = []rune("z")
	s2.promptIdx = 0
	s2.TypeKey('x') // pos >= len(prompt)
	assert.Greater(t, s2.incorrect, 0)
}

func TestMetricsAndEncodePaths(t *testing.T) {
	t.Parallel()
	s, err := NewSession(&contracts.TypingContent{
		PromptSetID:     "p",
		Prompts:         []contracts.TypingPrompt{{ID: "p1", Text: "go"}},
		SuccessWPM:      1000, // impossible
		SuccessAccuracy: 0.99,
	}, []contracts.Check{{
		ID: "m", Kind: contracts.CheckTypingMetrics,
		Params: map[string]any{"min_wpm": 1000.0, "min_accuracy": 0.99},
	}})
	require.NoError(t, err)
	s.now = func() time.Time { return time.Unix(100, 0) }
	s.TypeKey('g')
	s.now = func() time.Time { return time.Unix(101, 0) }
	s.TypeKey('o')
	m := s.Metrics()
	assert.True(t, m.Done)
	assert.False(t, m.ThresholdsMet)
	assert.Equal(t, 0, s.RemainingPrompts())
	id, text := s.CurrentPrompt()
	assert.Empty(t, id)
	assert.Empty(t, text)

	raw, err := s.EncodeState()
	require.NoError(t, err)
	s3, err := NewSession(&contracts.TypingContent{
		PromptSetID: "p",
		Prompts:     []contracts.TypingPrompt{{ID: "p1", Text: "go"}},
	}, nil)
	require.NoError(t, err)
	require.NoError(t, s3.RestoreState(raw))
	assert.True(t, s3.Done())
}
