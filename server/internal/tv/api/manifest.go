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

const contentManifestTag = "Content Manifest"

// ContentManifestEntryCreate is the body for a single desired-state upsert
// (used by bulk sync and optional single create).
type ContentManifestEntryCreate struct {
	Slug            string   `json:"slug" db:"slug" minLength:"1"`
	Title           string   `json:"title" db:"title" minLength:"1"`
	Year            *int     `json:"year,omitempty" db:"year" minimum:"0" required:"false"`
	Kind            string   `json:"kind" db:"kind" enum:"movie,series,youtube_channel,youtube_playlist,manual"`
	TMDBID          *int     `json:"tmdbId,omitempty" db:"tmdb_id" minimum:"0" required:"false"`
	TVDBID          *int     `json:"tvdbId,omitempty" db:"tvdb_id" minimum:"0" required:"false"`
	URL             *string  `json:"url,omitempty" db:"url" required:"false"`
	Class           string   `json:"class" db:"class" enum:"educational,entertainment,mixed"`
	SubjectTags     *[]string `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes   *[]string `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	Priority        *int     `json:"priority,omitempty" db:"priority" minimum:"0" required:"false"`
	ExcludeEpisodes *[]string `json:"excludeEpisodes,omitempty" db:"exclude_episodes" required:"false"`
	MaxEpisodes     *int     `json:"maxEpisodes,omitempty" db:"max_episodes" minimum:"0" required:"false"`
	Notes           *string  `json:"notes,omitempty" db:"notes" required:"false"`
}

// ContentManifestEntryUpdate patches desired-state fields or operator notes.
// Status transitions for present/attempt/fail go through dedicated endpoints
// so a generic PATCH cannot accidentally clear attempt counters.
type ContentManifestEntryUpdate struct {
	Title           *string   `json:"title,omitempty" db:"title" required:"false"`
	Year            *int      `json:"year,omitempty" db:"year" minimum:"0" required:"false"`
	Kind            *string   `json:"kind,omitempty" db:"kind" enum:"movie,series,youtube_channel,youtube_playlist,manual" required:"false"`
	TMDBID          *int      `json:"tmdbId,omitempty" db:"tmdb_id" minimum:"0" required:"false"`
	TVDBID          *int      `json:"tvdbId,omitempty" db:"tvdb_id" minimum:"0" required:"false"`
	URL             *string   `json:"url,omitempty" db:"url" required:"false"`
	Class           *string   `json:"class,omitempty" db:"class" enum:"educational,entertainment,mixed" required:"false"`
	SubjectTags     *[]string `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes   *[]string `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	Priority        *int      `json:"priority,omitempty" db:"priority" minimum:"0" required:"false"`
	ExcludeEpisodes *[]string `json:"excludeEpisodes,omitempty" db:"exclude_episodes" required:"false"`
	MaxEpisodes     *int      `json:"maxEpisodes,omitempty" db:"max_episodes" minimum:"0" required:"false"`
	Notes           *string   `json:"notes,omitempty" db:"notes" required:"false"`
	// Status is writable so an operator can re-open a failed entry (set back
	// to missing) or force-mark present after a manual rip.
	Status    *string `json:"status,omitempty" db:"status" enum:"missing,present,failed,manual" required:"false"`
	LastError *string `json:"lastError,omitempty" db:"last_error" required:"false"`
}

// ManifestSyncBody is the bulk desired-state payload from content-ingest.
type ManifestSyncBody struct {
	Items []ContentManifestEntryCreate `json:"items" minItems:"0"`
}

// ManifestSyncResponse summarizes a bulk upsert.
type ManifestSyncResponse struct {
	Created int `json:"created" doc:"Rows inserted."`
	Updated int `json:"updated" doc:"Rows whose desired-state fields changed."`
	Total   int `json:"total" doc:"Items in the request."`
}

// ManifestAttemptBody records one acquisition attempt against a missing entry.
type ManifestAttemptBody struct {
	// Error is an optional short reason when the attempt failed (indexer miss,
	// yt-dlp error, etc.). Empty means "attempted, still waiting."
	Error string `json:"error,omitempty"`
}

// ManifestPresentBody marks an entry present (available in Jellyfin).
type ManifestPresentBody struct {
	// PresentAt overrides the timestamp; defaults to now.
	PresentAt *time.Time `json:"presentAt,omitempty"`
}

