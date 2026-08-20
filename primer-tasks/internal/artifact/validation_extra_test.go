package artifact

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.White)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestValidateMediaKindsAndBounds(t *testing.T) {
	imageBytes := pngFixture(t, 2, 2)
	for _, tc := range []struct {
		kind     Kind
		content  string
		body     []byte
		duration int64
	}{{Image, "image/png", imageBytes, 0}, {Audio, "audio/mpeg", append(append([]byte("ID3"), []byte{0xff, 0xfb, 0x40, 0x00}...), make([]byte, 1000)...), 1000}, {Video, "video/mp4", append([]byte{0, 0, 0, 20, 'f', 't', 'y', 'p'}, make([]byte, 20)...), 1000}} {
		result, err := Validate(bytes.NewReader(tc.body), Input{Kind: tc.kind, DeclaredType: tc.content, ExpectedSize: int64(len(tc.body)), DurationMS: tc.duration}, DefaultLimits(tc.kind))
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if result.Size != int64(len(tc.body)) {
			t.Fatalf("%s size=%d", tc.kind, result.Size)
		}
	}
	if _, err := Validate(bytes.NewReader(imageBytes), Input{Kind: Image, DeclaredType: "image/png", ExpectedSize: int64(len(imageBytes)), ExpectedSHA256: "wrong"}, DefaultLimits(Image)); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if _, err := Validate(bytes.NewReader(imageBytes), Input{Kind: Image, DeclaredType: "image/png", ExpectedSize: int64(len(imageBytes))}, Limits{MaxBytes: int64(len(imageBytes)), MaxPixels: 1}); err == nil {
		t.Fatal("pixel limit accepted")
	}
	if _, err := Validate(bytes.NewReader([]byte("bad")), Input{Kind: Image, DeclaredType: "image/png", ExpectedSize: 3}, DefaultLimits(Image)); err == nil {
		t.Fatal("malformed image accepted")
	}
}
