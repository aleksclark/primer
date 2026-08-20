package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func mediaFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestProbeMediaUsesRealMetadataAndDecoder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		kind        Kind
		contentType string
		codec       string
	}{
		{"valid.mp4", Video, "video/mp4", "h264"},
		{"valid.mp3", Audio, "audio/mpeg", "mp3"},
	} {
		info, err := ProbeMedia(context.Background(), tc.kind, mediaFixture(t, tc.name), DefaultProbeConfig())
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if info.ContentType != tc.contentType || info.Codec != tc.codec || info.DurationMS != 1000 {
			t.Fatalf("%s: unexpected probe result %#v", tc.name, info)
		}
	}
}

func TestProbeMediaRejectsCorruptAndTruncatedFixtures(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind Kind
	}{
		{"truncated.mp4", Video},
		{"truncated.mp3", Audio},
		{"corrupt.mp4", Video},
		{"corrupt.mp3", Audio},
	} {
		if _, err := ProbeMedia(context.Background(), tc.kind, mediaFixture(t, tc.name), DefaultProbeConfig()); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
}

func TestProbeDurationRejectsSyntheticMetadata(t *testing.T) {
	if duration, ok := ProbeDuration(Video, []byte("mvhd")); ok || duration != 0 {
		t.Fatalf("synthetic video received duration=%d ok=%v", duration, ok)
	}
	if duration, ok := ProbeDuration(Audio, []byte{0xff, 0xfb, 0x40, 0x00}); ok || duration != 0 {
		t.Fatalf("truncated audio received duration=%d ok=%v", duration, ok)
	}
}