// registerContentManifest wires the catalog + acquisition-tracking endpoints.
func (s *Server) registerContentManifest() {
	guard := s.adminGuard()

	baseapi.RegisterCRUD[domain.ContentManifestEntry, ContentManifestEntryCreate, ContentManifestEntryUpdate](
		s.api, s.q, tvrepo.ContentManifestEntries,
		"content-manifest-entry", "content-manifest-entries", "/content-manifest-entries",
		guard)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "sync-content-manifest",
		Method:      http.MethodPost,
		Path:        "/content-manifest/sync",
		Summary:     "Upsert the desired-state content catalog",
		Description: "Mirrors curriculum/content-manifest.yaml into the TV database. Acquisition status and attempt counters on existing rows are preserved.",
		Tags:        []string{contentManifestTag},
	}), s.syncContentManifest)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "record-content-manifest-attempt",
		Method:      http.MethodPost,
		Path:        "/content-manifest-entries/{slug}/attempt",
		Summary:     "Record an acquisition attempt",
		Description: "Increments attempt_count for a missing entry. Marks the entry failed when TV_MANIFEST_FAIL_MAX_ATTEMPTS or TV_MANIFEST_FAIL_MAX_DAYS is exceeded.",
		Tags:        []string{contentManifestTag},
	}), s.recordManifestAttempt)

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "mark-content-manifest-present",
		Method:      http.MethodPost,
		Path:        "/content-manifest-entries/{slug}/present",
		Summary:     "Mark a catalog entry present",
		Description: "Records that the title is available in Jellyfin (and usually imported as media_items).",
		Tags:        []string{contentManifestTag},
	}), s.markManifestPresent)
}

type manifestSyncInput struct {
	Body ManifestSyncBody
}

type manifestSyncOutput struct {
	Body ManifestSyncResponse
}

func (s *Server) syncContentManifest(ctx context.Context, in *manifestSyncInput) (*manifestSyncOutput, error) {
	created, updated := 0, 0
	for _, item := range in.Body.Items {
		values := manifestDesiredValues(item)
		_, err := tvrepo.ContentManifestBySlug(ctx, s.q, item.Slug)
		exists := err == nil
		if _, err := tvrepo.UpsertManifestDesired(ctx, s.q, values); err != nil {
			return nil, baseapi.MapError(err)
		}
		if exists {
			updated++
		} else {
			created++
		}
	}
	return &manifestSyncOutput{Body: ManifestSyncResponse{
		Created: created,
		Updated: updated,
		Total:   len(in.Body.Items),
	}}, nil
}

type manifestAttemptInput struct {
	Slug string `path:"slug" doc:"Stable curriculum slug (manifest item id)."`
	Body ManifestAttemptBody
}

type manifestPresentInput struct {
	Slug string `path:"slug" doc:"Stable curriculum slug (manifest item id)."`
	Body ManifestPresentBody
}

type manifestEntryOutput struct {
	Body domain.ContentManifestEntry
}

func (s *Server) recordManifestAttempt(ctx context.Context, in *manifestAttemptInput) (*manifestEntryOutput, error) {
	entry, err := tvrepo.RecordManifestAttempt(ctx, s.q, in.Slug, in.Body.Error, tvrepo.ManifestFailPolicy{
		MaxAttempts: s.manifestFailMaxAttempts,
		MaxDays:     s.manifestFailMaxDays,
		Now:         s.now(),
	})
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return &manifestEntryOutput{Body: *entry}, nil
}

func (s *Server) markManifestPresent(ctx context.Context, in *manifestPresentInput) (*manifestEntryOutput, error) {
	at := s.now()
	if in.Body.PresentAt != nil {
		at = *in.Body.PresentAt
	}
	entry, err := tvrepo.MarkManifestPresent(ctx, s.q, in.Slug, at)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return &manifestEntryOutput{Body: *entry}, nil
}

func manifestDesiredValues(item ContentManifestEntryCreate) map[string]any {
	values := map[string]any{
		"slug":  item.Slug,
		"title": item.Title,
		"kind":  item.Kind,
		"class": item.Class,
	}
	if item.Year != nil {
		values["year"] = *item.Year
	}
	if item.TMDBID != nil {
		values["tmdb_id"] = *item.TMDBID
	}
	if item.TVDBID != nil {
		values["tvdb_id"] = *item.TVDBID
	}
	if item.URL != nil {
		values["url"] = *item.URL
	}
	if item.SubjectTags != nil {
		values["subject_tags"] = *item.SubjectTags
	}
	if item.StandardCodes != nil {
		values["standard_codes"] = *item.StandardCodes
	}
	if item.Priority != nil {
		values["priority"] = *item.Priority
	}
	if item.ExcludeEpisodes != nil {
		values["exclude_episodes"] = *item.ExcludeEpisodes
	}
	if item.MaxEpisodes != nil {
		values["max_episodes"] = *item.MaxEpisodes
	}
	if item.Notes != nil {
		values["notes"] = *item.Notes
	}
	return values
}
