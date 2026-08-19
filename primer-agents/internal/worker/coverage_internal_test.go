package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/aleksclark/primer/agents/runtime"
)

func TestCoverageWorkerPurePolicies(t *testing.T) {
	w := &Worker{}
	cfg := w.providerConfig()
	require.NotNil(t, cfg.Run)
	require.Equal(t, "execution_error", classifyError(errors.New("x")))
	require.Equal(t, "deadline_exceeded", classifyError(context.DeadlineExceeded))
	require.Equal(t, "context_canceled", classifyError(context.Canceled))
	require.True(t, isCancelRequest(errors.New("run cancelled by request")))
	require.False(t, isCancelRequest(errors.New("other")))

	text := "hello"
	e := agentruntime.RunEvent{RunID: "r", RootRunID: "root", AgentType: "tutor", Kind: agentruntime.KindStart, Text: text}
	p := safePayload(e)
	require.NotNil(t, p)
	require.Equal(t, text, *p)
	tool := safePayload(agentruntime.RunEvent{Kind: agentruntime.KindToolStart, ToolName: "allowed"})
	require.NotNil(t, tool)
	require.Nil(t, safePayload(agentruntime.RunEvent{Kind: agentruntime.KindText, Text: "content"}))
}
