package recovery_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/ingest/recovery"
)

func TestParseFFProbeJSON_TagsAndDuration(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
  "format": {
    "duration": "1483.197823",
    "tags": {
      "title": "Fire is a Tool, Here's How to Use it.",
      "artist": "Essential Craftsman",
      "date": "20260801",
      "comment": "https://www.youtube.com/watch?v=eKnuQfUSfyk",
      "description": "Regardless of the work you do."
    }
  }
}`)
	got, err := recovery.ParseFFProbeJSON(raw)
	require.NoError(t, err)
	assert.InDelta(t, 1483.197823, got.Duration, 0.000001)
	assert.Equal(t, "Fire is a Tool, Here's How to Use it.", got.Title)
	assert.Equal(t, "Essential Craftsman", got.Artist)
	assert.Equal(t, "20260801", got.Date)
	assert.Equal(t, "https://www.youtube.com/watch?v=eKnuQfUSfyk", got.Comment)
	assert.Equal(t, "Regardless of the work you do.", got.Description)
}

func TestYouTubeIDFromComment_ExactWatchURL(t *testing.T) {
	t.Parallel()
	id, reason := recovery.YouTubeIDFromComment("https://www.youtube.com/watch?v=eKnuQfUSfyk")
	assert.Equal(t, "eKnuQfUSfyk", id)
	assert.Empty(t, reason)

	id, reason = recovery.YouTubeIDFromComment("https://youtube.com/watch?v=abcdefghijk&t=12")
	assert.Equal(t, "abcdefghijk", id)
	assert.Empty(t, reason)
}

func TestYouTubeIDFromComment_NoGuessing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		reason string
	}{
		{"", recovery.ReasonMissingURL},
		{"not a url", recovery.ReasonInvalidURL},
		{"https://youtu.be/eKnuQfUSfyk", recovery.ReasonInvalidURL},
		{"https://music.youtube.com/watch?v=eKnuQfUSfyk", recovery.ReasonInvalidURL},
		{"https://www.youtube.com/embed/eKnuQfUSfyk", recovery.ReasonInvalidURL},
		{"https://www.youtube.com/watch", recovery.ReasonMissingURL},
		{"https://www.youtube.com/watch?v=", recovery.ReasonMissingURL},
		{"https://www.youtube.com/watch?v=short", recovery.ReasonInvalidID},
		{"https://www.youtube.com/watch?v=toolongid123", recovery.ReasonInvalidID},
		{"https://www.youtube.com/watch?v=eKnuQfUSfyk&v=abcdefghijk", recovery.ReasonAmbiguousID},
		{"ftp://www.youtube.com/watch?v=eKnuQfUSfyk", recovery.ReasonInvalidURL},
	}
	for _, tc := range cases {
		id, reason := recovery.YouTubeIDFromComment(tc.in)
		assert.Empty(t, id, tc.in)
		assert.Equal(t, tc.reason, reason, tc.in)
	}
}

func TestDefaultProbe_StubBinary(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "ffprobe")
	script := `#!/bin/sh
cat <<'EOF'
{"format":{"duration":"12.5","tags":{"title":"T","artist":"A","date":"20200101","comment":"https://www.youtube.com/watch?v=dQw4w9wgxcQ","description":"d"}}}
EOF
`
	require.NoError(t, os.WriteFile(stub, []byte(script), 0o755))
	probe := recovery.DefaultProbe(stub)
	got, err := probe(context.Background(), filepath.Join(dir, "unused.mp4"))
	require.NoError(t, err)
	assert.Equal(t, 12.5, got.Duration)
	assert.Equal(t, "T", got.Title)
	assert.Equal(t, "dQw4w9wgxcQ", func() string {
		id, reason := recovery.YouTubeIDFromComment(got.Comment)
		require.Empty(t, reason)
		return id
	}())
}
