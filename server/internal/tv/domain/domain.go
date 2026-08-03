// Package domain holds the core entity types persisted by the TV server.
//
// JSON tags are camelCase so entities surface cleanly through the
// Huma-generated OpenAPI spec and the clients generated from it. The db tags
// map columns for pgx struct scanning and drive the repo layer's column list.
package domain

import "time"

// Media item classes. Entertainment items are the ones subject to watch-once
// enforcement; educational and mixed items are replayable and are the ones
// reported to Primer as instructional time.
const (
	ClassEducational   = "educational"
	ClassEntertainment = "entertainment"
	ClassMixed         = "mixed"
)

// Device kinds.
const (
	DeviceTablet = "tablet"
	DeviceTVBox  = "tv_box"
)

// Play grant modes.
const (
	ModeOnDemand   = "on_demand"
	ModeProgrammed = "programmed"
)

// Schedule day-part blocks. They label a slot for the admin grid; the actual
// airing window is airs_at plus the item's runtime.
const (
	BlockMorning   = "morning"
	BlockMidday    = "midday"
	BlockAfternoon = "afternoon"
	BlockEvening   = "evening"
)

// Device is a paired client. The pairing token is never stored: only its
// hash, so a database disclosure cannot be replayed against the device API.
type Device struct {
	ID               string     `json:"id" db:"id" format:"uuid"`
	Name             string     `json:"name" db:"name"`
	Kind             string     `json:"kind" db:"kind" enum:"tablet,tv_box"`
	PairingCode      string     `json:"pairingCode" db:"pairing_code"`
	PairingExpiresAt *time.Time `json:"pairingExpiresAt,omitempty" db:"pairing_expires_at"`
	TokenHash        string     `json:"-" db:"token_hash"`
	PairedAt         *time.Time `json:"pairedAt,omitempty" db:"paired_at"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty" db:"revoked_at"`
	LastSeenAt       *time.Time `json:"lastSeenAt,omitempty" db:"last_seen_at"`
	CreatedAt        time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time  `json:"updatedAt" db:"updated_at"`
}

// Paired reports whether the device has completed pairing and is not revoked.
func (d Device) Paired() bool {
	return d.TokenHash != "" && d.RevokedAt == nil
}

// MediaItem is a curated entry from the Jellyfin library.
type MediaItem struct {
	ID             string     `json:"id" db:"id" format:"uuid"`
	JellyfinItemID string     `json:"jellyfinItemId" db:"jellyfin_item_id"`
	Title          string     `json:"title" db:"title"`
	SortTitle      string     `json:"sortTitle" db:"sort_title"`
	Overview       string     `json:"overview" db:"overview"`
	Class          string     `json:"class" db:"class" enum:"educational,entertainment,mixed"`
	RuntimeSeconds int        `json:"runtimeSeconds" db:"runtime_seconds"`
	SubjectTags    []string   `json:"subjectTags" db:"subject_tags"`
	StandardCodes  []string   `json:"standardCodes" db:"standard_codes"`
	QualityNotes   string     `json:"qualityNotes" db:"quality_notes"`
	Container      string     `json:"container" db:"container"`
	VideoCodec     string     `json:"videoCodec" db:"video_codec"`
	AudioCodec     string     `json:"audioCodec" db:"audio_codec"`
	DirectPlayOK   bool       `json:"directPlayOk" db:"direct_play_ok"`
	ImageTag       string     `json:"imageTag" db:"image_tag"`
	OrphanedAt     *time.Time `json:"orphanedAt,omitempty" db:"orphaned_at"`
	CreatedAt      time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time  `json:"updatedAt" db:"updated_at"`
}

// ConsumesPlay reports whether watching this item to completion should burn
// its availability window. Only entertainment is rationed.
func (m MediaItem) ConsumesPlay() bool { return m.Class == ClassEntertainment }

