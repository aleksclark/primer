package ytdlp_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestFakeRunner(t *testing.T) {
	t.Parallel()
	f := &ytdlp.FakeRunner{}
	err := f.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://youtube.com/@x", Slug: "paul-sellers", OutputDir: "/media",
	})
	require.NoError(t, err)
	require.Len(t, f.Calls, 1)
	assert.Equal(t, "paul-sellers", f.Calls[0].Slug)
}

func TestPathPrefix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, filepath.Join("Shows", "paul-sellers"), ytdlp.PathPrefix("paul-sellers"))
}

func TestExecRunnerValidates(t *testing.T) {
	t.Parallel()
	var r ytdlp.ExecRunner
	assert.Error(t, r.Download(context.Background(), ytdlp.DownloadOpts{}))
	assert.Error(t, r.Download(context.Background(), ytdlp.DownloadOpts{URL: "u"}))
	assert.Error(t, r.Download(context.Background(), ytdlp.DownloadOpts{URL: "u", Slug: "s"}))
}

func TestNormalizeYouTubeURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		"https://www.youtube.com/@PaulSellersWoodwork/videos",
		ytdlp.NormalizeYouTubeURL("https://www.youtube.com/@PaulSellersWoodwork"))
	assert.Equal(t,
		"https://www.youtube.com/playlist?list=PL8",
		ytdlp.NormalizeYouTubeURL("https://www.youtube.com/playlist?list=PL8"))
	assert.Equal(t,
		"https://www.youtube.com/@x/videos",
		ytdlp.NormalizeYouTubeURL("https://www.youtube.com/@x/videos"))
}
