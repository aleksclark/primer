package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateAndNormalizeOutput(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hi", truncate("hi", 10))
	assert.Equal(t, "hel…", truncate("hello", 3))
	assert.Equal(t, "a\nb", normalizeOutput("a\r\nb\n"))
	assert.Equal(t, "x", normalizeOutput("x\n\n"))
}

func TestAsIntEdges(t *testing.T) {
	t.Parallel()
	n, err := asInt(7)
	assert.NoError(t, err)
	assert.Equal(t, 7, n)
	n, err = asInt(int64(9))
	assert.NoError(t, err)
	assert.Equal(t, 9, n)
	n, err = asInt(float64(4))
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	_, err = asInt("nope")
	assert.Error(t, err)
}
