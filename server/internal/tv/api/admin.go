package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgconn"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	baserepo "github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/auth"
	"github.com/aleksclark/primer/server/internal/tv/directplay"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// Admin OpenAPI tags.
const (
	devicesTag  = "Devices"
	jellyfinTag = "Jellyfin"
	imagesTag   = "Images"
)

// pairingCodeAttempts bounds the retries when a freshly generated pairing code
// collides with an outstanding one.
const pairingCodeAttempts = 5

// syncPageSize is how many media items a metadata refresh loads at a time.
const syncPageSize = 100

// BrowseItem is an unimported Jellyfin library entry offered to the admin UI.
type BrowseItem struct {
	JellyfinItemID string `json:"jellyfinItemId"`
	Title          string `json:"title"`
	SortTitle      string `json:"sortTitle"`
	Overview       string `json:"overview"`
	Type           string `json:"type"`
	RuntimeSeconds int    `json:"runtimeSeconds"`
	Container      string `json:"container"`
	VideoCodec     string `json:"videoCodec"`
	AudioCodec     string `json:"audioCodec"`
	ImageTag       string `json:"imageTag"`
	Imported       bool   `json:"imported" doc:"Whether this item already exists as a media item."`
}

// BrowseResponse is the Jellyfin library listing.
type BrowseResponse struct {
	Items []BrowseItem `json:"items"`
}

// SyncResponse summarizes a metadata refresh.
type SyncResponse struct {
	Checked  int      `json:"checked" doc:"Media items examined."`
	Updated  int      `json:"updated" doc:"Media items whose cached metadata changed."`
	Orphaned []string `json:"orphaned" doc:"Media item IDs whose Jellyfin source has disappeared."`
}

// adminGuard is the CRUD option that authenticates generic admin resources.
func (s *Server) adminGuard() baseapi.CRUDOption {
	return baseapi.Guard(s.requireAdmin(), adminSecurityScheme)
}

// adminOp stamps admin authentication onto a hand-written operation.
func (s *Server) adminOp(op huma.Operation) huma.Operation {
	op.Security = append(op.Security, map[string][]string{adminSecurityScheme: {}})
	op.Middlewares = append(op.Middlewares, s.requireAdmin())
	op.Errors = append(op.Errors, http.StatusUnauthorized)
	return op
}

// registerAdminRoutes wires the admin API consumed by the SPA and by Primer.
func (s *Server) registerAdminRoutes() {
	s.registerAdminCRUD()
	s.registerDeviceAdmin()
	s.registerJellyfinAdmin()
	s.registerPrimerAdmin()
	s.registerContentManifest()
	s.registerMetrics()
	s.registerRotation()
	s.registerImageProxy()
}

// registerDeviceAdmin wires device management. Creation mints a pairing code
// rather than accepting one, so it cannot reuse the generic create handler.
func (s *Server) registerDeviceAdmin() {
	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID:   "create-device",
		Method:        http.MethodPost,
		Path:          "/devices",
		Summary:       "Register a device",
		Description:   "Creates an unpaired device and returns the pairing code to enter on the client.",
		Tags:          []string{devicesTag},
		DefaultStatus: http.StatusCreated,
	}), s.createDevice)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "regenerate-device-pairing-code",
		Method:      http.MethodPost,
		Path:        "/devices/{id}/pairing-code",
		Summary:     "Issue a fresh pairing code",
		Description: "Replaces any outstanding code and clears the stored token, forcing the device to pair again.",
		Tags:        []string{devicesTag},
	}), s.regeneratePairingCode)

	baseapi.RegisterCRUD[domain.Device, struct{}, DeviceUpdate](
		s.api, s.q, tvrepo.Devices, "device", "devices", "/devices",
		s.adminGuard(), baseapi.SkipCreate())
}

// createDeviceInput wraps the device registration body.
type createDeviceInput struct {
	Body DeviceCreate
}

// deviceOutput wraps a device entity.
type deviceOutput struct {
	Body domain.Device
}

