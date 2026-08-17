package broker

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aleksclark/primer/identity/internal/secrethash"
)

const (
	csrfContext     = "primer.broker.csrf"
	csrfTokenMaxLen = 64
	csrfMACRawLen   = 32
	csrfMACWireLen  = 43
	csrfVersionMax  = 65535
)

// MintCSRF returns a version-identified HMAC of cookie using the active
// broker-cookie pepper. The wire form is v<version>.<base64url-mac>.
func (s *Service) MintCSRF(cookie string) (string, error) {
	if s == nil || !validCSRFCookie(cookie) {
		return "", ErrUnboundCallback
	}
	version := s.secrets.BrokerCookieActiveVersion
	mac, err := secrethash.Hash(secrethash.Peppers(s.secrets.BrokerCookiePeppers), version, csrfContext, []byte(cookie))
	if err != nil {
		return "", ErrUnboundCallback
	}
	return formatCSRFToken(version, mac), nil
}

// ValidateCSRF reports whether token is a well-formed CSRF MAC for cookie
// under any configured broker-cookie pepper version.
func (s *Service) ValidateCSRF(cookie, token string) bool {
	if s == nil || !validCSRFCookie(cookie) || !validCSRFTokenWire(token) {
		return false
	}
	version, mac, ok := parseCSRFToken(token)
	if !ok {
		return false
	}
	expected, err := secrethash.Hash(secrethash.Peppers(s.secrets.BrokerCookiePeppers), version, csrfContext, []byte(cookie))
	if err != nil {
		return false
	}
	return secrethash.Equal(expected, mac)
}

func formatCSRFToken(version int, mac []byte) string {
	return "v" + strconv.Itoa(version) + "." + base64.RawURLEncoding.EncodeToString(mac)
}

func validCSRFCookie(cookie string) bool {
	return cookie != "" && len(cookie) <= maxCookieValueLen && utf8.ValidString(cookie) && !containsCSRFControl(cookie)
}

func validCSRFTokenWire(token string) bool {
	return token != "" && len(token) <= csrfTokenMaxLen && utf8.ValidString(token) && !containsCSRFControl(token)
}

func parseCSRFToken(token string) (int, []byte, bool) {
	if !strings.HasPrefix(token, "v") {
		return 0, nil, false
	}
	dot := strings.IndexByte(token, '.')
	if dot < 2 || dot >= len(token)-1 {
		return 0, nil, false
	}
	versionRaw := token[1:dot]
	macRaw := token[dot+1:]
	if !validCSRFVersionDigits(versionRaw) || len(macRaw) != csrfMACWireLen {
		return 0, nil, false
	}
	for i := 0; i < len(macRaw); i++ {
		c := macRaw[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return 0, nil, false
		}
	}
	version, err := strconv.Atoi(versionRaw)
	if err != nil || version <= 0 || version > csrfVersionMax {
		return 0, nil, false
	}
	mac, err := base64.RawURLEncoding.DecodeString(macRaw)
	if err != nil || len(mac) != csrfMACRawLen {
		return 0, nil, false
	}
	return version, mac, true
}

func validCSRFVersionDigits(raw string) bool {
	if raw == "" || len(raw) > 5 {
		return false
	}
	if raw[0] == '0' {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	return true
}

func containsCSRFControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return true
		}
	}
	return false
}
