package ytdlp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

const (
	archiveTxnNewID = "dQw4w9wgxcQ"
	archiveTxnOldID = "oldvideoid1"
)

// writeArchiveTxnStub records argv, dumps the --download-archive path and its
// pre-append contents, then appends the new id (yt-dlp behavior) and writes a
// staging bundle so FinalizeStaging can run.
func writeArchiveTxnStub(t *testing.T, dir, argsFile, archivePathFile, archiveDump, stagingDir, videoID string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	stub := filepath.Join(dir, "yt-dlp-stub")
	infoPath := filepath.Join(stagingDir, "clip ["+videoID+"].info.json")
	mkvPath := filepath.Join(stagingDir, "clip ["+videoID+"].mkv")
	infoObj := map[string]any{
		"id":          videoID,
		"title":       "Clip",
		"upload_date": "20200101",
		"duration":    120.0,
		"was_live":    false,
		"live_status": "not_live",
	}
	infoRaw, err := json.Marshal(infoObj)
	require.NoError(t, err)
	infoTmp := filepath.Join(dir, "info.json")
	require.NoError(t, os.WriteFile(infoTmp, infoRaw, 0o644))

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + shellQuote(argsFile) + "\n" +
		"prev=\"\"\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--download-archive\" ]; then\n" +
		"    printf '%s\\n' \"$a\" > " + shellQuote(archivePathFile) + "\n" +
		"    if [ -f \"$a\" ]; then cat \"$a\" > " + shellQuote(archiveDump) + "; else : > " + shellQuote(archiveDump) + "; fi\n" +
		"    printf 'youtube %s\\n' " + shellQuote(videoID) + " >> \"$a\"\n" +
		"    break\n" +
		"  fi\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		"mkdir -p " + shellQuote(stagingDir) + "\n" +
		"cp " + shellQuote(infoTmp) + " " + shellQuote(infoPath) + "\n" +
		"printf 'video' > " + shellQuote(mkvPath) + "\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755))
	return stub
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func TestExecRunner_FinalizeFailureDoesNotPersistArchiveID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "lib")
	slug := "demo-show"
	persistent := ytdlp.PerShowArchivePath(out, slug)
	require.NoError(t, os.MkdirAll(filepath.Dir(persistent), 0o755))
	require.NoError(t, os.WriteFile(persistent, []byte("youtube "+archiveTxnOldID+"\n"), 0o644))
	// Corrupt ledger so FinalizeStaging fails after yt-dlp returns.
	require.NoError(t, os.WriteFile(ytdlp.LedgerPath(out, slug), []byte("{not-json"), 0o644))

	argsFile := filepath.Join(dir, "args.txt")
	archivePathFile := filepath.Join(dir, "archive-path.txt")
	archiveDump := filepath.Join(dir, "archive-dump.txt")
	stub := writeArchiveTxnStub(t, dir, argsFile, archivePathFile, archiveDump, ytdlp.StagingDir(out, slug), archiveTxnNewID)

	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@Demo",
		Slug:        slug,
		OutputDir:   out,
		ArchivePath: persistent,
		Binary:      stub,
		Format:      "best",
		ShowTitle:   "Demo",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ytdlp:")
	assert.NotContains(t, err.Error(), persistent)
	assert.NotContains(t, err.Error(), "cookies")

	got, err := os.ReadFile(persistent)
	require.NoError(t, err)
	assert.Contains(t, string(got), "youtube "+archiveTxnOldID)
	assert.NotContains(t, string(got), "youtube "+archiveTxnNewID)

	usedPath, err := os.ReadFile(archivePathFile)
	require.NoError(t, err)
	assert.NotEqual(t, persistent, strings.TrimSpace(string(usedPath)))
	_, statErr := os.Stat(strings.TrimSpace(string(usedPath)))
	assert.True(t, os.IsNotExist(statErr), "run-temp archive must be removed")
}

func TestExecRunner_SuccessfulFinalizePersistsArchiveID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "lib")
	slug := "demo-ok"
	persistent := ytdlp.PerShowArchivePath(out, slug)
	require.NoError(t, os.MkdirAll(filepath.Dir(persistent), 0o755))
	require.NoError(t, os.WriteFile(persistent, []byte("youtube "+archiveTxnOldID+"\n"), 0o644))

	argsFile := filepath.Join(dir, "args.txt")
	archivePathFile := filepath.Join(dir, "archive-path.txt")
	archiveDump := filepath.Join(dir, "archive-dump.txt")
	stub := writeArchiveTxnStub(t, dir, argsFile, archivePathFile, archiveDump, ytdlp.StagingDir(out, slug), archiveTxnNewID)

	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@Demo",
		Slug:        slug,
		OutputDir:   out,
		ArchivePath: persistent,
		Binary:      stub,
		Format:      "best",
		ShowTitle:   "Demo",
	})
	require.NoError(t, err)

	got, err := os.ReadFile(persistent)
	require.NoError(t, err)
	assert.Contains(t, string(got), "youtube "+archiveTxnOldID)
	assert.Contains(t, string(got), "youtube "+archiveTxnNewID)

	usedPath, err := os.ReadFile(archivePathFile)
	require.NoError(t, err)
	used := strings.TrimSpace(string(usedPath))
	assert.NotEqual(t, persistent, used)
	_, statErr := os.Stat(used)
	assert.True(t, os.IsNotExist(statErr), "run-temp archive must be removed")

	seeded, err := os.ReadFile(archiveDump)
	require.NoError(t, err)
	assert.Contains(t, string(seeded), "youtube "+archiveTxnOldID)
}

func TestExecRunner_SkipFinalizeLeavesPersistentArchiveUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "lib")
	slug := "demo-skip"
	persistent := ytdlp.PerShowArchivePath(out, slug)
	require.NoError(t, os.MkdirAll(filepath.Dir(persistent), 0o755))
	require.NoError(t, os.WriteFile(persistent, []byte("youtube "+archiveTxnOldID+"\n"), 0o644))

	argsFile := filepath.Join(dir, "args.txt")
	archivePathFile := filepath.Join(dir, "archive-path.txt")
	archiveDump := filepath.Join(dir, "archive-dump.txt")
	stub := writeArchiveTxnStub(t, dir, argsFile, archivePathFile, archiveDump, ytdlp.StagingDir(out, slug), archiveTxnNewID)

	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL:          "https://www.youtube.com/@Demo",
		Slug:         slug,
		OutputDir:    out,
		ArchivePath:  persistent,
		Binary:       stub,
		Format:       "best",
		SkipFinalize: true,
	})
	require.NoError(t, err)

	got, err := os.ReadFile(persistent)
	require.NoError(t, err)
	assert.Contains(t, string(got), "youtube "+archiveTxnOldID)
	assert.NotContains(t, string(got), "youtube "+archiveTxnNewID)

	usedPath, err := os.ReadFile(archivePathFile)
	require.NoError(t, err)
	_, statErr := os.Stat(strings.TrimSpace(string(usedPath)))
	assert.True(t, os.IsNotExist(statErr), "run-temp archive must be removed")
}
