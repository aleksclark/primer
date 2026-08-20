package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/artifact"
	"primer-tasks/internal/artifactstore"
)

// ArtifactReservationInput is the student reservation request. The occurrence
// is represented by the path parameter, not duplicated in this JSON body.
type ArtifactReservationInput struct {
	RequirementID  string `json:"requirementId,omitempty" format:"uuid"`
	Kind           string `json:"kind" enum:"image,video,audio"`
	Filename       string `json:"filename,omitempty"`
	ContentType    string `json:"contentType" minLength:"1"`
	Size           int64  `json:"size" minimum:"1"`
	PartCount      int    `json:"partCount,omitempty" minimum:"1" maximum:"10000"`
	IdempotencyKey string `json:"idempotencyKey" minLength:"1" maxLength:"128"`
}

// ArtifactFinalizeInput is the student finalize request. Size, media type,
// and filename are retained as declared client metadata and are validated by
// the artifact service; the stored object and digest remain authoritative.
type ArtifactFinalizeInput struct {
	ArtifactID     string `json:"artifactId" format:"uuid"`
	RequirementID  string `json:"requirementId,omitempty" format:"uuid"`
	SHA256         string `json:"sha256,omitempty"`
	Digest         string `json:"digest" minLength:"64" maxLength:"64"`
	DurationMS     int64  `json:"durationMs,omitempty" minimum:"0"`
	IdempotencyKey string `json:"idempotencyKey,omitempty" maxLength:"128"`
	SizeBytes      int64  `json:"sizeBytes,omitempty" minimum:"0"`
	MediaType      string `json:"mediaType,omitempty"`
	Filename       string `json:"filename,omitempty"`
}

type ArtifactReservationOutput struct {
	ReservationID  string    `json:"reservationId" format:"uuid"`
	IdempotencyKey string    `json:"idempotencyKey"`
	ArtifactID     string    `json:"artifactId" format:"uuid"`
	UploadURL      string    `json:"uploadUrl"`
	PartCount      int       `json:"partCount"`
	ExpiresAt      time.Time `json:"expiresAt" format:"date-time"`
}

