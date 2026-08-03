package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/auth"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// deviceTag groups the device-facing operations in the OpenAPI spec.
const deviceTag = "Device"

// PairRequest is the body a device sends to redeem a pairing code.
type PairRequest struct {
	Code string `json:"code" minLength:"4" doc:"Pairing code displayed in the admin UI."`
}

// PairResponse carries the issued device token. The token is returned exactly
// once; only its hash is retained server-side.
type PairResponse struct {
	Token  string        `json:"token" doc:"Bearer token for subsequent device requests."`
	Device domain.Device `json:"device"`
}

// CatalogItem is an on-demand item a device may play right now.
type CatalogItem struct {
	MediaItem            domain.MediaItem `json:"mediaItem"`
	AvailabilityWindowID string           `json:"availabilityWindowId" format:"uuid"`
	WindowEndsAt         time.Time        `json:"windowEndsAt"`
	ImageURL             string           `json:"imageUrl" doc:"TV-server-proxied artwork URL."`
}

// CatalogResponse is the device catalog listing.
type CatalogResponse struct {
	Items      []CatalogItem `json:"items"`
	ServerTime time.Time     `json:"serverTime" doc:"Server clock; clients trust this over their own."`
}

// GrantResponse authorizes one playback.
type GrantResponse struct {
	GrantID              string    `json:"grantId" format:"uuid"`
	StreamURL            string    `json:"streamUrl"`
	StartOffsetSeconds   int       `json:"startOffsetSeconds" doc:"Where the player should start. On demand this is resume-30s from the furthest position; programmed is the broadcast offset."`
	FurthestPositionSeconds int    `json:"furthestPositionSeconds" doc:"Furthest playhead position this device has reached on the item. On-demand seek ceiling; zero when fresh."`
	Mode                 string    `json:"mode" enum:"on_demand,programmed"`
	ExpiresAt            time.Time `json:"expiresAt"`
	ServerTime           time.Time `json:"serverTime"`
}

// HeartbeatRequest reports playback progress.
type HeartbeatRequest struct {
	PositionSeconds int `json:"positionSeconds" minimum:"0" doc:"Current playhead position."`
	WatchedSeconds  int `json:"watchedSeconds,omitempty" minimum:"0" required:"false" doc:"Seconds actually watched; defaults to the position."`
}

// CompleteRequest closes out a playback session.
type CompleteRequest struct {
	PositionSeconds int   `json:"positionSeconds" minimum:"0"`
	WatchedSeconds  int   `json:"watchedSeconds,omitempty" minimum:"0" required:"false"`
	Completed       *bool `json:"completed,omitempty" required:"false" doc:"Whether playback reached the end. Defaults to true."`
}

// SessionResponse reports the stored state of a playback session.
type SessionResponse struct {
	Session      domain.PlaybackSession `json:"session"`
	PlayConsumed bool                   `json:"playConsumed" doc:"Whether this call burned the item's availability window."`
	ServerTime   time.Time              `json:"serverTime"`
}

// deviceOp stamps device authentication onto an operation.
func (s *Server) deviceOp(op huma.Operation) huma.Operation {
	op.Tags = []string{deviceTag}
	op.Security = []map[string][]string{{deviceSecurityScheme: {}}}
	op.Middlewares = huma.Middlewares{s.requireDevice()}
	op.Errors = append(op.Errors, http.StatusUnauthorized)
	return op
}

// registerDeviceRoutes wires the paired-device API.
func (s *Server) registerDeviceRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID:   "pair-device",
		Method:        http.MethodPost,
		Path:          "/devices/pair",
		Summary:       "Pair a device",
		Description:   "Exchange a pairing code shown in the admin UI for a device token. Codes are single-use and expire.",
		Tags:          []string{deviceTag},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusForbidden},
	}, s.pairDevice)

	deviceOp := s.deviceOp

	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID: "get-catalog",
		Method:      http.MethodGet,
		Path:        "/catalog",
		Summary:     "List available on-demand items",
		Description: "Media items with an availability window open right now whose play has not already been consumed.",
	}), s.getCatalog)

	s.registerScheduleRoutes(deviceOp)

	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID:   "create-grant",
		Method:        http.MethodPost,
		Path:          "/media/{id}/grant",
		Summary:       "Request a play grant",
		Description:   "Issues a single-use, short-lived playback authorization. In on-demand mode (the default) the item must have an open availability window with its play unconsumed. In programmed mode the item must be the programme airing on the channel right now, and the grant carries the offset to join it at. Returns 403 otherwise.",
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusForbidden},
	}), s.createGrant)

	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID: "heartbeat-grant",
		Method:      http.MethodPost,
		Path:        "/grants/{id}/heartbeat",
		Summary:     "Report playback progress",
		Description: "Redeems the grant on first call and advances the session's watch counters.",
		Errors:      []int{http.StatusForbidden},
	}), s.heartbeatGrant)

	huma.Register(s.api, deviceOp(huma.Operation{
		OperationID: "complete-grant",
		Method:      http.MethodPost,
		Path:        "/grants/{id}/complete",
		Summary:     "Finish a playback session",
		Description: "Closes the session and, for entertainment items watched past the completion threshold, consumes the availability window.",
		Errors:      []int{http.StatusForbidden},
	}), s.completeGrant)
}

