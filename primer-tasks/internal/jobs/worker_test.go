package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"primer-tasks/internal/agent"
)

type fakeJobs struct {
	queued            []Job
	claimed           Job
	claimOK           bool
	claimErr          error
	completed, failed int
	failCause         error
	requeued          int
}

func (f *fakeJobs) Enqueue(_ context.Context, j Job) error {
	f.queued = append(f.queued, j)
	return nil
}
func (f *fakeJobs) Claim(_ context.Context, owner string, lease time.Duration) (Job, bool, error) {
	if f.claimErr != nil {
		return Job{}, false, f.claimErr
	}
	if !f.claimOK {
		return Job{}, false, nil
	}
	f.claimed.LeaseOwner, f.claimed.LeaseUntil = owner, ptrTime(time.Now().Add(lease))
	f.claimOK = false
	return f.claimed, true, nil
}
func (f *fakeJobs) Renew(_ context.Context, id, owner string, _ time.Duration) error {
	if id == "" || owner == "" {
		return errors.New("bad renewal")
	}
	return nil
}
func (f *fakeJobs) Complete(_ context.Context, id, owner string) error {
	f.completed++
	if id == "" || owner == "" {
		return errors.New("bad completion")
	}
	return nil
}
func (f *fakeJobs) Fail(_ context.Context, id, owner string, cause error) error {
	f.failed++
	f.failCause = cause
	if id == "" || owner == "" {
		return errors.New("bad failure")
	}
	return nil
}
func (f *fakeJobs) RequeueExpired(context.Context, time.Time) error { f.requeued++; return nil }
func ptrTime(t time.Time) *time.Time                                { return &t }

type fakeRuns struct {
	transitions   []agent.RunStatus
	reconciled    int
	transitionErr error
}

func (f *fakeRuns) CreateConversation(context.Context, agent.Conversation) error { return nil }
func (f *fakeRuns) GetConversation(context.Context, string, string) (agent.Conversation, error) {
	return agent.Conversation{}, nil
}
func (f *fakeRuns) AppendUserMessage(context.Context, agent.Message) (agent.Message, bool, error) {
	return agent.Message{}, false, nil
}
func (f *fakeRuns) AppendMessage(context.Context, agent.Message) error { return nil }
func (f *fakeRuns) CreateRun(context.Context, agent.Run) error         { return nil }
func (f *fakeRuns) GetRun(context.Context, string, string) (agent.Run, error) {
	return agent.Run{}, nil
}
func (f *fakeRuns) TransitionRun(_ context.Context, _ string, _ string, status agent.RunStatus, _ int, _ agent.Usage) error {
	f.transitions = append(f.transitions, status)
	return f.transitionErr
}
func (f *fakeRuns) RequestCancel(context.Context, string, string) error { return nil }
func (f *fakeRuns) LeaseRun(context.Context, string, string, time.Duration) (agent.Run, bool, error) {
	return agent.Run{}, false, nil
}
func (f *fakeRuns) ReconcileExpiredLeases(context.Context, time.Time) error {
	f.reconciled++
	return nil
}
func (f *fakeRuns) AppendEvent(context.Context, agent.RunEvent) error { return nil }
func (f *fakeRuns) ReplayEvents(context.Context, string, string, int64, int) ([]agent.RunEvent, error) {
	return nil, nil
}
func (f *fakeRuns) PutPreview(context.Context, agent.ConfirmationPreview) error { return nil }
func (f *fakeRuns) ConsumePreview(context.Context, agent.ConfirmationPreview, time.Time) error {
	return nil
}

func TestWorkerReconcileAndStepCompletesOrFails(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, claimed: Job{ID: "job-1", TenantID: "tenant-a", RunID: "run-1"}}
	runs := &fakeRuns{}
	w := NewWorker(jobs, runs, func(context.Context, Job) error { return nil })
	w.Poll = time.Millisecond
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.requeued != 1 || runs.reconciled != 1 {
		t.Fatalf("reconcile jobs=%d runs=%d", jobs.requeued, runs.reconciled)
	}
	w.step(context.Background())
	if jobs.completed != 1 || len(runs.transitions) != 2 || runs.transitions[0] != agent.RunRunning || runs.transitions[1] != agent.RunSucceeded {
		t.Fatalf("complete jobs=%+v runs=%+v", jobs, runs.transitions)
	}

	failedJobs := &fakeJobs{claimOK: true, claimed: Job{ID: "job-2", TenantID: "tenant-a", RunID: "run-2"}}
	failedRuns := &fakeRuns{}
	w = NewWorker(failedJobs, failedRuns, func(context.Context, Job) error { return errors.New("secret provider fragment") })
	w.step(context.Background())
	if failedJobs.failed != 1 || failedJobs.failCause == nil || len(failedRuns.transitions) != 2 || failedRuns.transitions[1] != agent.RunFailed {
		t.Fatalf("failure jobs=%+v runs=%+v", failedJobs, failedRuns.transitions)
	}
}

func TestWorkerStepHandlesClaimErrorNoHandlerAndRunlessReconcile(t *testing.T) {
	claimError := &fakeJobs{claimErr: errors.New("claim unavailable")}
	w := NewWorker(claimError, nil, nil)
	w.step(context.Background())
	if claimError.completed != 0 && claimError.failed != 0 {
		t.Fatal("claim error was handled as a job")
	}
	noHandler := &fakeJobs{claimOK: true, claimed: Job{ID: "job", TenantID: "t", RunID: "r"}}
	w = NewWorker(noHandler, nil, nil)
	w.step(context.Background())
	if noHandler.failed != 1 || noHandler.failCause == nil {
		t.Fatalf("no handler failure=%d cause=%v", noHandler.failed, noHandler.failCause)
	}
	if err := NewWorker(&fakeJobs{}, nil, nil).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerRunStopsOnCancellation(t *testing.T) {
	jobs := &fakeJobs{}
	w := NewWorker(jobs, nil, nil)
	w.Poll = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Run(ctx)
	if jobs.requeued != 1 {
		t.Fatalf("startup reconcile=%d", jobs.requeued)
	}
}
