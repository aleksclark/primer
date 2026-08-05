package tutor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/tutor"
)

func TestFallbackHintEmptyHintsUsesDefault(t *testing.T) {
	t.Parallel()
	// No hints, no objective, no tutor → default.
	assert.Equal(t, tutor.DefaultFallback, tutor.FallbackHint(contracts.ActivityContent{}, nil, 0))

	// Empty objective after trim via paraphrase path is internal; empty goal summary falls through.
	act := contracts.ActivityContent{
		Tutor:     &contracts.TutorContext{GoalSummary: "   "},
		Objective: "  ",
	}
	assert.Equal(t, tutor.DefaultFallback, tutor.FallbackHint(act, nil, 1))

	// All-empty hints at levels → default.
	act = contracts.ActivityContent{
		Hints: []contracts.Hint{{ID: "h", Level: 1, Text: "  "}, {ID: "h2", Level: 2, Text: ""}},
	}
	assert.Equal(t, tutor.DefaultFallback, tutor.FallbackHint(act, nil, 1))

	// PriorHints used map skip blanks in activity then last empty → objective.
	act = contracts.ActivityContent{
		Hints:     []contracts.Hint{{ID: "h", Level: 1, Text: "only"}},
		Objective: "Do the thing",
	}
	// Level exact hit returns only; when prior has it and level high, last-hint returns only still.
	got := tutor.FallbackHint(act, []string{"only"}, 99)
	assert.Equal(t, "only", got)
}

func TestEchoProviderDefaultReplyAndEmptyMessage(t *testing.T) {
	t.Parallel()
	// Default multi-sentence echo with empty student message.
	e := &tutor.EchoProvider{}
	reply, err := e.Complete(context.Background(), tutor.Request{StudentMessage: ""})
	require.NoError(t, err)
	assert.Contains(t, reply, "need a hint")
	assert.GreaterOrEqual(t, strings.Count(reply, "."), 2)

	// Policy with empty raw ApplyResponsePolicy
	out, filtered := tutor.ApplyResponsePolicy("   ", tutor.DefaultPolicy())
	assert.Equal(t, "", out)
	assert.False(t, filtered)

	// Code fence stripping leaves only prose.
	out, filtered = tutor.ApplyResponsePolicy("```go\nfunc x(){}\n```\nHelpful tip here.", tutor.DefaultPolicy())
	assert.Contains(t, out, "Helpful")
	assert.NotContains(t, out, "func")
	assert.True(t, filtered)

	// Empty lines skipped; max sentences zero returns input from limitSentences.
	out, _ = tutor.ApplyResponsePolicy("One sentence only!", tutor.PolicyConfig{MaxSentences: 0, StripCodeLines: false})
	assert.Contains(t, out, "One sentence")
}

func TestPolicyProviderNameNilInnerViaEnabledPath(t *testing.T) {
	t.Parallel()
	// NewPolicy never leaves nil inner; exercise ProviderName on real service.
	svc := tutor.NewPolicy(tutor.NewFake(), tutor.DefaultPolicy())
	assert.Equal(t, "fake", svc.ProviderName())
	assert.True(t, svc.Enabled())

	// Rate limit branch already covered; empty session id skips rate limit counter.
	resp, err := svc.Coach(context.Background(), tutor.Request{
		Activity: contracts.ActivityContent{Objective: "o"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Reply)
}
