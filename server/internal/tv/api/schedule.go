package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// DayFormat is the calendar-day form accepted by GET /schedule.
const DayFormat = "2006-01-02"

// DefaultChannelTimezone is the IANA zone the programmed grid is bucketed into
// when Options leaves it unset. The family is in Tennessee, whose populous half
// (Nashville, Memphis) keeps Central time.
const DefaultChannelTimezone = "America/Chicago"

// Programme is one airing as a client should render it: the grid placement,
// the resolved end of the slot, and enough metadata for an EPG cell.
type Programme struct {
	ScheduleEntryID string    `json:"scheduleEntryId" format:"uuid"`
	MediaItemID     string    `json:"mediaItemId" format:"uuid"`
	Title           string    `json:"title"`
	Overview        string    `json:"overview"`
	Class           string    `json:"class" enum:"educational,entertainment,mixed"`
	SubjectTags     []string  `json:"subjectTags"`
	RuntimeSeconds  int       `json:"runtimeSeconds"`
	AirsAt          time.Time `json:"airsAt"`
	EndsAt          time.Time `json:"endsAt" doc:"airsAt plus the item's runtime; the slot is half-open."`
	Block           string    `json:"block" enum:"morning,midday,afternoon,evening"`
	JoinInProgress  bool      `json:"joinInProgress" doc:"Whether a device tuning in late joins at the broadcast offset instead of the start."`
	DirectPlayOK    bool      `json:"directPlayOk"`
	ImageURL        string    `json:"imageUrl" doc:"TV-server-proxied artwork URL."`
}

// NowResponse is the channel's current state.
//
// Both programme fields are optional and independently so: an empty grid has
// neither, a gap before the first evening slot has only next, and the last
// programme of the day has only onAir.
type NowResponse struct {
	OnAir               *Programme `json:"onAir,omitempty" doc:"The programme airing right now, absent when the channel is in a gap."`
	OffsetSeconds       int        `json:"offsetSeconds" doc:"Seconds elapsed since onAir started, per the server clock."`
	StartOffsetSeconds  int        `json:"startOffsetSeconds" doc:"Position a device tuning in now must start from. Zero when the entry forbids joining in progress."`
	Next                *Programme `json:"next,omitempty" doc:"The next programme to start, absent when nothing further is scheduled."`
	NextStartsInSeconds int        `json:"nextStartsInSeconds" doc:"Seconds until next starts."`
	ServerTime          time.Time  `json:"serverTime" doc:"Server clock; clients trust this over their own."`
}

// ScheduleResponse is one day of the programmed grid.
type ScheduleResponse struct {
	Day         string      `json:"day" doc:"The calendar day rendered, in YYYY-MM-DD."`
	Timezone    string      `json:"timezone" doc:"IANA zone the day boundaries were computed in."`
	DayStartsAt time.Time   `json:"dayStartsAt" doc:"Midnight opening the day, as an instant."`
	DayEndsAt   time.Time   `json:"dayEndsAt" doc:"Midnight closing the day, as an instant."`
	Programmes  []Programme `json:"programmes" doc:"Airings overlapping the day, earliest first. A programme running past midnight appears on both days."`
	ServerTime  time.Time   `json:"serverTime"`
}

// registerScheduleRoutes wires the device-facing channel endpoints.
func (s *Server) registerScheduleRoutes(deviceOp func(huma.Operation) huma.Operation) {
	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID: "get-now",
		Method:      http.MethodGet,
		Path:        "/now",
		Summary:     "What is on the channel now",
		Description: "Resolves the programmed grid against the server clock and returns the airing programme plus the offset to join it at. When the channel is in a gap, onAir is absent and next describes the upcoming programme.",
	}), s.getNow)

	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID: "get-schedule",
		Method:      http.MethodGet,
		Path:        "/schedule",
		Summary:     "One day of the programmed grid",
		Description: "The EPG for a calendar day. Days are bucketed in the channel's configured timezone, not UTC and not the client's, so the grid a parent authored as \"Tuesday morning\" reads back as Tuesday morning on every device.",
		Errors:      []int{http.StatusBadRequest},
	}), s.getSchedule)
}

// nowOutput wraps the channel state.
type nowOutput struct {
	Body NowResponse
}