type ArtifactOutput struct {
	ID            string `json:"id" format:"uuid"`
	Kind          string `json:"kind" enum:"image,video,audio"`
	ContentType   string `json:"contentType"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	DurationMS    int64  `json:"durationMs,omitempty"`
	Status        string `json:"status" enum:"reserved,uploaded,finalized,rejected"`
	SubmissionID  string `json:"submissionId,omitempty" format:"uuid"`
	DerivativeURL string `json:"derivativeUrl,omitempty"`
}

type ArtifactUploadOutput struct {
	Status    string    `json:"status" enum:"already_uploaded,uploaded"`
	Part      int       `json:"part,omitempty"`
	Size      int64     `json:"size"`
	ExpiresAt time.Time `json:"expiresAt" format:"date-time"`
}

type ArtifactRubricCriterionOutput struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type ArtifactRubricConfigOutput struct {
	AcceptedKinds      []string                        `json:"acceptedKinds" nullable:"false"`
	MaxBytes           int64                           `json:"maxBytes,omitempty"`
	MaxCount           int                             `json:"maxCount,omitempty"`
	MaxDurationMS      int64                           `json:"maxDurationMs,omitempty"`
	MaxDurationSeconds int                             `json:"maxDurationSeconds,omitempty"`
	MaxPixels          int64                           `json:"maxPixels,omitempty"`
	Criteria           []ArtifactRubricCriterionOutput `json:"criteria" nullable:"false"`
	PassRule           string                          `json:"passRule" enum:"all_required"`
	ReviewPolicy       string                          `json:"reviewPolicy" enum:"reject,parent_review"`
	FeedbackStyle      string                          `json:"feedbackStyle,omitempty" enum:"age_appropriate,concise"`
}

type ArtifactCriterionEvaluationOutput struct {
	ID          string `json:"id" format:"uuid"`
	CriterionID string `json:"criterionId"`
	Required    bool   `json:"required"`
	Status      string `json:"status" enum:"accepted,rejected,unavailable,pending"`
	Evidence    string `json:"evidence,omitempty"`
	Feedback    string `json:"feedback,omitempty"`
}

type ArtifactEvaluationOutput struct {
	Status         string                              `json:"status" enum:"ready,queued,loading,evaluating,rejected,review,complete,error"`
	Accepted       bool                                `json:"accepted"`
	NextStep       string                              `json:"nextStep,omitempty"`
	Provider       string                              `json:"provider,omitempty"`
	Model          string                              `json:"model,omitempty"`
	PolicyVersion  string                              `json:"policyVersion,omitempty"`
	ArtifactDigest string                              `json:"artifactDigest,omitempty"`
	RubricRevision string                              `json:"rubricRevision,omitempty"`
	Criteria       []ArtifactCriterionEvaluationOutput `json:"criteria" nullable:"false"`
}

type ArtifactSubmissionOutput struct {
	ID         string                    `json:"id" format:"uuid"`
	ArtifactID string                    `json:"artifactId" format:"uuid"`
	Kind       string                    `json:"kind" enum:"image,video,audio"`
	MediaType  string                    `json:"mediaType"`
	SizeBytes  int64                     `json:"sizeBytes"`
	Digest     string                    `json:"digest,omitempty"`
	Status     string                    `json:"status" enum:"reserved,uploading,validating,evaluating,rejected,review,complete"`
	CreatedAt  time.Time                 `json:"createdAt" format:"date-time"`
	Evaluation *ArtifactEvaluationOutput `json:"evaluation,omitempty"`
}

type ArtifactStateResponse struct {
	OccurrenceID       string                     `json:"occurrenceId" format:"uuid"`
	RequirementID      string                     `json:"requirementId" format:"uuid"`
	RubricRevision     string                     `json:"rubricRevision" format:"uuid"`
	Config             ArtifactRubricConfigOutput `json:"config"`
	Status             string                     `json:"status" enum:"ready,queued,loading,evaluating,rejected,review,complete,error"`
	Submissions        []ArtifactSubmissionOutput `json:"submissions" nullable:"false"`
	ActiveSubmissionID string                     `json:"activeSubmissionId,omitempty" format:"uuid"`
	Evaluation         *ArtifactEvaluationOutput  `json:"evaluation,omitempty"`
}

// The old HTTP façades retain an occurrence in their internal payload because
// they predate the path-parameter Huma boundary. They are never used to derive
// the public OpenAPI schema.
type artifactReservationInput struct {
	OccurrenceID   string `json:"occurrenceId"`
	RequirementID  string `json:"requirementId"`
	Kind           string `json:"kind"`
	Filename       string `json:"filename"`
	ContentType    string `json:"contentType"`
	Size           int64  `json:"size"`
	PartCount      int    `json:"partCount"`
	IdempotencyKey string `json:"idempotencyKey"`
}
type artifactFinalizeInput struct {
	ArtifactID     string `json:"artifactId"`
	OccurrenceID   string `json:"occurrenceId"`
	RequirementID  string `json:"requirementId"`
	SHA256         string `json:"sha256"`
	Digest         string `json:"digest"`
	DurationMS     int64  `json:"durationMs"`
	IdempotencyKey string `json:"idempotencyKey"`
	SizeBytes      int64  `json:"sizeBytes"`
	MediaType      string `json:"mediaType"`
	Filename       string `json:"filename"`
}
type artifactReservationOutput = ArtifactReservationOutput
type artifactOutput = ArtifactOutput

func (s *Server) registerArtifactRoutes(r *chi.Mux) {
	// JSON reserve/state/finalize/retry/inspect routes are Huma operations.
	// Only bounded binary PUT/GET façades remain here; their object keys and
	// storage-provider URLs never enter the browser contract.
	r.Put("/student/artifacts/{id}/upload", s.requireStudent(s.uploadArtifact))
	r.Put("/student/artifacts/{id}/parts/{part}", s.requireStudent(s.uploadArtifactPart))
	r.Get("/student/artifacts/{id}/derivative/{kind}", s.requireStudent(s.studentDerivative))
	r.Get("/parent/artifacts/{id}/original", s.requireParent(s.parentOriginal))
	r.Get("/occurrences/{occurrence}/artifacts/{id}/derivative", s.requireParent(s.parentDerivative))
}
func (s *Server) artifactTenant(ctx context.Context, student uuid.UUID) (uuid.UUID, error) {
	var t uuid.UUID
	err := s.DB.QueryRow(ctx, `SELECT tenant_id FROM students WHERE id=$1 AND archived_at IS NULL`, student).Scan(&t)
	return t, err
}
func parseArtifactKind(v string) (artifact.Kind, error) {
	k := artifact.Kind(strings.ToLower(strings.TrimSpace(v)))
	if k != artifact.Image && k != artifact.Video && k != artifact.Audio {
		return "", errors.New("unsupported artifact kind")
	}
	return k, nil
}

type artifactLimitsConfig struct {
	MaxBytes           int64 `json:"maxBytes"`
	MaxDurationMS      int64 `json:"maxDurationMs"`
	MaxDurationSeconds int64 `json:"maxDurationSeconds"`
	MaxPixels          int64 `json:"maxPixels"`
	MaxCount           int   `json:"maxCount"`
}

func mediaLimits(kind artifact.Kind, raw []byte) artifact.Limits {
	limits := artifact.DefaultLimits(kind)
	var config artifactLimitsConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return limits
	}
	if config.MaxBytes > 0 {
		limits.MaxBytes = config.MaxBytes
	}
	if config.MaxDurationMS > 0 {
		limits.MaxDurationMS = config.MaxDurationMS
	} else if config.MaxDurationSeconds > 0 {
		limits.MaxDurationMS = config.MaxDurationSeconds * 1000
	}
	if config.MaxPixels > 0 {
		limits.MaxPixels = config.MaxPixels
	}
	if config.MaxCount > 0 {
		limits.MaxCount = config.MaxCount
	}
	return limits
}
func uploadKey(tenant, id uuid.UUID) string {
	return fmt.Sprintf("tenants/%s/artifacts/%s/original", tenant, id)
}

func (s *Server) reserveArtifactData(ctx context.Context, student uuid.UUID, occurrence string, in ArtifactReservationInput) (ArtifactReservationOutput, error) {
	if s.DB == nil || s.Artifacts == nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusServiceUnavailable, "artifact storage is not configured")
	}
	tenant, err := s.artifactTenant(ctx, student)
	if err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusUnauthorized, "student session required")
	}
	kind, err := parseArtifactKind(in.Kind)
	if err != nil || in.Size <= 0 || len(in.Filename) > 255 {
		return ArtifactReservationOutput{}, newProblem(http.StatusBadRequest, "artifact metadata is invalid")
	}
	if in.PartCount < 1 {
		in.PartCount = 1
	}
	if in.PartCount > 10000 {
		return ArtifactReservationOutput{}, newProblem(http.StatusBadRequest, "part count is too large")
	}
	if in.RequirementID == "" {
		_ = s.DB.QueryRow(ctx, `SELECT vr.id FROM verification_requirements vr JOIN task_occurrences o ON o.tenant_id=vr.tenant_id AND o.revision_id=vr.revision_id WHERE o.id=$1 AND o.student_id=$2 AND vr.executor='fantasy' AND vr.kind='agent_artifact_rubric' ORDER BY vr.ordinal LIMIT 1`, occurrence, student).Scan(&in.RequirementID)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" || len(in.IdempotencyKey) > 128 {
		return ArtifactReservationOutput{}, newProblem(http.StatusBadRequest, "idempotency key is required")
	}
	var config []byte
	var accepted string
	err = s.DB.QueryRow(ctx, `SELECT vr.config,COALESCE(vr.config->>'acceptedKinds','') FROM verification_requirements vr JOIN task_occurrences o ON o.tenant_id=vr.tenant_id AND o.revision_id=vr.revision_id WHERE vr.tenant_id=$1 AND vr.id=$2 AND o.id=$3 AND o.student_id=$4 AND o.status NOT IN ('completed','canceled')`, tenant, in.RequirementID, occurrence, student).Scan(&config, &accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactReservationOutput{}, newProblem(http.StatusNotFound, "artifact requirement not found")
	}
	if err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusInternalServerError, "unable to load artifact requirement")
	}
	if accepted != "" && !strings.Contains(strings.ToLower(accepted), string(kind)) {
		return ArtifactReservationOutput{}, newProblem(http.StatusBadRequest, "artifact kind is not allowed by requirement")
	}
	limits := mediaLimits(kind, config)
	var prior int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM artifact_upload_reservations u JOIN artifacts a ON a.tenant_id=u.tenant_id AND a.id=u.artifact_id WHERE u.tenant_id=$1 AND u.student_id=$2 AND u.occurrence_id=$3 AND u.requirement_id=$4 AND u.status IN ('reserved','finalized') AND a.status IN ('reserved','uploaded','finalized') AND u.expires_at>now() AND NOT EXISTS (SELECT 1 FROM artifact_submissions sub WHERE sub.tenant_id=u.tenant_id AND sub.artifact_id=u.artifact_id AND sub.status IN ('rejected','review'))`, tenant, student, occurrence, in.RequirementID).Scan(&prior); err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusInternalServerError, "unable to count artifact submissions")
	}
	if limits.MaxCount > 0 && prior >= limits.MaxCount {
		return ArtifactReservationOutput{}, newProblem(http.StatusConflict, "artifact count limit has been reached")
	}
	if in.Size > limits.MaxBytes {
		return ArtifactReservationOutput{}, newProblem(http.StatusBadRequest, "artifact exceeds requirement size limit")
	}
	var existing ArtifactReservationOutput
	err = s.DB.QueryRow(ctx, `SELECT r.id,r.artifact_id,r.part_count,r.expires_at FROM artifact_upload_reservations r WHERE r.tenant_id=$1 AND r.student_id=$2 AND r.idempotency_key=$3 AND r.status IN ('reserved','finalized') AND r.expires_at>now()`, tenant, student, in.IdempotencyKey).Scan(&existing.ReservationID, &existing.ArtifactID, &existing.PartCount, &existing.ExpiresAt)
	if err == nil {
		existing.UploadURL = "/api/student/artifacts/" + existing.ArtifactID + "/upload"
		existing.IdempotencyKey = in.IdempotencyKey
		return existing, nil
	}
	rid, aid := uuid.New(), uuid.New()
	expires := time.Now().UTC().Add(20 * time.Minute)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusInternalServerError, "unable to reserve artifact")
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, aid, tenant, student, uploadKey(tenant, aid), kind, in.Filename, in.ContentType, in.Size, expires)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO artifact_upload_reservations(id,tenant_id,student_id,artifact_id,occurrence_id,requirement_id,idempotency_key,part_count,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, rid, tenant, student, aid, occurrence, in.RequirementID, in.IdempotencyKey, in.PartCount, expires)
	}
	if err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusConflict, "upload reservation already exists or is invalid")
	}
	if err = tx.Commit(ctx); err != nil {
		return ArtifactReservationOutput{}, newProblem(http.StatusInternalServerError, "unable to commit reservation")
	}
	return ArtifactReservationOutput{ReservationID: rid.String(), ArtifactID: aid.String(), IdempotencyKey: in.IdempotencyKey, UploadURL: "/api/student/artifacts/" + aid.String() + "/upload", PartCount: in.PartCount, ExpiresAt: expires}, nil
}

