package securityreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"net/url"
	"strings"
	"testing"
)

func TestCapabilityGatingFailsClosedForUnsupportedAudioVideo(t *testing.T) {
	for _, kind := range []MediaKind{Audio, Video} {
		mode, err := ReviewModeFor(kind, ProviderCapabilities{Image: true})
		if err != nil || mode != ParentReview {
			t.Fatalf("%s = (%s, %v), want parent review", kind, mode, err)
		}
	}
	mode, err := ReviewModeFor(Image, ProviderCapabilities{Image: true})
	if err != nil || mode != AutomaticReview {
		t.Fatalf("image mode = (%s, %v), want automatic", mode, err)
	}
	if _, err := ReviewModeFor(MediaKind("application/octet-stream"), ProviderCapabilities{}); err == nil {
		t.Fatal("unknown media kind was accepted")
	}
}

func TestReviewAssertionsRejectLeaksAndDigestSubstitution(t *testing.T) {
	if err := SafeWirePayload([]byte(`{"status":"evaluating","message":"Checking the rubric"}`)); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{
		`{"reasoning_delta":"private"}`,
		`{"tool_input":{"authorization":"Bearer secret"}}`,
		`{"url":"https://bucket.s3.amazonaws.com/tenant/a"}`,
	} {
		if err := SafeWirePayload([]byte(payload)); err == nil {
			t.Fatalf("unsafe payload accepted: %s", payload)
		}
	}
	data := JPEGWithEXIF()
	sum := sha256.Sum256(data)
	if !DigestMatches(data, hex.EncodeToString(sum[:])) || DigestMatches([]byte("different"), hex.EncodeToString(sum[:])) {
		t.Fatal("digest comparison did not bind the uploaded bytes")
	}
	if err := TenantObjectKey("tenant/tenant-a/not-an-opaque-file.jpg", "tenant-a"); err == nil {
		t.Fatal("filename-shaped object key accepted")
	}
	if err := TenantObjectKey("tenant/tenant-a/0123456789abcdef0123456789abcdef", "tenant-a"); err != nil {
		t.Fatal(err)
	}
}

func TestEXIFDerivativeStripsMetadataWithoutChangingDecodeability(t *testing.T) {
	original := JPEGWithEXIF()
	if !bytes.Contains(original, []byte("Exif")) {
		t.Fatal("fixture does not contain EXIF")
	}
	derivative, err := StripJPEGMetadata(original)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(derivative, []byte("Exif")) || bytes.Contains(derivative, []byte("GPS")) {
		t.Fatal("derivative retained EXIF marker")
	}
	img, err := jpeg.Decode(bytes.NewReader(derivative))
	if err != nil || img.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("derivative decode = (%v, %v)", img.Bounds(), err)
	}
}

func TestWAVFixtureIsARealPCMFile(t *testing.T) {
	wav := WAVWithPCM()
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || !bytes.Contains(wav, []byte("fmt ")) {
		t.Fatal("WAV fixture is not a valid PCM container")
	}
}

func TestCSPReviewRequiresNoObjectOrInlineScriptLeak(t *testing.T) {
	good := "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
	if err := ValidateCSP(good); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{
		"default-src *; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
		"default-src 'self'; script-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
		"default-src 'self'; script-src 'self'; object-src 'none'",
	} {
		if err := ValidateCSP(policy); err == nil {
			t.Fatalf("unsafe CSP accepted: %s", policy)
		}
	}
}

func TestShortLivedURLRequiresBoundedExpiry(t *testing.T) {
	good := "https://objects.example.test/tenant/a/opaque?X-Amz-Expires=60&X-Amz-Signature=deterministic"
	if err := ShortLivedURL(good, 300); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"https://objects.example.test/tenant/a/opaque",
		"https://objects.example.test/tenant/a/opaque?X-Amz-Expires=301",
		"https://objects.example.test/tenant/a/opaque?expires=0",
	} {
		if err := ShortLivedURL(raw, 300); err == nil {
			t.Fatalf("unbounded URL accepted: %s", raw)
		}
	}
	if u, _ := url.Parse(good); strings.Contains(u.Host, "minio") {
		t.Fatal("review URL unexpectedly names an internal object host")
	}
}
