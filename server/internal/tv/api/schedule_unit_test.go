package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/domain"
)

func TestChannelTimezoneDefaultsToCentral(t *testing.T) {
	t.Parallel()
	assert.Equal(t, DefaultChannelTimezone, ChannelLocation("").String())
	assert.Equal(t, "America/New_York", ChannelLocation("America/New_York").String())
}

func TestUnknownChannelTimezoneFallsBackToUTC(t *testing.T) {
	t.Parallel()
	// A misconfigured zone must not take the channel off the air.
	assert.Equal(t, time.UTC, ChannelLocation("Mars/Olympus_Mons"))
}

func TestDayBoundsUseCalendarDaysAcrossDST(t *testing.T) {
	t.Parallel()
	s := &Server{channelLocation: ChannelLocation(DefaultChannelTimezone)}

	// 9 March 2031 is the US spring-forward Sunday: the local day is 23 hours
	// long, and a fixed 24-hour span would land an hour into the next day.
	start, end, err := s.dayBounds("2031-03-09", time.Now())
	require.NoError(t, err)
	assert.Equal(t, 23*time.Hour, end.Sub(start))
	assert.Equal(t, "2031-03-09", start.In(s.location()).Format(DayFormat))
	assert.Equal(t, "2031-03-10", end.In(s.location()).Format(DayFormat))
}

func TestDayBoundsDefaultToTheServersToday(t *testing.T) {
	t.Parallel()
	s := &Server{channelLocation: ChannelLocation(DefaultChannelTimezone)}

	// 03:00 UTC on 9 April is still 8 April in Central time.
	now := time.Date(2031, 4, 9, 3, 0, 0, 0, time.UTC)
	start, _, err := s.dayBounds("", now)
	require.NoError(t, err)
	assert.Equal(t, "2031-04-08", start.In(s.location()).Format(DayFormat),
		"the day is the one the household is living, not the one UTC is")
}

func TestDayBoundsRejectMalformedDays(t *testing.T) {
	t.Parallel()
	s := &Server{channelLocation: time.UTC}

	for _, day := range []string{"2031-13-40", "08/04/2031", "tomorrow"} {
		_, _, err := s.dayBounds(day, time.Now())
		assert.Error(t, err, "day %q must be refused", day)
	}
}

func TestLocationFallsBackToUTCWhenUnset(t *testing.T) {
	t.Parallel()
	s := &Server{}
	assert.Equal(t, time.UTC, s.location())
}

func TestBlockForLabelsTheDayPart(t *testing.T) {
	t.Parallel()
	loc := ChannelLocation(DefaultChannelTimezone)
	day := func(hour int) time.Time { return time.Date(2031, 4, 8, hour, 0, 0, 0, loc) }

	assert.Equal(t, domain.BlockMorning, blockFor(day(6)))
	assert.Equal(t, domain.BlockMorning, blockFor(day(10)))
	assert.Equal(t, domain.BlockMidday, blockFor(day(11)))
	assert.Equal(t, domain.BlockMidday, blockFor(day(13)))
	assert.Equal(t, domain.BlockAfternoon, blockFor(day(14)))
	assert.Equal(t, domain.BlockAfternoon, blockFor(day(17)))
	assert.Equal(t, domain.BlockEvening, blockFor(day(18)))
	assert.Equal(t, domain.BlockEvening, blockFor(day(23)))
}

func TestShiftDaysInZonePreservesWallClockTimeAcrossDST(t *testing.T) {
	t.Parallel()
	loc := ChannelLocation(DefaultChannelTimezone)

	// 09:00 Central on the Tuesday before the spring-forward Sunday.
	before := time.Date(2031, 3, 4, 9, 0, 0, 0, loc)
	after := shiftDaysInZone(before, DaysPerWeek, loc)

	assert.Equal(t, 9, after.In(loc).Hour(),
		"a 9am programme is still 9am the week after the clocks change")
	assert.Equal(t, time.Tuesday, after.In(loc).Weekday())
	// The instants are 167 hours apart, not 168: the intervening day was short.
	assert.Equal(t, 167*time.Hour, after.Sub(before))
}

func TestDerefOrFallsBackOnNil(t *testing.T) {
	t.Parallel()
	value := "set"
	assert.Equal(t, "set", derefOr(&value, "fallback"))
	assert.Equal(t, "fallback", derefOr((*string)(nil), "fallback"))
	assert.True(t, derefOr((*bool)(nil), true))
}
