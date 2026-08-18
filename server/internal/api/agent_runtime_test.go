package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/agent"
	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/stretchr/testify/require"
)

func TestAgentRuntimeParentBoundaryAndDetachedStatus(t *testing.T) {
	prov := &agent.ScriptedProvider{Updates: []*mafagent.ResponseUpdate{{}}}
	root := agent.NewScriptedAgent(mafagent.Config{ID: "api-root", Name: "Overseer"}, prov)
	controller := agent.NewController(agent.ControllerConfig{
		Agent: root,
		Spec:  agent.AgentSpec{Type: "overseer", Name: "Overseer", MaxChildren: 0},
	})
	h, q := testutil.API(t, testutil.Options{AgentController: controller})

	// The runtime boundary is parent-authenticated, never anonymously callable.
	resp := h.Post("/agent/runs", map[string]any{"text": "hello"})
	require.Equal(t, http.StatusUnauthorized, resp.Code, resp.Body.String())

	const password = "agent-runtime-parent"
	ed := factory.EducatorWithPassword(t, q, password, factory.Override{
		"email": "agent-runtime-parent@example.com",
		"role":  "parent",
	})
	resp = h.Post("/auth/login", map[string]any{"email": ed.Email, "password": password})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	token := decode[objMap](t, resp.Body.Bytes())["token"].(string)
	auth := parentAuthHeader(token)

	resp = h.Post("/agent/runs", map[string]any{"text": "hello"}, auth)
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
	started := decode[objMap](t, resp.Body.Bytes())
	runID := started["runId"].(string)
	require.NotEmpty(t, runID)

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp = h.Get("/agent/runs/"+runID, auth)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		status := decode[objMap](t, resp.Body.Bytes())["state"].(string)
		if status == string(agent.RunSucceeded) {
			break
		}
		require.Less(t, time.Now(), deadline, "run did not detach and finish")
		time.Sleep(10 * time.Millisecond)
	}

	// The public SSE boundary exposes the buffered attributed run events.
	resp = h.Get("/agent/runs/"+runID+"/events", auth)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	require.Contains(t, resp.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, resp.Body.String(), "event: run")
	require.Contains(t, resp.Body.String(), runID)

	// A student device token is not accepted by the parent boundary.
	resp = h.Get("/agent/runs/"+runID, "Authorization: Bearer not-a-parent-session")
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}