func (s *Server) getNow(ctx context.Context, _ *struct{}) (*nowOutput, error) {
	if _, err := device(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	body := NowResponse{ServerTime: now}

	airing, err := tvrepo.AiringAt(ctx, s.q, now)
	switch {
	case err == nil:
		programme := toProgramme(*airing)
		body.OnAir = &programme
		body.OffsetSeconds = airing.OffsetSecondsAt(now)
		body.StartOffsetSeconds = airing.StartOffsetSecondsAt(now)
	case errors.Is(err, baserepo.ErrNotFound):
	default:
		return nil, baseapi.MapError(err)
	}

	next, err := tvrepo.NextAiringAfter(ctx, s.q, now)
	switch {
	case err == nil:
		programme := toProgramme(*next)
		body.Next = &programme
		body.NextStartsInSeconds = int(next.AirsAt.Sub(now).Seconds())
	case errors.Is(err, baserepo.ErrNotFound):
	default:
		return nil, baseapi.MapError(err)
	}

	return &nowOutput{Body: body}, nil
}

// scheduleInput selects the day to render.
type scheduleInput struct {
	Day string `query:"day" doc:"Calendar day as YYYY-MM-DD in the channel timezone. Defaults to today."`
}

// scheduleOutput wraps the day's grid.
type scheduleOutput struct {
	Body ScheduleResponse
}

func (s *Server) getSchedule(ctx context.Context, in *scheduleInput) (*scheduleOutput, error) {
	if _, err := device(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	start, end, err := s.dayBounds(in.Day, now)
	if err != nil {
		return nil, err
	}

	airings, err := tvrepo.AiringsBetween(ctx, s.q, start, end)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	programmes := make([]Programme, 0, len(airings))
	for _, a := range airings {
		programmes = append(programmes, toProgramme(a))
	}

	return &scheduleOutput{Body: ScheduleResponse{
		Day:         start.In(s.location()).Format(DayFormat),
		Timezone:    s.location().String(),
		DayStartsAt: start,
		DayEndsAt:   end,
		Programmes:  programmes,
		ServerTime:  now,
	}}, nil
}

// location is the channel's timezone, falling back to UTC only if the
// configured zone could not be loaded (which New reports at startup).
func (s *Server) location() *time.Location {
	if s.channelLocation == nil {
		return time.UTC
	}
	return s.channelLocation
}

// dayBounds resolves a YYYY-MM-DD day to the instants that open and close it in
// the channel timezone. An empty day means today. Midnight-to-midnight is
// computed by adding a calendar day rather than 24 hours, so a DST transition
// does not shift the boundary by an hour.
func (s *Server) dayBounds(day string, now time.Time) (time.Time, time.Time, error) {
	loc := s.location()
	local := now.In(loc)
	if day != "" {
		parsed, err := time.ParseInLocation(DayFormat, day, loc)
		if err != nil {
			return time.Time{}, time.Time{}, huma.Error400BadRequest("day must be a calendar date in YYYY-MM-DD form")
		}
		local = parsed
	}
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 1), nil
}

// toProgramme flattens a resolved airing into its wire form.
func toProgramme(a tvrepo.Airing) Programme {
	tags := a.SubjectTags
	if tags == nil {
		tags = []string{}
	}
	return Programme{
		ScheduleEntryID: a.ID,
		MediaItemID:     a.MediaItemID,
		Title:           a.Title,
		Overview:        a.Overview,
		Class:           a.Class,
		SubjectTags:     tags,
		RuntimeSeconds:  a.RuntimeSeconds,
		AirsAt:          a.AirsAt,
		EndsAt:          a.EndsAt,
		Block:           a.Block,
		JoinInProgress:  a.JoinInProgress,
		DirectPlayOK:    a.DirectPlayOK,
		ImageURL:        imageProxyPath(a.MediaItemID, "Primary"),
	}
}

// programmedGrant issues a grant for the channel's current programme.
//
// It refuses anything that is not airing at this instant: the channel is a
// broadcast, and the plan's rule is that a missed slot is missed, so there are
// no catch-up grants for a programme that has already ended or has not started.
func (s *Server) programmedGrant(ctx context.Context, mediaItemID, deviceID string, now time.Time) (*domain.PlayGrant, error) {
	airing, err := tvrepo.AiringAt(ctx, s.q, now)
	if err != nil {
		if errors.Is(err, baserepo.ErrNotFound) {
			return nil, huma.Error403Forbidden("nothing is airing on the channel right now")
		}
		return nil, baseapi.MapError(err)
	}
	if airing.MediaItemID != mediaItemID {
		return nil, huma.Error403Forbidden("that programme is not airing now; the channel does not offer catch-up")
	}
	if !airing.DirectPlayOK {
		return nil, huma.Error403Forbidden("item is not direct-play compatible")
	}

	grant, err := tvrepo.PlayGrants.Create(ctx, s.q, map[string]any{
		"media_item_id":        airing.MediaItemID,
		"device_id":            deviceID,
		"schedule_entry_id":    airing.ID,
		"mode":                 domain.ModeProgrammed,
		"stream_url":           s.jellyfin.StreamURL(airing.JellyfinItemID),
		"start_offset_seconds": airing.StartOffsetSecondsAt(now),
		"issued_at":            now,
		"expires_at":           now.Add(s.grantTTL),
	})
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return grant, nil
}
