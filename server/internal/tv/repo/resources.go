// Package repo holds the TV server's repositories. It reuses the generic
// resource/list machinery from internal/repo; only queries that generic CRUD
// cannot express (catalog resolution, watch-once bookkeeping, grant
// redemption) are written by hand.
package repo

import (
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// Repositories for each TV resource. Search/sort/filter column lists must
// stay in sync with the migrations in internal/tv/db/migrations. Selected
// columns are derived from domain struct db tags (see repo.NewResource).

// Devices is the device repository.
var Devices = repo.NewResource[domain.Device](repo.ListConfig{
	Table:             "devices",
	SearchColumns:     []string{"name"},
	SortableColumns:   []string{"name", "kind", "last_seen_at", "paired_at", "created_at", "updated_at"},
	FilterableColumns: []string{"kind"},
})

// MediaItems is the media item repository. id is sortable so the metadata sync
// can page over the whole library on a key it does not itself rewrite.
var MediaItems = repo.NewResource[domain.MediaItem](repo.ListConfig{
	Table:             "media_items",
	SearchColumns:     []string{"title", "sort_title", "overview"},
	SortableColumns:   []string{"id", "title", "sort_title", "class", "runtime_seconds", "created_at", "updated_at"},
	FilterableColumns: []string{"class", "direct_play_ok", "jellyfin_item_id"},
})

// AvailabilityWindows is the availability window repository.
var AvailabilityWindows = repo.NewResource[domain.AvailabilityWindow](repo.ListConfig{
	Table:             "availability_windows",
	SearchColumns:     []string{"note"},
	SortableColumns:   []string{"starts_at", "ends_at", "created_at", "updated_at"},
	FilterableColumns: []string{"media_item_id"},
})

// PlayLedger is the watch-once ledger repository.
var PlayLedger = repo.NewResource[domain.PlayLedgerEntry](repo.ListConfig{
	Table:             "play_ledger",
	SortableColumns:   []string{"consumed_at", "created_at", "updated_at"},
	FilterableColumns: []string{"media_item_id", "device_id", "availability_window_id"},
})

// ScheduleEntries is the programmed schedule repository.
var ScheduleEntries = repo.NewResource[domain.ScheduleEntry](repo.ListConfig{
	Table:             "schedule_entries",
	SortableColumns:   []string{"airs_at", "block", "created_at", "updated_at"},
	FilterableColumns: []string{"media_item_id", "block", "join_in_progress"},
})

// PlayGrants is the play grant repository.
var PlayGrants = repo.NewResource[domain.PlayGrant](repo.ListConfig{
	Table:             "play_grants",
	SortableColumns:   []string{"issued_at", "expires_at", "consumed_at", "created_at", "updated_at"},
	FilterableColumns: []string{"media_item_id", "device_id", "mode"},
})

// PlaybackSessions is the playback session repository.
var PlaybackSessions = repo.NewResource[domain.PlaybackSession](repo.ListConfig{
	Table:             "playback_sessions",
	SortableColumns:   []string{"started_at", "ended_at", "watched_seconds", "completed", "created_at", "updated_at"},
	FilterableColumns: []string{"media_item_id", "device_id", "completed", "grant_id"},
})

// PrimerReports is the Primer export ledger repository.
var PrimerReports = repo.NewResource[domain.PrimerReport](repo.ListConfig{
	Table:             "primer_reports",
	SortableColumns:   []string{"reported_at", "created_at", "updated_at"},
	FilterableColumns: []string{"playback_session_id"},
})

// ContentManifestEntries is the desired-state content catalog repository.
var ContentManifestEntries = repo.NewResource[domain.ContentManifestEntry](repo.ListConfig{
	Table:             "content_manifest_entries",
	SearchColumns:     []string{"slug", "title", "notes", "last_error"},
	SortableColumns:   []string{"slug", "title", "kind", "status", "priority", "attempt_count", "last_attempt_at", "created_at", "updated_at"},
	FilterableColumns: []string{"slug", "kind", "status", "class"},
})
