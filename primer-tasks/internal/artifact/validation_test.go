package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func fixtureJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 20, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestValidateImageUsesBytesAndDigestNotClientType(t *testing.T) {
	b := fixtureJPEG(t, 12, 8)
	sum := sha256.Sum256(b)
	r, e := Validate(bytes.NewReader(b), Input{Kind: Image, DeclaredType: "image/png", ExpectedSize: int64(len(b)), ExpectedSHA256: hex.EncodeToString(sum[:])}, Limits{MaxBytes: 1 << 20, MaxPixels: 1000})
	if e != nil {
		t.Fatal(e)
	}
	if r.ContentType != "image/jpeg" || r.Width != 12 || r.Height != 8 {
		t.Fatalf("unexpected result %#v", r)
	}
	if _, e = Validate(bytes.NewReader(b), Input{Kind: Image, ExpectedSize: int64(len(b)), ExpectedSHA256: "00"}, Limits{MaxBytes: 1 << 20, MaxPixels: 1000}); e == nil {
		t.Fatal("accepted digest mismatch")
	}
}
func TestValidateRejectsPixelBombAndTruncation(t *testing.T) {
	b := fixtureJPEG(t, 40, 40)
	if _, e := Validate(bytes.NewReader(b), Input{Kind: Image, ExpectedSize: int64(len(b))}, Limits{MaxBytes: 1 << 20, MaxPixels: 100}); e == nil {
		t.Fatal("accepted pixel limit")
	}
	if _, e := Validate(bytes.NewReader(b[:len(b)/2]), Input{Kind: Image, ExpectedSize: int64(len(b))}, Limits{MaxBytes: 1 << 20, MaxPixels: 10000}); e == nil {
		t.Fatal("accepted truncated image")
	}
}
func TestThumbnailStripsContainerMetadataAndBoundsPixels(t *testing.T) {
	b := fixtureJPEG(t, 120, 60)
	out, w, h, e := Thumbnail(b, 30, 30)
	if e != nil {
		t.Fatal(e)
	}
	if w != 30 || h != 15 {
		t.Fatalf("dimensions=%dx%d", w, h)
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(out))
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Width != 30 || cfg.Height != 15 {
		t.Fatalf("encoded dimensions=%dx%d", cfg.Width, cfg.Height)
	}
}
