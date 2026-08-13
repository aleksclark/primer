package validatecmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func TestIsFilesystemKind(t *testing.T) {
	t.Parallel()
	assert.True(t, isFilesystemKind(contracts.CheckFileExists))
	assert.True(t, isFilesystemKind(contracts.CheckContentEquals))
	assert.False(t, isFilesystemKind(contracts.CheckCwd))
	assert.False(t, isFilesystemKind(contracts.CheckCommandProperties))
}