// AvailabilityWindow is a span during which a media item may be played
// on demand.
type AvailabilityWindow struct {
	ID          string    `json:"id" db:"id" format:"uuid"`
	MediaItemID string    `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	StartsAt    time.Time `json:"startsAt" db:"starts_at"`
	EndsAt      time.Time `json:"endsAt" db:"ends_at"`
	MaxPlays    *int      `json:"maxPlays,omitempty" db:"max_plays"`
	Note        string    `json:"note" db:"note"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

// ActiveAt reports whether the window is open at the given instant. The start
// is inclusive and the end exclusive.
func (w AvailabilityWindow) ActiveAt(at time.Time) bool {
	return !at.Before(w.StartsAt) && at.Before(w.EndsAt)
}

// PlayLedgerEntry records that a media item's play was consumed for a window.
type PlayLedgerEntry struct {
	ID                   string    `json:"id" db:"id" format:"uuid"`
	MediaItemID          string    `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	DeviceID             *string   `json:"deviceId,omitempty" db:"device_id" format:"uuid"`
	AvailabilityWindowID string    `json:"availabilityWindowId" db:"availability_window_id" format:"uuid"`
	ConsumedAt           time.Time `json:"consumedAt" db:"consumed_at"`
	CreatedAt            time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time `json:"updatedAt" db:"updated_at"`
}

// ScheduleEntry places a media item in the programmed channel grid.
type ScheduleEntry struct {
	ID             string    `json:"id" db:"id" format:"uuid"`
	MediaItemID    string    `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	AirsAt         time.Time `json:"airsAt" db:"airs_at"`
	JoinInProgress bool      `json:"joinInProgress" db:"join_in_progress"`
	Block          string    `json:"block" db:"block" enum:"morning,midday,afternoon,evening"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}

// PlayGrant is a single-use, short-lived authorization to stream an item.
type PlayGrant struct {
	ID                   string     `json:"id" db:"id" format:"uuid"`
	MediaItemID          string     `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	DeviceID             string     `json:"deviceId" db:"device_id" format:"uuid"`
	AvailabilityWindowID *string    `json:"availabilityWindowId,omitempty" db:"availability_window_id" format:"uuid"`
	ScheduleEntryID      *string    `json:"scheduleEntryId,omitempty" db:"schedule_entry_id" format:"uuid"`
	Mode                 string     `json:"mode" db:"mode" enum:"on_demand,programmed"`
	StreamURL            string     `json:"streamUrl" db:"stream_url"`
	StartOffsetSeconds   int        `json:"startOffsetSeconds" db:"start_offset_seconds"`
	IssuedAt             time.Time  `json:"issuedAt" db:"issued_at"`
	ExpiresAt            time.Time  `json:"expiresAt" db:"expires_at"`
	ConsumedAt           *time.Time `json:"consumedAt,omitempty" db:"consumed_at"`
	CreatedAt            time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time  `json:"updatedAt" db:"updated_at"`
}

// Redeemable reports whether the grant can still start playback at the given
// instant: unconsumed and unexpired.
func (g PlayGrant) Redeemable(at time.Time) bool {
	return g.ConsumedAt == nil && at.Before(g.ExpiresAt)
}

// PlaybackSession accumulates watch metrics for one redeemed grant.
type PlaybackSession struct {
	ID                 string     `json:"id" db:"id" format:"uuid"`
	GrantID            string     `json:"grantId" db:"grant_id" format:"uuid"`
	MediaItemID        string     `json:"mediaItemId" db:"media_item_id" format:"uuid"`
	DeviceID           string     `json:"deviceId" db:"device_id" format:"uuid"`
	StartedAt          time.Time  `json:"startedAt" db:"started_at"`
	EndedAt            *time.Time `json:"endedAt,omitempty" db:"ended_at"`
	WatchedSeconds     int        `json:"watchedSeconds" db:"watched_seconds"`
	MaxPositionSeconds int        `json:"maxPositionSeconds" db:"max_position_seconds"`
	Completed          bool       `json:"completed" db:"completed"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt" db:"updated_at"`
}

// PrimerReport is the idempotency record for one exported session.
type PrimerReport struct {
	ID                string    `json:"id" db:"id" format:"uuid"`
	PlaybackSessionID string    `json:"playbackSessionId" db:"playback_session_id" format:"uuid"`
	ReportedAt        time.Time `json:"reportedAt" db:"reported_at"`
	PrimerRef         string    `json:"primerRef" db:"primer_ref"`
	CreatedAt         time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt         time.Time `json:"updatedAt" db:"updated_at"`
}

// CompletionThreshold is the watched fraction at which a session counts as
// finished for watch-once purposes, so a student who stops just before the
// credits still burns an entertainment play.
const CompletionThreshold = 0.8

// ResumeRewindSeconds is how far before the furthest on-demand position a
// fresh grant starts playback, so the student hears a beat of context.
const ResumeRewindSeconds = 30

// SeekRewindLimitSeconds is how far behind the furthest on-demand position the
// client may allow seeking. Forward seeks past the watermark are refused.
const SeekRewindLimitSeconds = 5 * 60

// ResumePositionSeconds is where on-demand playback should start given the
// furthest position reached for an item. Zero furthest means a fresh start.
func ResumePositionSeconds(furthestPositionSeconds int) int {
	if furthestPositionSeconds <= 0 {
		return 0
	}
	resume := furthestPositionSeconds - ResumeRewindSeconds
	if resume < 0 {
		return 0
	}
	return resume
}

// SeekFloorSeconds is the earliest on-demand position the student may scrub to
// given the furthest watermark. The ceiling is the watermark itself.
func SeekFloorSeconds(furthestPositionSeconds int) int {
	if furthestPositionSeconds <= 0 {
		return 0
	}
	floor := furthestPositionSeconds - SeekRewindLimitSeconds
	if floor < 0 {
		return 0
	}
	return floor
}

// SessionCompletesPlay reports whether a session has progressed far enough to
// consume the item's play. Runtime of zero means the item's duration is
// unknown, in which case only an explicit completion counts.
func SessionCompletesPlay(completed bool, maxPositionSeconds, runtimeSeconds int) bool {
	if completed {
		return true
	}
	if runtimeSeconds <= 0 {
		return false
	}
	return float64(maxPositionSeconds) >= CompletionThreshold*float64(runtimeSeconds)
}
