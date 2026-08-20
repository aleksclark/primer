package artifact

import (
	"errors"
	"strings"
	"testing"
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

func TestProbeDurationAndProcessErrorBoundaries(t *testing.T) {
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
