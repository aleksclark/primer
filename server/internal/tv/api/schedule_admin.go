package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// scheduleTag groups the admin schedule operations in the OpenAPI spec.
const scheduleTag = "Schedule Entries"

// MaxGridDays bounds the span the week-grid endpoint will render, so a stray
// query cannot ask the server to walk a year of the grid.
const MaxGridDays = 31

// DaysPerWeek is the copy-week shift, in calendar days.
const DaysPerWeek = 7

// GridResponse is a span of the programmed grid for the admin editor.
type GridResponse struct {
	Timezone   string      `json:"timezone" doc:"IANA zone the span's day boundaries were computed in."`
	StartsAt   time.Time   `json:"startsAt"`
	EndsAt     time.Time   `json:"endsAt"`
	Days       int         `json:"days"`
	Programmes []Programme `json:"programmes" doc:"Airings overlapping the span, earliest first."`
	ServerTime time.Time   `json:"serverTime"`
}

// CopyWeekRequest duplicates one week of the grid into another.
type CopyWeekRequest struct {
	FromWeekStart string `json:"fromWeekStart" doc:"First day of the source week, YYYY-MM-DD in the channel timezone."`
	ToWeekStart   string `json:"toWeekStart" doc:"First day of the destination week, YYYY-MM-DD in the channel timezone."`
	Replace       *bool  `json:"replace,omitempty" required:"false" doc:"Delete the destination week's existing entries first. Defaults to false, which keeps them and skips whatever would collide."`
}

// CopiedEntry is one airing the copy could not place.
type SkippedEntry struct {
	MediaItemID string    `json:"mediaItemId" format:"uuid"`
	Title       string    `json:"title"`
	AirsAt      time.Time `json:"airsAt" doc:"Where the entry would have landed."`
	Reason      string    `json:"reason"`
}

// CopyWeekResponse summarizes a copy.
type CopyWeekResponse struct {
	Copied  int            `json:"copied"`
	Deleted int            `json:"deleted" doc:"Destination entries removed because replace was set."`
	Skipped []SkippedEntry `json:"skipped" doc:"Airings that could not be placed, with the reason."`
}

// registerScheduleAdmin wires the admin half of the programmed channel.
//
// Create and update are hand-written because placing an airing is not a plain
// insert: the grid is a single linear stream, so a new or moved entry has to be
// refused when it would overlap one already scheduled. List, get, and delete
// keep the generic CRUD handlers.
func (s *Server) registerScheduleAdmin() {
	guard := s.adminGuard()
	baseapi.RegisterCRUD[domain.ScheduleEntry, struct{}, struct{}](
		s.api, s.q, tvrepo.ScheduleEntries, "schedule-entry", "schedule-entries", "/schedule-entries",
		guard, baseapi.SkipCreate(), baseapi.SkipUpdate())

	conflictErrors := []int{http.StatusConflict, http.StatusUnprocessableEntity}

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID:   "create-schedule-entry",
		Method:        http.MethodPost,
		Path:          "/schedule-entries",
		Summary:       "Place an airing in the grid",
		Description:   "Schedules a media item. The slot runs from airsAt for the item's runtime; an airing that would overlap one already in the grid is refused with 409.",
		Tags:          []string{scheduleTag},
		DefaultStatus: http.StatusCreated,
		Errors:        conflictErrors,
	}), s.createScheduleEntry)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "update-schedule-entry",
		Method:      http.MethodPatch,
		Path:        "/schedule-entries/{id}",
		Summary:     "Move or retime an airing",
		Description: "Partial update: only provided fields change. A move that would overlap another airing is refused with 409.",
		Tags:        []string{scheduleTag},
		Errors:      append(conflictErrors, http.StatusNotFound),
	}), s.updateScheduleEntry)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "get-schedule-grid",
		Method:      http.MethodGet,
		Path:        "/schedule-grid",
		Summary:     "Read a span of the grid",
		Description: "The programmed grid over a run of calendar days, resolved against media metadata. Backs the week-grid editor.",
		Tags:        []string{scheduleTag},
		Errors:      []int{http.StatusBadRequest},
	}), s.getScheduleGrid)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "copy-schedule-week",
		Method:      http.MethodPost,
		Path:        "/schedule-entries/copy-week",
		Summary:     "Copy one week of the grid onto another",
		Description: "Re-airs a week's programming a whole number of weeks later, preserving each airing's weekday and time of day across DST. Airings that would collide are reported rather than dropped silently.",
		Tags:        []string{scheduleTag},
		Errors:      []int{http.StatusBadRequest},
	}), s.copyScheduleWeek)
}

