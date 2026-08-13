package api

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// Create/update request bodies. The db tags map fields to columns; update
// bodies use pointer fields so that omitted fields are left unchanged.

// DeviceCreate registers a device and issues its pairing code.
type DeviceCreate struct {
	Name string  `json:"name" db:"name" minLength:"1"`
	Kind *string `json:"kind,omitempty" db:"kind" enum:"tablet,tv_box" required:"false"`
}

// DeviceUpdate renames or revokes a device.
type DeviceUpdate struct {
	Name      *string    `json:"name,omitempty" db:"name" required:"false"`
	Kind      *string    `json:"kind,omitempty" db:"kind" enum:"tablet,tv_box" required:"false"`
	RevokedAt *time.Time `json:"revokedAt,omitempty" db:"revoked_at" required:"false"`
}

// MediaItemCreate imports a Jellyfin item into the curated library.
// Lock flags are server-owned defaults and are not accepted on create.
type MediaItemCreate struct {
	JellyfinItemID string    `json:"jellyfinItemId" db:"jellyfin_item_id" minLength:"1"`
	Title          string    `json:"title" db:"title" minLength:"1"`
	SortTitle      *string   `json:"sortTitle,omitempty" db:"sort_title" required:"false"`
	Overview       *string   `json:"overview,omitempty" db:"overview" required:"false"`
	Class          string    `json:"class" db:"class" enum:"educational,entertainment,mixed"`
	RuntimeSeconds *int      `json:"runtimeSeconds,omitempty" db:"runtime_seconds" minimum:"0" required:"false"`
	SubjectTags    *[]string `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes  *[]string `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	QualityNotes   *string   `json:"qualityNotes,omitempty" db:"quality_notes" required:"false"`
	Container      *string   `json:"container,omitempty" db:"container" required:"false"`
	VideoCodec     *string   `json:"videoCodec,omitempty" db:"video_codec" required:"false"`
	AudioCodec     *string   `json:"audioCodec,omitempty" db:"audio_codec" required:"false"`
	DirectPlayOK   *bool     `json:"directPlayOk,omitempty" db:"direct_play_ok" required:"false"`
	ImageTag       *string   `json:"imageTag,omitempty" db:"image_tag" required:"false"`
	YouTubeVideoID *string   `json:"youtubeVideoId,omitempty" db:"youtube_video_id" required:"false" doc:"11-char YouTube video id when known."`
	ManifestSlug   *string   `json:"manifestSlug,omitempty" db:"manifest_slug" required:"false"`
	EpisodeKey     *string   `json:"episodeKey,omitempty" db:"episode_key" required:"false" doc:"Ledger episode key, e.g. S01E007."`
	// UploadDate is a calendar day YYYY-MM-DD (DATE column); accepted as string
	// so clients need not send a full RFC3339 timestamp.
	UploadDate *string `json:"uploadDate,omitempty" required:"false" doc:"Upload calendar day as YYYY-MM-DD."`
}

// MediaItemUpdate reclassifies or retags a media item. jellyfinItemId is not
// writable (immutable join key). Lock flags are read-only echoes set by the
// server when a curator patches title / overview / classification fields.
type MediaItemUpdate struct {
	Title          *string    `json:"title,omitempty" db:"title" required:"false"`
	SortTitle      *string    `json:"sortTitle,omitempty" db:"sort_title" required:"false"`
	Overview       *string    `json:"overview,omitempty" db:"overview" required:"false"`
	Class          *string    `json:"class,omitempty" db:"class" enum:"educational,entertainment,mixed" required:"false"`
	RuntimeSeconds *int       `json:"runtimeSeconds,omitempty" db:"runtime_seconds" minimum:"0" required:"false"`
	SubjectTags    *[]string  `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes  *[]string  `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	QualityNotes   *string    `json:"qualityNotes,omitempty" db:"quality_notes" required:"false"`
	Container      *string    `json:"container,omitempty" db:"container" required:"false"`
	VideoCodec     *string    `json:"videoCodec,omitempty" db:"video_codec" required:"false"`
	AudioCodec     *string    `json:"audioCodec,omitempty" db:"audio_codec" required:"false"`
	DirectPlayOK   *bool      `json:"directPlayOk,omitempty" db:"direct_play_ok" required:"false"`
	ImageTag       *string    `json:"imageTag,omitempty" db:"image_tag" required:"false"`
	OrphanedAt     *time.Time `json:"orphanedAt,omitempty" db:"orphaned_at" required:"false"`
	// YouTube provenance may be set by ingest after create (e.g. recovered id).
	YouTubeVideoID *string `json:"youtubeVideoId,omitempty" db:"youtube_video_id" required:"false"`
	ManifestSlug   *string `json:"manifestSlug,omitempty" db:"manifest_slug" required:"false"`
	EpisodeKey     *string `json:"episodeKey,omitempty" db:"episode_key" required:"false"`
	UploadDate     *string `json:"uploadDate,omitempty" required:"false" doc:"Upload calendar day as YYYY-MM-DD."`
}

