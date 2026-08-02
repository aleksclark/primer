package primer

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory Ingester for tests and for running the TV server
// against no LMS at all. It keeps the same idempotency contract as the real
// endpoint: a source reference it has already seen comes back with
// Created false.
type Fake struct {
	mu sync.Mutex

	// Logs is every log accepted, in arrival order.
	Logs []InstructionLog
	// Err, when set, is returned instead of accepting a log.
	Err error

	// Calls counts Ingest invocations, including refused ones.
	Calls int

	seen map[string]string
}

var _ Ingester = (*Fake)(nil)

// NewFake builds an empty fake ingester.
func NewFake() *Fake { return &Fake{seen: map[string]string{}} }

// Ingest records a log, or returns the configured error.
func (f *Fake) Ingest(_ context.Context, log InstructionLog) (*IngestResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	if f.seen == nil {
		f.seen = map[string]string{}
	}
	key := log.Source + "/" + log.SourceRef
	if id, ok := f.seen[key]; ok {
		return &IngestResult{LogID: id, Created: false}, nil
	}
	id := fmt.Sprintf("log-%d", len(f.seen)+1)
	f.seen[key] = id
	f.Logs = append(f.Logs, log)
	return &IngestResult{LogID: id, Created: true}, nil
}

// Accepted returns the logs recorded so far.
func (f *Fake) Accepted() []InstructionLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]InstructionLog(nil), f.Logs...)
}