// gridInput selects the span to render.
type gridInput struct {
	From string `query:"from" doc:"First calendar day, YYYY-MM-DD in the channel timezone. Defaults to today."`
	Days int    `query:"days" minimum:"1" maximum:"31" default:"7" doc:"Number of days to render."`
}

// gridOutput wraps a span of the grid.
type gridOutput struct {
	Body GridResponse
}

func (s *Server) getScheduleGrid(ctx context.Context, in *gridInput) (*gridOutput, error) {
	now := s.now()
	start, _, err := s.dayBounds(in.From, now)
	if err != nil {
		return nil, err
	}
	days := in.Days
	if days <= 0 {
		days = DaysPerWeek
	}
	if days > MaxGridDays {
		days = MaxGridDays
	}
	// Calendar days, not multiples of 24h: a week that spans a DST change is
	// still seven midnights.
	end := start.AddDate(0, 0, days)

	airings, err := tvrepo.AiringsBetween(ctx, s.q, start, end)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	programmes := make([]Programme, 0, len(airings))
	for _, a := range airings {
		programmes = append(programmes, toProgramme(a))
	}

	return &gridOutput{Body: GridResponse{
		Timezone:   s.location().String(),
		StartsAt:   start,
		EndsAt:     end,
		Days:       days,
		Programmes: programmes,
		ServerTime: now,
	}}, nil
}

// createScheduleEntryInput wraps the placement body.
type createScheduleEntryInput struct {
	Body ScheduleEntryCreate
}

// scheduleEntryOutput wraps one grid row.
type scheduleEntryOutput struct {
	Body domain.ScheduleEntry
}

func (s *Server) createScheduleEntry(ctx context.Context, in *createScheduleEntryInput) (*scheduleEntryOutput, error) {
	item, err := tvrepo.MediaItems.Get(ctx, s.q, in.Body.MediaItemID)
	if err != nil {
		if errors.Is(err, baserepo.ErrNotFound) {
			return nil, huma.Error422UnprocessableEntity("no such media item")
		}
		return nil, baseapi.MapError(err)
	}
	if item.RuntimeSeconds <= 0 {
		return nil, huma.Error422UnprocessableEntity("cannot schedule an item with an unknown runtime; sync its metadata from Jellyfin first")
	}

	entry, err := tvrepo.CreateScheduleEntry(ctx, s.q,
		in.Body.MediaItemID,
		in.Body.AirsAt,
		derefOr(in.Body.JoinInProgress, true),
		derefOr(in.Body.Block, blockFor(in.Body.AirsAt.In(s.location()))),
	)
	if err != nil {
		return nil, s.scheduleWriteError(ctx, err, in.Body.MediaItemID, in.Body.AirsAt, "")
	}
	return &scheduleEntryOutput{Body: *entry}, nil
}

// updateScheduleEntryInput wraps a partial move.
type updateScheduleEntryInput struct {
	ID   string `path:"id" format:"uuid" doc:"Schedule entry ID."`
	Body ScheduleEntryUpdate
}

func (s *Server) updateScheduleEntry(ctx context.Context, in *updateScheduleEntryInput) (*scheduleEntryOutput, error) {
	current, err := tvrepo.ScheduleEntries.Get(ctx, s.q, in.ID)
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	mediaItemID := derefOr(in.Body.MediaItemID, current.MediaItemID)
	airsAt := derefOr(in.Body.AirsAt, current.AirsAt)
	joinInProgress := derefOr(in.Body.JoinInProgress, current.JoinInProgress)
	block := derefOr(in.Body.Block, current.Block)

	entry, err := tvrepo.UpdateScheduleEntry(ctx, s.q, in.ID, mediaItemID, airsAt, joinInProgress, block)
	if err != nil {
		return nil, s.scheduleWriteError(ctx, err, mediaItemID, airsAt, in.ID)
	}
	return &scheduleEntryOutput{Body: *entry}, nil
}

