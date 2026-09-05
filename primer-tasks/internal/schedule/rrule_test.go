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
func TestDSTGapSkipsMissingWallTime(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 7, 2, 30, 0, 0, loc)
	s, e := Parse("FREQ=DAILY;COUNT=4", "America/New_York", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 6))
	if len(xs) != 4 {
		t.Fatalf("gap series=%d %v", len(xs), xs)
	}
	for _, x := range xs {
		local := x.In(loc)
		if local.Month() == time.March && local.Day() == 8 {
			t.Fatalf("invented gap instant %s", local)
		}
		if local.Hour() != 2 || local.Minute() != 30 {
			t.Fatalf("local wall moved: %s", local)
		}
	}
}

func TestDSTFoldEmitsBothRepeatedInstants(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 31, 1, 30, 0, 0, loc)
	s, e := Parse("FREQ=DAILY;COUNT=4", "America/New_York", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 6))
	var fold []time.Time
	for _, x := range xs {
		local := x.In(loc)
		if local.Month() == time.November && local.Day() == 1 && local.Hour() == 1 && local.Minute() == 30 {
			fold = append(fold, x)
		}
	}
	if len(fold) != 2 {
		t.Fatalf("fold instants=%d series=%v", len(fold), xs)
	}
	if fold[0].Equal(fold[1]) || fold[1].Sub(fold[0]) != time.Hour {
		t.Fatalf("fold instants were not the two repeated 01:30 offsets: %v", fold)
	}
}

func TestCivilInstantsSkipsGapAndKeepsFoldOrder(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if got := civilInstants(loc, 2026, time.March, 8, 2, 30, 0, 0, 0); len(got) != 0 {
		t.Fatalf("gap instants=%v", got)
	}
	fold := civilInstants(loc, 2026, time.November, 1, 1, 30, 0, 0, 0)
	if len(fold) != 2 || fold[1].Sub(fold[0]) != time.Hour {
		t.Fatalf("fold instants=%v", fold)
	}
}

func TestOccurrencesUnknownTimezoneIsEmpty(t *testing.T) {
	s := Spec{Frequency: "DAILY", Interval: 1, Start: time.Now(), Timezone: "No/Such"}
	if got := s.Occurrences(time.Now().Add(time.Hour)); len(got) != 0 {
		t.Fatalf("unknown timezone emitted %v", got)
	}
}

func TestWeeklyIntervalSkipsOffWeeks(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	s, e := Parse("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;COUNT=3", "UTC", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 40))
	if len(xs) != 3 {
		t.Fatalf("weekly interval count=%d %v", len(xs), xs)
	}
	if xs[1].Sub(xs[0]) != 14*24*time.Hour {
		t.Fatalf("off-week emitted: %v", xs)
	}
}

func TestParseRejectsMalformedClauseAndUnknownTimezone(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	if _, e := Parse("FREQ", "UTC", start, nil); e == nil {
		t.Fatal("clause without equals accepted")
	}
	if _, e := Parse("FREQ=DAILY", "No/Such", start, nil); e == nil {
		t.Fatal("unknown timezone accepted")
	}
}

func TestUntilClauseStopsSeries(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	s, e := Parse("FREQ=DAILY;UNTIL=20260107T080000Z", "UTC", start, nil)
	if e != nil {
		t.Fatal(e)
	}
	xs := s.Occurrences(start.AddDate(0, 0, 10))
	if len(xs) != 3 {
		t.Fatalf("until series=%d %v", len(xs), xs)
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
