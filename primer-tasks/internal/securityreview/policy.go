// Package securityreview contains small, dependency-free assertions shared by
// the media/security integration suites. It deliberately has no storage or
// provider implementation: the assertions are review gates for those
// implementations.
package securityreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type MediaKind string

const (
	Image MediaKind = "image"
	Audio MediaKind = "audio"
	Video MediaKind = "video"
)

type ProviderCapabilities struct {
	Image bool
	Audio bool
	Video bool
}

type ReviewMode string

const (
	AutomaticReview ReviewMode = "automatic"
	ParentReview    ReviewMode = "parent_review"
)

// ReviewModeFor fails closed. In particular, audio/video must not be treated as
// understood merely because a provider can accept text or an opaque file.
func ReviewModeFor(kind MediaKind, capabilities ProviderCapabilities) (ReviewMode, error) {
	var supported bool
	switch kind {
	case Image:
		supported = capabilities.Image
	case Audio:
		supported = capabilities.Audio
	case Video:
		supported = capabilities.Video
	default:
		return ParentReview, fmt.Errorf("unsupported media kind %q", kind)
	}
	if !supported {
		return ParentReview, nil
	}
	return AutomaticReview, nil
}

var sensitiveField = regexp.MustCompile(`(?i)(chain.?of.?thought|reasoning(?:_delta)?|provider.?metadata|raw.?prompt|tool.?input|tool.?arguments|answer.?key|authorization|bearer|api[_ -]?key)`)
var objectURL = regexp.MustCompile(`(?i)(?:s3|s3a|gs|https?)://[^\s"']*(?:amazonaws\.com|storage\.googleapis\.com|minio|x-amz-|x-goog-)`)

// SafeWirePayload checks serialized public progress/events. It is intentionally
// conservative: a false positive is preferable to shipping provider material.
func SafeWirePayload(payload []byte) error {
	text := string(payload)
	if sensitiveField.MatchString(text) {
		return errors.New("wire payload contains a protected field")
	}
	if objectURL.MatchString(text) {
		return errors.New("wire payload contains an object-store URL")
	}
	return nil
}

// TenantObjectKey validates the shape of an opaque, tenant-scoped key. The
// filename and client path are intentionally absent from the allowed shape.
func TenantObjectKey(key, tenant string) error {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) != 3 || parts[0] != "tenant" || parts[1] != tenant {
		return errors.New("object key is not tenant scoped")
	}
	for _, part := range parts[2:] {
		if len(part) < 16 || strings.ContainsAny(part, "\\:.") || part == "." || part == ".." {
			return errors.New("object key is not opaque")
		}
	}
	return nil
}

// DigestMatches compares bytes, not a client-provided content type or name.
func DigestMatches(bytes []byte, expected string) bool {
	sum := sha256.Sum256(bytes)
	return strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(expected))
}

// ShortLivedURL checks the S3-style expiry query used by presigned URLs. URLs
// without an expiry are rejected; long-lived bearer/object URLs are not a
// supported capability.
// ValidateCSP checks the minimum browser policy required by media pages. It
// rejects wildcard/script-inline policy and requires object embedding to be
// disabled; callers can add stricter directives for their deployment.
func ValidateCSP(policy string) error {
	if !utf8.ValidString(policy) {
		return errors.New("CSP is not valid UTF-8")
	}
	directives := map[string]string{}
	for _, raw := range strings.Split(policy, ";") {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		directives[strings.ToLower(fields[0])] = strings.Join(fields[1:], " ")
	}
	for _, required := range []string{"default-src", "script-src", "object-src", "base-uri", "frame-ancestors"} {
		if _, ok := directives[required]; !ok {
			return fmt.Errorf("CSP is missing %s", required)
		}
	}
	if directives["default-src"] != "'self'" || directives["script-src"] != "'self'" || directives["object-src"] != "'none'" || directives["base-uri"] != "'none'" || directives["frame-ancestors"] != "'none'" {
		return errors.New("CSP has an unsafe required directive")
	}
	if strings.Contains(policy, "*") || strings.Contains(strings.ToLower(policy), "'unsafe-inline'") {
		return errors.New("CSP permits wildcard or inline script content")
	}
	return nil
}

func ShortLivedURL(raw string, maxSeconds int64) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("invalid signed URL")
	}
	q := u.Query()
	expires := q.Get("X-Amz-Expires")
	if expires == "" {
		expires = q.Get("expires")
	}
	seconds, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || seconds <= 0 || seconds > maxSeconds {
		return errors.New("URL expiry is missing or too long")
	}
	return nil
}

// StripJPEGMetadata is a review helper for derivative tests. Re-encoding via
// image/jpeg removes APP1/EXIF and other source metadata while retaining pixels.
func StripJPEGMetadata(input []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG derivative: %w", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