func (s *Server) reserveArtifact(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	if s.DB == nil || s.Artifacts == nil {
		problem(w, 503, "unavailable", "artifact storage is not configured")
		return
	}
	var in artifactReservationInput
	if !decode(w, r, &in) {
		return
	}
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	kind, err := parseArtifactKind(in.Kind)
	if err != nil || in.Size <= 0 || len(in.Filename) > 255 {
		problem(w, 400, "invalid_request", "artifact metadata is invalid")
		return
	}
	if in.PartCount < 1 {
		in.PartCount = 1
	}
	if in.PartCount > 10000 {
		problem(w, 400, "invalid_request", "part count is too large")
		return
	}
	if in.OccurrenceID == "" {
		in.OccurrenceID = chi.URLParam(r, "occurrence")
	}
	if in.RequirementID == "" && in.OccurrenceID != "" {
		_ = s.DB.QueryRow(r.Context(), `SELECT vr.id FROM verification_requirements vr JOIN task_occurrences o ON o.tenant_id=vr.tenant_id AND o.revision_id=vr.revision_id WHERE o.id=$1 AND o.student_id=$2 AND vr.executor='fantasy' AND vr.kind='agent_artifact_rubric' ORDER BY vr.ordinal LIMIT 1`, in.OccurrenceID, student).Scan(&in.RequirementID)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" || len(in.IdempotencyKey) > 128 {
		problem(w, 400, "invalid_request", "idempotency key is required")
		return
	}
	var config []byte
	var accepted string
	err = s.DB.QueryRow(r.Context(), `SELECT vr.config,COALESCE(vr.config->>'acceptedKinds','') FROM verification_requirements vr JOIN task_occurrences o ON o.tenant_id=vr.tenant_id AND o.revision_id=vr.revision_id WHERE vr.tenant_id=$1 AND vr.id=$2 AND o.id=$3 AND o.student_id=$4 AND o.status NOT IN ('completed','canceled')`, tenant, in.RequirementID, in.OccurrenceID, student).Scan(&config, &accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact requirement not found")
		return
	}
	if err != nil {
		problem(w, 500, "internal", "unable to load artifact requirement")
		return
	}
	if accepted != "" && !strings.Contains(strings.ToLower(accepted), string(kind)) {
		problem(w, 400, "invalid_request", "artifact kind is not allowed by requirement")
		return
	}
	limits := mediaLimits(kind, config)
	var prior int
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM artifact_upload_reservations u JOIN artifacts a ON a.tenant_id=u.tenant_id AND a.id=u.artifact_id WHERE u.tenant_id=$1 AND u.student_id=$2 AND u.occurrence_id=$3 AND u.requirement_id=$4 AND u.status IN ('reserved','finalized') AND a.status IN ('reserved','uploaded','finalized') AND u.expires_at>now() AND NOT EXISTS (SELECT 1 FROM artifact_submissions sub WHERE sub.tenant_id=u.tenant_id AND sub.artifact_id=u.artifact_id AND sub.status IN ('rejected','review'))`, tenant, student, in.OccurrenceID, in.RequirementID).Scan(&prior); e != nil {
		problem(w, 500, "internal", "unable to count artifact submissions")
		return
	}
	if limits.MaxCount > 0 && prior >= limits.MaxCount {
		problem(w, 409, "conflict", "artifact count limit has been reached")
		return
	}
	if in.Size > limits.MaxBytes {
		problem(w, 400, "invalid_request", "artifact exceeds requirement size limit")
		return
	}
	var existing artifactReservationOutput
	err = s.DB.QueryRow(r.Context(), `SELECT r.id,r.artifact_id,r.part_count,r.expires_at FROM artifact_upload_reservations r WHERE r.tenant_id=$1 AND r.student_id=$2 AND r.idempotency_key=$3 AND r.status IN ('reserved','finalized') AND r.expires_at>now()`, tenant, student, in.IdempotencyKey).Scan(&existing.ReservationID, &existing.ArtifactID, &existing.PartCount, &existing.ExpiresAt)
	if err == nil {
		existing.UploadURL = "/api/student/artifacts/" + existing.ArtifactID + "/upload"
		existing.IdempotencyKey = in.IdempotencyKey
		jsonStatus(w, existing, 200)
		return
	}
	rid, aid := uuid.New(), uuid.New()
	expires := time.Now().UTC().Add(20 * time.Minute)
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", "unable to reserve artifact")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `INSERT INTO artifacts(id,tenant_id,student_id,object_key,kind,original_name,declared_content_type,byte_size,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, aid, tenant, student, uploadKey(tenant, aid), kind, in.Filename, in.ContentType, in.Size, expires)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO artifact_upload_reservations(id,tenant_id,student_id,artifact_id,occurrence_id,requirement_id,idempotency_key,part_count,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, rid, tenant, student, aid, in.OccurrenceID, in.RequirementID, in.IdempotencyKey, in.PartCount, expires)
	}
	if e != nil {
		problem(w, 409, "conflict", "upload reservation already exists or is invalid")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", "unable to commit reservation")
		return
	}
	out := artifactReservationOutput{ReservationID: rid.String(), ArtifactID: aid.String(), IdempotencyKey: in.IdempotencyKey, UploadURL: "/api/student/artifacts/" + aid.String() + "/upload", PartCount: in.PartCount, ExpiresAt: expires}
	// Browser bytes always travel through the same-origin allowlisted binary
	// façade. Object-store keys and signed provider URLs never enter a page
	// contract or browser network request.
	jsonStatus(w, out, http.StatusCreated)
}

