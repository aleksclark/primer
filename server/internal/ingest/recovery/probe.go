package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// youtubeIDRe matches a bare 11-char YouTube video id.
var youtubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ProbeFormat is the subset of ffprobe -show_format we consume.
type ProbeFormat struct {
	Duration    float64
	Title       string
	Artist      string
	Date        string
	Comment     string
	Description string
}

// Prober obtains container metadata for one media file. Tests inject a fake.
type Prober func(ctx context.Context, path string) (ProbeFormat, error)

// DefaultProbe shells out to ffprobe -show_format -print_format json.
func DefaultProbe(bin string) Prober {
	if bin == "" {
		bin = "ffprobe"
	}
	return func(ctx context.Context, path string) (ProbeFormat, error) {
		if ctx == nil {
			ctx = context.Background()
		}
		cmd := exec.CommandContext(ctx, bin, "-v", "error", "-show_format", "-print_format", "json", "--", path)
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				msg := bytes.TrimSpace(ee.Stderr)
				if len(msg) > 0 {
					return ProbeFormat{}, fmt.Errorf("ffprobe: %s: %w", msg, err)
				}
			}
			return ProbeFormat{}, fmt.Errorf("ffprobe: %w", err)
		}
		return ParseFFProbeJSON(out)
	}
}

type ffprobeJSON struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// ParseFFProbeJSON decodes ffprobe JSON from -show_format -print_format json.
func ParseFFProbeJSON(raw []byte) (ProbeFormat, error) {
	var parsed ffprobeJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ProbeFormat{}, fmt.Errorf("ffprobe json: %w", err)
	}
	var out ProbeFormat
	if d := strings.TrimSpace(parsed.Format.Duration); d != "" {
		f, err := strconv.ParseFloat(d, 64)
		if err != nil {
			return ProbeFormat{}, fmt.Errorf("ffprobe duration: %w", err)
		}
		out.Duration = f
	}
	out.Title = tagValue(parsed.Format.Tags, "title")
	out.Artist = tagValue(parsed.Format.Tags, "artist")
	out.Date = tagValue(parsed.Format.Tags, "date")
	out.Comment = tagValue(parsed.Format.Tags, "comment")
	out.Description = tagValue(parsed.Format.Tags, "description")
	return out, nil
}

func tagValue(tags map[string]string, key string) string {
	if len(tags) == 0 {
		return ""
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// YouTubeIDFromComment extracts an 11-char id from a YouTube watch URL.
// On failure it returns a skip reason; it never guesses.
func YouTubeIDFromComment(comment string) (id, reason string) {
	s := strings.TrimSpace(comment)
	if s == "" {
		return "", ReasonMissingURL
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return "", ReasonInvalidURL
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", ReasonInvalidURL
	}
	host := strings.ToLower(u.Hostname())
	if host != "www.youtube.com" && host != "youtube.com" {
		return "", ReasonInvalidURL
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	if !strings.EqualFold(path, "/watch") {
		return "", ReasonInvalidURL
	}
	vs := u.Query()["v"]
	if len(vs) == 0 || strings.TrimSpace(vs[0]) == "" {
		return "", ReasonMissingURL
	}
	if len(vs) > 1 {
		return "", ReasonAmbiguousID
	}
	id = strings.TrimSpace(vs[0])
	if !youtubeIDRe.MatchString(id) {
		return "", ReasonInvalidID
	}
	return id, ""
}

func validUploadDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	_, err := time.Parse("20060102", s)
	return err == nil
}
