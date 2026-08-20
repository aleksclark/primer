package repo

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/securityreview"
	"primer-tasks/internal/verification"
)

type VerifierCatalog struct {
	ID                           uuid.UUID
	Name, EndpointURL            string
	Active                       bool
	SchemaVersions, Capabilities []string
	SecretRef, SecretVersion     string
	Timeout                      time.Duration
	MaxAttempts                  int
	MaxAge                       time.Duration
	EgressPolicy                 map[string]any
	CreatedAt, UpdatedAt         time.Time
}

type VerifierCatalogRepository struct{ DB *pgxpool.Pool }

func NewVerifierCatalogRepository(db *pgxpool.Pool) *VerifierCatalogRepository {
	return &VerifierCatalogRepository{DB: db}
}

func (r *VerifierCatalogRepository) Create(ctx context.Context, v VerifierCatalog) error {
	if r == nil || r.DB == nil {
		return errors.New("verifier catalog database is required")
	}
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	if strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.SecretRef) == "" || strings.TrimSpace(v.SecretVersion) == "" {
		return errors.New("verifier catalog fields are required")
	}
	if err := ValidateCatalogEndpoint(ctx, v.EndpointURL, v.EgressPolicy); err != nil {
		return err
	}
	if v.Timeout <= 0 {
		v.Timeout = 30 * time.Second
	}
	if v.MaxAttempts <= 0 {
		v.MaxAttempts = 5
	}
	if v.MaxAge <= 0 {
		v.MaxAge = 24 * time.Hour
	}
	if v.EgressPolicy == nil {
		v.EgressPolicy = map[string]any{}
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO external_verifier_catalog(id,name,endpoint_url,active,schema_versions,capabilities,secret_ref,secret_version,timeout_seconds,max_attempts,max_age_seconds,egress_policy) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, v.ID, strings.TrimSpace(v.Name), v.EndpointURL, v.Active, jsonValue(v.SchemaVersions), jsonValue(v.Capabilities), v.SecretRef, v.SecretVersion, int(v.Timeout/time.Second), v.MaxAttempts, int(v.MaxAge/time.Second), jsonValue(v.EgressPolicy))
	if err != nil {
		return err
	}
	_, err = r.DB.Exec(ctx, `INSERT INTO external_verifier_secret_versions(verifier_id,secret_ref,secret_version) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, v.ID, v.SecretRef, v.SecretVersion)
	return err
}
func (r *VerifierCatalogRepository) Get(ctx context.Context, id uuid.UUID) (VerifierCatalog, error) {
	var v VerifierCatalog
	var schemas, caps, policy []byte
	var timeout, age int
	if r == nil || r.DB == nil {
		return v, errors.New("verifier catalog database is required")
	}
	err := r.DB.QueryRow(ctx, `SELECT id,name,endpoint_url,active,schema_versions,capabilities,secret_ref,secret_version,timeout_seconds,max_attempts,max_age_seconds,egress_policy,created_at,updated_at FROM external_verifier_catalog WHERE id=$1`, id).Scan(&v.ID, &v.Name, &v.EndpointURL, &v.Active, &schemas, &caps, &v.SecretRef, &v.SecretVersion, &timeout, &v.MaxAttempts, &age, &policy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	_ = json.Unmarshal(schemas, &v.SchemaVersions)
	_ = json.Unmarshal(caps, &v.Capabilities)
	_ = json.Unmarshal(policy, &v.EgressPolicy)
	v.Timeout, v.MaxAge = time.Duration(timeout)*time.Second, time.Duration(age)*time.Second
	return v, nil
}
func (r *VerifierCatalogRepository) List(ctx context.Context, activeOnly bool) ([]VerifierCatalog, error) {
	rows, err := r.DB.Query(ctx, `SELECT id FROM external_verifier_catalog WHERE ($1=false OR active) ORDER BY name,id`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VerifierCatalog, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		v, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *VerifierCatalogRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	_, err := r.DB.Exec(ctx, `UPDATE external_verifier_catalog SET active=$2,updated_at=now() WHERE id=$1`, id, active)
	return err
}
func (r *VerifierCatalogRepository) RotateSecret(ctx context.Context, id uuid.UUID, secretRef, secretVersion string) error {
	if strings.TrimSpace(secretRef) == "" || strings.TrimSpace(secretVersion) == "" {
		return errors.New("secret reference and version are required")
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE external_verifier_catalog SET secret_ref=$2,secret_version=$3,updated_at=now() WHERE id=$1`, id, strings.TrimSpace(secretRef), strings.TrimSpace(secretVersion)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE external_verifier_secret_versions SET retired_at=COALESCE(retired_at,now()) WHERE verifier_id=$1 AND retired_at IS NULL`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO external_verifier_secret_versions(verifier_id,secret_ref,secret_version) VALUES($1,$2,$3) ON CONFLICT (verifier_id,secret_version) DO UPDATE SET secret_ref=EXCLUDED.secret_ref,retired_at=NULL`, id, strings.TrimSpace(secretRef), strings.TrimSpace(secretVersion)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ExternalDelivery struct {
	ID, TenantID, AttemptID, RequirementID, VerifierID, RequestID, IdempotencyKey string
	SchemaVersion, CallbackPath, PayloadDigest                                    string
	Envelope                                                                      []byte
	Status                                                                        string
	Attempts, MaxAttempts                                                         int
	AvailableAt, ExpiresAt                                                        time.Time
	LeaseOwner                                                                    string
	LeaseUntil                                                                    *time.Time
	LastErrorCode                                                                 string
	CreatedAt, UpdatedAt                                                          time.Time
}
type ExternalBinding = verification.ExternalBinding
type ExternalRepository struct{ DB *pgxpool.Pool }

func NewExternalRepository(db *pgxpool.Pool) *ExternalRepository { return &ExternalRepository{DB: db} }

func (r *ExternalRepository) Enqueue(ctx context.Context, d ExternalDelivery) error {
	if r == nil || r.DB == nil {
		return errors.New("external repository database is required")
	}
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.RequestID == "" {
		d.RequestID = uuid.NewString()
	}
	if d.TenantID == "" || d.AttemptID == "" || d.RequirementID == "" || d.VerifierID == "" || d.IdempotencyKey == "" || len(d.Envelope) == 0 || d.MaxAttempts < 1 || d.ExpiresAt.IsZero() {
		return errors.New("invalid external delivery")
	}
	if _, err := verification.DecodeRequest(d.Envelope); err != nil {
		return err
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO external_verifier_outbox(id,tenant_id,attempt_id,requirement_id,verifier_id,request_id,idempotency_key,schema_version,callback_path,envelope,payload_digest,max_attempts,expires_at,available_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,COALESCE($14,now())) ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`, d.ID, d.TenantID, d.AttemptID, d.RequirementID, d.VerifierID, d.RequestID, d.IdempotencyKey, d.SchemaVersion, d.CallbackPath, d.Envelope, d.PayloadDigest, d.MaxAttempts, d.ExpiresAt, d.AvailableAt)
	return err
}
func (r *ExternalRepository) Claim(ctx context.Context, owner string, lease time.Duration) (ExternalDelivery, bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return ExternalDelivery{}, false, err
	}
	defer tx.Rollback(ctx)
	var d ExternalDelivery
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT id FROM external_verifier_outbox WHERE status IN ('queued','waiting','retryable_error') AND available_at<=now() AND expires_at>now() AND (lease_until IS NULL OR lease_until<now()) AND attempts<max_attempts ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE external_verifier_outbox o SET status='running',attempts=attempts+1,lease_owner=$1,lease_until=now()+$2::interval,updated_at=now() FROM candidate WHERE o.id=candidate.id RETURNING o.id,o.tenant_id,o.attempt_id,o.requirement_id,o.verifier_id,o.request_id,o.idempotency_key,o.schema_version,o.callback_path,o.envelope,o.payload_digest,o.status,o.attempts,o.max_attempts,o.available_at,o.expires_at,o.lease_owner,o.lease_until,o.last_error_code,o.created_at,o.updated_at`, owner, leaseInterval(lease)).Scan(&d.ID, &d.TenantID, &d.AttemptID, &d.RequirementID, &d.VerifierID, &d.RequestID, &d.IdempotencyKey, &d.SchemaVersion, &d.CallbackPath, &d.Envelope, &d.PayloadDigest, &d.Status, &d.Attempts, &d.MaxAttempts, &d.AvailableAt, &d.ExpiresAt, &d.LeaseOwner, &d.LeaseUntil, &d.LastErrorCode, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return d, false, err
	}
	return d, true, nil
}
func (r *ExternalRepository) Renew(ctx context.Context, id, owner string, lease time.Duration) error {
	var ok bool
	err := r.DB.QueryRow(ctx, `UPDATE external_verifier_outbox SET lease_until=now()+$3::interval,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2 RETURNING true`, id, owner, leaseInterval(lease)).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("external delivery lease is no longer owned")
	}
	return nil
}
func (r *ExternalRepository) Finish(ctx context.Context, d ExternalDelivery, owner, status, code string, retryAt time.Time) error {
	valid := map[string]bool{"queued": true, "waiting": true, "dead": true, "accepted": true, "rejected": true, "retryable_error": true, "terminal_error": true, "canceled": true}
	if !valid[status] {
		return errors.New("invalid external delivery status")
	}
	var err error
	if retryAt.IsZero() {
		_, err = r.DB.Exec(ctx, `UPDATE external_verifier_outbox SET status=$4,lease_owner=NULL,lease_until=NULL,last_error_code=$5,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2 AND tenant_id=$3`, d.ID, owner, d.TenantID, status, code)
	} else {
		_, err = r.DB.Exec(ctx, `UPDATE external_verifier_outbox SET status=$4,available_at=$5,lease_owner=NULL,lease_until=NULL,last_error_code=$6,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2 AND tenant_id=$3`, d.ID, owner, d.TenantID, status, retryAt, code)
	}
	return err
}
func (r *ExternalRepository) RequeueExpired(ctx context.Context, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE external_verifier_outbox SET status=CASE WHEN expires_at<=$1 OR attempts>=max_attempts THEN 'dead' ELSE 'queued' END,lease_owner=NULL,lease_until=NULL,available_at=CASE WHEN expires_at<=$1 THEN available_at ELSE $1 END,updated_at=$1 WHERE status='running' AND lease_until<$1`, now)
	return err
}

func (r *ExternalRepository) Binding(ctx context.Context, requestID, verifierID, attemptID string) (ExternalBinding, error) {
	var b ExternalBinding
	err := r.DB.QueryRow(ctx, `SELECT o.tenant_id,o.attempt_id,o.requirement_id,o.verifier_id,o.request_id,o.payload_digest,o.callback_path,o.schema_version,a.occurrence_id FROM external_verifier_outbox o JOIN verification_attempts a ON a.tenant_id=o.tenant_id AND a.id=o.attempt_id WHERE o.request_id=$1 AND o.verifier_id=$2 AND o.attempt_id=$3`, requestID, verifierID, attemptID).Scan(&b.TenantID, &b.AttemptID, &b.RequirementID, &b.VerifierID, &b.RequestID, &b.PayloadDigest, &b.CallbackPath, &b.SchemaVersion, &b.OccurrenceID)
	return b, err
}
func (r *ExternalRepository) RecordCallback(ctx context.Context, b ExternalBinding, c verification.CallbackEnvelope, body []byte) (bool, error) {
	if c.RequestDigest != b.PayloadDigest {
		return false, verification.ErrExternalBinding
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var existing string
	err = tx.QueryRow(ctx, `INSERT INTO external_verifier_callbacks(tenant_id,callback_id,request_id,attempt_id,verifier_id,sequence,result_type,request_digest,body_digest,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(tenant_id,request_id,sequence) DO NOTHING RETURNING callback_id`, b.TenantID, c.CallbackID, b.RequestID, b.AttemptID, b.VerifierID, c.Sequence, c.Type, c.RequestDigest, verification.ExternalPayloadDigest(body), body).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT callback_id FROM external_verifier_callbacks WHERE tenant_id=$1 AND request_id=$2 AND sequence=$3`, b.TenantID, b.RequestID, c.Sequence).Scan(&existing); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_verifier_events(tenant_id,request_id,sequence,kind,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, b.TenantID, b.RequestID, c.Sequence, "external."+c.Type, body)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE external_verifier_outbox SET status=CASE WHEN $3='progress' THEN 'waiting' WHEN $3='retryable_error' THEN 'retryable_error' WHEN $3='terminal_error' THEN 'terminal_error' ELSE 'waiting' END,lease_owner=NULL,lease_until=NULL,available_at=CASE WHEN $3='retryable_error' THEN now()+interval '1 minute' ELSE available_at END,updated_at=now() WHERE tenant_id=$1 AND request_id=$2 AND status NOT IN ('accepted','rejected','dead','canceled')`, b.TenantID, b.RequestID, c.Type)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}
func (r *ExternalRepository) SecurityFailure(ctx context.Context, tenant, verifier, request, class string) {
	_, _ = r.DB.Exec(ctx, `INSERT INTO external_verifier_security_events(tenant_id,verifier_id,request_id,failure_class) VALUES(NULLIF($1,'')::uuid,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4)`, tenant, verifier, request, class)
}
func jsonValue(v any) []byte { b, _ := json.Marshal(v); return b }
func VerifierEgressTarget(ctx context.Context, raw string, policy map[string]any) (securityreview.EgressTarget, error) {
	var allowlist []string
	if values, ok := policy["allowlist"].([]any); ok {
		for _, value := range values {
			if entry, ok := value.(string); ok {
				allowlist = append(allowlist, entry)
			}
		}
	}
	if len(allowlist) == 0 {
		return securityreview.EgressTarget{}, errors.New("verifier endpoint allowlist is required")
	}
	return securityreview.ValidateVerifierEndpoint(ctx, raw, allowlist, nil)
}

func ValidateCatalogEndpoint(ctx context.Context, raw string, policy map[string]any) error {
	if os.Getenv("TASKS_ENV") != "production" && policy != nil && policy["testFixture"] == true {
		if err := verification.ValidateEndpoint(raw); err == nil {
			return nil
		}
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("test fixture endpoint is invalid")
		}
		host := strings.ToLower(u.Hostname())
		if host != "external-verifier-fixture" && host != "fixture" {
			return errors.New("test fixture endpoint host is not explicit")
		}
		return nil
	}
	if _, err := VerifierEgressTarget(ctx, raw, policy); err != nil {
		return err
	}
	return nil
}

func leaseInterval(d time.Duration) string {
	if d <= 0 {
		d = time.Minute
	}
	return d.String()
}
