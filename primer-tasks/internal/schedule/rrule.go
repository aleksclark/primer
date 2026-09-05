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
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil
	}
	start := s.Start.In(loc)
	year, month, day := start.Date()
	hour, minute, second := start.Clock()
	nano := start.Nanosecond()
	out := []time.Time{}
	n := 0
	for offset := 0; offset < 4000; offset++ {
		instants := civilInstants(loc, year, month, day, hour, minute, second, nano, offset)
		if len(instants) == 0 {
			continue
		}
		for _, cur := range instants {
			if cur.After(horizon.In(loc)) {
				return out
			}
			if s.Count > 0 && n >= s.Count {
				return out
			}
			if s.Until != nil && cur.After(*s.Until) {
				return out
			}
			if !includeOccurrence(s, loc, offset, cur) {
				continue
			}
			out = append(out, cur)
			n++
			if s.Frequency == "ONCE" {
				return out
			}
		}
	}
	return out
}

func includeOccurrence(s Spec, loc *time.Location, offset int, cur time.Time) bool {
	local := cur.In(loc)
	include := s.Frequency == "ONCE" || s.Frequency == "DAILY" || contains(s.ByWeekday, local.Weekday())
	if s.Frequency == "DAILY" && s.Interval > 1 && offset%s.Interval != 0 {
		return false
	}
	if s.Frequency == "WEEKLY" && s.Interval > 1 {
		start := s.Start.In(loc)
		days := int(local.Sub(time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)).Hours() / 24)
		if days < 0 || (days/7)%s.Interval != 0 {
			return false
		}
	}
	return include
}

// civilInstants maps a civil local wall time onto one or more instants.
// DST policy for named IANA zones:
//   - gap (spring-forward): skip the missing wall time; do not invent 02:30.
//   - fold (fall-back): emit both instants that share the repeated wall time.
func civilInstants(loc *time.Location, year int, month time.Month, day, hour, minute, second, nano, offsetDays int) []time.Time {
	noon := time.Date(year, month, day, 12, 0, 0, 0, loc).AddDate(0, 0, offsetDays)
	y, m, d := noon.Date()
	first := time.Date(y, m, d, hour, minute, second, nano, loc)
	if first.Hour() != hour || first.Minute() != minute {
		return nil
	}
	out := []time.Time{first}
	later := first.Add(time.Hour)
	if later.In(loc).Hour() == hour && later.In(loc).Minute() == minute && !later.Equal(first) {
		out = append(out, later)
	}
	earlier := first.Add(-time.Hour)
	if earlier.In(loc).Hour() == hour && earlier.In(loc).Minute() == minute && !earlier.Equal(first) {
		out = []time.Time{earlier, first}
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
