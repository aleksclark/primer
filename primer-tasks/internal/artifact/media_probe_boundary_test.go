package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeJSONAndAllowlistBoundaries(t *testing.T) {
	var envelope probeEnvelope
	if err := decodeProbeJSON([]byte(`{"streams":[],"format":{"format_name":"mp4","duration":"1.25"}}`), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{``, `{`, `{"format":{}}  {"extra":true}`} {
		if err := decodeProbeJSON([]byte(raw), &envelope); err == nil {
			t.Fatalf("accepted malformed/trailing ffprobe JSON %q", raw)
		}
	}
	for _, tc := range []struct {
		kind Kind
		name string
		want string
		ok   bool
	}{
		{Video, "mov,mp4,m4a,3gp,3g2,mj2", "mp4", true},
		{Audio, "mp4", "mp4", true},
		{Audio, "wav", "wav", true},
		{Video, "avi", "", false},
		{Audio, "", "", false},
	} {
		got, err := allowlistedFormat(tc.kind, &probeFormat{FormatName: tc.name})
		if (err == nil) != tc.ok || got != tc.want {
			t.Fatalf("format %s/%q = %q, err=%v", tc.kind, tc.name, got, err)
		}
	}
}

func TestProbeStreamDurationAndContentTypeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		kind    Kind
		streams []probeStream
		want    int64
		ok      bool
	}{
		{Video, []probeStream{{Index: 0, CodecName: "h264", CodecType: "video", Duration: "1.5"}}, 1500, true},
		{Audio, []probeStream{{Index: 0, CodecName: "mp3", CodecType: "audio", Duration: "0"}}, 0, false},
		{Video, []probeStream{{Index: -1, CodecName: "h264", CodecType: "video", Duration: "1"}}, 0, false},
		{Video, []probeStream{{Index: 0, CodecName: "v210", CodecType: "video", Duration: "1"}}, 0, false},
		{Audio, []probeStream{{Index: 0, CodecName: "mp3", CodecType: "audio", Duration: "NaN"}}, 0, false},
	} {
		selected, streamErr := allowlistedStreams(tc.kind, tc.streams)
		if !tc.ok {
			if streamErr == nil {
				if _, durationErr := encodedDuration(nil, selected); durationErr == nil {
					t.Fatalf("invalid stream fixture accepted: %#v", tc)
				}
			}
			continue
		}
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		got, err := encodedDuration(nil, selected)
		if err != nil || got != tc.want {
			t.Fatalf("duration=%d err=%v, want %d", got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		kind         Kind
		format, want string
	}{
		{Video, "webm", "video/webm"}, {Video, "matroska", "video/x-matroska"}, {Video, "mov", "video/quicktime"},
		{Audio, "mp3", "audio/mpeg"}, {Audio, "ogg", "audio/ogg"}, {Audio, "wav", "audio/wav"}, {Audio, "flac", "audio/flac"}, {Audio, "matroska", "audio/x-matroska"},
	} {
		if got := mediaContentType(tc.kind, tc.format); got != tc.want {
			t.Errorf("content type %s/%s=%s, want %s", tc.kind, tc.format, got, tc.want)
		}
	}
}

func probeCommand(t *testing.T, body string, exitCode int, stderr bool) string {
	t.Helper()
	// The probe is intentionally exercised through exec.CommandContext, just
	// like production. Keep the fixture a tiny executable rather than calling
	// the parser directly so argument passing, bounded output, and cancellation
	// remain part of the test.
	quoted := strings.ReplaceAll(body, "'", "'\"'\"'")
	stream := "printf '%s' '" + quoted + "'"
	if stderr {
		stream += " >&2"
	}
	content := "#!/bin/sh\n" + stream + "\nexit " + string(rune('0'+exitCode)) + "\n"
	path := filepath.Join(t.TempDir(), "probe-command")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbeMediaProcessBoundaryRejectsMetadataAndDecoderFailures(t *testing.T) {
	valid := `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","duration":"1"}],"format":{"format_name":"mp4","duration":"1"}}`
	decoder := probeCommand(t, "", 0, false)
	for _, tc := range []struct {
		name   string
		probe  string
		want   string
		stderr bool
		exit   int
	}{
		{"probe process", "probe stderr", "ffprobe failed", true, 1},
		{"invalid json", "not json", "invalid ffprobe JSON", false, 0},
		{"missing format", `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","duration":"1"}]}`, "media format", false, 0},
		{"missing stream", `{"streams":[],"format":{"format_name":"mp4","duration":"1"}}`, "did not report a video stream", false, 0},
		{"bad codec", `{"streams":[{"index":0,"codec_name":"avi","codec_type":"video","duration":"1"}],"format":{"format_name":"mp4"}}`, "codec", false, 0},
		{"missing duration", `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video"}],"format":{"format_name":"mp4","duration":"N/A"}}`, "duration", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := probeCommand(t, tc.probe, tc.exit, tc.stderr)
			_, err := ProbeMedia(context.Background(), Video, []byte("fixture"), ProbeConfig{FFProbePath: probe, FFmpegPath: decoder})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}

	decoderFailure := probeCommand(t, "decoder stderr", 1, true)
	if _, err := ProbeMedia(context.Background(), Video, []byte("fixture"), ProbeConfig{FFProbePath: probeCommand(t, valid, 0, false), FFmpegPath: decoderFailure}); err == nil || !strings.Contains(err.Error(), "media decoder failed") {
		t.Fatalf("decoder failure=%v", err)
	}
	if _, err := ProbeMedia(context.Background(), Video, []byte("fixture"), ProbeConfig{FFProbePath: probeCommand(t, valid, 0, false), FFmpegPath: probeCommand(t, "", 0, false), Timeout: time.Nanosecond}); err == nil {
		t.Fatal("probe boundary ignored a canceled timeout")
	}
}

func TestProbeMediaInputAndOutputBounds(t *testing.T) {
	if _, err := ProbeMedia(context.Background(), Image, []byte("not media"), ProbeConfig{}); err == nil {
		t.Fatal("image unexpectedly entered the audio/video probe")
	}
	if _, err := ProbeMedia(nil, Video, []byte("too large"), ProbeConfig{MaxInputBytes: 2}); err == nil || !strings.Contains(err.Error(), "input bound") {
		t.Fatalf("input bound error=%v", err)
	}
}

func TestProbeDurationAndProcessErrorBoundaries(t *testing.T) {
	probe := probeCommand(t, `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","duration":"1.25"}],"format":{"format_name":"mp4","duration":"1.25"}}`, 0, false)
	decoder := probeCommand(t, "", 0, false)
	t.Setenv("TASKS_FFPROBE_PATH", probe)
	t.Setenv("TASKS_FFMPEG_PATH", decoder)
	if duration, ok := ProbeDuration(Video, []byte("authorized encoded bytes")); !ok || duration != 1250 {
		t.Fatalf("successful compatibility duration=%d ok=%v", duration, ok)
	}
	for _, raw := range []string{"", "N/A", "NaN", "+Inf", "-1"} {
		if _, err := parseDuration(raw); err == nil {
			t.Errorf("accepted invalid duration %q", raw)
		}
	}
	if got, err := parseDuration("1.25"); err != nil || got != 1.25 {
		t.Fatalf("duration parse=%v err=%v", got, err)
	}
	if got := processError(errors.New("probe failed"), []byte("  unsafe input\n")); !strings.Contains(got.Error(), "unsafe input") {
		t.Fatal("process error lost diagnostic")
	}
	var bounded boundedBuffer
	bounded.limit = 3
	if _, err := bounded.Write([]byte("1234")); err == nil || !bounded.tooLarge {
		t.Fatal("bounded probe output was not rejected")
	}
}
