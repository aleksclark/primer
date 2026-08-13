package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ok", truncate("ok", 5))
	assert.Equal(t, "ab…", truncate("abcd", 2))
}
