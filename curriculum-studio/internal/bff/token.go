package bff

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const tokenFormMaxBytes = 32 * 1024

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) exchangeCode(r *http.Request, rec PreAuth, code string) (tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {rec.RedirectURI},
		"resource":      {rec.Resource},
		"code_verifier": {rec.CodeVerifier},
		"client_id":     {rec.ClientID},
	}
	endpoint := strings.TrimRight(h.cfg.Issuer, "/") + "/oauth/token"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := h.cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, tokenFormMaxBytes+1))
	if err != nil {
		return tokenResponse{}, err
	}
	if resp.StatusCode != http.StatusOK || len(raw) == 0 || len(raw) > tokenFormMaxBytes || !utf8.Valid(raw) {
		return tokenResponse{}, fmt.Errorf("bff token: exchange failed")
	}
	if mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		return tokenResponse{}, fmt.Errorf("bff token: invalid content type")
	}
	var out tokenResponse
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return tokenResponse{}, fmt.Errorf("bff token: invalid response")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return tokenResponse{}, fmt.Errorf("bff token: trailing response")
	}
	if out.AccessToken == "" || out.TokenType != "Bearer" || out.ExpiresIn <= 0 || out.Scope == "" || out.RefreshToken == "" ||
		!validCallbackText(out.AccessToken, 16*1024) || !validCallbackText(out.RefreshToken, 16*1024) || !validCallbackText(out.Scope, 4096) {
		return tokenResponse{}, fmt.Errorf("bff token: missing or invalid grant")
	}
	return out, nil
}
