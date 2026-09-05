package schedule

import (
	"testing"
	"time"
)

func TestDailyKeepsLocalTimeAcrossDST(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	start := time.Date(2026, 3, 7, 7, 30, 0, 0, loc)
	s, e := Parse("FREQ=DAILY;COUNT=4", "America/New_York", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 5))
	if len(xs) != 4 {
		t.Fatalf("got %d", len(xs))
	}
	for _, x := range xs {
		if x.In(loc).Hour() != 7 || x.In(loc).Minute() != 30 {
			t.Fatalf("local time moved: %s", x.In(loc))
		}
	}
}
func TestRejectsUnsupportedRRULE(t *testing.T) {
	_, e := Parse("FREQ=MONTHLY", "UTC", time.Now(), nil)
	if e == nil {
		t.Fatal("monthly accepted")
	}
	_, e = Parse("FREQ=DAILY;BYHOUR=2", "UTC", time.Now(), nil)
	if e == nil {
		t.Fatal("unsupported clause accepted")
	}
}
func TestWeeklyByDay(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	s, e := Parse("FREQ=WEEKLY;BYDAY=MO,WE;COUNT=4", "UTC", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 20))
	if len(xs) != 4 || xs[1].Weekday() != time.Wednesday {
		t.Fatalf("unexpected %v", xs)
	}
}
