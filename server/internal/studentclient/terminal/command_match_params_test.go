package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandAndPipelineMatchFromParams(t *testing.T) {
	t.Parallel()
	m, err := commandMatchFromParams(map[string]any{
		"executable":     "ls",
		"args":           []any{"-la"},
		"exitCode":       0,
		"stdoutContains": "x",
		"stdoutEquals":   "x",
		"stdoutPattern":  "^x$",
		"stderrContains": "e",
		"stderrEquals":   "e",
		"stderrPattern":  "^e$",
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "ls", m.Executable)
	assert.True(t, m.ArgsSet)
	assert.NotNil(t, m.ExitCode)
	assert.True(t, m.RequireStdoutTrusted)
	assert.True(t, m.RequireStderrTrusted)

	_, err = commandMatchFromParams(map[string]any{}, true)
	assert.Error(t, err)

	pm, err := pipelineMatchFromParams(map[string]any{"contains": "hi", "pattern": "h.*"})
	require.NoError(t, err)
	assert.True(t, pm.RequireStructured)
	assert.NotEmpty(t, pm.StdoutContains)
}
