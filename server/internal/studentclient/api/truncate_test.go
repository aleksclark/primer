package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", truncate("short", 10))
	assert.Equal(t, "abc…", truncate("abcdef", 3))
	long := strings.Repeat("x", 250)
	out := truncate(long, 200)
	assert.True(t, strings.HasSuffix(out, "…"))
	assert.Equal(t, 201, len([]rune(out))) // 200 + ellipsis char may be multi-byte; check suffix
	assert.Contains(t, out, "…")
}
