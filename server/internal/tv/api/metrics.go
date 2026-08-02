package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// metricsTag groups the viewing-statistics operations in the spec.
const metricsTag = "Metrics"

// DefaultMetricsDays is the reporting window used when none is given: a
// fortnight is long enough to show a rotation cycle without burying this week.
const DefaultMetricsDays = 14

// MetricsResponse is the whole viewing picture for one window. It is returned
// as a single document because every panel of the dashboard is drawn from the
// same window, and four round trips to draw one screen would be wasteful on a
// household LAN.
type MetricsResponse struct {
	From          time.Time                   `json:"from" doc:"Start of the reporting window."`
	To            time.Time                   `json:"to" doc:"End of the reporting window, exclusive."`
	Timezone      string                      `json:"timezone" doc:"IANA zone the daily buckets use."`
	ByClass       []tvrepo.WatchTimeByClass   `json:"byClass"`
	BySubject     []tvrepo.WatchTimeBySubject `json:"bySubject" doc:"A viewing counts once per subject tag its item carries, so these may sum to more than the total."`
	ByDay         []tvrepo.WatchTimeByDay     `json:"byDay"`
	Completion    tvrepo.CompletionStats      `json:"completion"`
	Entertainment tvrepo.EntertainmentUsage   `json:"entertainment"`
}

// metricsInput selects the reporting window.
type metricsInput struct {
	Days int `query:"days" minimum:"1" maximum:"365" default:"14" doc:"Length of the reporting window in days, counting back from now."`
}

type metricsOutput struct {
	Body MetricsResponse
}

// registerMetrics wires the viewing dashboard's data source.
func (s *Server) registerMetrics() {
	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "get-metrics",
		Method:      http.MethodGet,
		Path:        "/metrics",
		Summary:     "Viewing statistics",
		Description: "Watch time by class, subject, and day, plus completion rates and how much of the entertainment ration was spent.",
		Tags:        []string{metricsTag},
	}), s.getMetrics)
}

func (s *Server) getMetrics(ctx context.Context, in *metricsInput) (*metricsOutput, error) {
	days := in.Days
	if days <= 0 {
		days = DefaultMetricsDays
	}
	// The window runs to the end of today in the channel's zone so a viewing
	// from an hour ago is inside it, and starts at a day boundary so the first
	// and last buckets are whole days.
	now := s.now().In(s.channelLocation)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.channelLocation).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -days)

	byClass, err := tvrepo.WatchTimeByClassBetween(ctx, s.q, from, to)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	bySubject, err := tvrepo.WatchTimeBySubjectBetween(ctx, s.q, from, to)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	byDay, err := tvrepo.WatchTimeByDayBetween(ctx, s.q, from, to, s.channelLocation.String())
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	completion, err := tvrepo.CompletionStatsBetween(ctx, s.q, from, to)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	entertainment, err := tvrepo.EntertainmentUsageBetween(ctx, s.q, from, to)
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	return &metricsOutput{Body: MetricsResponse{
		From:          from,
		To:            to,
		Timezone:      s.channelLocation.String(),
		ByClass:       nonNil(byClass),
		BySubject:     nonNil(bySubject),
		ByDay:         nonNil(byDay),
		Completion:    *completion,
		Entertainment: *entertainment,
	}}, nil
}

// nonNil renders an empty result as [] rather than null, so the SPA can map
// over it without a guard.
func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
