package ytdlp_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestLedger_FirstIngestSortsByUploadDateThenID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "essential-craftsman"
	// Unsorted input: later date first, then earlier, with id tie-break needed.
	candidates := []ytdlp.EpisodeCandidate{
		{ID: "bbbbbbbbbbb", Title: "Second", UploadDate: "20160615"},
		{ID: "aaaaaaaaaaa", Title: "First", UploadDate: "20160501"},
		{ID: "ccccccccccc", Title: "Third same day", UploadDate: "20160615"}, // id after b
	}
	now := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	led, err := ytdlp.AssignEpisodes(dir, slug, "Essential Craftsman", "UCchannel", candidates, now)
	require.NoError(t, err)
	require.Equal(t, 4, led.NextEpisode)
	require.Equal(t, 1, led.Episodes["aaaaaaaaaaa"].Episode)
	require.Equal(t, 2, led.Episodes["bbbbbbbbbbb"].Episode)
	require.Equal(t, 3, led.Episodes["ccccccccccc"].Episode)
	assert.Equal(t, 1, led.Version)
	assert.Equal(t, slug, led.Slug)
	assert.Equal(t, "Essential Craftsman", led.Title)
	assert.Equal(t, "UCchannel", led.ChannelID)
	assert.Equal(t, "first_seen_sequential", led.Numbering)
	assert.Equal(t, 1, led.Season)

	// Persist round-trip
	path := filepath.Join(dir, "Shows", slug, ".primer-index.json")
	loaded, err := ytdlp.LoadLedger(path)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.Episodes["aaaaaaaaaaa"].Episode)
	assert.Equal(t, 3, loaded.Episodes["ccccccccccc"].Episode)
}

func TestLedger_ReorderDoesNotChangeExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "mark-rober"
	now := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	first := []ytdlp.EpisodeCandidate{
		{ID: "aaaaaaaaaaa", Title: "A", UploadDate: "20180101"},
		{ID: "bbbbbbbbbbb", Title: "B", UploadDate: "20180201"},
	}
	led, err := ytdlp.AssignEpisodes(dir, slug, "Mark Rober", "", first, now)
	require.NoError(t, err)
	assert.Equal(t, 1, led.Episodes["aaaaaaaaaaa"].Episode)
	assert.Equal(t, 2, led.Episodes["bbbbbbbbbbb"].Episode)

	// Reorder input + insert new id: existing keep numbers; new gets next.
	second := []ytdlp.EpisodeCandidate{
		{ID: "bbbbbbbbbbb", Title: "B-renamed", UploadDate: "20180201"},
		{ID: "ccccccccccc", Title: "C-new", UploadDate: "20170101"}, // older but later-seen
		{ID: "aaaaaaaaaaa", Title: "A", UploadDate: "20180101"},
	}
	led2, err := ytdlp.AssignEpisodes(dir, slug, "Mark Rober", "", second, now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1, led2.Episodes["aaaaaaaaaaa"].Episode)
	assert.Equal(t, 2, led2.Episodes["bbbbbbbbbbb"].Episode)
	assert.Equal(t, 3, led2.Episodes["ccccccccccc"].Episode)
	assert.Equal(t, 4, led2.NextEpisode)
	// Title of existing is not rewritten by reorder assign (stable assignment).
	assert.Equal(t, "A", led2.Episodes["aaaaaaaaaaa"].Title)
}

func TestLedger_GapsNeverReused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	slug := "townsends"
	now := time.Now().UTC()
	_, err := ytdlp.AssignEpisodes(dir, slug, "Townsends", "", []ytdlp.EpisodeCandidate{
		{ID: "aaaaaaaaaaa", Title: "A", UploadDate: "20200101"},
		{ID: "bbbbbbbbbbb", Title: "B", UploadDate: "20200201"},
	}, now)
	require.NoError(t, err)

	// Simulate deleted video keeping gap: remove from candidates but leave ledger.
	// Manually bump: load, delete one from map is NOT done — gaps stay because
	// next_episode only increments. Assign only new id.
	led, err := ytdlp.AssignEpisodes(dir, slug, "Townsends", "", []ytdlp.EpisodeCandidate{
		{ID: "ccccccccccc", Title: "C", UploadDate: "20200301"},
	}, now)
	require.NoError(t, err)
	assert.Equal(t, 1, led.Episodes["aaaaaaaaaaa"].Episode)
	assert.Equal(t, 2, led.Episodes["bbbbbbbbbbb"].Episode)
	assert.Equal(t, 3, led.Episodes["ccccccccccc"].Episode)
	assert.Equal(t, 4, led.NextEpisode)
}

func TestLedger_LoadMissingReturnsEmpty(t *testing.T) {
	t.Parallel()
	led, err := ytdlp.LoadLedger(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	assert.Equal(t, 1, led.Version)
	assert.Equal(t, 1, led.NextEpisode)
	assert.Empty(t, led.Episodes)
}
