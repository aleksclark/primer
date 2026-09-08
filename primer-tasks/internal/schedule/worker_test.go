package schedule

import (
	"testing"
	"time"
)

func TestNewWorkerHorizonFromEnvironment(t *testing.T) {
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "10")
	w := NewWorker(nil)
	if w.Horizon != 10*24*time.Hour || w.Interval != time.Minute || w.Owner == "" {
		t.Fatalf("horizon worker=%+v", w)
	}
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "0")
	w = NewWorker(nil)
	if w.Horizon != 45*24*time.Hour {
		t.Fatalf("invalid horizon replaced default=%s", w.Horizon)
	}
	t.Setenv("TASKS_SCHEDULE_HORIZON_DAYS", "731")
	w = NewWorker(nil)
	if w.Horizon != 45*24*time.Hour {
		t.Fatalf("oversized horizon accepted=%s", w.Horizon)
	}
}
