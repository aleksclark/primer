package token

import (
	"fmt"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/identity/internal/domain"
)

func normalizeConfiguredIssuer(raw string) (string, error) {
	normalized, err := normalizeIssuer(raw, true)
	if err != nil {
		return "", denyInvalid()
	}
	return normalized, nil
}

func normalizeIssuer(raw string, requireHTTPS bool) (string, error) {
	value := strings.Trim(raw, " ")
	if value == "" || len(value) > maxIssuerBytes || !validIssuerText(value) || strings.ContainsAny(value, `%\`) {
		return "", fmt.Errorf("issuer is invalid")
	}
	u, err := url.Parse(value)
	if err != nil || !u.IsAbs() || u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("issuer is invalid")
	}
	scheme := strings.ToLower(u.Scheme)
	if requireHTTPS {
		if scheme != "https" {
			return "", fmt.Errorf("issuer is invalid")
		}
	} else if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("issuer is invalid")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.ContainsAny(value, "?#") || u.RawPath != "" {
		return "", fmt.Errorf("issuer is invalid")
	}
	if !asciiOnly(u.Host) || !asciiOnly(u.Path) || !validIssuerText(u.Host) || !validIssuerText(u.Path) {
		return "", fmt.Errorf("issuer is invalid")
	}
	if strings.Contains(u.Path, "//") {
		return "", fmt.Errorf("issuer is invalid")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("issuer is invalid")
		}
	}
	canonicalHost, err := canonicalAuthority(u.Host, scheme)
	if err != nil {
		return "", fmt.Errorf("issuer is invalid")
	}
	path := strings.TrimSuffix(u.Path, "/")
	out := scheme + "://" + canonicalHost + path
	if len(out) > maxIssuerBytes {
		return "", fmt.Errorf("issuer is invalid")
	}
	return out, nil
}

func canonicalAuthority(rawHost, scheme string) (string, error) {
	if rawHost == "" {
		return "", fmt.Errorf("invalid authority")
	}
	host := rawHost
	portText := ""
	if strings.HasPrefix(rawHost, "[") {
		close := strings.IndexByte(rawHost, ']')
		if close < 0 || close == 1 {
			return "", fmt.Errorf("invalid ipv6 authority")
		}
		host = rawHost[1:close]
		if close+1 < len(rawHost) {
			if rawHost[close+1] != ':' {
				return "", fmt.Errorf("invalid ipv6 authority")
			}
			portText = rawHost[close+2:]
		}
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return "", fmt.Errorf("invalid ipv6 authority")
		}
		host = "[" + addr.String() + "]"
	} else {
		if strings.Contains(rawHost, ":") {
			colon := strings.LastIndexByte(rawHost, ':')
			host, portText = rawHost[:colon], rawHost[colon+1:]
			if strings.Contains(host, ":") {
				return "", fmt.Errorf("unbracketed ipv6 authority")
			}
		}
		if host == "" || !validDNSHost(host) {
			return "", fmt.Errorf("invalid dns authority")
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			host = addr.String()
		} else {
			if numericHostLike(host) {
				return "", fmt.Errorf("non-canonical numeric authority")
			}
			host = strings.ToLower(host)
		}
	}
	if portText == "" && strings.HasSuffix(rawHost, ":") {
		return "", fmt.Errorf("empty port")
	}
	if portText == "" {
		return host, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port")
	}
	if (scheme == "https" && port == 443) || (scheme == "http" && port == 80) {
		return host, nil
	}
	return host + ":" + strconv.Itoa(port), nil
}

func numericHostLike(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

func validDNSHost(host string) bool {
	if host == "" || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func asciiOnly(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > 0x7f {
			return false
		}
	}
	return true
}

func validIssuerText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return false
		}
	}
	return true
}

func canonicalScope(raw string) (string, error) {
	if err := boundedUTF8("scope", raw, maxScopeBytes); err != nil {
		return "", err
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 || len(parts) > maxScopeTokens {
		return "", denyInvalid()
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 || len(part) > maxScopeTokenBytes || !validScopeToken(part) {
			return "", denyInvalid()
		}
		if _, dup := seen[part]; dup {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return "", denyInvalid()
	}
	sort.Strings(out)
	joined := strings.Join(out, " ")
	if len(joined) > maxScopeBytes {
		return "", denyInvalid()
	}
	return joined, nil
}

func validScopeToken(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}

func requireUUID(value string) error {
	if err := boundedUTF8("id", value, 36); err != nil {
		return err
	}
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil || parsed.String() != value {
		return denyInvalid()
	}
	return nil
}

func requireAudience(value string) error {
	if err := boundedUTF8("aud", value, maxAudienceBytes); err != nil {
		return err
	}
	if strings.ContainsAny(value, " \t,[]") {
		return denyInvalid()
	}
	return nil
}

func requireClientID(value string) error {
	if err := domain.ValidatePublicClientID(value); err != nil {
		return denyInvalid()
	}
	if err := boundedUTF8("client_id", value, maxClientIDBytes); err != nil {
		return err
	}
	return nil
}

func requireHumanSubject(value string) error {
	return requireUUID(value)
}

func classifySubject(value string) (Kind, error) {
	if requireUUID(value) == nil {
		return KindHuman, nil
	}
	if strings.HasPrefix(value, "identity:svc:") {
		rest := strings.TrimPrefix(value, "identity:svc:")
		if err := boundedUTF8("sub", value, maxServiceSubject); err != nil {
			return "", err
		}
		if rest == "" {
			return "", denyInvalid()
		}
		return KindService, nil
	}
	return "", denyInvalid()
}

func acceptPublicJWK(wantKid string, jwk domain.PublicJWK) (*ecdsaPublic, error) {
	if err := domain.ValidateSigningKid(jwk.Kid); err != nil {
		return nil, denyInvalid()
	}
	if wantKid != "" && jwk.Kid != wantKid {
		return nil, denyInvalid()
	}
	if err := domain.ValidateSigningKid(wantKid); err != nil && wantKid != "" {
		return nil, denyInvalid()
	}
	pub, err := jwk.ECDSAPublic()
	if err != nil {
		return nil, denyInvalid()
	}
	return &ecdsaPublic{key: pub, jwk: jwk}, nil
}

type ecdsaPublic struct {
	key any
	jwk domain.PublicJWK
}

func acceptTrustedPublicJWK(wantKid string, jwk domain.PublicJWK) (pub any, err error) {
	accepted, err := acceptPublicJWK(wantKid, jwk)
	if err != nil {
		return nil, denyUnavailable()
	}
	return accepted.key, nil
}
