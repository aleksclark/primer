package contracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFilesystemCheckKindAndIsCapstone(t *testing.T) {
	t.Parallel()
	for _, k := range []string{
		CheckFileExists, CheckFileNotExists, CheckContentEquals, CheckContentContains,
		CheckContentMatch, CheckPathType, CheckPathMode,
	} {
		assert.True(t, isFilesystemCheckKind(k), k)
	}
	assert.False(t, isFilesystemCheckKind(CheckCwd))
	assert.False(t, isFilesystemCheckKind(CheckCommandProperties))
	assert.False(t, isFilesystemCheckKind(""))

	assert.False(t, isCapstone(&ActivityDocument{Slug: "intro"}))
	assert.True(t, isCapstone(&ActivityDocument{Slug: "final-capstone-project"}))
	assert.True(t, isCapstone(&ActivityDocument{
		Slug: "x", Metadata: map[string]string{"capstone": "true"},
	}))
	assert.True(t, isCapstone(&ActivityDocument{
		Slug: "x", Metadata: map[string]string{"lesson": "19"},
	}))
	assert.True(t, isCapstone(&ActivityDocument{
		Slug: "x", Metadata: map[string]string{"lesson": "20"},
	}))
	assert.False(t, isCapstone(&ActivityDocument{
		Slug: "x", Metadata: map[string]string{"lesson": "01"},
	}))
}

func TestErrIncompatibleRevisionError(t *testing.T) {
	t.Parallel()
	assert.Contains(t, ErrIncompatibleRevision{}.Error(), "structured_command_evidence")
	assert.Equal(t, "custom", ErrIncompatibleRevision{Msg: "custom"}.Error())
}

func TestCheckParamsSize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, checkParamsSize(nil))
	n := checkParamsSize(map[string]any{"path": "a.txt", "n": 3})
	assert.Greater(t, n, len("path")+len("a.txt"))
}
