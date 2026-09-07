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

func TestDownloadBudgetFinalizesCheckpointOnYtDlpExit101(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "library")
	slug := "bounded"
	stub := writeArchiveTxnStub(t, dir, filepath.Join(dir, "args"), filepath.Join(dir, "archive-path"), filepath.Join(dir, "archive-dump"), ytdlp.StagingDir(out, slug), archiveTxnNewID)
	body, err := os.ReadFile(stub)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stub, []byte(strings.Replace(string(body), "exit 0\n", "exit 101\n", 1)), 0o755))
	err = (ytdlp.ExecRunner{}).Download(context.Background(), ytdlp.DownloadOpts{URL: "https://www.youtube.com/@Demo/videos", Slug: slug, OutputDir: out, Binary: stub, MaxDownloads: 1})
	require.NoError(t, err, "the configured batch budget is a checkpoint, not an acquisition failure")
	archive, err := os.ReadFile(ytdlp.PerShowArchivePath(out, slug))
	require.NoError(t, err)
	assert.Equal(t, "youtube "+archiveTxnNewID+"\n", string(archive))
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	require.NoError(t, err)
	assert.Contains(t, string(args), "--max-downloads\n1\n")
}

func TestDownloadBudgetUnlimitedOmitsFlag(t *testing.T) {
	args := ytdlp.BuildArgs(ytdlp.DownloadOpts{URL: "https://www.youtube.com/@Demo", Slug: "demo", OutputDir: t.TempDir()})
	assert.NotContains(t, args, "--max-downloads")
}
