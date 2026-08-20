package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
	"primer-tasks/internal/verification"
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
func (s *Server) listExternalVerifiers(w http.ResponseWriter, r *http.Request, sc scope) {
	if sc.Role != "admin" && sc.Role != "educator" {
		problem(w, http.StatusForbidden, "forbidden", "administrator role required")
		return
	}
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
func (s *Server) createExternalVerifier(w http.ResponseWriter, r *http.Request, sc scope) {
	if sc.Role != "admin" && sc.Role != "educator" {
		problem(w, http.StatusForbidden, "forbidden", "administrator role required")
		return
	}
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
func (s *Server) setExternalVerifierActive(w http.ResponseWriter, r *http.Request, sc scope) {
	if sc.Role != "admin" && sc.Role != "educator" {
		problem(w, http.StatusForbidden, "forbidden", "administrator role required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		problem(w, 404, "not_found", "verifier not found")
		return
	}
	var in struct {
		Active        bool   `json:"active"`
		SecretRef     string `json:"secretRef"`
		SecretVersion string `json:"secretVersion"`
	}
	if !decode(w, r, &in) {
		return
	}
	catalog := repo.NewVerifierCatalogRepository(s.DB)
	if err = catalog.SetActive(r.Context(), id, in.Active); err != nil {
		problem(w, 404, "not_found", "verifier not found")
		return
	}
	if strings.TrimSpace(in.SecretRef) != "" || strings.TrimSpace(in.SecretVersion) != "" {
		if err = catalog.RotateSecret(r.Context(), id, in.SecretRef, in.SecretVersion); err != nil {
			problem(w, 400, "invalid_request", "secret reference rotation is invalid")
			return
		}
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
	if callback, decodeErr := verification.DecodeCallback(body); decodeErr == nil && r.Header.Get("X-Primer-Request-ID") != callback.CallbackID {
		repo.NewExternalRepository(s.DB).SecurityFailure(r.Context(), "", chi.URLParam(r, "id"), callback.RequestID, "callback_request_binding_invalid")
		http.Error(w, "callback rejected", 401)
		return
	}
	ok, err := p.Process(r.Context(), r.Method, path, r.Header.Get("X-Primer-Key-ID"), r.Header.Get("X-Primer-Timestamp"), r.Header.Get("X-Primer-Signature"), body)
	if err != nil {
		class := "callback_rejected"
		if errors.Is(err, verification.ErrExternalInvalidSignature) {
			class = "callback_signature_invalid"
		}
		if errors.Is(err, verification.ErrExternalBinding) {
			class = "callback_binding_invalid"
		}
		var requestID string
		if callback, decodeErr := verification.DecodeCallback(body); decodeErr == nil {
			requestID = callback.RequestID
		}
		repo.NewExternalRepository(s.DB).SecurityFailure(r.Context(), "", chi.URLParam(r, "id"), requestID, class)
		http.Error(w, "callback rejected", 401)
		return
	}
	if callback, decodeErr := verification.DecodeCallback(body); decodeErr == nil {
		s.publishExternalCallback(r.Context(), callback)
	}
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
}

type externalSubmitInput struct {
	IdempotencyKey string         `json:"idempotencyKey"`
	PublicPayload  map[string]any `json:"publicPayload"`
}
type externalStateOutput struct {
	OccurrenceID  string           `json:"occurrenceId"`
	AttemptID     string           `json:"attemptId"`
	VerifierID    string           `json:"verifierId"`
	Capability    string           `json:"capability"`
	SchemaVersion string           `json:"schemaVersion"`
	Source        string           `json:"source"`
	Status        string           `json:"status"`
	Progress      []map[string]any `json:"progress"`
	CanRetry      bool             `json:"canRetry"`
	CanCancel     bool             `json:"canCancel"`
	Fallback      bool             `json:"fallbackAvailable"`
	SafeRationale string           `json:"safeRationale,omitempty"`
}

func (s *Server) validateExternalConfig(ctx context.Context, raw map[string]any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	config, err := externalConfig(encoded)
	if err != nil {
		return err
	}
	var active bool
	var versions, capabilities []byte
	if err = s.DB.QueryRow(ctx, `SELECT active,schema_versions,capabilities FROM external_verifier_catalog WHERE id=$1`, config.VerifierID).Scan(&active, &versions, &capabilities); err != nil {
		return err
	}
	if !active {
		return errors.New("verifier is disabled")
	}
	var supportedVersions, supportedCapabilities []string
	if err = json.Unmarshal(versions, &supportedVersions); err != nil {
		return err
	}
	if err = json.Unmarshal(capabilities, &supportedCapabilities); err != nil {
		return err
	}
	if !containsString(supportedVersions, config.Schema) || !containsString(supportedCapabilities, config.Capability) {
		return errors.New("verifier capability or schema is unsupported")
	}
	return nil
}

func externalConfig(raw []byte) (verification.ExternalConfig, error) {
	var config verification.ExternalConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, err
	}
	return config, config.Validate()
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func snapshotExternalAttempt(ctx context.Context, tx pgx.Tx, tenant string, attempt string, requirement string, config verification.ExternalConfig) error {
	var verifierID, secretRef, secretVersion, name string
	var active bool
	var versions, capabilities []byte
	if err := tx.QueryRow(ctx, `SELECT id,name,active,schema_versions,capabilities,secret_ref,secret_version FROM external_verifier_catalog WHERE id=$1`, config.VerifierID).Scan(&verifierID, &name, &active, &versions, &capabilities, &secretRef, &secretVersion); err != nil {
		return err
	}
	if !active {
		return errors.New("verifier is disabled")
	}
	var supportedVersions, supportedCapabilities []string
	if err := json.Unmarshal(versions, &supportedVersions); err != nil {
		return err
	}
	if err := json.Unmarshal(capabilities, &supportedCapabilities); err != nil {
		return err
	}
	if !containsString(supportedVersions, config.Schema) || !containsString(supportedCapabilities, config.Capability) {
		return errors.New("verifier capability or schema is unsupported")
	}
	options := config.Options
	if options == nil {
		options = map[string]any{}
	}
	manifest, _ := json.Marshal(map[string]any{"name": name, "schemaVersions": supportedVersions, "capabilities": supportedCapabilities, "secretVersion": secretVersion})
	_, err := tx.Exec(ctx, `INSERT INTO external_verifier_attempts(tenant_id,attempt_id,verifier_id,capability,schema_version,public_options,manifest_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7)`, tenant, attempt, verifierID, config.Capability, config.Schema, requirementConfigJSON(options), manifest)
	return err
}

func (s *Server) submitExternal(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	occurrence := chi.URLParam(r, "id")
	var input externalSubmitInput
	if !decode(w, r, &input) || strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 200 {
		problem(w, 400, "invalid_request", "an idempotency key and public payload are required")
		return
	}
	payload, err := json.Marshal(map[string]any{"submission": input.PublicPayload})
	if err != nil || len(payload) > 1<<20 {
		problem(w, 400, "invalid_request", "public payload is too large")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 500, "internal", "unable to begin external submission")
		return
	}
	defer tx.Rollback(ctx)
	var tenant, attempt, requirement, verifier, capability, schema string
	var options []byte
	err = tx.QueryRow(ctx, `SELECT o.tenant_id,a.id,a.requirement_id,e.verifier_id,e.capability,e.schema_version,e.public_options FROM task_occurrences o JOIN verification_attempts a ON a.tenant_id=o.tenant_id AND a.occurrence_id=o.id JOIN external_verifier_attempts e ON e.tenant_id=a.tenant_id AND e.attempt_id=a.id WHERE o.id=$1 AND o.student_id=$2 AND a.status='open' ORDER BY a.number DESC LIMIT 1 FOR UPDATE`, occurrence, student).Scan(&tenant, &attempt, &requirement, &verifier, &capability, &schema, &options)
	if err != nil {
		problem(w, 404, "not_found", "external verification attempt not found")
		return
	}
	callbackPath := "/external/verifiers/" + verifier + "/callback"
	envelope, err := verification.NewRequestEnvelope(uuid.NewString(), attempt, requirement, schema, input.IdempotencyKey, callbackPath, map[string]any{"capability": capability, "options": json.RawMessage(options), "payload": json.RawMessage(payload)}, time.Now().UTC(), 24*time.Hour, []string{"attempt", "requirement"})
	if err != nil {
		problem(w, 400, "invalid_request", "external submission is invalid")
		return
	}
	body, _ := verification.MarshalRequestEnvelope(envelope)
	deliveryID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO external_verifier_outbox(id,tenant_id,attempt_id,requirement_id,verifier_id,request_id,idempotency_key,schema_version,callback_path,envelope,payload_digest,max_attempts,expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,c.max_attempts,$12 FROM external_verifier_catalog c WHERE c.id=$5 ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`, deliveryID, tenant, attempt, requirement, verifier, envelope.RequestID, input.IdempotencyKey, schema, callbackPath, body, envelope.PayloadDigest, envelope.ExpiresAt)
	if err != nil {
		problem(w, 409, "conflict", "external submission could not be queued")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_submissions(id,tenant_id,attempt_id,submitted_by,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, uuid.New(), tenant, attempt, student.String(), input.PublicPayload)
	if err != nil {
		problem(w, 500, "internal", "external submission could not be recorded")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_verifier_events(tenant_id,request_id,sequence,kind,payload) VALUES($1,$2,0,'verification.requested',$3) ON CONFLICT DO NOTHING`, tenant, envelope.RequestID, json.RawMessage(`{"schemaVersion":1,"status":"queued"}`))
	if err != nil {
		problem(w, 500, "internal", "external submission event could not be recorded")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 500, "internal", "external submission could not be committed")
		return
	}
	jsonStatus(w, map[string]any{"occurrenceId": occurrence, "attemptId": attempt, "requestId": envelope.RequestID, "status": "queued"}, http.StatusAccepted)
}

func (s *Server) externalState(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	state, err := s.loadExternalState(r.Context(), chi.URLParam(r, "id"), student.String(), false)
	if err != nil {
		problem(w, 404, "not_found", "external verification state not found")
		return
	}
	jsonOK(w, state)
}
func (s *Server) inspectExternal(w http.ResponseWriter, r *http.Request, sc scope) {
	state, err := s.loadExternalState(r.Context(), chi.URLParam(r, "id"), sc.Tenant, true)
	if err != nil {
		problem(w, 404, "not_found", "external verification state not found")
		return
	}
	jsonOK(w, state)
}
func (s *Server) loadExternalState(ctx context.Context, occurrence, subject string, parent bool) (externalStateOutput, error) {
	var out externalStateOutput
	var query string
	var args []any
	if parent {
		query = `SELECT o.id::text,a.id::text,e.verifier_id::text,e.capability,e.schema_version,c.name,o.status,COALESCE(d.status,'') FROM task_occurrences o JOIN verification_attempts a ON a.tenant_id=o.tenant_id AND a.occurrence_id=o.id JOIN external_verifier_attempts e ON e.tenant_id=a.tenant_id AND e.attempt_id=a.id JOIN external_verifier_catalog c ON c.id=e.verifier_id LEFT JOIN external_verifier_outbox d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id WHERE o.id=$1 ORDER BY a.number DESC LIMIT 1`
		args = []any{occurrence}
	} else {
		query = `SELECT o.id::text,a.id::text,e.verifier_id::text,e.capability,e.schema_version,c.name,o.status,COALESCE(d.status,'') FROM task_occurrences o JOIN verification_attempts a ON a.tenant_id=o.tenant_id AND a.occurrence_id=o.id JOIN external_verifier_attempts e ON e.tenant_id=a.tenant_id AND e.attempt_id=a.id JOIN external_verifier_catalog c ON c.id=e.verifier_id LEFT JOIN external_verifier_outbox d ON d.tenant_id=a.tenant_id AND d.attempt_id=a.id WHERE o.id=$1 AND o.student_id=$2 ORDER BY a.number DESC LIMIT 1`
		args = []any{occurrence, subject}
	}
	var verifierName, occurrenceStatus, deliveryStatus string
	if err := s.DB.QueryRow(ctx, query, args...).Scan(&out.OccurrenceID, &out.AttemptID, &out.VerifierID, &out.Capability, &out.SchemaVersion, &verifierName, &occurrenceStatus, &deliveryStatus); err != nil {
		return out, err
	}
	out.Source = verifierName
	out.Status = occurrenceStatus
	if deliveryStatus != "" && occurrenceStatus != "completed" && occurrenceStatus != "canceled" {
		out.Status = deliveryStatus
	}
	out.CanCancel = out.Status != "completed" && out.Status != "canceled"
	out.Fallback = parent
	rows, err := s.DB.Query(ctx, `SELECT payload FROM external_verifier_events e JOIN verification_attempts a ON a.tenant_id=e.tenant_id AND a.id=$2 JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=$1 WHERE e.request_id IN (SELECT request_id FROM external_verifier_outbox WHERE tenant_id=a.tenant_id AND attempt_id=a.id) ORDER BY e.sequence`, occurrence, out.AttemptID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) == nil {
				var item map[string]any
				if json.Unmarshal(raw, &item) == nil {
					out.Progress = append(out.Progress, item)
				}
			}
		}
	}
	out.CanRetry = out.Status == "dead" || out.Status == "retryable_error" || out.Status == "terminal_error"
	return out, nil
}