// pairInput wraps the pairing request body.
type pairInput struct {
	Body PairRequest
}

// pairOutput wraps the pairing response.
type pairOutput struct {
	Body PairResponse
}

func (s *Server) pairDevice(ctx context.Context, in *pairInput) (*pairOutput, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return nil, huma.Error500InternalServerError("issue device token")
	}
	device, err := tvrepo.ClaimPairingCode(ctx, s.q, in.Body.Code, hash, s.now())
	if err != nil {
		if errors.Is(err, baserepo.ErrNotFound) {
			return nil, huma.Error403Forbidden("pairing code is invalid, expired, or already used")
		}
		return nil, baseapi.MapError(err)
	}
	return &pairOutput{Body: PairResponse{Token: token, Device: *device}}, nil
}

// catalogOutput wraps the catalog listing.
type catalogOutput struct {
	Body CatalogResponse
}

func (s *Server) getCatalog(ctx context.Context, _ *struct{}) (*catalogOutput, error) {
	if _, err := device(ctx); err != nil {
		return nil, err
	}
	now := s.now()
	entries, err := tvrepo.Catalog(ctx, s.q, now)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	items := make([]CatalogItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, CatalogItem{
			MediaItem:            e.MediaItem,
			AvailabilityWindowID: e.AvailabilityWindowID,
			WindowEndsAt:         e.WindowEndsAt,
			ImageURL:             imageProxyPath(e.MediaItem.ID, "Primary"),
		})
	}
	return &catalogOutput{Body: CatalogResponse{Items: items, ServerTime: now}}, nil
}

// grantInput identifies the media item to authorize and the mode to play it
// in. Mode is a query parameter rather than a body field so the two playback
// paths share one operation, one set of session bookkeeping, and one client
// method.
type grantInput struct {
	ID   string `path:"id" format:"uuid" doc:"Media item ID."`
	Mode string `query:"mode" enum:"on_demand,programmed" default:"on_demand" doc:"on_demand plays from the rotation catalog; programmed joins the channel's current airing."`
}

// grantOutput wraps the issued grant.
type grantOutput struct {
	Body GrantResponse
}

func (s *Server) createGrant(ctx context.Context, in *grantInput) (*grantOutput, error) {
	dev, err := device(ctx)
	if err != nil {
		return nil, err
	}
	if s.jellyfin == nil {
		return nil, huma.Error503ServiceUnavailable("media source is not configured")
	}

	now := s.now()
	if in.Mode == domain.ModeProgrammed {
		grant, err := s.programmedGrant(ctx, in.ID, dev.ID, now)
		if err != nil {
			return nil, err
		}
		// Programmed playback has no on-demand seek window; furthest is unused.
		return &grantOutput{Body: grantResponse(grant, 0, now)}, nil
	}

	entry, err := tvrepo.CatalogEntryFor(ctx, s.q, in.ID, now)
	if err != nil {
		if errors.Is(err, baserepo.ErrNotFound) {
			return nil, huma.Error403Forbidden("item is unavailable or its play was already consumed")
		}
		return nil, baseapi.MapError(err)
	}
	if !entry.DirectPlayOK {
		return nil, huma.Error403Forbidden("item is not direct-play compatible")
	}

	furthest, err := tvrepo.MaxPositionForDeviceMedia(ctx, s.q, dev.ID, entry.MediaItem.ID)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	resumeAt := domain.ResumePositionSeconds(furthest)

	expiresAt := now.Add(s.grantTTL)
	grant, err := tvrepo.PlayGrants.Create(ctx, s.q, map[string]any{
		"media_item_id":          entry.MediaItem.ID,
		"device_id":              dev.ID,
		"availability_window_id": entry.AvailabilityWindowID,
		"mode":                   domain.ModeOnDemand,
		"stream_url":             s.jellyfin.StreamURL(entry.JellyfinItemID),
		"start_offset_seconds":   resumeAt,
		"issued_at":              now,
		"expires_at":             expiresAt,
	})
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	return &grantOutput{Body: grantResponse(grant, furthest, now)}, nil
}