func (s *Server) createDevice(ctx context.Context, in *createDeviceInput) (*deviceOutput, error) {
	kind := domain.DeviceTablet
	if in.Body.Kind != nil {
		kind = *in.Body.Kind
	}
	values := map[string]any{"name": in.Body.Name, "kind": kind}
	dev, err := s.withFreshPairingCode(values, func(v map[string]any) (*domain.Device, error) {
		return tvrepo.Devices.Create(ctx, s.q, v)
	})
	if err != nil {
		return nil, err
	}
	return &deviceOutput{Body: *dev}, nil
}

// regenerateInput identifies the device to re-issue a code for.
type regenerateInput struct {
	ID string `path:"id" format:"uuid" doc:"Device ID."`
}

func (s *Server) regeneratePairingCode(ctx context.Context, in *regenerateInput) (*deviceOutput, error) {
	values := map[string]any{"token_hash": "", "paired_at": nil}
	dev, err := s.withFreshPairingCode(values, func(v map[string]any) (*domain.Device, error) {
		return tvrepo.Devices.Update(ctx, s.q, in.ID, v)
	})
	if err != nil {
		return nil, err
	}
	return &deviceOutput{Body: *dev}, nil
}

// withFreshPairingCode fills in a unique pairing code and its expiry, retrying
// on the unique-index collision that a duplicate code would cause.
func (s *Server) withFreshPairingCode(values map[string]any, save func(map[string]any) (*domain.Device, error)) (*domain.Device, error) {
	var lastErr error
	for range pairingCodeAttempts {
		code, err := auth.NewPairingCode()
		if err != nil {
			return nil, huma.Error500InternalServerError("generate pairing code")
		}
		values["pairing_code"] = code
		values["pairing_expires_at"] = s.now().Add(s.pairingTTL)

		dev, err := save(values)
		if err == nil {
			return dev, nil
		}
		lastErr = err
		if !isUniqueViolation(err) {
			return nil, baseapi.MapError(err)
		}
	}
	return nil, baseapi.MapError(lastErr)
}

// registerJellyfinAdmin wires the library browse and metadata sync endpoints.
func (s *Server) registerJellyfinAdmin() {
	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "browse-jellyfin",
		Method:      http.MethodGet,
		Path:        "/jellyfin/browse",
		Summary:     "Browse the Jellyfin library",
		Description: "Lists library items with a flag showing which are already imported.",
		Tags:        []string{jellyfinTag},
		Errors:      []int{http.StatusServiceUnavailable, http.StatusBadGateway},
	}), s.browseJellyfin)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "sync-jellyfin",
		Method:      http.MethodPost,
		Path:        "/jellyfin/sync",
		Summary:     "Refresh cached metadata",
		Description: "Re-reads each imported item from Jellyfin, updating cached metadata and flagging items whose source has disappeared.",
		Tags:        []string{jellyfinTag},
		Errors:      []int{http.StatusServiceUnavailable, http.StatusBadGateway},
	}), s.syncJellyfin)
}

// browseInput filters the Jellyfin library listing.
type browseInput struct {
	ParentID   string `query:"parentId" doc:"Restrict to one library folder or collection."`
	Q          string `query:"q" doc:"Filter by title."`
	Limit      int    `query:"limit" minimum:"1" maximum:"200" default:"50" doc:"Page size."`
	StartIndex int    `query:"startIndex" minimum:"0" default:"0" doc:"Paging offset."`
}

// browseOutput wraps the library listing.
type browseOutput struct {
	Body BrowseResponse
}

func (s *Server) browseJellyfin(ctx context.Context, in *browseInput) (*browseOutput, error) {
	if s.jellyfin == nil {
		return nil, huma.Error503ServiceUnavailable("media source is not configured")
	}
	items, err := s.jellyfin.Browse(ctx, jellyfin.BrowseParams{
		ParentID:   in.ParentID,
		SearchTerm: in.Q,
		Limit:      in.Limit,
		StartIndex: in.StartIndex,
	})
	if err != nil {
		return nil, huma.Error502BadGateway("browse jellyfin library", err)
	}

	imported, err := tvrepo.AllJellyfinIDs(ctx, s.q)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	known := make(map[string]bool, len(imported))
	for _, id := range imported {
		known[id] = true
	}

	out := make([]BrowseItem, 0, len(items))
	for _, it := range items {
		out = append(out, BrowseItem{
			JellyfinItemID: it.ID,
			Title:          it.Name,
			SortTitle:      it.SortName,
			Overview:       it.Overview,
			Type:           it.Type,
			RuntimeSeconds: it.RuntimeSeconds(),
			Container:      it.Container,
			VideoCodec:     it.VideoCodec,
			AudioCodec:     it.AudioCodec,
			ImageTag:       it.ImageTag,
			Imported:       known[it.ID],
		})
	}
	return &browseOutput{Body: BrowseResponse{Items: out}}, nil
}

