package activities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/studentclient/activities"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestResolveKindExplicitAndInferred(t *testing.T) {
	t.Parallel()

	// Explicit wins even when content has the other kind.
	kind, err := activities.ResolveKind(contracts.KindTyping, contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{},
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTyping, kind)

	kind, err = activities.ResolveKind(contracts.KindTerminal, contracts.ActivityContent{
		Typing: &contracts.TypingContent{},
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTerminal, kind)

	// Infer terminal.
	kind, err = activities.ResolveKind("", contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{RuntimeProfile: "coreutils-basic"},
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTerminal, kind)

	// Infer typing when only typing present.
	kind, err = activities.ResolveKind("", contracts.ActivityContent{
		Typing: &contracts.TypingContent{PromptSetID: "p"},
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTyping, kind)

	// Prefer terminal when both present and explicit empty.
	kind, err = activities.ResolveKind("", contracts.ActivityContent{
		Terminal: &contracts.TerminalContent{},
		Typing:   &contracts.TypingContent{},
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.KindTerminal, kind)

	// Neither → error.
	_, err = activities.ResolveKind("", contracts.ActivityContent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no terminal or typing")

	// KindOf passthrough / default.
	assert.Equal(t, contracts.KindTerminal, activities.KindOf(contracts.KindTerminal))
	assert.Equal(t, contracts.KindTyping, activities.KindOf(contracts.KindTyping))
	assert.Equal(t, "custom", activities.KindOf("custom"))
}
