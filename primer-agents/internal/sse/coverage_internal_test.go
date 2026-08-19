package sse

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/domain"
)

type coverageFlusher struct{}

func (coverageFlusher) Flush() {}

func TestCoverageFrameAndCursorEdges(t *testing.T) {
	payload := "safe"
	root, parent, agent, typ := "root", "parent", "agent", "tutor"
	depth := 1
	e := &domain.RunEvent{RunID: "run", Sequence: 3, SchemaVersion: 1, RootRunID: &root, ParentRunID: &parent, AgentID: &agent, AgentType: &typ, AgentDepth: &depth, Kind: "text", Payload: &payload, CreatedAt: time.Now()}
	frame := frameFromEvent(e)
	require.Equal(t, int64(3), frame.Sequence)
	require.Equal(t, "parent", frame.ParentRunID)

	for _, raw := range []string{"7", "0", "nope", "-1"} {
		r := httptest.NewRequest("GET", "/?afterSeq="+raw, nil)
		if raw == "7" {
			require.Equal(t, int64(7), ParseCursor(r))
		}
	}
	r := httptest.NewRequest("GET", "/?afterSeq=9", nil)
	r.Header.Set("Last-Event-ID", "4")
	require.Equal(t, int64(4), ParseCursor(r))

	rec := httptest.NewRecorder()
	require.NoError(t, writeFrame(rec, coverageFlusher{}, e, time.Second))
	require.Contains(t, rec.Body.String(), "id: 3")
	require.Contains(t, rec.Body.String(), "event: text")
}
