package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProbeConfig controls the external media validation boundary. The probe and
// decoder are deliberately separate processes: ffprobe supplies metadata and
// ffmpeg has to decode every selected audio/video stream before bytes are
// accepted.
type ProbeConfig struct {
	FFProbePath   string
	FFmpegPath    string
	Timeout       time.Duration
	MaxInputBytes int64
}

const (
	defaultProbeTimeout = 5 * time.Second
	defaultProbeMaxJSON = 1 << 20
)

// DefaultProbeConfig returns the production boundary configuration. The paths
// may be overridden for a deliberately controlled deployment or test, but are
// always passed as executable paths to exec.CommandContext; they are never
// interpreted by a shell.
func DefaultProbeConfig() ProbeConfig {
	return ProbeConfig{
		FFProbePath:   envOrDefault("TASKS_FFPROBE_PATH", "ffprobe"),
		FFmpegPath:    envOrDefault("TASKS_FFMPEG_PATH", "ffmpeg"),
		Timeout:       defaultProbeTimeout,
		MaxInputBytes: DefaultLimits(Video).MaxBytes,
	}
}

type probeEnvelope struct {
	Streams []probeStream `json:"streams"`
	Format  *probeFormat  `json:"format"`
}

type probeStream struct {
	Index     int    `json:"index"`
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	Duration  string `json:"duration"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

// ProbeMedia validates encoded media using a bounded temporary file and the
// ffprobe/decoder process boundary. Browser-declared MIME types and durations
// are not consulted.
func ProbeMedia(parent context.Context, kind Kind, data []byte, cfg ProbeConfig) (MediaInfo, error) {
	if kind != Audio && kind != Video {
		return MediaInfo{}, errors.New("media probe only supports audio and video")
	}
	if parent == nil {
		parent = context.Background()
	}
	if cfg.FFProbePath == "" {
		cfg.FFProbePath = "ffprobe"
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultProbeTimeout
	}
	if cfg.MaxInputBytes <= 0 {
		cfg.MaxInputBytes = DefaultLimits(kind).MaxBytes
	}
	if int64(len(data)) > cfg.MaxInputBytes {
		return MediaInfo{}, errors.New("encoded media exceeds probe input bound")
	}

	dir, err := os.MkdirTemp("", "primer-media-probe-")
	if err != nil {
		return MediaInfo{}, fmt.Errorf("create probe sandbox: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return MediaInfo{}, fmt.Errorf("lock probe sandbox: %w", err)
	}
	input := filepath.Join(dir, "input")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		return MediaInfo{}, fmt.Errorf("write probe input: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()
	stdout, stderr, err := runProbe(ctx, cfg.FFProbePath, input)
	if err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe failed: %w", processError(err, stderr))
	}
	var report probeEnvelope
	if err := decodeProbeJSON(stdout, &report); err != nil {
		return MediaInfo{}, fmt.Errorf("invalid ffprobe JSON: %w", err)
	}
	format, err := allowlistedFormat(kind, report.Format)
	if err != nil {
		return MediaInfo{}, err
	}
	streams, err := allowlistedStreams(kind, report.Streams)
	if err != nil {
		return MediaInfo{}, err
	}
	duration, err := encodedDuration(report.Format, streams)
	if err != nil {
		return MediaInfo{}, err
	}
	if err := runDecoder(ctx, cfg.FFmpegPath, input, streams); err != nil {
		return MediaInfo{}, fmt.Errorf("media decoder failed: %w", err)
	}
	return MediaInfo{
		Format:      format,
		Codec:       streams[0].CodecName,
		ContentType: mediaContentType(kind, format),
		DurationMS:  duration,
	}, nil
}

// MediaInfo is authoritative metadata extracted from and decoded against the
// uploaded bytes.
type MediaInfo struct {
	Format      string
	Codec       string
	ContentType string
	DurationMS  int64
}

// ProbeDuration is retained as a small compatibility helper for callers that
// only need duration. Validation uses ProbeMedia directly so it can preserve
// the authoritative content type as well.
func ProbeDuration(kind Kind, data []byte) (int64, bool) {
	info, err := ProbeMedia(context.Background(), kind, data, DefaultProbeConfig())
	if err != nil {
		return 0, false
	}
	return info.DurationMS, true
}

func runProbe(ctx context.Context, executable, input string) (stdout, stderr []byte, err error) {
	var out, errOut boundedBuffer
	out.limit, errOut.limit = defaultProbeMaxJSON, defaultProbeMaxJSON
	cmd := exec.CommandContext(ctx, executable,
		"-v", "error", "-hide_banner",
		"-print_format", "json", "-show_error", "-show_format", "-show_streams", input,
	)
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Run()
	if ctx.Err() != nil {
		return out.Bytes(), errOut.Bytes(), ctx.Err()
	}
	if out.tooLarge || errOut.tooLarge {
		return out.Bytes(), errOut.Bytes(), errors.New("ffprobe output exceeds bound")
	}
	return out.Bytes(), errOut.Bytes(), err
}

func runDecoder(ctx context.Context, executable, input string, streams []probeStream) error {
	args := []string{"-v", "error", "-nostdin", "-xerror", "-err_detect", "explode", "-threads", "1", "-i", input}
	for _, stream := range streams {
		args = append(args, "-map", fmt.Sprintf("0:%d", stream.Index))
	}
	args = append(args, "-f", "null", "-")
	cmd := exec.CommandContext(ctx, executable, args...)
	var stderr boundedBuffer
	stderr.limit = defaultProbeMaxJSON
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return processError(err, stderr.Bytes())
	}
	if stderr.tooLarge {
		return errors.New("decoder output exceeds bound")
	}
	return nil
}

func decodeProbeJSON(data []byte, dst *probeEnvelope) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func allowlistedFormat(kind Kind, format *probeFormat) (string, error) {
	if format == nil || strings.TrimSpace(format.FormatName) == "" {
		return "", errors.New("ffprobe did not report a media format")
	}
	allowed := map[string]bool{}
	switch kind {
	case Video:
		allowed = map[string]bool{"mp4": true, "mov": true, "matroska": true, "webm": true}
	case Audio:
		allowed = map[string]bool{"mp3": true, "mp4": true, "m4a": true, "ogg": true, "oga": true, "opus": true, "wav": true, "flac": true, "matroska": true}
	}
	names := strings.Split(strings.ToLower(format.FormatName), ",")
	// ffprobe reports the MP4 demuxer as "mov,mp4,m4a,...". Prefer the
	// stable browser-facing MP4 identity over the implementation alias.
	for _, preferred := range []string{"mp4", "m4a"} {
		for _, name := range names {
			if strings.TrimSpace(name) == preferred && allowed[preferred] {
				return preferred, nil
			}
		}
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if allowed[name] {
			return name, nil
		}
	}
	return "", fmt.Errorf("media format %q is not allowed", format.FormatName)
}

func allowlistedStreams(kind Kind, streams []probeStream) ([]probeStream, error) {
	allowedVideo := map[string]bool{"h264": true, "hevc": true, "h265": true, "mpeg4": true, "vp8": true, "vp9": true}
	allowedAudio := map[string]bool{"aac": true, "mp3": true, "ac3": true, "eac3": true, "dts": true, "opus": true, "vorbis": true, "flac": true, "pcm_s16le": true, "pcm_s24le": true, "pcm_s32le": true}
	want := string(kind)
	selected := make([]probeStream, 0, len(streams))
	for _, stream := range streams {
		if stream.CodecType != want {
			continue
		}
		codec := strings.ToLower(strings.TrimSpace(stream.CodecName))
		allowed := kind == Video && allowedVideo[codec] || kind == Audio && allowedAudio[codec]
		if !allowed {
			return nil, fmt.Errorf("%s codec %q is not allowed", kind, stream.CodecName)
		}
		if stream.Index < 0 {
			return nil, errors.New("ffprobe reported an invalid stream index")
		}
		stream.CodecName = codec
		selected = append(selected, stream)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("ffprobe did not report a %s stream", kind)
	}
	return selected, nil
}

func encodedDuration(format *probeFormat, streams []probeStream) (int64, error) {
	var duration float64
	for _, stream := range streams {
		value, err := parseDuration(stream.Duration)
		if err != nil {
			return 0, errors.New("ffprobe did not report a finite stream duration")
		}
		if value > duration {
			duration = value
		}
	}
	if duration == 0 && format != nil {
		var err error
		duration, err = parseDuration(format.Duration)
		if err != nil {
			return 0, errors.New("ffprobe did not report a finite format duration")
		}
	}
	if duration <= 0 {
		return 0, errors.New("encoded media duration is unavailable")
	}
	milliseconds := int64(math.Round(duration * 1000))
	if milliseconds <= 0 {
		return 0, errors.New("encoded media duration is zero")
	}
	return milliseconds, nil
}

func parseDuration(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, errors.New("invalid duration")
	}
	return value, nil
}

func mediaContentType(kind Kind, format string) string {
	if kind == Video {
		switch format {
		case "webm":
			return "video/webm"
		case "matroska":
			return "video/x-matroska"
		case "mov":
			return "video/quicktime"
		default:
			return "video/mp4"
		}
	}
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "ogg", "oga", "opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "matroska":
		return "audio/x-matroska"
	default:
		return "audio/mp4"
	}
}

func processError(err error, stderr []byte) error {
	if len(stderr) == 0 {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(stderr)))
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	tooLarge bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.tooLarge = true
		return 0, errors.New("output bound exceeded")
	}
	return b.Buffer.Write(p)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
