package schedule

import (
	"testing"
	"time"
)

func TestParseRRULEValidationBranches(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	cases := []string{"FREQ=DAILY;INTERVAL=0", "FREQ=DAILY;INTERVAL=31", "FREQ=DAILY;COUNT=0", "FREQ=DAILY;COUNT=367", "FREQ=DAILY;COUNT=2;COUNT=3", "FREQ=DAILY;UNTIL=bad", "FREQ=WEEKLY;BYDAY=XX", "FREQ=DAILY;BYDAY=MO", "FREQ=DAILY;NOPE=x", "INTERVAL=2"}
	for _, rule := range cases {
		if _, e := Parse(rule, "UTC", start, nil); e == nil {
			t.Errorf("accepted %s", rule)
		}
	}
	if _, e := Parse("FREQ=DAILY", "No/Such", start, nil); e == nil {
		t.Fatal("bad timezone accepted")
	}
}
func TestAllWeekdayCodes(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	if _, err := Parse("FREQ=WEEKLY;BYDAY=SU,MO,TU,WE,TH,FR,SA;COUNT=1", "UTC", start, nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseAndOccurrencesOneOffAndIntervals(t *testing.T) {
	start := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	s, e := Parse("", "UTC", start, nil)
	if e != nil || len(s.Occurrences(start.Add(time.Hour))) != 1 {
		t.Fatal("one off failed")
	}
	s, e = Parse("FREQ=DAILY;INTERVAL=2;COUNT=3", "UTC", start, nil)
	if e != nil || len(s.Occurrences(start.AddDate(0, 0, 10))) != 3 {
		t.Fatalf("daily interval failed %v", e)
	}
	until := start.AddDate(0, 0, 1)
	s, e = Parse("FREQ=DAILY", "UTC", start, &until)
	if e != nil || len(s.Occurrences(start.AddDate(0, 0, 5))) != 2 {
		t.Fatalf("until failed %v", e)
	}
	s, e = Parse("FREQ=WEEKLY;INTERVAL=2", "UTC", start, nil)
	if e != nil || len(s.Occurrences(start.AddDate(0, 0, 30))) == 0 {
		t.Fatalf("weekly interval failed %v", e)
	}
	if len(s.Occurrences(start.AddDate(0, 0, -1))) != 0 {
		t.Fatal("past horizon emitted occurrence")
	}
}
