package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestValidateRejectsUnsupportedKindsDurationAndContent(t *testing.T) {
	body := []byte("not media")
	if _, err := Validate(bytes.NewReader(body), Input{Kind: Kind("document"), ExpectedSize: int64(len(body))}, DefaultLimits(Image)); err == nil {
		t.Fatal("unsupported kind accepted")
	}
	if _, err := Validate(bytes.NewReader(body), Input{Kind: Audio, ExpectedSize: int64(len(body)), DurationMS: -1}, DefaultLimits(Audio)); err == nil {
		t.Fatal("negative duration accepted")
	}
	if _, err := Validate(bytes.NewReader([]byte("ID3")), Input{Kind: Audio, ExpectedSize: 3, DurationMS: 31 * 60 * 1000}, DefaultLimits(Audio)); err == nil {
		t.Fatal("audio duration limit bypassed")
	}
	if _, err := Validate(bytes.NewReader(body), Input{Kind: Video, ExpectedSize: int64(len(body)), DurationMS: 1000}, DefaultLimits(Video)); err == nil {
		t.Fatal("non-video content accepted")
	}
	if _, err := Validate(bytes.NewReader(body), Input{Kind: Image, ExpectedSize: 20}, DefaultLimits(Image)); err == nil {
		t.Fatal("size mismatch accepted")
	}
}

func TestValidateMediaKindsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		kind    Kind
		name    string
		content string
	}{
		{Image, "", "image/png"},
		{Audio, "valid.mp3", "audio/not-trusted"},
		{Video, "valid.mp4", "video/not-trusted"},
	} {
		var body []byte
		if tc.kind == Image {
			body = pngFixture(t, 2, 2)
		} else {
			body = mediaFixture(t, tc.name)
		}
		result, err := Validate(bytes.NewReader(body), Input{Kind: tc.kind, DeclaredType: tc.content, ExpectedSize: int64(len(body)), DurationMS: 1}, DefaultLimits(tc.kind))
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if result.Size != int64(len(body)) {
			t.Fatalf("%s size=%d", tc.kind, result.Size)
		}
		if tc.kind != Image && (result.DurationMS != 1000 || !result.DurationAuthoritative) {
			t.Fatalf("%s did not use encoded duration: %#v", tc.kind, result)
		}
	}
	if _, err := Validate(bytes.NewReader(mediaFixture(t, "truncated.mp4")), Input{Kind: Video, ExpectedSize: int64(len(mediaFixture(t, "truncated.mp4")))}, DefaultLimits(Video)); err == nil {
		t.Fatal("truncated video accepted")
	}
	imageBytes := pngFixture(t, 2, 2)
	sum := sha256.Sum256(imageBytes)
	if _, err := Validate(bytes.NewReader(imageBytes), Input{Kind: Image, DeclaredType: "image/png", ExpectedSize: int64(len(imageBytes)), ExpectedSHA256: hex.EncodeToString(sum[:])}, DefaultLimits(Image)); err != nil {
		t.Fatal(err)
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
