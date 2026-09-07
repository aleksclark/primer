package ytdlp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A real channel can contain one removed video after several good downloads.
// yt-dlp exits 1 even with --ignore-errors: the completed bundles must still
// become usable, without pretending the whole acquisition succeeded.
func TestExecRunner_PartialDownloadFinalizesGoodBundlesButReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "library")
	slug := "partial-show"
	persistent := ytdlp.PerShowArchivePath(out, slug)
	stub := writeArchiveTxnStub(t, dir, filepath.Join(dir, "args"), filepath.Join(dir, "archive-path"), filepath.Join(dir, "archive-dump"), ytdlp.StagingDir(out, slug), archiveTxnNewID)
	script, err := os.ReadFile(stub)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stub, []byte(strings.Replace(string(script), "exit 0\n", "exit 1\n", 1)), 0o755))
	err = (ytdlp.ExecRunner{}).Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@Demo/videos", Slug: slug, OutputDir: out, Binary: stub, ShowTitle: "Demo",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	archived, err := os.ReadFile(persistent)
	require.NoError(t, err)
	assert.Equal(t, "youtube "+archiveTxnNewID+"\n", string(archived))
	entries, err := os.ReadDir(ytdlp.SeasonDir(out, slug))
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "["+archiveTxnNewID+"].mkv") {
			found = true
		}
	}
	assert.True(t, found, "completed media must leave staging despite partial failure")
	nfo, err := os.ReadFile(filepath.Join(ytdlp.SeasonDir(out, slug), slug+" - S01E001 - Clip ["+archiveTxnNewID+"].nfo"))
	require.NoError(t, err)
	assert.Contains(t, string(nfo), archiveTxnNewID)
}

func TestExecRunner_InvocationFailureDoesNotFinalize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "library")
	slug := "bad-invocation"
	stub := writeArchiveTxnStub(t, dir, filepath.Join(dir, "args"), filepath.Join(dir, "archive-path"), filepath.Join(dir, "archive-dump"), ytdlp.StagingDir(out, slug), archiveTxnNewID)
	raw, err := os.ReadFile(stub)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stub, []byte(strings.Replace(string(raw), "exit 0\n", "exit 2\n", 1)), 0o755))
	err = (ytdlp.ExecRunner{}).Download(context.Background(), ytdlp.DownloadOpts{URL: "https://www.youtube.com/@Demo", Slug: slug, OutputDir: out, Binary: stub})
	require.Error(t, err)
	_, err = os.Stat(ytdlp.PerShowArchivePath(out, slug))
	assert.True(t, os.IsNotExist(err), "invocation failures must not commit archive")
}
