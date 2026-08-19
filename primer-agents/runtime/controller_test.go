package agentruntime_test

import (
	"context"
	"iter"
	"testing"
	"time"

	agentruntime "github.com/aleksclark/primer/agents/runtime"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/stretchr/testify/require"
)

func TestControllerDetachedRunAndExplicitCancel(t *testing.T) {
	started := make(chan struct{})
	prov := &agentruntime.ScriptedProvider{RunFn: func(ctx context.Context, _ []*message.Message, _ ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
		return func(yield func(*mafagent.ResponseUpdate, error) bool) {
			close(started)
			<-ctx.Done()
			yield(nil, ctx.Err())
		}
	}}
	root := agentruntime.NewScriptedAgent(mafagent.Config{ID: "controller-root", Name: "Overseer"}, prov)
	controller := agentruntime.NewController(agentruntime.ControllerConfig{
		Agent:     root,
		RunBudget: time.Second,
		Spec:      agentruntime.AgentSpec{Type: "overseer", Name: "Overseer", MaxChildren: 0},
	})

	snapshot, err := controller.Start("hold")
	require.NoError(t, err)
	<-started
	before, ok := controller.Get(snapshot.ID)
	require.True(t, ok)
	require.Equal(t, agentruntime.RunRunning, before.State)
	controller.Cancel(snapshot.ID)

	terminal, err := controller.Wait(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, agentruntime.RunCanceled, terminal.State)
	require.NotEmpty(t, terminal.Error)
}

func TestRunnerConcurrentPrepareRunUsesOnePendingID(t *testing.T) {
	root := agentruntime.NewScriptedAgent(mafagent.Config{ID: "prepare-root", Name: "Overseer"}, &agentruntime.ScriptedProvider{})
	runner := agentruntime.NewRunner(agentruntime.AgentSpec{Type: "overseer", MaxChildren: 0}, root, nil)
	ids := make(chan string, 32)
	for i := 0; i < cap(ids); i++ {
		go func() { ids <- runner.PrepareRun() }()
	}
	first := <-ids
	for i := 1; i < cap(ids); i++ {
		require.Equal(t, first, <-ids)
	}
}

func TestControllerRedactsProviderErrors(t *testing.T) {
	secret := "provider-token=do-not-expose"
	prov := &agentruntime.ScriptedProvider{RunFn: func(context.Context, []*message.Message, ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
		return func(yield func(*mafagent.ResponseUpdate, error) bool) { yield(nil, requireError(secret)) }
	}}
	root := agentruntime.NewScriptedAgent(mafagent.Config{ID: "error-root", Name: "Overseer"}, prov)
	controller := agentruntime.NewController(agentruntime.ControllerConfig{Agent: root})
	run, err := controller.Start("fail")
	require.NoError(t, err)
	terminal, err := controller.Wait(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, agentruntime.RunFailed, terminal.State)
	require.Equal(t, "provider_error", terminal.ErrorClass)
	require.Equal(t, "agent provider failed", terminal.Error)
	require.NotContains(t, terminal.Error, secret)
}

func requireError(text string) error { return &testError{text: text} }

type testError struct{ text string }

func (e *testError) Error() string { return e.text }

func TestControllerStartIsDisabledWithoutAgent(t *testing.T) {
	controller := agentruntime.NewController(agentruntime.ControllerConfig{})
	_, err := controller.Start("hello")
	require.Error(t, err)
}
