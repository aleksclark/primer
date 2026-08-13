package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndCheckPassword(t *testing.T) {
	t.Parallel()
	assert.False(t, CheckPassword("", "x"))
	assert.False(t, CheckPassword("not-a-hash", "x"))
	h, err := HashPassword("s3cret!")
	require.NoError(t, err)
	assert.True(t, CheckPassword(h, "s3cret!"))
	assert.False(t, CheckPassword(h, "wrong"))
}

func TestDecodeRubricAndHashes(t *testing.T) {
	t.Parallel()
	assert.Nil(t, decodeRubric(nil))
	assert.Nil(t, decodeRubric([]byte("not-json")))
	rows := decodeRubric([]byte(`[{"id":"r1","description":"d","required":true}]`))
	require.Len(t, rows, 1)
	assert.Equal(t, "r1", rows[0]["id"])

	assert.Len(t, hashBody("hi"), 64)
	assert.Len(t, responseDigest("t1", "body"), 64)
	assert.NotEqual(t, hashBody("a"), hashBody("b"))
}
