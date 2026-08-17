package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/identity/internal/domain"
)

const (
	jwksMaxBytes     = 64 * 1024
	metadataMaxBytes = 32 * 1024
	metadataCache    = "public, max-age=300"
	jwksPath         = "/.well-known/jwks.json"
	metadataRootPath = "/.well-known/oauth-authorization-server"
)

type cachedDocument struct {
	status      int
	contentType string
	etag        string
	body        []byte
}

func (s *Server) registerMetadataRoutes(api huma.API) {
	if !s.exposeWellKnownInventory() {
		return
	}
	s.registerCachedDocument(api, huma.Operation{
		OperationID:   "jwks",
		Method:        http.MethodGet,
		Path:          jwksPath,
		Summary:       "JSON Web Key Set",
		Tags:          []string{"OAuth"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusServiceUnavailable},
	}, s.jwksDocument)
	s.registerCachedDocument(api, huma.Operation{
		OperationID:   "oauthAuthorizationServer",
		Method:        http.MethodGet,
		Path:          s.metadataPath(),
		Summary:       "OAuth authorization server metadata",
		Tags:          []string{"OAuth"},
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusServiceUnavailable},
	}, s.metadataDocument)
}

func (s *Server) exposeWellKnownInventory() bool {
	return s.jwks != nil || s.registerMetadata || s.registerBrokerInventory || s.broker != nil
}

func (s *Server) serveWellKnownAtRuntime() bool {
	return s.jwks != nil
}

func (s *Server) registerCachedDocument(api huma.API, op huma.Operation, load func(context.Context) (cachedDocument, error)) {
	huma.Register(api, op, func(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
		if !s.serveWellKnownAtRuntime() {
			return nil, genericUnavailable()
		}
		doc, err := load(ctx)
		if err != nil {
			return nil, genericUnavailable()
		}
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			writeCachedDocument(hctx, doc, hctx.Header("If-None-Match"))
		}}, nil
	})
}

func (s *Server) metadataPath() string {
	if suffix := issuerWellKnownSuffix(s.issuer); suffix != "" {
		return metadataRootPath + suffix
	}
	return metadataRootPath
}

func (s *Server) jwksDocument(ctx context.Context) (cachedDocument, error) {
	if s.jwks == nil {
		return cachedDocument{}, genericUnavailable()
	}
	pubs, err := s.jwks.PublicJWKS(ctx)
	if err != nil {
		return cachedDocument{}, genericUnavailable()
	}
	etag, err := s.jwks.PublicSetETag(ctx)
	if err != nil {
		return cachedDocument{}, genericUnavailable()
	}
	body, err := encodeJWKS(pubs)
	if err != nil {
		return cachedDocument{}, genericUnavailable()
	}
	return cachedDocument{
		status:      http.StatusOK,
		contentType: "application/jwk-set+json",
		etag:        etag,
		body:        body,
	}, nil
}

func (s *Server) metadataDocument(context.Context) (cachedDocument, error) {
	meta, err := authorizationServerDocument(s.issuer)
	if err != nil {
		return cachedDocument{}, genericUnavailable()
	}
	body, err := json.Marshal(meta)
	if err != nil || len(body) == 0 || len(body) > metadataMaxBytes {
		return cachedDocument{}, genericUnavailable()
	}
	return cachedDocument{
		status:      http.StatusOK,
		contentType: "application/json; charset=utf-8",
		etag:        metadataETag(body),
		body:        body,
	}, nil
}

type authorizationServerMetadata struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	JWKSURI                                    string   `json:"jwks_uri"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	ResponseModesSupported                     []string `json:"response_modes_supported"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	TokenEndpointAuthSigningAlgValuesSupported []string `json:"token_endpoint_auth_signing_alg_values_supported"`
	ScopesSupported                            []string `json:"scopes_supported"`
	AuthorizationResponseISSParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
}

func authorizationServerDocument(issuer string) (authorizationServerMetadata, error) {
	base, err := issuerBase(issuer)
	if err != nil {
		return authorizationServerMetadata{}, err
	}
	return authorizationServerMetadata{
		Issuer:                                     strings.TrimSpace(issuer),
		AuthorizationEndpoint:                      base + "/oauth/authorize",
		TokenEndpoint:                              base + "/oauth/token",
		JWKSURI:                                    base + jwksPath,
		ResponseTypesSupported:                     []string{"code"},
		ResponseModesSupported:                     []string{"query"},
		GrantTypesSupported:                        []string{"authorization_code"},
		CodeChallengeMethodsSupported:              []string{"S256"},
		TokenEndpointAuthMethodsSupported:          []string{"none", "client_secret_basic", "private_key_jwt"},
		TokenEndpointAuthSigningAlgValuesSupported: []string{"ES256"},
		ScopesSupported:                            []string{"openid", "studio.publish", "studio.read"},
		AuthorizationResponseISSParameterSupported: true,
	}, nil
}

func issuerBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !u.IsAbs() || u.Host == "" {
		return "", genericUnavailable()
	}
	base := u.Scheme + "://" + u.Host
	if path := strings.TrimSuffix(u.Path, "/"); path != "" && path != "/" {
		base += path
	}
	return base, nil
}

func issuerWellKnownSuffix(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Path == "" || u.Path == "/" {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/")
}

func encodeJWKS(pubs []domain.PublicJWK) ([]byte, error) {
	sorted := append([]domain.PublicJWK(nil), pubs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Kid < sorted[j].Kid })
	var buf bytes.Buffer
	buf.WriteString(`{"keys":[`)
	for i, pub := range sorted {
		raw, err := pub.MarshalJSON()
		if err != nil {
			return nil, err
		}
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(raw)
	}
	buf.WriteString(`]}`)
	if buf.Len() == 0 || buf.Len() > jwksMaxBytes {
		return nil, genericUnavailable()
	}
	return buf.Bytes(), nil
}

func metadataETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + base64.RawURLEncoding.EncodeToString(sum[:]) + `"`
}

func writeCachedDocument(hctx huma.Context, doc cachedDocument, match string) {
	hctx.SetHeader("Content-Type", doc.contentType)
	hctx.SetHeader("Cache-Control", metadataCache)
	if doc.etag != "" {
		hctx.SetHeader("ETag", doc.etag)
	}
	if match != "" && match == doc.etag {
		hctx.SetStatus(http.StatusNotModified)
		return
	}
	hctx.SetStatus(doc.status)
	_, _ = hctx.BodyWriter().Write(doc.body)
}

func (s *Server) wellKnownRuntimePolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.serveWellKnownAtRuntime() || !s.isWellKnownPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func (s *Server) isWellKnownPath(path string) bool {
	return path == jwksPath || path == s.metadataPath()
}

func genericUnavailable() error {
	return huma.Error503ServiceUnavailable("unavailable")
}
