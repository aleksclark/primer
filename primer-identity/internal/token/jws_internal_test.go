package token

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func TestDecodeStrictJSONObjectRejectsInvalidTrailingBytes(t *testing.T) {
	_, err := decodeStrictJSONObject(
		[]byte("{\"alg\":\"ES256\",\"typ\":\"at+jwt\",\"kid\":\"11111111-2222-3333-4444-555555555555\"}garbage"),
		headerAllowed,
		maxHeaderBytes,
	)
	if err == nil {
		t.Fatal("invalid trailing bytes must be rejected")
	}
}

func TestDecodeStrictJSONRequiresRawUTF8AndExactEOF(t *testing.T) {
	valid := []byte(`{"alg":"ES256","typ":"at+jwt","kid":"11111111-2222-3333-4444-555555555555"}`)
	if _, err := decodeStrictJSONObject(valid, headerAllowed, maxHeaderBytes); err != nil {
		t.Fatalf("canonical object must parse: %v", err)
	}
	invalidUTF8 := append([]byte(`{"alg":"`), 0xff, 0xfe)
	invalidUTF8 = append(invalidUTF8, []byte(`","typ":"at+jwt","kid":"11111111-2222-3333-4444-555555555555"}`)...)
	if utf8.Valid(invalidUTF8) {
		t.Fatal("fixture must be invalid UTF-8")
	}
	if _, err := decodeStrictJSONObject(invalidUTF8, headerAllowed, maxHeaderBytes); err == nil {
		t.Fatal("invalid UTF-8 must be rejected")
	}
}

func TestDecodeJSONScalarsRequireExactEOF(t *testing.T) {
	if _, err := decodeJSONString(json.RawMessage(`"hello"}`)); err == nil {
		t.Fatal("string with trailing unmatched delimiter must be rejected")
	}
	if _, err := decodeJSONInt(json.RawMessage(`120}`)); err == nil {
		t.Fatal("number with trailing unmatched delimiter must be rejected")
	}
}

func TestDecodeJSONRejectsLoneSurrogatesAndLossyReplacement(t *testing.T) {
	cases := []json.RawMessage{
		json.RawMessage(`"\uD800"`),
		json.RawMessage(`"\uDC00"`),
		json.RawMessage(`"\uFFFD"`),
	}
	for _, raw := range cases {
		if _, err := decodeJSONString(raw); err == nil {
			t.Fatalf("lossy or lone-surrogate string must be rejected: %s", raw)
		}
	}
	got, err := decodeJSONString(json.RawMessage(`"\uD83D\uDE00"`))
	if err != nil || got != "😀" {
		t.Fatalf("valid surrogate pair must be accepted: got %q err=%v", got, err)
	}
}

func TestNormalizeIssuerRejectsHTTPAndAcceptsHTTPS(t *testing.T) {
	if _, err := normalizeConfiguredIssuer("http://localhost:8090"); err == nil {
		t.Fatal("http issuer must be rejected")
	}
	got, err := normalizeConfiguredIssuer("https://identity.example.test/")
	if err != nil || got != "https://identity.example.test" {
		t.Fatalf("https issuer: got %q err=%v", got, err)
	}
}