func (s *Server) cancelExternal(w http.ResponseWriter, r *http.Request, sc scope) {
	n, err := s.DB.Exec(r.Context(), `UPDATE external_verifier_outbox d SET status='canceled',lease_owner=NULL,lease_until=NULL,updated_at=now() FROM verification_attempts a WHERE d.tenant_id=$1 AND d.attempt_id=a.id AND a.occurrence_id=$2 AND d.status NOT IN ('accepted','rejected','canceled')`, sc.Tenant, chi.URLParam(r, "id"))
	if err != nil || n.RowsAffected() == 0 {
		problem(w, 404, "not_found", "external delivery not found")
		return
	}
	jsonOK(w, map[string]string{"status": "canceled"})
}
func (s *Server) retryExternal(w http.ResponseWriter, r *http.Request, sc scope) {
	n, err := s.DB.Exec(r.Context(), `UPDATE external_verifier_outbox d SET status='queued',available_at=now(),lease_owner=NULL,lease_until=NULL,last_error_code='',updated_at=now() FROM verification_attempts a WHERE d.tenant_id=$1 AND d.attempt_id=a.id AND a.occurrence_id=$2 AND d.status IN ('dead','retryable_error','terminal_error')`, sc.Tenant, chi.URLParam(r, "id"))
	if err != nil || n.RowsAffected() == 0 {
		problem(w, 409, "conflict", "external delivery is not retryable")
		return
	}
	jsonOK(w, map[string]string{"status": "queued"})
}
func (s *Server) fallbackExternal(w http.ResponseWriter, r *http.Request, sc scope) {
	var in DecisionInput2
	if !decode(w, r, &in) {
		return
	}
	var attempt, occ string
	if err := s.DB.QueryRow(r.Context(), `SELECT a.id,a.occurrence_id FROM verification_attempts a JOIN external_verifier_attempts e ON e.tenant_id=a.tenant_id AND e.attempt_id=a.id WHERE a.tenant_id=$1 AND a.occurrence_id=$2 AND a.status='open' ORDER BY a.number DESC LIMIT 1`, sc.Tenant, chi.URLParam(r, "id")).Scan(&attempt, &occ); err != nil {
		problem(w, 409, "conflict", "external fallback unavailable")
		return
	}
	inserted, err := s.CommitDecision(r.Context(), verification.Decision{ID: uuid.NewString(), TenantID: sc.Tenant, AttemptID: attempt, OccurrenceID: occ, Accepted: in.Accepted, Reason: in.Reason, DecidedBy: "human_fallback"})
	if err != nil {
		problem(w, 409, "conflict", "external fallback could not be recorded")
		return
	}
	jsonOK(w, map[string]any{"accepted": in.Accepted, "inserted": inserted, "status": "decided"})
}

