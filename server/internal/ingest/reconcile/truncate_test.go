package reconcile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hi", truncate("  hi  ", 10))
	assert.Equal(t, "hel…", truncate("hello world", 4))
}