// syncOutput wraps the sync summary.
type syncOutput struct {
	Body SyncResponse
}

func (s *Server) syncJellyfin(ctx context.Context, _ *struct{}) (*syncOutput, error) {
	if s.jellyfin == nil {
		return nil, huma.Error503ServiceUnavailable("media source is not configured")
	}

	out := SyncResponse{Orphaned: []string{}}
	// Walk the whole library a page at a time: a single page would silently
	// leave later items stale and their orphans undetected. Paging is ordered
	// by primary key because sync itself rewrites updated_at.
	for offset := 0; ; offset += syncPageSize {
		page, err := tvrepo.MediaItems.List(ctx, s.q, baserepo.ListParams{
			Limit:  syncPageSize,
			Offset: offset,
			Sort:   "id",
			Dir:    baserepo.SortAsc,
		})
		if err != nil {
			return nil, baseapi.MapError(err)
		}
		for _, item := range page.Items {
			if err := s.syncItem(ctx, item, &out); err != nil {
				return nil, err
			}
		}
		if offset+len(page.Items) >= page.TotalCount || len(page.Items) == 0 {
			break
		}
	}
	return &syncOutput{Body: out}, nil
}

// syncItem refreshes one media item's cached metadata, recording the outcome
// in the running summary.
func (s *Server) syncItem(ctx context.Context, item domain.MediaItem, out *SyncResponse) error {
	out.Checked++
	remote, err := s.jellyfin.Item(ctx, item.JellyfinItemID)
	if errors.Is(err, jellyfin.ErrNotFound) {
		if err := s.markOrphaned(ctx, item); err != nil {
			return err
		}
		out.Orphaned = append(out.Orphaned, item.ID)
		return nil
	}
	if err != nil {
		return huma.Error502BadGateway("fetch jellyfin item", err)
	}

	values := metadataDiff(item, remote)
	if len(values) == 0 {
		return nil
	}
	if _, err := tvrepo.MediaItems.Update(ctx, s.q, item.ID, values); err != nil {
		return baseapi.MapError(err)
	}
	out.Updated++
	return nil
}

// markOrphaned records that a media item's Jellyfin source has disappeared,
// leaving the row in place so the admin UI can surface the breakage.
func (s *Server) markOrphaned(ctx context.Context, item domain.MediaItem) error {
	if item.OrphanedAt != nil {
		return nil
	}
	if _, err := tvrepo.MediaItems.Update(ctx, s.q, item.ID, map[string]any{"orphaned_at": s.now()}); err != nil {
		return baseapi.MapError(err)
	}
	return nil
}

// isStaleAutoCodecBlock reports whether quality_notes look like a machine-written
// codec block from an older allowlist (e.g. "audio codec eac3"). Blank and
// free-form curator notes are not safe evidence for false→true repair.
func isStaleAutoCodecBlock(notes string) bool {
	trimmed := strings.TrimSpace(notes)
	return trimmed != "" && directplay.IsAutoGeneratedNote(trimmed)
}