func (s *Server) publishExternalCallback(ctx context.Context, callback verification.CallbackEnvelope) {
	var tenant, occurrence, student string
	if err := s.DB.QueryRow(ctx, `SELECT a.tenant_id::text,a.occurrence_id::text,o.student_id::text FROM verification_attempts a JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=a.occurrence_id WHERE a.id=$1`, callback.AttemptRef).Scan(&tenant, &occurrence, &student); err != nil {
		return
	}
	status := callback.Type
	message := "External verification updated"
	if callback.Progress != nil {
		message = callback.Progress.Message
	}
	if callback.Accepted != nil {
		message = callback.Accepted.Rationale
	}
	if callback.Rejected != nil {
		message = callback.Rejected.Rationale
	}
	if len(message) > 300 {
		message = message[:300]
	}
	s.studentDialogueHub().publish(wireStudentEvent{Type: "external_progress", ProtocolVersion: studentProtocolVersion, TenantID: tenant, StudentID: student, OccurrenceID: occurrence, AttemptID: callback.AttemptRef, Sequence: callback.Sequence, Cursor: callback.Sequence, Phase: callback.Type, Status: status, Message: message, Retryable: callback.Error != nil && callback.Error.Retryable, Time: time.Now().UTC()})
}

func (s *Server) registerExternalRoutes(r *chi.Mux) {
	// Callback delivery is intentionally kept outside the browser-generated
	// client surface. It is authenticated by the signed protocol, not a parent
	// session, and accepts no caller-selected verifier or tenant.
	r.Post("/external/verifiers/{id}/callback", http.HandlerFunc(s.externalCallback).ServeHTTP)
}
