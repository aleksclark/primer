package app

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/config"
	"github.com/aleksclark/primer/agents/internal/profile"
)

func TestLiveLLMAgentFactoryBuildsBoundedAgent(t *testing.T) {
	t.Setenv("PRIMER_AGENTS_LIVE_LLM_API_KEY", "named-test-key")
	cfg := &config.Config{
		LiveLLMEnabled:  true,
		LiveLLMModel:    "gpt-4o-mini",
		LiveLLMBaseURL:  "https://api.openai.com/v1",
		LiveLLMMaxCalls: 1,
	}
	agent := liveLLMAgentFactory(cfg)(profile.Spec{MAFConfig: mafagent.Config{
		ID:          "test-agent-id",
		Name:        "test-agent",
		Description: "test",
	}})
	require.NotNil(t, agent)
	require.NotEmpty(t, agent.ID())
}

func TestBudgetTransportAllowsExactlyOneRequest(t *testing.T) {
	var calls atomic.Int64
	transport := budgetTransport{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}),
		calls: &calls,
		max:   1,
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1", nil)
	require.NoError(t, err)
	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	_, err = transport.RoundTrip(req)
	require.Error(t, err)
	require.Equal(t, int64(2), calls.Load())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