func (s *Server) uploadArtifact(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	aid, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	var key string
	var size int64
	var partCount int
	var status string
	var expires time.Time
	e = s.DB.QueryRow(r.Context(), `SELECT a.object_key,a.byte_size,u.part_count,a.status,a.expires_at FROM artifacts a JOIN artifact_upload_reservations u ON u.tenant_id=a.tenant_id AND u.artifact_id=a.id WHERE a.tenant_id=$1 AND a.student_id=$2 AND a.id=$3 AND u.status IN ('reserved','finalized') AND (u.status='finalized' OR u.expires_at>now())`, tenant, student, aid).Scan(&key, &size, &partCount, &status, &expires)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "upload reservation not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", "unable to load reservation")
		return
	}
	if status == "finalized" {
		jsonOK(w, ArtifactUploadOutput{Status: "already_uploaded", Size: size, ExpiresAt: expires})
		return
	}
	if partCount != 1 {
		problem(w, 409, "conflict", "multipart reservation requires part endpoints")
		return
	}
	// A proxy can de-chunk the browser request and omit Content-Length. Read at
	// most one byte beyond the reservation, then give the S3 client a stable
	// seekable byte reader. AWS SDK retries/hash middleware cannot safely replay
	// a streaming http.MaxBytesReader body.
	cancel := func() {
		_, _ = s.DB.Exec(r.Context(), `UPDATE artifacts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, aid)
		_, _ = s.DB.Exec(r.Context(), `UPDATE artifact_upload_reservations SET status='canceled' WHERE tenant_id=$1 AND artifact_id=$2 AND status='reserved'`, tenant, aid)
	}
	if r.ContentLength > size {
		cancel()
		problem(w, 400, "invalid_request", "upload size exceeds reservation")
		return
	}
	body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, size+1))
	if readErr != nil || int64(len(body)) != size {
		cancel()
		problem(w, 400, "invalid_request", "upload size does not match reservation")
		return
	}
	obj, e := s.Artifacts.Put(r.Context(), key, r.Header.Get("Content-Type"), bytes.NewReader(body), int64(len(body)))
	if e != nil {
		slog.Warn("artifact upload storage failure", "code", "object_put_failed", "kind", r.Header.Get("Content-Type"))
		cancel()
		problem(w, 400, "invalid_request", "upload could not be stored")
		return
	}
	_, e = s.DB.Exec(r.Context(), `UPDATE artifacts SET status='uploaded',detected_content_type=COALESCE(NULLIF($1,''),declared_content_type) WHERE tenant_id=$2 AND id=$3 AND status='reserved'`, obj.ContentType, tenant, aid)
	if e != nil {
		problem(w, 500, "internal", "unable to record upload")
		return
	}
	jsonOK(w, ArtifactUploadOutput{Status: "uploaded", Size: obj.Size, ExpiresAt: expires})
}

func (s *Server) finalizeArtifactData(ctx context.Context, student uuid.UUID, occurrence string, in ArtifactFinalizeInput) (ArtifactOutput, error) {
	tenant, err := s.artifactTenant(ctx, student)
	if err != nil {
		return ArtifactOutput{}, newProblem(http.StatusUnauthorized, "student session required")
	}
	artifactID := in.ArtifactID
	aid, e := uuid.Parse(artifactID)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusNotFound, "artifact not found")
	}
	if in.RequirementID == "" {
		_ = s.DB.QueryRow(ctx, `SELECT requirement_id FROM artifact_upload_reservations WHERE tenant_id=$1 AND artifact_id=$2`, tenant, aid).Scan(&in.RequirementID)
	}
	if _, e = uuid.Parse(in.RequirementID); e != nil {
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "requirementId is required")
	}
	if in.SHA256 == "" {
		in.SHA256 = in.Digest
	}
	if strings.TrimSpace(in.SHA256) == "" {
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "sha256 digest is required")
	}
	var key, kind, declared, status string
	var expected int64
	var partCount int
	var config []byte
	e = s.DB.QueryRow(ctx, `SELECT a.object_key,a.kind,a.declared_content_type,a.byte_size,a.status,u.part_count,vr.config FROM artifacts a JOIN artifact_upload_reservations u ON u.tenant_id=a.tenant_id AND u.artifact_id=a.id JOIN verification_requirements vr ON vr.tenant_id=u.tenant_id AND vr.id=u.requirement_id JOIN task_occurrences o ON o.tenant_id=u.tenant_id AND o.id=u.occurrence_id AND o.student_id=$2 WHERE a.tenant_id=$1 AND a.student_id=$2 AND a.id=$3 AND u.occurrence_id=$4 AND u.requirement_id=$5 AND u.status IN ('reserved','finalized') AND (u.status='finalized' OR u.expires_at>now())`, tenant, student, aid, occurrence, in.RequirementID).Scan(&key, &kind, &declared, &expected, &status, &partCount, &config)
	if errors.Is(e, pgx.ErrNoRows) {
		return ArtifactOutput{}, newProblem(http.StatusNotFound, "artifact reservation not found")
	}
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to load artifact")
	}
	if status == "finalized" {
		var sub string
		_ = s.DB.QueryRow(ctx, `SELECT id FROM artifact_submissions WHERE tenant_id=$1 AND artifact_id=$2 ORDER BY created_at DESC LIMIT 1`, tenant, aid).Scan(&sub)
		return ArtifactOutput{ID: aid.String(), Kind: kind, Status: status, SubmissionID: sub}, nil
	}
	// Direct presigned PUTs do not pass through the API. Finalize therefore
	// reconciles the scoped object-store key itself; the client cannot claim
	// that an upload exists by changing metadata.
	if status != "uploaded" && status != "reserved" {
		return ArtifactOutput{}, newProblem(http.StatusConflict, "artifact is not available for finalize")
	}
	var occurrenceStatus string
	if e = s.DB.QueryRow(ctx, `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2`, tenant, occurrence).Scan(&occurrenceStatus); e != nil || occurrenceStatus == "completed" || occurrenceStatus == "canceled" {
		return ArtifactOutput{}, newProblem(http.StatusConflict, "occurrence is no longer accepting evidence")
	}
	if partCount > 1 {
		rows, re := s.DB.Query(ctx, `SELECT object_key FROM artifact_upload_parts p JOIN artifact_upload_reservations u ON u.tenant_id=p.tenant_id AND u.id=p.reservation_id WHERE p.tenant_id=$1 AND u.artifact_id=$2 ORDER BY p.part_number`, tenant, aid)
		if re != nil {
			return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to load upload parts")
		}
		var parts []string
		for rows.Next() {
			var p string
			if re = rows.Scan(&p); re != nil {
				rows.Close()
				return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to load upload parts")
			}
			parts = append(parts, p)
		}
		rows.Close()
		if len(parts) != partCount {
			return ArtifactOutput{}, newProblem(http.StatusConflict, "not all upload parts are present")
		}
		composer, ok := s.Artifacts.(artifactstore.Composer)
		if !ok {
			return ArtifactOutput{}, newProblem(http.StatusServiceUnavailable, "multipart composition is not configured")
		}
		if _, re = composer.Compose(ctx, key, declared, parts, expected); re != nil {
			return ArtifactOutput{}, newProblem(http.StatusBadRequest, "upload parts could not be composed")
		}
	}
	f, obj, e := s.Artifacts.Open(ctx, key)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "stored object is missing")
	}
	defer f.Close()
	k, parseErr := parseArtifactKind(kind)
	if parseErr != nil {
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "artifact kind is invalid")
	}
	// Browser duration and declared content type are advisory only. The
	// authoritative A/V values come from the bounded ffprobe/decoder boundary.
	result, e := artifact.ValidateContext(ctx, f, artifact.Input{Kind: k, DeclaredType: declared, ExpectedSize: expected, ExpectedSHA256: in.SHA256, DurationMS: in.DurationMS}, mediaLimits(k, config))
	if e != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE artifacts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, aid)
		_, _ = s.DB.Exec(ctx, `UPDATE artifact_upload_reservations SET status='canceled' WHERE tenant_id=$1 AND artifact_id=$2 AND status='reserved'`, tenant, aid)
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, e.Error())
	}
	if k != artifact.Image && !result.DurationAuthoritative {
		_, _ = s.DB.Exec(ctx, `UPDATE artifacts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, aid)
		_, _ = s.DB.Exec(ctx, `UPDATE artifact_upload_reservations SET status='canceled' WHERE tenant_id=$1 AND artifact_id=$2 AND status='reserved'`, tenant, aid)
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "encoded media duration could not be verified")
	}
	if obj.Size != expected {
		return ArtifactOutput{}, newProblem(http.StatusBadRequest, "stored object size does not match reservation")
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to finalize artifact")
	}
	defer tx.Rollback(ctx)
	var attempt uuid.UUID
	var attemptStatus string
	var hasDecision bool
	e = tx.QueryRow(ctx, `SELECT va.id,va.status,EXISTS(SELECT 1 FROM verification_decisions d WHERE d.tenant_id=va.tenant_id AND d.attempt_id=va.id) FROM verification_attempts va WHERE va.tenant_id=$1 AND va.occurrence_id=$2 AND va.requirement_id=$3 ORDER BY va.number DESC LIMIT 1`, tenant, occurrence, in.RequirementID).Scan(&attempt, &attemptStatus, &hasDecision)
	if errors.Is(e, pgx.ErrNoRows) || attemptStatus != "open" || hasDecision {
		e = tx.QueryRow(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) SELECT $1,$2,$3,$4,COALESCE(MAX(number),0)+1 FROM verification_attempts WHERE tenant_id=$2 AND occurrence_id=$3 AND requirement_id=$4 RETURNING id`, uuid.New(), tenant, occurrence, in.RequirementID).Scan(&attempt)
	}
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusConflict, "verification attempt is unavailable")
	}
	sub := uuid.New()
	idem := in.IdempotencyKey
	if idem == "" {
		idem = result.SHA256
	}
	var existing string
	e = tx.QueryRow(ctx, `INSERT INTO artifact_submissions(id,tenant_id,student_id,occurrence_id,requirement_id,attempt_id,artifact_id,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'submitted') ON CONFLICT(tenant_id,student_id,occurrence_id,idempotency_key) DO UPDATE SET id=artifact_submissions.id RETURNING id`, sub, tenant, student, occurrence, in.RequirementID, attempt, aid, idem).Scan(&existing)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusConflict, "submission idempotency key is already used")
	}
	sub = uuid.MustParse(existing)
	_, e = tx.Exec(ctx, `UPDATE artifacts SET status='finalized',detected_content_type=$1,sha256=$2,width=$3,height=$4,duration_ms=$5,finalized_at=now() WHERE tenant_id=$6 AND id=$7`, result.ContentType, result.SHA256, result.Width, result.Height, result.DurationMS, tenant, aid)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to finalize artifact")
	}
	_, e = tx.Exec(ctx, `UPDATE artifact_upload_reservations SET status='finalized' WHERE tenant_id=$1 AND artifact_id=$2`, tenant, aid)
	if e != nil {
		return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to close reservation")
	}
	_, _ = tx.Exec(ctx, `INSERT INTO artifact_scans(tenant_id,artifact_id,scanner,status,detail) VALUES($1,$2,'none','not_configured','no malware scanner configured') ON CONFLICT(tenant_id,artifact_id) DO NOTHING`, tenant, aid)
	_, _ = tx.Exec(ctx, `INSERT INTO artifact_retention(tenant_id,artifact_id,retain_original_until,retain_derivatives_until) VALUES($1,$2,now()+interval '30 days',now()+interval '180 days') ON CONFLICT DO NOTHING`, tenant, aid)
	var requirementKind string
	_ = tx.QueryRow(ctx, `SELECT kind FROM verification_requirements WHERE tenant_id=$1 AND id=$2`, tenant, in.RequirementID).Scan(&requirementKind)
	if requirementKind == "agent_artifact_rubric" {
		_, e = tx.Exec(ctx, `INSERT INTO artifact_rubric_jobs(id,tenant_id,submission_id,rubric_snapshot,available_at) VALUES($1,$2,$3,$4,now()+interval '1 second') ON CONFLICT(tenant_id,submission_id) DO NOTHING`, uuid.New(), tenant, sub, config)
		if e != nil {
			return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to enqueue artifact review")
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return ArtifactOutput{}, newProblem(http.StatusInternalServerError, "unable to commit artifact")
	}
	out := artifactOutput{ID: aid.String(), Kind: kind, ContentType: result.ContentType, Size: result.Size, SHA256: result.SHA256, Width: result.Width, Height: result.Height, DurationMS: result.DurationMS, Status: "finalized", SubmissionID: sub.String()}
	if k == artifact.Image {
		if thumb, tw, th, te := artifact.Thumbnail(result.Bytes, 640, 640); te == nil {
			dk := fmt.Sprintf("tenants/%s/artifacts/%s/derivatives/thumbnail", tenant, aid)
			if _, pe := s.Artifacts.Put(ctx, dk, "image/png", bytes.NewReader(thumb), int64(len(thumb))); pe == nil {
				_, _ = s.DB.Exec(ctx, `INSERT INTO artifact_derivatives(id,tenant_id,artifact_id,derivative_kind,object_key,content_type,byte_size,sha256,width,height) VALUES($1,$2,$3,'thumbnail',$4,'image/png',$5,$6,$7,$8) ON CONFLICT(tenant_id,artifact_id,derivative_kind) DO NOTHING`, uuid.New(), tenant, aid, dk, len(thumb), digestBytes(thumb), tw, th)
				out.DerivativeURL = "/student/artifacts/" + aid.String() + "/derivative/thumbnail"
			}
		}
	} else {
		// Audio/video previews are byte-preserving, authorization-scoped copies;
		// unlike image thumbnails there is no metadata-stripping transcode here.
		dk := fmt.Sprintf("tenants/%s/artifacts/%s/derivatives/preview", tenant, aid)
		previewType := result.ContentType
		if strings.HasPrefix(strings.ToLower(declared), string(k)+"/") {
			previewType = declared
		}
		if _, pe := s.Artifacts.Put(ctx, dk, previewType, bytes.NewReader(result.Bytes), int64(len(result.Bytes))); pe == nil {
			_, _ = s.DB.Exec(ctx, `INSERT INTO artifact_derivatives(id,tenant_id,artifact_id,derivative_kind,object_key,content_type,byte_size,sha256,metadata_stripped) VALUES($1,$2,$3,'preview',$4,$5,$6,$7,false) ON CONFLICT(tenant_id,artifact_id,derivative_kind) DO NOTHING`, uuid.New(), tenant, aid, dk, previewType, len(result.Bytes), digestBytes(result.Bytes))
			out.DerivativeURL = "/student/artifacts/" + aid.String() + "/derivative/preview"
		}
	}
	return out, nil
}

func (s *Server) finalizeArtifact(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	var in artifactFinalizeInput
	if !decode(w, r, &in) {
		return
	}
	occurrence := in.OccurrenceID
	if occurrence == "" {
		occurrence = chi.URLParam(r, "occurrence")
	}
	out, err := s.finalizeArtifactData(r.Context(), student, occurrence, ArtifactFinalizeInput{
		ArtifactID: in.ArtifactID, RequirementID: in.RequirementID, SHA256: in.SHA256,
		Digest: in.Digest, DurationMS: in.DurationMS, IdempotencyKey: in.IdempotencyKey,
		SizeBytes: in.SizeBytes, MediaType: in.MediaType, Filename: in.Filename,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if statusErr, ok := err.(interface{ GetStatus() int }); ok {
			status = statusErr.GetStatus()
		}
		problem(w, status, "invalid_request", err.Error())
		return
	}
	jsonStatus(w, out, http.StatusOK)
}

func (s *Server) studentDerivative(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	tenant, e := s.artifactTenant(r.Context(), student)
	if e != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	aid, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	var key, ct string
	e = s.DB.QueryRow(r.Context(), `SELECT d.object_key,d.content_type FROM artifact_derivatives d JOIN artifacts a ON a.tenant_id=d.tenant_id AND a.id=d.artifact_id WHERE d.tenant_id=$1 AND a.student_id=$2 AND d.artifact_id=$3 AND d.derivative_kind=$4 AND a.status IN ('finalized','tombstoned')`, tenant, student, aid, chi.URLParam(r, "kind")).Scan(&key, &ct)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact derivative not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", "unable to load derivative")
		return
	}
	f, _, e := s.Artifacts.Open(r.Context(), key)
	if e != nil {
		problem(w, 404, "not_found", "artifact derivative not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = io.Copy(w, f)
}
func (s *Server) parentOriginal(w http.ResponseWriter, r *http.Request, sc scope) {
	aid, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	var key, ct string
	e = s.DB.QueryRow(r.Context(), `SELECT object_key,COALESCE(NULLIF(detected_content_type,''),declared_content_type) FROM artifacts WHERE tenant_id=$1 AND id=$2 AND status='finalized'`, sc.Tenant, aid).Scan(&key, &ct)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", "unable to load artifact")
		return
	}
	f, _, e := s.Artifacts.Open(r.Context(), key)
	if e != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = io.Copy(w, f)
}

func (s *Server) artifactState(ctx context.Context, tenant uuid.UUID, occurrence string, student *uuid.UUID) (ArtifactStateResponse, error) {
	var reqID uuid.UUID
	var configBytes []byte
	query := `SELECT vr.id,vr.config FROM task_occurrences o JOIN verification_requirements vr ON vr.tenant_id=o.tenant_id AND vr.revision_id=o.revision_id WHERE o.tenant_id=$1 AND o.id=$2 AND vr.kind='agent_artifact_rubric'`
	args := []any{tenant, occurrence}
	if student != nil {
		query += ` AND o.student_id=$3`
		args = append(args, *student)
	}
	query += ` ORDER BY vr.ordinal LIMIT 1`
	if err := s.DB.QueryRow(ctx, query, args...).Scan(&reqID, &configBytes); err != nil {
		return ArtifactStateResponse{}, err
	}

	config := ArtifactRubricConfigOutput{}
	if len(configBytes) > 0 {
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return ArtifactStateResponse{}, fmt.Errorf("decode artifact rubric config: %w", err)
		}
	}
	rows, err := s.DB.Query(ctx, `SELECT sub.id,sub.artifact_id,a.kind,a.declared_content_type,a.byte_size,a.sha256,sub.status,sub.created_at FROM artifact_submissions sub JOIN artifacts a ON a.tenant_id=sub.tenant_id AND a.id=sub.artifact_id WHERE sub.tenant_id=$1 AND sub.occurrence_id=$2 AND sub.requirement_id=$3 ORDER BY sub.created_at`, tenant, occurrence, reqID)
	if err != nil {
		return ArtifactStateResponse{}, err
	}
	defer rows.Close()
	submissions := make([]ArtifactSubmissionOutput, 0)
	latest := ""
	state := "ready"
	for rows.Next() {
		var submission ArtifactSubmissionOutput
		var rawStatus string
		if err := rows.Scan(&submission.ID, &submission.ArtifactID, &submission.Kind, &submission.MediaType, &submission.SizeBytes, &submission.Digest, &rawStatus, &submission.CreatedAt); err != nil {
			return ArtifactStateResponse{}, err
		}
		latest = submission.ID
		submission.Status = artifactSubmissionStatus(rawStatus)
		state = submission.Status
		submissions = append(submissions, submission)
	}
	if err := rows.Err(); err != nil {
		return ArtifactStateResponse{}, err
	}

	var evaluation *ArtifactEvaluationOutput
	if latest != "" {
		current := &ArtifactEvaluationOutput{Status: state, Accepted: state == "complete", Criteria: make([]ArtifactCriterionEvaluationOutput, 0)}
		_ = s.DB.QueryRow(ctx, `SELECT provider,model,'agent_artifact_rubric.v1' FROM artifact_rubric_jobs WHERE tenant_id=$1 AND submission_id=$2`, tenant, latest).Scan(&current.Provider, &current.Model, &current.PolicyVersion)
		criteriaRows, criteriaErr := s.DB.Query(ctx, `SELECT id::text,criterion_id,required,status,evidence,feedback FROM artifact_criterion_evaluations WHERE tenant_id=$1 AND submission_id=$2 ORDER BY created_at`, tenant, latest)
		if criteriaErr == nil {
			for criteriaRows.Next() {
				var criterion ArtifactCriterionEvaluationOutput
				if scanErr := criteriaRows.Scan(&criterion.ID, &criterion.CriterionID, &criterion.Required, &criterion.Status, &criterion.Evidence, &criterion.Feedback); scanErr != nil {
					criteriaRows.Close()
					return ArtifactStateResponse{}, scanErr
				}
				current.Criteria = append(current.Criteria, criterion)
			}
			if closeErr := criteriaRows.Err(); closeErr != nil {
				criteriaRows.Close()
				return ArtifactStateResponse{}, closeErr
			}
			criteriaRows.Close()
		}
		evaluation = current
	}
	return ArtifactStateResponse{
		OccurrenceID: occurrence, RequirementID: reqID.String(), RubricRevision: reqID.String(),
		Config: config, Status: state, Submissions: submissions, ActiveSubmissionID: latest, Evaluation: evaluation,
	}, nil
}

func artifactSubmissionStatus(status string) string {
	switch status {
	case "submitted", "evaluating":
		return "evaluating"
	case "accepted":
		return "complete"
	case "rejected":
		return "rejected"
	case "review":
		return "review"
	default:
		return "queued"
	}
}

func (s *Server) retryArtifactState(ctx context.Context, student uuid.UUID, occurrence string) (ArtifactStateResponse, error) {
	tenant, err := s.artifactTenant(ctx, student)
	if err != nil {
		return ArtifactStateResponse{}, newProblem(http.StatusUnauthorized, "student session required")
	}
	tag, err := s.DB.Exec(ctx, `UPDATE artifact_rubric_jobs j SET status='queued',available_at=now(),lease_owner=NULL,lease_until=NULL,updated_at=now() FROM artifact_submissions sub WHERE sub.tenant_id=j.tenant_id AND sub.id=j.submission_id AND sub.tenant_id=$1 AND sub.occurrence_id=$2 AND sub.student_id=$3 AND j.status IN ('failed','review')`, tenant, occurrence, student)
	if err != nil {
		return ArtifactStateResponse{}, newProblem(http.StatusInternalServerError, "unable to retry artifact review")
	}
	if tag.RowsAffected() == 0 {
		return ArtifactStateResponse{}, newProblem(http.StatusConflict, "artifact review is not retryable")
	}
	state, err := s.artifactState(ctx, tenant, occurrence, &student)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactStateResponse{}, newProblem(http.StatusNotFound, "artifact requirement not found")
	}
	if err != nil {
		return ArtifactStateResponse{}, newProblem(http.StatusInternalServerError, "unable to load artifact state")
	}
	return state, nil
}

func (s *Server) studentArtifactState(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	state, err := s.artifactState(r.Context(), tenant, chi.URLParam(r, "occurrence"), &student)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact requirement not found")
		return
	}
	if err != nil {
		problem(w, 500, "internal", "unable to load artifact state")
		return
	}
	jsonOK(w, state)
}

func (s *Server) parentArtifactState(w http.ResponseWriter, r *http.Request, sc scope) {
	state, err := s.artifactState(r.Context(), uuid.MustParse(sc.Tenant), chi.URLParam(r, "occurrence"), nil)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact requirement not found")
		return
	}
	if err != nil {
		problem(w, 500, "internal", "unable to load artifact state")
		return
	}
	jsonOK(w, state)
}

func (s *Server) retryArtifactEvaluation(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	occurrence := chi.URLParam(r, "occurrence")
	tag, err := s.DB.Exec(r.Context(), `UPDATE artifact_rubric_jobs j SET status='queued',available_at=now(),lease_owner=NULL,lease_until=NULL,updated_at=now() FROM artifact_submissions sub WHERE sub.tenant_id=j.tenant_id AND sub.id=j.submission_id AND sub.tenant_id=$1 AND sub.occurrence_id=$2 AND sub.student_id=$3 AND j.status IN ('failed','review')`, tenant, occurrence, student)
	if err != nil {
		problem(w, 500, "internal", "unable to retry artifact review")
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 409, "conflict", "artifact review is not retryable")
		return
	}
	state, err := s.artifactState(r.Context(), tenant, occurrence, &student)
	if err != nil {
		problem(w, 500, "internal", "unable to load artifact state")
		return
	}
	jsonOK(w, state)
}

func (s *Server) parentDerivative(w http.ResponseWriter, r *http.Request, sc scope) {
	aid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		problem(w, 404, "not_found", "artifact derivative not found")
		return
	}
	var key, ct string
	err = s.DB.QueryRow(r.Context(), `SELECT d.object_key,d.content_type FROM artifact_derivatives d JOIN artifacts a ON a.tenant_id=d.tenant_id AND a.id=d.artifact_id JOIN artifact_submissions sub ON sub.tenant_id=a.tenant_id AND sub.artifact_id=a.id JOIN task_occurrences o ON o.tenant_id=sub.tenant_id AND o.id=sub.occurrence_id WHERE d.tenant_id=$1 AND d.artifact_id=$2 AND o.id=$3`, sc.Tenant, aid, chi.URLParam(r, "occurrence")).Scan(&key, &ct)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact derivative not found")
		return
	}
	if err != nil {
		problem(w, 500, "internal", "unable to load derivative")
		return
	}
	f, _, err := s.Artifacts.Open(r.Context(), key)
	if err != nil {
		problem(w, 404, "not_found", "artifact derivative not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = io.Copy(w, f)
}

func digestBytes(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func (s *Server) CleanupArtifactOrphans(ctx context.Context, now time.Time) error {
	parts, e := s.DB.Query(ctx, `SELECT p.object_key FROM artifact_upload_parts p JOIN artifact_upload_reservations u ON u.tenant_id=p.tenant_id AND u.id=p.reservation_id WHERE u.expires_at<$1`, now)
	if e != nil {
		return e
	}
	for parts.Next() {
		var key string
		if e = parts.Scan(&key); e != nil {
			parts.Close()
			return e
		}
		if e = s.Artifacts.Delete(ctx, key); e != nil {
			parts.Close()
			return e
		}
	}
	if e = parts.Err(); e != nil {
		parts.Close()
		return e
	}
	parts.Close()
	rows, e := s.DB.Query(ctx, `SELECT id,object_key FROM artifacts WHERE status IN ('reserved','uploaded') AND expires_at<$1`, now)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var key string
		if e = rows.Scan(&id, &key); e != nil {
			return e
		}
		if de := s.Artifacts.Delete(ctx, key); de != nil {
			return de
		}
		if _, e = s.DB.Exec(ctx, `UPDATE artifacts SET status='tombstoned',deleted_at=now() WHERE id=$1 AND status IN ('reserved','uploaded')`, id); e != nil {
			return e
		}
		_, _ = s.DB.Exec(ctx, `UPDATE artifact_upload_reservations SET status='expired' WHERE tenant_id=(SELECT tenant_id FROM artifacts WHERE id=$1) AND artifact_id=$1 AND status='reserved'`, id)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	rows.Close()
	originals, e := s.DB.Query(ctx, `SELECT a.id,a.tenant_id,a.object_key FROM artifacts a JOIN artifact_retention r ON r.tenant_id=a.tenant_id AND r.artifact_id=a.id WHERE a.status='finalized' AND r.retain_original_until<$1 AND a.deleted_at IS NULL`, now)
	if e != nil {
		return e
	}
	for originals.Next() {
		var id, tenant uuid.UUID
		var key string
		if e = originals.Scan(&id, &tenant, &key); e != nil {
			originals.Close()
			return e
		}
		if e = s.Artifacts.Delete(ctx, key); e != nil {
			originals.Close()
			return e
		}
		if _, e = s.DB.Exec(ctx, `UPDATE artifacts SET status='tombstoned',deleted_at=now() WHERE tenant_id=$1 AND id=$2 AND status='finalized'`, tenant, id); e != nil {
			originals.Close()
			return e
		}
		if _, e = s.DB.Exec(ctx, `UPDATE artifact_retention SET tombstoned_at=now() WHERE tenant_id=$1 AND artifact_id=$2`, tenant, id); e != nil {
			originals.Close()
			return e
		}
	}
	if e = originals.Err(); e != nil {
		originals.Close()
		return e
	}
	originals.Close()
	derivatives, e := s.DB.Query(ctx, `SELECT d.object_key,d.tenant_id,d.artifact_id FROM artifact_derivatives d JOIN artifact_retention r ON r.tenant_id=d.tenant_id AND r.artifact_id=d.artifact_id WHERE r.retain_derivatives_until<$1`, now)
	if e != nil {
		return e
	}
	for derivatives.Next() {
		var key string
		var tenant, id uuid.UUID
		if e = derivatives.Scan(&key, &tenant, &id); e != nil {
			derivatives.Close()
			return e
		}
		if e = s.Artifacts.Delete(ctx, key); e != nil {
			derivatives.Close()
			return e
		}
		if _, e = s.DB.Exec(ctx, `DELETE FROM artifact_derivatives WHERE tenant_id=$1 AND artifact_id=$2`, tenant, id); e != nil {
			derivatives.Close()
			return e
		}
	}
	return derivatives.Err()
}
