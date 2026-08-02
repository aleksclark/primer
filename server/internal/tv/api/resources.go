package api

import (
	"time"

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
}

// MediaItemUpdate reclassifies or retags a media item.
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

// registerAdminCRUD wires the admin CRUD endpoints. Devices get a bespoke
// create handler (it must mint a pairing code), so only the read/update/delete
// half of the generic stack is reused for them.
func (s *Server) registerAdminCRUD() {
	guard := s.adminGuard()

	baseapi.RegisterCRUD[domain.MediaItem, MediaItemCreate, MediaItemUpdate](
		s.api, s.q, tvrepo.MediaItems, "media-item", "media-items", "/media-items", guard)
	baseapi.RegisterCRUD[domain.AvailabilityWindow, AvailabilityWindowCreate, AvailabilityWindowUpdate](
		s.api, s.q, tvrepo.AvailabilityWindows, "availability-window", "availability-windows", "/availability-windows", guard)
	s.registerScheduleAdmin()
	baseapi.RegisterCRUD[domain.PlaybackSession, struct{}, struct{}](
		s.api, s.q, tvrepo.PlaybackSessions, "playback-session", "playback-sessions", "/playback-sessions",
		guard, baseapi.SkipCreate(), baseapi.SkipUpdate(), baseapi.SkipDelete())
}
