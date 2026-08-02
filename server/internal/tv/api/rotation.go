package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// rotationTag groups the on-demand rotation operations in the spec.
const rotationTag = "Rotation"

// DefaultRotationDays is how long a rotation's windows stay open when the
// caller does not say. A week matches the household rhythm the channel is
// scheduled around.
const DefaultRotationDays = 7

// RotationSuggestion is an item worth putting back in front of the student.
type RotationSuggestion struct {
	MediaItem         domain.MediaItem `json:"mediaItem"`
	LastWindowEndedAt time.Time        `json:"lastWindowEndedAt" doc:"When this item was last on offer. The Unix epoch means it never has been."`
}

// RotateRequest asks the server to close what is open and offer a fresh set.
type RotateRequest struct {
	MediaItemIDs []string `json:"mediaItemIds" doc:"Items to offer. Empty takes the server's own suggestions." required:"false"`
	Days         int      `json:"days,omitempty" minimum:"1" maximum:"90" doc:"How long the new windows stay open. Defaults to 7." required:"false"`
	ExpireOpen   bool     `json:"expireOpen,omitempty" doc:"Close everything currently on offer first, so the catalog turns over rather than growing." required:"false"`
	Limit        int      `json:"limit,omitempty" minimum:"1" maximum:"50" doc:"How many suggestions to take when no items are named. Defaults to 8." required:"false"`
}

// RotateResponse reports what the rotation actually changed.
type RotateResponse struct {
	Expired int                         `json:"expired" doc:"Windows closed."`
	Opened  []domain.AvailabilityWindow `json:"opened" doc:"Windows opened. Items that already had one open are skipped."`
}

type suggestionsInput struct {
	Limit int `query:"limit" minimum:"1" maximum:"50" default:"20" doc:"Maximum suggestions to return."`
}

type suggestionsOutput struct {
	Body struct {
		Suggestions []RotationSuggestion `json:"suggestions"`
	}
}

type rotateInput struct {
	Body RotateRequest
}

type rotateOutput struct {
	Body RotateResponse
}

// registerRotation wires the rotation helpers that spare the parent from
// hand-curating the on-demand catalog every week.
func (s *Server) registerRotation() {
	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "list-rotation-suggestions",
		Method:      http.MethodGet,
		Path:        "/rotation/suggestions",
		Summary:     "Suggest what to offer next",
		Description: "Direct-playable items with nothing currently on offer and no viewing yet spent, oldest offer first.",
		Tags:        []string{rotationTag},
	}), s.rotationSuggestions)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "rotate-availability",
		Method:      http.MethodPost,
		Path:        "/rotation/rotate",
		Summary:     "Turn the catalog over",
		Description: "Optionally closes every open window, then offers the named items (or the server's suggestions) for a fixed span.",
		Tags:        []string{rotationTag},
	}), s.rotate)
}

func (s *Server) rotationSuggestions(ctx context.Context, in *suggestionsInput) (*suggestionsOutput, error) {
	candidates, err := tvrepo.SuggestRotation(ctx, s.q, s.now(), in.Limit)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	out := &suggestionsOutput{}
	out.Body.Suggestions = make([]RotationSuggestion, 0, len(candidates))
	for _, c := range candidates {
		out.Body.Suggestions = append(out.Body.Suggestions, RotationSuggestion{
			MediaItem:         c.MediaItem,
			LastWindowEndedAt: c.LastWindowEndedAt,
		})
	}
	return out, nil
}

func (s *Server) rotate(ctx context.Context, in *rotateInput) (*rotateOutput, error) {
	now := s.now()

	days := in.Body.Days
	if days <= 0 {
		days = DefaultRotationDays
	}

	ids := in.Body.MediaItemIDs
	if len(ids) == 0 {
		limit := in.Body.Limit
		if limit <= 0 {
			limit = 8
		}
		// Suggestions are taken before anything is expired, so closing the
		// current offers cannot make this week's titles suggest themselves
		// straight back.
		candidates, err := tvrepo.SuggestRotation(ctx, s.q, now, limit)
		if err != nil {
			return nil, baseapi.MapError(err)
		}
		for _, c := range candidates {
			ids = append(ids, c.ID)
		}
	}

	body := RotateResponse{Opened: []domain.AvailabilityWindow{}}

	if in.Body.ExpireOpen {
		expired, err := tvrepo.ExpireOpenWindows(ctx, s.q, now)
		if err != nil {
			return nil, baseapi.MapError(err)
		}
		body.Expired = expired
	}

	opened, err := tvrepo.OpenWindowsFor(ctx, s.q, ids, now, now.AddDate(0, 0, days))
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	if opened != nil {
		body.Opened = opened
	}
	return &rotateOutput{Body: body}, nil
}
