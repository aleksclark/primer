package api

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
)

type externalVerifierInput struct {
	Name           string         `json:"name"`
	EndpointURL    string         `json:"endpointUrl"`
	Active         bool           `json:"active"`
	SchemaVersions []string       `json:"schemaVersions"`
	Capabilities   []string       `json:"capabilities"`
	SecretRef      string         `json:"secretRef"`
	SecretVersion  string         `json:"secretVersion"`
	TimeoutSeconds int            `json:"timeoutSeconds"`
	MaxAttempts    int            `json:"maxAttempts"`
	MaxAgeSeconds  int            `json:"maxAgeSeconds"`
	EgressPolicy   map[string]any `json:"egressPolicy"`
}
type externalVerifierOutput struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Active         bool      `json:"active"`
	SchemaVersions []string  `json:"schemaVersions"`
	Capabilities   []string  `json:"capabilities"`
	SecretVersion  string    `json:"secretVersion"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
}

func externalVerifierView(v repo.VerifierCatalog) externalVerifierOutput {
	return externalVerifierOutput{ID: v.ID, Name: v.Name, Active: v.Active, SchemaVersions: v.SchemaVersions, Capabilities: v.Capabilities, SecretVersion: v.SecretVersion, CreatedAt: v.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"), UpdatedAt: v.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00")}
}
func (s *Server) listExternalVerifiers(w http.ResponseWriter, r *http.Request, _ scope) {
	items, err := repo.NewVerifierCatalogRepository(s.DB).List(r.Context(), false)
	if err != nil {
		problem(w, 500, "internal", "unable to list verifier catalog")
		return
	}
	out := make([]externalVerifierOutput, 0, len(items))
	for _, v := range items {
		out = append(out, externalVerifierView(v))
	}
	jsonOK(w, map[string]any{"items": out})
}
func (s *Server) createExternalVerifier(w http.ResponseWriter, r *http.Request, _ scope) {
	var in externalVerifierInput
	if !decode(w, r, &in) {
		return
	}
	v := repo.VerifierCatalog{ID: uuid.New(), Name: strings.TrimSpace(in.Name), EndpointURL: strings.TrimSpace(in.EndpointURL), Active: in.Active, SchemaVersions: in.SchemaVersions, Capabilities: in.Capabilities, SecretRef: strings.TrimSpace(in.SecretRef), SecretVersion: strings.TrimSpace(in.SecretVersion), EgressPolicy: in.EgressPolicy}
	if in.TimeoutSeconds > 0 {
		v.Timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}
	if in.MaxAttempts > 0 {
		v.MaxAttempts = in.MaxAttempts
	}
	if in.MaxAgeSeconds > 0 {
		v.MaxAge = time.Duration(in.MaxAgeSeconds) * time.Second
	}
	if err := repo.NewVerifierCatalogRepository(s.DB).Create(r.Context(), v); err != nil {
		problem(w, 400, "invalid_request", "verifier catalog entry is invalid")
		return
	}
	v, err := repo.NewVerifierCatalogRepository(s.DB).Get(r.Context(), v.ID)
	if err != nil {
		problem(w, 500, "internal", "unable to load verifier catalog entry")
		return
	}
	jsonStatus(w, externalVerifierView(v), http.StatusCreated)
}
func (s *Server) setExternalVerifierActive(w http.ResponseWriter, r *http.Request, _ scope) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		problem(w, 404, "not_found", "verifier not found")
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err = repo.NewVerifierCatalogRepository(s.DB).SetActive(r.Context(), id, in.Active); err != nil {
		problem(w, 404, "not_found", "verifier not found")
		return
	}
	v, err := repo.NewVerifierCatalogRepository(s.DB).Get(r.Context(), id)
	if err != nil {
		problem(w, 404, "not_found", "verifier not found")
		return
	}
	jsonOK(w, externalVerifierView(v))
}
func (s *Server) externalCallback(w http.ResponseWriter, r *http.Request) {
	if s.ExternalSecrets == nil {
		http.Error(w, "callback unavailable", 503)
		return
	}
	p := &jobs.CallbackProcessor{Outbox: repo.NewExternalRepository(s.DB), Catalog: repo.NewVerifierCatalogRepository(s.DB), Secrets: s.ExternalSecrets, Committer: s, MaxSkew: 5 * time.Minute}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "callback rejected", 400)
		return
	}
	path := r.URL.EscapedPath()
	ok, err := p.Process(r.Context(), r.Method, path, r.Header.Get("X-Primer-Key-ID"), r.Header.Get("X-Primer-Timestamp"), r.Header.Get("X-Primer-Signature"), body)
	if err != nil {
		http.Error(w, "callback rejected", 401)
		return
	}
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
}
func (s *Server) registerExternalRoutes(r *chi.Mux) {
	r.Get("/admin/verifiers", s.requireParent(s.listExternalVerifiers))
	r.Post("/admin/verifiers", s.requireParent(s.createExternalVerifier))
	r.Patch("/admin/verifiers/{id}", s.requireParent(s.setExternalVerifierActive))
	r.Post("/external/verifiers/{id}/callback", http.HandlerFunc(s.externalCallback).ServeHTTP)
}
