package engine

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStatusTUIProgramQuitsOnQ(t *testing.T) {
	t.Parallel()
	// Drive StatusModel through tea.Program with injected IO (same shape as RunStatusTUI).
	in := strings.NewReader("q")
	p := tea.NewProgram(
		NewStatusModel("coverage-status", Status{Phase: "running", Message: "hello"}),
		tea.WithInput(in),
		tea.WithOutput(io.Discard),
	)
	final, err := p.Run()
	require.NoError(t, err)
	m, ok := final.(StatusModel)
	require.True(t, ok)
	assert.True(t, m.quitting)
}
