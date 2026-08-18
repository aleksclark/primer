package agent_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/aleksclark/primer/server/internal/agent"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/stretchr/testify/require"
)

func TestControllerDetachedRunAndExplicitCancel(t *testing.T) {
	started := make(chan struct{})
	prov := &agent.ScriptedProvider{RunFn: func(ctx context.Context, _ []*message.Message, _ ...mafagent.Option) iter.Seq2[*mafagent.ResponseUpdate, error] {
		return func(yield func(*mafagent.ResponseUpdate, error) bool) {
			close(started)
			<-ctx.Done()
			yield(nil, ctx.Err())
		}
	}}
	root := agent.NewScriptedAgent(mafagent.Config{ID: "controller-root", Name: "Overseer"}, prov)
	controller := agent.NewController(agent.ControllerConfig{
		Agent:     root,
		RunBudget: time.Second,
		Spec:      agent.AgentSpec{Type: "overseer", Name: "Overseer", MaxChildren: 0},
	})

	snapshot, err := controller.Start("hold")
	require.NoError(t, err)
	<-started
	before, ok := controller.Get(snapshot.ID)
	require.True(t, ok)
	require.Equal(t, agent.RunRunning, before.State)
	controller.Cancel(snapshot.ID)

	terminal, err := controller.Wait(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, agent.RunCanceled, terminal.State)
	require.NotEmpty(t, terminal.Error)
}

func TestRunnerConcurrentPrepareRunUsesOnePendingID(t *testing.T) {
	root := agent.NewScriptedAgent(mafagent.Config{ID: "prepare-root", Name: "Overseer"}, &agent.ScriptedProvider{})
	runner := agent.NewRunner(agent.AgentSpec{Type: "overseer", MaxChildren: 0}, root, nil)
	ids := make(chan string, 32)
	for i := 0; i < cap(ids); i++ {
		go func() { ids <- runner.PrepareRun() }()
	}
	first := <-ids
	for i := 1; i < cap(ids); i++ {
		require.Equal(t, first, <-ids)
	}
}

func TestControllerStartIsDisabledWithoutAgent(t *testing.T) {
	controller := agent.NewController(agent.ControllerConfig{})
	_, err := controller.Start("hello")
	require.Error(t, err)
}
