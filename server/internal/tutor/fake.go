package tutor

import (
	"context"
	"strings"
)

// FakeService returns deterministic activity-local hints. It never mutates
// mastery or external state and is the default provider for tests and local dev.
type FakeService struct{}

// NewFake returns a deterministic tutor Service.
func NewFake() *FakeService {
	return &FakeService{}
}

// Name implements Provider.
func (f *FakeService) Name() string { return "fake" }

// Complete implements Provider.
func (f *FakeService) Complete(_ context.Context, req Request) (string, error) {
	// Student message is intentionally ignored for content selection so prompt
	// injection cannot steer the fake reply away from activity hints.
	_ = SanitizeStudentMessage(req.StudentMessage)
	return FallbackHint(req.Activity, req.PriorHints, req.HintLevel), nil
}

// Coach implements Service.
func (f *FakeService) Coach(ctx context.Context, req Request) (Response, error) {
	reply, err := f.Complete(ctx, req)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Reply:    reply,
		Provider: f.Name(),
		Fallback: true, // activity-local by design
	}, nil
}

// EchoProvider is a test double that echoes a fixed prefix plus sanitized
// student text. Useful for verifying policy wrapping without AWS.
type EchoProvider struct {
	// Delay, if non-zero, sleeps before returning (for timeout tests via slow complete).
	Slow func(ctx context.Context) error
	// Reply overrides the default echo body when non-empty.
	Reply string
	// FailWith forces an error from Complete.
	FailWith error
}

// Name implements Provider.
func (e *EchoProvider) Name() string { return "echo" }

// Complete implements Provider.
func (e *EchoProvider) Complete(ctx context.Context, req Request) (string, error) {
	if e.Slow != nil {
		if err := e.Slow(ctx); err != nil {
			return "", err
		}
	}
	if e.FailWith != nil {
		return "", e.FailWith
	}
	if e.Reply != "" {
		return e.Reply, nil
	}
	msg := SanitizeStudentMessage(req.StudentMessage)
	if msg == "" {
		msg = "need a hint"
	}
	// Intentionally produce a multi-sentence reply so policy can trim it in tests.
	return "Try discovering with a short command first. " +
		"Then check the workspace listing. " +
		"Student said: " + strings.ReplaceAll(msg, "\n", " "), nil
}
