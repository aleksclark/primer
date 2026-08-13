package activities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "z", truncate("z", 1))
	assert.Equal(t, "z…", truncate("zz", 1))
}