// metadataDiff returns the columns whose cached values differ from Jellyfin.
// It also re-evaluates direct_play_ok from the effective codecs on every sync
// so allowlist changes (e.g. AC3/EAC3/DTS via Media3 FFmpeg) repair stale rows
// even when codec strings themselves did not change — but only when the row
// still carries a stale auto codec note, never a curator withhold.
func metadataDiff(item domain.MediaItem, remote *jellyfin.Item) map[string]any {
	values := map[string]any{}
	if remote.Name != "" && remote.Name != item.Title {
		values["title"] = remote.Name
	}
	if remote.SortName != "" && remote.SortName != item.SortTitle {
		values["sort_title"] = remote.SortName
	}
	if remote.Overview != "" && remote.Overview != item.Overview {
		values["overview"] = remote.Overview
	}
	if seconds := remote.RuntimeSeconds(); seconds > 0 && seconds != item.RuntimeSeconds {
		values["runtime_seconds"] = seconds
	}
	if remote.Container != "" && remote.Container != item.Container {
		values["container"] = remote.Container
	}

	videoCodec := item.VideoCodec
	if remote.VideoCodec != "" && remote.VideoCodec != item.VideoCodec {
		values["video_codec"] = remote.VideoCodec
		videoCodec = remote.VideoCodec
	}
	audioCodec := item.AudioCodec
	if remote.AudioCodec != "" && remote.AudioCodec != item.AudioCodec {
		values["audio_codec"] = remote.AudioCodec
		audioCodec = remote.AudioCodec
	}

	// Reconcile direct-play policy against the post-sync codecs.
	// Unsupported codecs always force true→false. Allowed codecs may repair
	// false→true only when the existing nonblank quality note is a recognizably
	// auto-generated codec issue (stale allowlist); blank/manual notes mean a
	// curator withhold and must not be clobbered.
	eval := directplay.Evaluate(videoCodec, audioCodec)
	if eval.OK != item.DirectPlayOK {
		if !eval.OK {
			values["direct_play_ok"] = false
		} else if isStaleAutoCodecBlock(item.QualityNotes) {
			values["direct_play_ok"] = true
		}
	}
	if notes, update := directplay.ReconcileQualityNotes(item.QualityNotes, eval); update {
		values["quality_notes"] = notes
	}

	if remote.ImageTag != "" && remote.ImageTag != item.ImageTag {
		values["image_tag"] = remote.ImageTag
	}
	if item.OrphanedAt != nil {
		values["orphaned_at"] = (*time.Time)(nil)
	}
	return values
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation, which for pairing codes means "try another code" rather than
// "the request is bad".
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// imageProxyPath is the API path serving a media item's artwork.
func imageProxyPath(mediaItemID, imageType string) string {
	return fmt.Sprintf("/images/%s/%s", mediaItemID, imageType)
}

// registerImageProxy wires the artwork proxy. Artwork is proxied so the client
// never needs Jellyfin credentials. It is deliberately left unauthenticated:
// both the admin SPA and paired devices render covers from it, the only thing
// it discloses is cover art already listed in the catalog, and keeping it open
// lets clients use plain <img> tags. Item IDs are UUIDs, so it is not
// enumerable.
func (s *Server) registerImageProxy() {
	huma.Register(s.api, huma.Operation{
		OperationID: "get-media-item-image",
		Method:      http.MethodGet,
		Path:        "/images/{mediaItemId}/{type}",
		Summary:     "Get media item artwork",
		Description: "Proxies artwork from Jellyfin so clients need no Jellyfin access.",
		Tags:        []string{imagesTag},
		Errors:      []int{http.StatusNotFound, http.StatusServiceUnavailable, http.StatusBadGateway},
	}, s.getImage)
}

// imageInput identifies the artwork to serve.
type imageInput struct {
	MediaItemID string `path:"mediaItemId" format:"uuid" doc:"Media item ID."`
	Type        string `path:"type" enum:"Primary,Backdrop,Thumb,Logo" doc:"Jellyfin image type."`
}

// imageOutput carries raw artwork bytes.
type imageOutput struct {
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	Body         []byte
}

func (s *Server) getImage(ctx context.Context, in *imageInput) (*imageOutput, error) {
	if s.jellyfin == nil {
		return nil, huma.Error503ServiceUnavailable("media source is not configured")
	}
	item, err := tvrepo.MediaItems.Get(ctx, s.q, in.MediaItemID)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	data, contentType, err := s.jellyfin.FetchImage(ctx, item.JellyfinItemID, in.Type, item.ImageTag)
	if err != nil {
		if errors.Is(err, jellyfin.ErrNotFound) {
			return nil, huma.Error404NotFound("artwork not found")
		}
		return nil, huma.Error502BadGateway("fetch artwork", err)
	}
	return &imageOutput{
		ContentType:  contentType,
		CacheControl: "public, max-age=86400",
		Body:         data,
	}, nil
}
