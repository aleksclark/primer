package ptyterm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripANSI(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", stripANSI("hello"))
	assert.Equal(t, "hi", stripANSI("\x1b[31mhi\x1b[0m"))
	assert.Equal(t, "x", stripANSI("\x1b]0;title\x07x"))
	assert.Equal(t, "y", stripANSI("\x1b]0;title\x1b\\y"))
	assert.Equal(t, "z", stripANSI("\x1bz")) // lone ESC skipped
	assert.Equal(t, "ab", stripANSI("a\x1b[1;32mb"))
}
