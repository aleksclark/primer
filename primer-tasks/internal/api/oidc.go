package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	res, err := http.DefaultClient.Do(req)
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
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return identityClaims{}, fmt.Errorf("malformed id token")
	}
	unsigned := parts[0] + "." + parts[1]
	if s.Auth.Mode == "test" {
		mac := hmac.New(sha256.New, s.Auth.IssuerSecret)
		mac.Write([]byte(unsigned))
		want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(want), []byte(parts[2])) {
			return identityClaims{}, fmt.Errorf("invalid id token signature")
		}
	} else {
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
	if err = json.Unmarshal(payload, &c); err != nil || c.Sub == "" || c.Iss != s.Auth.PublicIssuerURL || c.Exp <= time.Now().Unix() {
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

// cryptoRandRead is a variable-shaped wrapper to keep the state codec easy to
// exercise in tests without introducing a fake persistence or identity layer.
var cryptoRandRead = func(dst []byte) (int, error) { return rand.Read(dst) }
