package schedule

import (
	"fmt"
	"strings"
	"time"
)

type Spec struct {
	Frequency string
	Interval  int
	ByWeekday []time.Weekday
	Count     int
	Until     *time.Time
	Start     time.Time
	Timezone  string
}

func Parse(rule, zone string, start time.Time, end *time.Time) (Spec, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Spec{}, fmt.Errorf("timezone: %w", err)
	}
	if rule == "" {
		return Spec{Frequency: "ONCE", Interval: 1, Start: start.In(loc), Timezone: zone}, nil
	}
	s := Spec{Interval: 1, Start: start.In(loc), Timezone: zone}
	seen := map[string]bool{}
	for _, part := range strings.Split(rule, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return Spec{}, fmt.Errorf("invalid RRULE clause")
		}
		k, v := strings.ToUpper(kv[0]), strings.ToUpper(kv[1])
		if seen[k] {
			return Spec{}, fmt.Errorf("duplicate RRULE clause")
		}
		seen[k] = true
		switch k {
		case "FREQ":
			if v != "DAILY" && v != "WEEKLY" {
				return Spec{}, fmt.Errorf("unsupported frequency")
			}
			s.Frequency = v
		case "INTERVAL":
			if _, e := fmt.Sscanf(v, "%d", &s.Interval); e != nil || s.Interval < 1 || s.Interval > 30 {
				return Spec{}, fmt.Errorf("invalid interval")
			}
		case "COUNT":
			if _, e := fmt.Sscanf(v, "%d", &s.Count); e != nil || s.Count < 1 || s.Count > 366 {
				return Spec{}, fmt.Errorf("invalid count")
			}
		case "UNTIL":
			t, e := time.Parse("20060102T150405Z", v)
			if e != nil {
				return Spec{}, fmt.Errorf("invalid until")
			}
			x := t.In(loc)
			s.Until = &x
		case "BYDAY":
			if s.Frequency != "WEEKLY" && s.Frequency != "" {
				return Spec{}, fmt.Errorf("BYDAY requires weekly")
			}
			for _, d := range strings.Split(v, ",") {
				wd, e := weekday(d)
				if e != nil {
					return Spec{}, e
				}
				s.ByWeekday = append(s.ByWeekday, wd)
			}
		default:
			return Spec{}, fmt.Errorf("unsupported RRULE clause %s", k)
		}
	}
	if s.Frequency == "" {
		return Spec{}, fmt.Errorf("FREQ required")
	}
	if s.Frequency == "DAILY" && len(s.ByWeekday) > 0 {
		return Spec{}, fmt.Errorf("BYDAY requires weekly")
	}
	if s.Frequency == "WEEKLY" && len(s.ByWeekday) == 0 {
		s.ByWeekday = []time.Weekday{s.Start.Weekday()}
	}
	if end != nil && s.Until == nil {
		x := end.In(loc)
		s.Until = &x
	}
	return s, nil
}
func weekday(v string) (time.Weekday, error) {
	switch v {
	case "SU":
		return time.Sunday, nil
	case "MO":
		return time.Monday, nil
	case "TU":
		return time.Tuesday, nil
	case "WE":
		return time.Wednesday, nil
	case "TH":
		return time.Thursday, nil
	case "FR":
		return time.Friday, nil
	case "SA":
		return time.Saturday, nil
	}
	return 0, fmt.Errorf("invalid weekday")
}
func (s Spec) Occurrences(horizon time.Time) []time.Time {
	loc, _ := time.LoadLocation(s.Timezone)
	cur := s.Start.In(loc)
	out := []time.Time{}
	n := 0
	for !cur.After(horizon.In(loc)) {
		if s.Count > 0 && n >= s.Count {
			break
		}
		if s.Until != nil && cur.After(*s.Until) {
			break
		}
		include := s.Frequency == "ONCE" || s.Frequency == "DAILY" || contains(s.ByWeekday, cur.Weekday())
		if include {
			out = append(out, cur)
			n++
			if s.Frequency == "ONCE" {
				break
			}
		}
		cur = cur.AddDate(0, 0, 1)
		if s.Frequency == "WEEKLY" && cur.Weekday() == s.Start.Weekday() {
			cur = cur.AddDate(0, 0, 7*(s.Interval-1))
		}
		if s.Frequency == "DAILY" && s.Interval > 1 {
			cur = cur.AddDate(0, 0, s.Interval-1)
		}
	}
	return out
}
func contains(xs []time.Weekday, x time.Weekday) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
