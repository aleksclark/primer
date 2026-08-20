package workflow

import (
	"sync"
	"time"
)

// StageMetric is a process-local operational counter. Workflow durability and
// correctness never depend on it; a restart simply starts a fresh interval.
type StageMetric struct {
	Attempts uint64
	Failures uint64
	Latency  time.Duration
}

var stageMetrics struct {
	sync.Mutex
	values map[string]StageMetric
}

func recordStage(stage string, latency time.Duration, failed bool) {
	stageMetrics.Lock()
	defer stageMetrics.Unlock()
	if stageMetrics.values == nil {
		stageMetrics.values = make(map[string]StageMetric)
	}
	v := stageMetrics.values[stage]
	v.Attempts++
	v.Latency += latency
	if failed {
		v.Failures++
	}
	stageMetrics.values[stage] = v
}

// Metrics returns a snapshot suitable for process metrics adapters and tests.
func Metrics() map[string]StageMetric {
	stageMetrics.Lock()
	defer stageMetrics.Unlock()
	out := make(map[string]StageMetric, len(stageMetrics.values))
	for key, value := range stageMetrics.values {
		out[key] = value
	}
	return out
}
