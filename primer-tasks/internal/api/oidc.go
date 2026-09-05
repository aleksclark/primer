package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type identityClaims struct{ Subject string }

func (s *Server) seal(value string) ([]byte, error) {
	key := sha256.Sum256(s.Auth.SessionSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = randRead(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(value), nil), nil
}
func (s *Server) open(value []byte) (string, error) {
	key := sha256.Sum256(s.Auth.SessionSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(value) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid sealed state")
	}
	plain, err := gcm.Open(nil, value[:gcm.NonceSize()], value[gcm.NonceSize():], nil)
	return string(plain), err
}

func randRead(dst []byte) (int, error) { return cryptoRandRead(dst) }

// exchange is deliberately an HTTP authorization-code exchange. There is no
// local principal shortcut: even test mode must complete the same wire
// protocol exposed by Primer Identity.
func (s *Server) exchange(ctx context.Context, code, verifier, redirectURI, clientID string) (identityClaims, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}, "client_id": {clientID}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Auth.IssuerURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return identityClaims{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.httpDo(req)
	if err != nil {
		return identityClaims{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return identityClaims{}, fmt.Errorf("token endpoint returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return identityClaims{}, err
	}
	var token struct {
		IDToken string `json:"id_token"`
	}
	if err = json.Unmarshal(body, &token); err != nil || token.IDToken == "" {
		return identityClaims{}, fmt.Errorf("token response has no id_token")
	}
	return s.verifyIDToken(token.IDToken)
}

func (s *Server) verifyIDToken(raw string) (identityClaims, error) {
	return s.verifyIDTokenContext(context.Background(), raw)
}

func (s *Server) verifyIDTokenContext(ctx context.Context, raw string) (identityClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return identityClaims{}, fmt.Errorf("malformed id token")
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return identityClaims{}, fmt.Errorf("malformed id token header")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err = json.Unmarshal(headerRaw, &header); err != nil || header.Alg == "" {
		return identityClaims{}, fmt.Errorf("malformed id token header")
	}
	unsigned := parts[0] + "." + parts[1]
	switch s.Auth.Mode {
	case "test":
		if header.Alg != "HS256" {
			return identityClaims{}, fmt.Errorf("unsupported id token algorithm")
		}
		mac := hmac.New(sha256.New, s.Auth.IssuerSecret)
		mac.Write([]byte(unsigned))
		want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(want), []byte(parts[2])) {
			return identityClaims{}, fmt.Errorf("invalid id token signature")
		}
	case "oidc":
		if header.Alg != "ES256" {
			return identityClaims{}, fmt.Errorf("unsupported id token algorithm")
		}
		if header.Kid == "" {
			return identityClaims{}, fmt.Errorf("id token missing kid")
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil || len(sig) != 64 {
			return identityClaims{}, fmt.Errorf("invalid id token signature")
		}
		key, err := s.jwksKey(ctx, header.Kid)
		if err != nil {
			return identityClaims{}, err
		}
		hash := sha256.Sum256([]byte(unsigned))
		r := new(big.Int).SetBytes(sig[:32])
		sigS := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(key, hash[:], r, sigS) {
			return identityClaims{}, fmt.Errorf("invalid id token signature")
		}
	default:
		return identityClaims{}, fmt.Errorf("OIDC signature verifier is not configured")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return identityClaims{}, err
	}
	var c struct {
		Iss string `json:"iss"`
		Aud any    `json:"aud"`
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	expectedIssuer := s.Auth.PublicIssuerURL
	if expectedIssuer == "" {
		expectedIssuer = s.Auth.IssuerURL
	}
	if err = json.Unmarshal(payload, &c); err != nil || c.Sub == "" || c.Iss != expectedIssuer || c.Exp <= time.Now().Unix() {
		return identityClaims{}, fmt.Errorf("invalid id token claims")
	}
	validAud := false
	switch aud := c.Aud.(type) {
	case string:
		validAud = aud == s.Auth.ClientID
	case []any:
		for _, v := range aud {
			if v == s.Auth.ClientID {
				validAud = true
			}
		}
	}
	if !validAud {
		return identityClaims{}, fmt.Errorf("invalid id token audience")
	}
	return identityClaims{Subject: c.Sub}, nil
}

type jwksCache struct {
	mu      sync.Mutex
	expires time.Time
	keys    map[string]*ecdsa.PublicKey
}

var oidcHTTPClient = &http.Client{Timeout: 5 * time.Second}

func (s *Server) httpDo(req *http.Request) (*http.Response, error) {
	client := s.httpClient
	if client == nil {
		client = oidcHTTPClient
	}
	return client.Do(req)
}

func (s *Server) jwksKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	s.jwks.mu.Lock()
	defer s.jwks.mu.Unlock()
	now := time.Now()
	if now.Before(s.jwks.expires) {
		if key, ok := s.jwks.keys[kid]; ok {
			return key, nil
		}
		return nil, fmt.Errorf("unknown signing kid")
	}
	keys, err := s.fetchOIDCKeys(ctx)
	if err != nil {
		return nil, err
	}
	s.jwks.keys = keys
	s.jwks.expires = now.Add(5 * time.Minute)
	key, ok := keys[kid]
	if !ok {
		return nil, fmt.Errorf("unknown signing kid")
	}
	return key, nil
}

func (s *Server) fetchOIDCKeys(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.Auth.IssuerURL, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	res, err := s.httpDo(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openid discovery returned %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err = json.Unmarshal(body, &meta); err != nil || meta.JWKSURI == "" {
		return nil, fmt.Errorf("openid discovery missing jwks_uri")
	}
	expectedIssuer := s.Auth.PublicIssuerURL
	if expectedIssuer == "" {
		expectedIssuer = s.Auth.IssuerURL
	}
	if meta.Issuer != "" && meta.Issuer != expectedIssuer && meta.Issuer != s.Auth.IssuerURL {
		return nil, fmt.Errorf("openid discovery issuer mismatch")
	}
	jwksReq, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.JWKSURI, nil)
	if err != nil {
		return nil, err
	}
	jwksRes, err := s.httpDo(jwksReq)
	if err != nil {
		return nil, err
	}
	defer jwksRes.Body.Close()
	if jwksRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks returned %s", jwksRes.Status)
	}
	jwksBody, err := io.ReadAll(io.LimitReader(jwksRes.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Use string `json:"use"`
			Alg string `json:"alg"`
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err = json.Unmarshal(jwksBody, &set); err != nil {
		return nil, err
	}
	out := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Alg != "ES256" || k.Kid == "" {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			continue
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			continue
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jwks contained no usable ES256 keys")
	}
	return out, nil
}

// cryptoRandRead is a variable-shaped wrapper to keep the state codec easy to
// exercise in tests without introducing a fake persistence or identity layer.
var cryptoRandRead = func(dst []byte) (int, error) { return rand.Read(dst) }
