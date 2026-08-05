package ytdlp_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/ytdlp"
)

func TestNormalizeYouTubeURLMoreForms(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                                          " ",
		"https://www.youtube.com/c/Name":            "https://www.youtube.com/c/Name/videos",
		"https://www.youtube.com/c/Name/":           "https://www.youtube.com/c/Name/videos",
		"https://www.youtube.com/channel/UCabc":     "https://www.youtube.com/channel/UCabc/videos",
		"https://www.youtube.com/user/bob":          "https://www.youtube.com/user/bob/videos",
		"https://www.youtube.com/watch?v=1":         "https://www.youtube.com/watch?v=1",
		"https://www.youtube.com/@x/shorts":         "https://www.youtube.com/@x/shorts",
		"https://www.youtube.com/@x/streams":        "https://www.youtube.com/@x/streams",
		"https://youtu.be/abc":                      "https://youtu.be/abc",
		"  https://www.youtube.com/@TrimMe  ":       "https://www.youtube.com/@TrimMe/videos",
		"https://www.youtube.com/playlist?list=PL1": "https://www.youtube.com/playlist?list=PL1",
	}
	// empty after trim
	assert.Equal(t, "", ytdlp.NormalizeYouTubeURL("   "))
	for in, want := range cases {
		if in == "" {
			continue
		}
		assert.Equal(t, want, ytdlp.NormalizeYouTubeURL(in), in)
	}
}

func TestExecRunnerWithStubBinary(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "yt-dlp-stub")
	// Minimal successful stub
	require.NoError(t, os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	out := filepath.Join(dir, "lib")
	archive := filepath.Join(dir, "meta", "archive.txt")
	var r ytdlp.ExecRunner
	err := r.Download(context.Background(), ytdlp.DownloadOpts{
		URL:         "https://www.youtube.com/@Demo",
		Slug:        "demo-show",
		OutputDir:   out,
		ArchivePath: archive,
		Binary:      stub,
		Format:      "best",
		ExtraArgs:   []string{"--quiet"},
	})
	require.NoError(t, err)
	// Output dir and archive parent created
	st, err := os.Stat(out)
	require.NoError(t, err)
	assert.True(t, st.IsDir())
	_, err = os.Stat(filepath.Dir(archive))
	require.NoError(t, err)

	// Failing binary surfaces error
	bad := filepath.Join(dir, "yt-dlp-bad")
	require.NoError(t, os.WriteFile(bad, []byte("#!/bin/sh\nexit 2\n"), 0o755))
	err = r.Download(context.Background(), ytdlp.DownloadOpts{
		URL: "https://www.youtube.com/@Demo", Slug: "x", OutputDir: out, Binary: bad,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ytdlp:")
}

func TestFakeRunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	f := &ytdlp.FakeRunner{Err: assert.AnError}
	err := f.Download(context.Background(), ytdlp.DownloadOpts{URL: "u", Slug: "s"})
	require.ErrorIs(t, err, assert.AnError)
	require.Len(t, f.Calls, 1)
}
