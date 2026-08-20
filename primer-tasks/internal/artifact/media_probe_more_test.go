package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMediaProbeHelpersFailClosedAtFormatAndOutputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		kind   Kind
		format string
		want   string
	}{
		{Video, "webm", "video/webm"},
		{Video, "matroska", "video/x-matroska"},
		{Video, "mov", "video/quicktime"},
		{Video, "unknown", "video/mp4"},
		{Audio, "mp3", "audio/mpeg"},
		{Audio, "opus", "audio/ogg"},
		{Audio, "wav", "audio/wav"},
		{Audio, "flac", "audio/flac"},
		{Audio, "matroska", "audio/x-matroska"},
		{Audio, "unknown", "audio/mp4"},
	} {
		if got := mediaContentType(tc.kind, tc.format); got != tc.want {
			t.Errorf("%s/%s content type=%q, want %q", tc.kind, tc.format, got, tc.want)
		}
	}
	if got := envOrDefault("TASKS_MEDIA_HELPER_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("unset environment fallback=%q", got)
	}
	t.Setenv("TASKS_MEDIA_HELPER_UNSET", " configured ")
	if got := envOrDefault("TASKS_MEDIA_HELPER_UNSET", "fallback"); got != "configured" {
		t.Fatalf("configured environment=%q", got)
	}
	var bounded boundedBuffer
	bounded.limit = 3
	if n, err := bounded.Write([]byte("ok")); err != nil || n != 2 {
		t.Fatalf("bounded write n=%d err=%v", n, err)
	}
	if _, err := bounded.Write([]byte("too large")); err == nil || !bounded.tooLarge {
		t.Fatalf("oversized probe output err=%v tooLarge=%v", err, bounded.tooLarge)
	}
	base := errors.New("ffprobe failed")
	if got := processError(base, nil); !errors.Is(got, base) {
		t.Fatalf("bare process error=%v", got)
	}
	if got := processError(base, []byte("  stderr detail  ")); !strings.Contains(got.Error(), "stderr detail") {
		t.Fatalf("stderr process error=%v", got)
	}
}

func TestProbeMediaRejectsConfigurationAndContextBoundaries(t *testing.T) {
	if _, err := ProbeMedia(context.Background(), Image, []byte("image"), ProbeConfig{}); err == nil {
		t.Fatal("image probe accepted")
	}
	if _, err := ProbeMedia(context.Background(), Kind("document"), []byte("document"), ProbeConfig{}); err == nil {
		t.Fatal("unsupported probe kind accepted")
	}
	if _, err := ProbeMedia(context.Background(), Audio, []byte("too large"), ProbeConfig{MaxInputBytes: 1}); err == nil {
		t.Fatal("probe input bound ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ProbeMedia(ctx, Audio, mediaFixture(t, "valid.mp3"), ProbeConfig{FFProbePath: "/definitely/missing/ffprobe"}); err == nil {
		t.Fatal("canceled probe unexpectedly succeeded")
	}
	if _, err := ProbeMedia(context.Background(), Audio, mediaFixture(t, "valid.mp3"), ProbeConfig{FFProbePath: "/definitely/missing/ffprobe", Timeout: time.Millisecond}); err == nil {
		t.Fatal("missing probe executable unexpectedly succeeded")
	}
}

func TestEncodedDurationUsesFormatAndRejectsNonFiniteValues(t *testing.T) {
	streams := []probeStream{{Index: 0, CodecName: "mp3", CodecType: "audio", Duration: "0"}}
	if got, err := encodedDuration(&probeFormat{Duration: "2.25"}, streams); err != nil || got != 2250 {
		t.Fatalf("format duration=%d err=%v", got, err)
	}
	for _, format := range []*probeFormat{{Duration: "N/A"}, {Duration: "0"}, nil} {
		if _, err := encodedDuration(format, streams); err == nil {
			t.Fatalf("invalid format duration accepted: %#v", format)
		}
	}
	if _, err := encodedDuration(&probeFormat{Duration: "0.0001"}, nil); err == nil {
		t.Fatal("sub-millisecond encoded duration accepted")
	}
	for _, raw := range []string{"N/A", "NaN", "+Inf", "-1"} {
		if _, err := parseDuration(raw); err == nil {
			t.Fatalf("invalid duration %q accepted", raw)
		}
	}
	if got, err := encodedDuration(&probeFormat{Duration: "1"}, nil); err != nil || got != 1000 {
		t.Fatalf("format-only duration=%d err=%v", got, err)
	}
}

type failingMediaReader struct{}

func (failingMediaReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestValidateContextBoundsReaderAndThumbnailResize(t *testing.T) {
	imageBytes := pngFixture(t, 4, 4)
	for _, input := range []Input{
		{Kind: Image, ExpectedSize: -1},
		{Kind: Image, ExpectedSize: int64(len(imageBytes) + 1)},
	} {
		if _, err := ValidateContext(context.Background(), bytes.NewReader(imageBytes), input, DefaultLimits(Image)); err == nil {
			t.Fatalf("invalid size accepted: %+v", input)
		}
	}
	if _, _, _, err := Thumbnail(imageBytes, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateContext(context.Background(), failingMediaReader{}, Input{Kind: Image, ExpectedSize: 1}, DefaultLimits(Image)); err == nil {
		t.Fatal("reader failure was accepted")
	}
	if _, err := ValidateContext(context.Background(), bytes.NewReader(imageBytes), Input{Kind: Image, ExpectedSize: int64(len(imageBytes)), DurationMS: 2}, Limits{MaxBytes: int64(len(imageBytes)), MaxDurationMS: 1, MaxPixels: 100}); err == nil {
		t.Fatal("image duration policy was bypassed")
	}
	if _, err := ValidateContext(context.Background(), bytes.NewReader(imageBytes), Input{Kind: Image, ExpectedSize: int64(len(imageBytes))}, Limits{MaxBytes: 1}); err == nil {
		t.Fatal("byte limit was bypassed")
	}
	if _, err := ProbeMedia(nil, Audio, mediaFixture(t, "valid.mp3"), ProbeConfig{}); err != nil {
		t.Fatalf("nil parent context rejected valid media: %v", err)
	}
}