// AvailabilityWindowCreate opens an on-demand window for an item. maxPlays is
// deliberately not writable: the ledger's unique (item, window) key caps a
// window at one play, so accepting a larger number would silently do nothing.
// Multiple plays are expressed by opening multiple windows.
type AvailabilityWindowCreate struct {
	MediaItemID string     `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	StartsAt    *time.Time `json:"startsAt,omitempty" db:"starts_at" required:"false"`
	EndsAt      time.Time  `json:"endsAt" db:"ends_at"`
	Note        *string    `json:"note,omitempty" db:"note" required:"false"`
}

// AvailabilityWindowUpdate adjusts or expires a window.
type AvailabilityWindowUpdate struct {
	StartsAt *time.Time `json:"startsAt,omitempty" db:"starts_at" required:"false"`
	EndsAt   *time.Time `json:"endsAt,omitempty" db:"ends_at" required:"false"`
	Note     *string    `json:"note,omitempty" db:"note" required:"false"`
}

// ScheduleEntryCreate places an item in the programmed grid. Unlike the other
// create bodies this one is not fed to the generic insert: an airing has to be
// checked against the rest of the grid, so the handler unpacks it by hand and
// there are no db tags.
type ScheduleEntryCreate struct {
	MediaItemID    string    `json:"mediaItemId" format:"uuid"`
	AirsAt         time.Time `json:"airsAt"`
	JoinInProgress *bool     `json:"joinInProgress,omitempty" required:"false" doc:"Whether a device tuning in late joins at the broadcast offset. Defaults to true."`
	Block          *string   `json:"block,omitempty" enum:"morning,midday,afternoon,evening" required:"false" doc:"Day-part label. Defaults to the one matching airsAt in the channel timezone."`
}

// ScheduleEntryUpdate moves or retimes a scheduled airing.
type ScheduleEntryUpdate struct {
	MediaItemID    *string    `json:"mediaItemId,omitempty" format:"uuid" required:"false"`
	AirsAt         *time.Time `json:"airsAt,omitempty" required:"false"`
	JoinInProgress *bool      `json:"joinInProgress,omitempty" required:"false"`
	Block          *string    `json:"block,omitempty" enum:"morning,midday,afternoon,evening" required:"false"`
}

// mediaItemsTag groups media-item admin operations in the OpenAPI spec.
const mediaItemsTag = "Media Items"

// registerAdminCRUD wires the admin CRUD endpoints. Devices get a bespoke
// create handler (it must mint a pairing code), so only the read/update/delete
// half of the generic stack is reused for them. Media items use custom
// create/update so curator PATCH can flip metadata locks and jellyfinItemId
// stays immutable.
func (s *Server) registerAdminCRUD() {
	guard := s.adminGuard()

	baseapi.RegisterCRUD[domain.MediaItem, struct{}, struct{}](
		s.api, s.q, tvrepo.MediaItems, "media-item", "media-items", "/media-items",
		guard, baseapi.SkipCreate(), baseapi.SkipUpdate())
	s.registerMediaItemMutations()

	baseapi.RegisterCRUD[domain.AvailabilityWindow, AvailabilityWindowCreate, AvailabilityWindowUpdate](
		s.api, s.q, tvrepo.AvailabilityWindows, "availability-window", "availability-windows", "/availability-windows", guard)
	s.registerScheduleAdmin()
	baseapi.RegisterCRUD[domain.PlaybackSession, struct{}, struct{}](
		s.api, s.q, tvrepo.PlaybackSessions, "playback-session", "playback-sessions", "/playback-sessions",
		guard, baseapi.SkipCreate(), baseapi.SkipUpdate(), baseapi.SkipDelete())
}

// registerMediaItemMutations wires create/update with lock side effects.
func (s *Server) registerMediaItemMutations() {
	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID:   "create-media-item",
		Method:        http.MethodPost,
		Path:          "/media-items",
		Summary:       "Create a media item",
		Description:   "Imports a Jellyfin item into the curated library. Optional YouTube provenance fields may be set; lock flags default to false and are not client-settable.",
		Tags:          []string{mediaItemsTag},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusConflict, http.StatusUnprocessableEntity},
	}), s.createMediaItem)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "update-media-item",
		Method:      http.MethodPatch,
		Path:        "/media-items/{id}",
		Summary:     "Update a media item",
		Description: "Partial update: only provided fields are changed. Patching title, overview, or classification fields sets the corresponding lock so Jellyfin sync will not clobber curator edits. jellyfinItemId is immutable.",
		Tags:        []string{mediaItemsTag},
		Errors:      []int{http.StatusNotFound, http.StatusConflict, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity},
	}), s.updateMediaItem)
}

type createMediaItemInput struct {
	Body MediaItemCreate
}

type updateMediaItemInput struct {
	ID   string `path:"id" format:"uuid" doc:"Media item ID."`
	Body MediaItemUpdate
}

type mediaItemOutput struct {
	Body domain.MediaItem
}

func (s *Server) createMediaItem(ctx context.Context, in *createMediaItemInput) (*mediaItemOutput, error) {
	values, err := mediaItemCreateValues(in.Body)
	if err != nil {
		return nil, err
	}
	item, err := tvrepo.MediaItems.Create(ctx, s.q, values)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return &mediaItemOutput{Body: *item}, nil
}

func (s *Server) updateMediaItem(ctx context.Context, in *updateMediaItemInput) (*mediaItemOutput, error) {
	values, err := mediaItemUpdateValues(in.Body)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		item, err := tvrepo.MediaItems.Get(ctx, s.q, in.ID)
		if err != nil {
			return nil, baseapi.MapError(err)
		}
		return &mediaItemOutput{Body: *item}, nil
	}
	item, err := tvrepo.MediaItems.Update(ctx, s.q, in.ID, values)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return &mediaItemOutput{Body: *item}, nil
}

// mediaItemCreateValues maps a create body to DB columns. Empty optional
// YouTube ids are omitted so the UNIQUE NULL column stays multi-null friendly.
func mediaItemCreateValues(body MediaItemCreate) (map[string]any, error) {
	values := structToDBValues(body)
	if err := applyUploadDate(values, body.UploadDate); err != nil {
		return nil, err
	}
	normalizeOptionalYouTubeID(values)
	return values, nil
}

// mediaItemUpdateValues maps a patch body and sets lock flags when the curator
// edits title, overview, or classification fields.
func mediaItemUpdateValues(body MediaItemUpdate) (map[string]any, error) {
	values := structToDBValues(body)
	if err := applyUploadDate(values, body.UploadDate); err != nil {
		return nil, err
	}
	normalizeOptionalYouTubeID(values)
	if body.Title != nil {
		values["title_locked"] = true
	}
	if body.Overview != nil {
		values["overview_locked"] = true
	}
	if body.Class != nil || body.SubjectTags != nil || body.StandardCodes != nil {
		values["classification_locked"] = true
	}
	return values, nil
}

func applyUploadDate(values map[string]any, raw *string) error {
	if raw == nil {
		return nil
	}
	day := *raw
	if day == "" {
		values["upload_date"] = nil
		return nil
	}
	parsed, err := time.Parse(DayFormat, day)
	if err != nil {
		return huma.Error422UnprocessableEntity("uploadDate must be a calendar day in YYYY-MM-DD form")
	}
	values["upload_date"] = parsed
	return nil
}

func normalizeOptionalYouTubeID(values map[string]any) {
	raw, ok := values["youtube_video_id"]
	if !ok {
		return
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		delete(values, "youtube_video_id")
	}
}

// structToDBValues converts a request body struct into a column->value map
// using db tags. Nil pointer fields are omitted (unset).
func structToDBValues(v any) map[string]any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	t := rv.Type()
	out := make(map[string]any, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		col := t.Field(i).Tag.Get("db")
		if col == "" || col == "-" {
			continue
		}
		fv := rv.Field(i)
		if fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		out[col] = fv.Interface()
	}
	return out
}
