// Package artifact contains media policy and content validation independent of
// HTTP and persistence. It is deliberately strict: metadata supplied by a
// browser is advisory and never becomes authoritative evidence.
package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"
)

type Kind string

const (
	Image Kind = "image"
	Video Kind = "video"
	Audio Kind = "audio"
)

type Limits struct {
	MaxBytes      int64
	MaxCount      int
	MaxDurationMS int64
	MaxPixels     int64
}

func DefaultLimits(kind Kind) Limits {
	switch kind {
	case Image:
		return Limits{10 << 20, 1, 0, 25_000_000}
	case Video:
		return Limits{250 << 20, 1, 15 * 60 * 1000, 0}
	case Audio:
		return Limits{100 << 20, 1, 30 * 60 * 1000, 0}
	default:
		return Limits{}
	}
}

type Input struct {
	Kind           Kind
	DeclaredType   string
	ExpectedSize   int64
	ExpectedSHA256 string
	DurationMS     int64
}
type Result struct {
	ContentType           string
	Size                  int64
	SHA256                string
	Width, Height         int
	DurationMS            int64
	DurationAuthoritative bool
	Bytes                 []byte
}

var ErrInvalid = errors.New("invalid artifact")

func Validate(r io.Reader, in Input, l Limits) (Result, error) {
	if l.MaxBytes <= 0 {
		l = DefaultLimits(in.Kind)
	}
	if in.Kind != Image && in.Kind != Video && in.Kind != Audio {
		return Result{}, fmt.Errorf("%w: unsupported media kind", ErrInvalid)
	}
	if in.ExpectedSize < 0 || in.ExpectedSize > l.MaxBytes {
		return Result{}, fmt.Errorf("%w: size exceeds limit", ErrInvalid)
	}
	b, err := io.ReadAll(io.LimitReader(r, l.MaxBytes+1))
	if err != nil {
		return Result{}, err
	}
	if int64(len(b)) > l.MaxBytes || in.ExpectedSize >= 0 && int64(len(b)) != in.ExpectedSize {
		return Result{}, fmt.Errorf("%w: size mismatch", ErrInvalid)
	}
	digest := sha256.Sum256(b)
	got := hex.EncodeToString(digest[:])
	if in.ExpectedSHA256 != "" && !strings.EqualFold(got, in.ExpectedSHA256) {
		return Result{}, fmt.Errorf("%w: digest mismatch", ErrInvalid)
	}
	ct := http.DetectContentType(b)
	if !allowed(in.Kind, ct, b) {
		return Result{}, fmt.Errorf("%w: content does not match media kind", ErrInvalid)
	}
	probedDuration, authoritative := ProbeDuration(in.Kind, b)
	if authoritative && in.Kind != Image {
		in.DurationMS = probedDuration
	}
	out := Result{ContentType: ct, Size: int64(len(b)), SHA256: got, DurationMS: in.DurationMS, DurationAuthoritative: authoritative, Bytes: b}
	if in.Kind == Image {
		cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
		if e != nil {
			return Result{}, fmt.Errorf("%w: image decode failed", ErrInvalid)
		}
		out.Width, out.Height = cfg.Width, cfg.Height
		if l.MaxPixels > 0 && int64(cfg.Width)*int64(cfg.Height) > l.MaxPixels {
			return Result{}, fmt.Errorf("%w: pixel limit exceeded", ErrInvalid)
		}
		if format == "" {
			return Result{}, fmt.Errorf("%w: unknown image format", ErrInvalid)
		}
	}
	if in.DurationMS < 0 || l.MaxDurationMS > 0 && in.DurationMS > l.MaxDurationMS {
		return Result{}, fmt.Errorf("%w: duration exceeds limit", ErrInvalid)
	}
	return out, nil
}
func allowed(k Kind, ct string, b []byte) bool {
	media, _, _ := mime.ParseMediaType(ct)
	switch k {
	case Image:
		return media == "image/jpeg" || media == "image/png" || media == "image/gif"
	case Video:
		return strings.HasPrefix(media, "video/") || bytes.HasPrefix(b, []byte("RIFF")) || hasISOBaseMediaHeader(b) || bytes.HasPrefix(b, []byte{0x1a, 0x45, 0xdf, 0xa3})
	case Audio:
		return strings.HasPrefix(media, "audio/") || bytes.HasPrefix(b, []byte("ID3")) || bytes.HasPrefix(b, []byte("OggS")) || hasISOBaseMediaHeader(b)
	}
	return false
}
func hasISOBaseMediaHeader(b []byte) bool { return len(b) >= 8 && string(b[4:8]) == "ftyp" }

// Thumbnail decodes and re-encodes pixels, intentionally dropping EXIF and
// all container metadata. PNG is lossless and deterministic for tests.
func Thumbnail(src []byte, maxWidth, maxHeight int) ([]byte, int, int, error) {
	cfg, format, e := image.DecodeConfig(bytes.NewReader(src))
	if e != nil {
		return nil, 0, 0, e
	}
	if maxWidth < 1 || maxHeight < 1 {
		return nil, 0, 0, errors.New("invalid derivative bounds")
	}
	w, h := cfg.Width, cfg.Height
	if w > maxWidth || h > maxHeight {
		sx := float64(maxWidth) / float64(w)
		sy := float64(maxHeight) / float64(h)
		if sy < sx {
			sx = sy
		}
		w, h = int(float64(w)*sx), int(float64(h)*sx)
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	// DecodeConfig is enough for validation; use a bounded nearest-neighbor
	// rasterizer to avoid carrying metadata into the derivative.
	img, _, e := image.Decode(bytes.NewReader(src))
	if e != nil {
		return nil, 0, 0, e
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, img.At(x*cfg.Width/w, y*cfg.Height/h))
		}
	}
	var out bytes.Buffer
	switch format {
	default:
		e = pngEncode(&out, dst)
	}
	return out.Bytes(), w, h, e
}
func pngEncode(w io.Writer, img image.Image) error { return png.Encode(w, img) }