// grantResponse renders an issued grant for the client.
func grantResponse(grant *domain.PlayGrant, furthestPositionSeconds int, now time.Time) GrantResponse {
	return GrantResponse{
		GrantID:                 grant.ID,
		StreamURL:               grant.StreamURL,
		StartOffsetSeconds:      grant.StartOffsetSeconds,
		FurthestPositionSeconds: furthestPositionSeconds,
		Mode:                    grant.Mode,
		ExpiresAt:               grant.ExpiresAt,
		ServerTime:              now,
	}
}

// heartbeatInput wraps a progress report for a grant.
type heartbeatInput struct {
	ID   string `path:"id" format:"uuid" doc:"Grant ID."`
	Body HeartbeatRequest
}

// sessionOutput wraps a playback session response.
type sessionOutput struct {
	Body SessionResponse
}

func (s *Server) heartbeatGrant(ctx context.Context, in *heartbeatInput) (*sessionOutput, error) {
	dev, err := device(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	grant, err := s.activeGrant(ctx, in.ID, dev.ID, now)
	if err != nil {
		return nil, err
	}

	watched := in.Body.WatchedSeconds
	if watched == 0 {
		watched = in.Body.PositionSeconds
	}
	session, err := tvrepo.RecordHeartbeat(ctx, s.q, grant, in.Body.PositionSeconds, watched, now)
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	// A student who watches past the threshold and then closes the app never
	// sends a completion, so the play has to be charged from the heartbeat.
	consumed, err := s.consumeIfEarned(ctx, grant, session, now)
	if err != nil {
		return nil, err
	}
	return &sessionOutput{Body: SessionResponse{
		Session:      *session,
		PlayConsumed: consumed,
		ServerTime:   now,
	}}, nil
}

// completeInput wraps a completion report for a grant.
type completeInput struct {
	ID   string `path:"id" format:"uuid" doc:"Grant ID."`
	Body CompleteRequest
}

func (s *Server) completeGrant(ctx context.Context, in *completeInput) (*sessionOutput, error) {
	dev, err := device(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	grant, err := s.activeGrant(ctx, in.ID, dev.ID, now)
	if err != nil {
		return nil, err
	}

	completed := true
	if in.Body.Completed != nil {
		completed = *in.Body.Completed
	}
	watched := in.Body.WatchedSeconds
	if watched == 0 {
		watched = in.Body.PositionSeconds
	}

	session, err := tvrepo.CompleteSession(ctx, s.q, grant, in.Body.PositionSeconds, watched, completed, now)
	if err != nil {
		return nil, baseapi.MapError(err)
	}

	consumed, err := s.consumeIfEarned(ctx, grant, session, now)
	if err != nil {
		return nil, err
	}
	return &sessionOutput{Body: SessionResponse{
		Session:      *session,
		PlayConsumed: consumed,
		ServerTime:   now,
	}}, nil
}

// activeGrant loads a grant for the device, redeeming it on first use. Expiry
// gates only the *start* of playback: a feature-length film runs far longer
// than the grant TTL, so once a grant has been redeemed its session keeps
// accepting progress reports until the client stops sending them. A grant that
// was never redeemed and has since expired is refused.
func (s *Server) activeGrant(ctx context.Context, grantID, deviceID string, now time.Time) (*domain.PlayGrant, error) {
	grant, err := tvrepo.RedeemGrant(ctx, s.q, grantID, deviceID, now)
	if err == nil {
		return grant, nil
	}
	if !errors.Is(err, baserepo.ErrNotFound) {
		return nil, baseapi.MapError(err)
	}

	grant, err = tvrepo.GrantForDevice(ctx, s.q, grantID, deviceID)
	if err != nil {
		if errors.Is(err, baserepo.ErrNotFound) {
			return nil, huma.Error404NotFound("grant not found")
		}
		return nil, baseapi.MapError(err)
	}
	if grant.ConsumedAt == nil {
		return nil, huma.Error403Forbidden("grant expired before playback started")
	}
	return grant, nil
}

// consumeIfEarned writes the watch-once ledger row when an entertainment item
// has been watched far enough. It reports whether this call consumed the play.
func (s *Server) consumeIfEarned(ctx context.Context, grant *domain.PlayGrant, session *domain.PlaybackSession, now time.Time) (bool, error) {
	if grant.AvailabilityWindowID == nil {
		return false, nil
	}
	item, err := tvrepo.MediaItems.Get(ctx, s.q, grant.MediaItemID)
	if err != nil {
		return false, baseapi.MapError(err)
	}
	if !item.ConsumesPlay() {
		return false, nil
	}
	if !domain.SessionCompletesPlay(session.Completed, session.MaxPositionSeconds, item.RuntimeSeconds) {
		return false, nil
	}
	consumed, err := tvrepo.ConsumePlay(ctx, s.q, item.ID, *grant.AvailabilityWindowID, &grant.DeviceID, now)
	if err != nil {
		return false, baseapi.MapError(err)
	}
	return consumed, nil
}
