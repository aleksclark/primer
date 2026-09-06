package domain_test

import (
	"strings"
	"testing"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestP17CommentBodyCodePointBound(t *testing.T) {
	for _, char := range []string{"a", "界", "🙂"} {
		require.NoError(t, domain.ValidateCommentBody(strings.Repeat(char, domain.MaxCommentLength)))
		require.Error(t, domain.ValidateCommentBody(strings.Repeat(char, domain.MaxCommentLength+1)))
	}
	require.Error(t, domain.ValidateCommentBody(strings.Repeat("x", domain.MaxCommentLength)+" "))
	require.Error(t, domain.ValidateCommentBody(" \t\n"))
	require.Error(t, domain.ValidateCommentBody(string([]byte{0xff})))
}