// scheduleWriteError turns a refused placement into a 409 naming what it hit,
// so the editor can tell the parent which airing is in the way rather than
// just that something is.
func (s *Server) scheduleWriteError(ctx context.Context, err error, mediaItemID string, airsAt time.Time, excludeID string) error {
	if !errors.Is(err, tvrepo.ErrScheduleConflict) {
		return baseapi.MapError(err)
	}
	conflicts, listErr := tvrepo.ConflictingAirings(ctx, s.q, mediaItemID, airsAt, excludeID)
	if listErr != nil || len(conflicts) == 0 {
		return huma.Error409Conflict("that slot overlaps an airing already in the grid")
	}
	names := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		names = append(names, fmt.Sprintf("%s (%s)", c.Title, c.AirsAt.In(s.location()).Format("Mon 15:04")))
	}
	return huma.Error409Conflict("that slot overlaps " + strings.Join(names, ", "))
}

// copyWeekInput wraps the copy request.
type copyWeekInput struct {
	Body CopyWeekRequest
}

// copyWeekOutput wraps the copy summary.
type copyWeekOutput struct {
	Body CopyWeekResponse
}

func (s *Server) copyScheduleWeek(ctx context.Context, in *copyWeekInput) (*copyWeekOutput, error) {
	from, _, err := s.dayBounds(in.Body.FromWeekStart, s.now())
	if err != nil {
		return nil, err
	}
	to, _, err := s.dayBounds(in.Body.ToWeekStart, s.now())
	if err != nil {
		return nil, err
	}
	if from.Equal(to) {
		return nil, huma.Error400BadRequest("the source and destination weeks are the same")
	}

	// The shift is expressed in whole days so an airing keeps its weekday and
	// its wall-clock time even when the two weeks sit on opposite sides of a
	// DST change. A duration shift would move a 9am programme to 8am.
	shiftDays := int(to.Sub(from).Hours()/24 + 0.5)

	source, err := tvrepo.AiringsBetween(ctx, s.q, from, from.AddDate(0, 0, DaysPerWeek))
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	out := CopyWeekResponse{Skipped: []SkippedEntry{}}
	if in.Body.Replace != nil && *in.Body.Replace {
		existing, err := tvrepo.AiringsBetween(ctx, s.q, to, to.AddDate(0, 0, DaysPerWeek))
		if err != nil {
			return nil, baseapi.MapError(err)
		}
		for _, e := range existing {
			if err := tvrepo.ScheduleEntries.Delete(ctx, s.q, e.ID); err != nil {
				return nil, baseapi.MapError(err)
			}
			out.Deleted++
		}
	}

	for _, a := range source {
		// Only airings that start inside the source week are copied: one that
		// began the previous week and is merely still running would otherwise
		// be re-aired from its middle.
		if a.AirsAt.Before(from) {
			continue
		}
		airsAt := shiftDaysInZone(a.AirsAt, shiftDays, s.location())
		_, err := tvrepo.CreateScheduleEntry(ctx, s.q, a.MediaItemID, airsAt, a.JoinInProgress, a.Block)
		switch {
		case err == nil:
			out.Copied++
		case errors.Is(err, tvrepo.ErrScheduleConflict):
			out.Skipped = append(out.Skipped, SkippedEntry{
				MediaItemID: a.MediaItemID,
				Title:       a.Title,
				AirsAt:      airsAt,
				Reason:      "overlaps an airing already scheduled in the destination week",
			})
		default:
			return nil, baseapi.MapError(err)
		}
	}
	return &copyWeekOutput{Body: out}, nil
}

// shiftDaysInZone moves an instant by whole calendar days in the given zone,
// preserving its wall-clock time across a DST boundary.
func shiftDaysInZone(at time.Time, days int, loc *time.Location) time.Time {
	return at.In(loc).AddDate(0, 0, days).UTC()
}

// blockFor labels an airing's day-part from its local start time, so the
// parent does not have to restate what the clock already says.
func blockFor(local time.Time) string {
	switch hour := local.Hour(); {
	case hour < 11:
		return domain.BlockMorning
	case hour < 14:
		return domain.BlockMidday
	case hour < 18:
		return domain.BlockAfternoon
	default:
		return domain.BlockEvening
	}
}

// derefOr returns the pointee, or the fallback when the pointer is nil.
func derefOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
