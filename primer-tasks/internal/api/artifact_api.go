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
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/artifact"
	"primer-tasks/internal/artifactstore"
)

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
}
type artifactReservationOutput struct {
	ReservationID string    `json:"reservationId"`
	ArtifactID    string    `json:"artifactId"`
	UploadURL     string    `json:"uploadUrl"`
	PartCount     int       `json:"partCount"`
	ExpiresAt     time.Time `json:"expiresAt"`
}
type artifactOutput struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ContentType   string `json:"contentType"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	DurationMS    int64  `json:"durationMs,omitempty"`
	Status        string `json:"status"`
	SubmissionID  string `json:"submissionId,omitempty"`
	DerivativeURL string `json:"derivativeUrl,omitempty"`
}

func (s *Server) registerArtifactRoutes(r *chi.Mux) {
	// The occurrence-prefixed aliases are the browser contract. The shorter
	// artifact routes remain for direct bounded-streaming and multipart clients.
	r.Post("/student/occurrences/{occurrence}/artifacts/reserve", s.requireStudent(s.reserveArtifact))
	r.Post("/student/artifacts/reserve", s.requireStudent(s.reserveArtifact))
	r.Get("/student/occurrences/{occurrence}/artifacts", s.requireStudent(s.studentArtifactState))
	r.Put("/student/artifacts/{id}/upload", s.requireStudent(s.uploadArtifact))
	r.Put("/student/artifacts/{id}/parts/{part}", s.requireStudent(s.uploadArtifactPart))
	r.Post("/student/occurrences/{occurrence}/artifacts/finalize", s.requireStudent(s.finalizeArtifact))
	r.Post("/student/artifacts/{id}/finalize", s.requireStudent(s.finalizeArtifact))
	r.Post("/student/occurrences/{occurrence}/artifacts/retry", s.requireStudent(s.retryArtifactEvaluation))
	r.Get("/student/artifacts/{id}/derivative/{kind}", s.requireStudent(s.studentDerivative))
	r.Get("/parent/artifacts/{id}/original", s.requireParent(s.parentOriginal))
	r.Get("/occurrences/{occurrence}/artifacts/inspect", s.requireParent(s.parentArtifactState))
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
func mediaLimits(kind artifact.Kind, raw []byte) artifact.Limits {
	l := artifact.DefaultLimits(kind)
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	for key, dst := range map[string]*int64{"maxBytes": &l.MaxBytes, "maxDurationMs": &l.MaxDurationMS, "maxPixels": &l.MaxPixels} {
		if n, ok := c[key].(float64); ok && n > 0 {
			*dst = int64(n)
		}
	}
	if n, ok := c["maxCount"].(float64); ok && n > 0 {
		l.MaxCount = int(n)
	}
	if a, ok := c["acceptedKinds"].([]any); ok && len(a) > 0 { /* acceptedKinds is checked by caller; limits remain kind-specific */
	}
	return l
}
func uploadKey(tenant, id uuid.UUID) string {
	return fmt.Sprintf("tenants/%s/artifacts/%s/original", tenant, id)
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
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM artifact_upload_reservations WHERE tenant_id=$1 AND student_id=$2 AND occurrence_id=$3 AND requirement_id=$4 AND status IN ('reserved','finalized') AND expires_at>now()`, tenant, student, in.OccurrenceID, in.RequirementID).Scan(&prior); e != nil {
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
		existing.UploadURL = "/student/artifacts/" + existing.ArtifactID + "/upload"
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
	out := artifactReservationOutput{ReservationID: rid.String(), ArtifactID: aid.String(), UploadURL: "/student/artifacts/" + aid.String() + "/upload", PartCount: in.PartCount, ExpiresAt: expires}
	if ps, ok := s.Artifacts.(interface {
		PresignPut(context.Context, string, string, int64, time.Duration) (string, error)
	}); ok {
		if u, e := ps.PresignPut(r.Context(), uploadKey(tenant, aid), in.ContentType, in.Size, 10*time.Minute); e == nil {
			out.UploadURL = u
		}
	}
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
		jsonOK(w, map[string]string{"status": "already_uploaded"})
		return
	}
	if partCount != 1 {
		problem(w, 409, "conflict", "multipart reservation requires part endpoints")
		return
	}
	if r.ContentLength < 0 || r.ContentLength != size {
		problem(w, 400, "invalid_request", "upload size does not match reservation")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, size+1)
	obj, e := s.Artifacts.Put(r.Context(), key, r.Header.Get("Content-Type"), r.Body, size)
	if e != nil {
		problem(w, 400, "invalid_request", "upload could not be stored")
		return
	}
	_, e = s.DB.Exec(r.Context(), `UPDATE artifacts SET status='uploaded',detected_content_type=COALESCE(NULLIF($1,''),declared_content_type) WHERE tenant_id=$2 AND id=$3 AND status='reserved'`, obj.ContentType, tenant, aid)
	if e != nil {
		problem(w, 500, "internal", "unable to record upload")
		return
	}
	jsonOK(w, map[string]any{"status": "uploaded", "size": obj.Size, "expiresAt": expires})
}

func (s *Server) finalizeArtifact(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	var in artifactFinalizeInput
	if !decode(w, r, &in) {
		return
	}
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		artifactID = in.ArtifactID
	}
	aid, e := uuid.Parse(artifactID)
	if e != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	if in.OccurrenceID == "" {
		in.OccurrenceID = chi.URLParam(r, "occurrence")
	}
	if _, e = uuid.Parse(in.OccurrenceID); e != nil {
		problem(w, 400, "invalid_request", "occurrenceId is required")
		return
	}
	if in.RequirementID == "" {
		_ = s.DB.QueryRow(r.Context(), `SELECT requirement_id FROM artifact_upload_reservations WHERE tenant_id=$1 AND artifact_id=$2`, tenant, aid).Scan(&in.RequirementID)
	}
	if _, e = uuid.Parse(in.RequirementID); e != nil {
		problem(w, 400, "invalid_request", "requirementId is required")
		return
	}
	if in.SHA256 == "" {
		in.SHA256 = in.Digest
	}
	var key, kind, declared, status string
	var expected int64
	var partCount int
	var config []byte
	e = s.DB.QueryRow(r.Context(), `SELECT a.object_key,a.kind,a.declared_content_type,a.byte_size,a.status,u.part_count,vr.config FROM artifacts a JOIN artifact_upload_reservations u ON u.tenant_id=a.tenant_id AND u.artifact_id=a.id JOIN verification_requirements vr ON vr.tenant_id=u.tenant_id AND vr.id=u.requirement_id JOIN task_occurrences o ON o.tenant_id=u.tenant_id AND o.id=u.occurrence_id AND o.student_id=$2 WHERE a.tenant_id=$1 AND a.student_id=$2 AND a.id=$3 AND u.occurrence_id=$4 AND u.requirement_id=$5`, tenant, student, aid, in.OccurrenceID, in.RequirementID).Scan(&key, &kind, &declared, &expected, &status, &partCount, &config)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "artifact reservation not found")
		return
	}
	if e != nil {
		problem(w, 500, "internal", "unable to load artifact")
		return
	}
	if status == "finalized" {
		var sub string
		_ = s.DB.QueryRow(r.Context(), `SELECT id FROM artifact_submissions WHERE tenant_id=$1 AND artifact_id=$2 ORDER BY created_at DESC LIMIT 1`, tenant, aid).Scan(&sub)
		jsonStatus(w, artifactOutput{ID: aid.String(), Kind: kind, Status: status, SubmissionID: sub}, http.StatusOK)
		return
	}
	// Direct presigned PUTs do not pass through the API. Finalize therefore
	// reconciles the scoped object-store key itself; the client cannot claim
	// that an upload exists by changing metadata.
	if status != "uploaded" && status != "reserved" {
		problem(w, 409, "conflict", "artifact is not available for finalize")
		return
	}
	var occurrenceStatus string
	if e = s.DB.QueryRow(r.Context(), `SELECT status FROM task_occurrences WHERE tenant_id=$1 AND id=$2`, tenant, in.OccurrenceID).Scan(&occurrenceStatus); e != nil || occurrenceStatus == "completed" || occurrenceStatus == "canceled" {
		problem(w, 409, "conflict", "occurrence is no longer accepting evidence")
		return
	}
	if partCount > 1 {
		rows, re := s.DB.Query(r.Context(), `SELECT object_key FROM artifact_upload_parts p JOIN artifact_upload_reservations u ON u.tenant_id=p.tenant_id AND u.id=p.reservation_id WHERE p.tenant_id=$1 AND u.artifact_id=$2 ORDER BY p.part_number`, tenant, aid)
		if re != nil {
			problem(w, 500, "internal", "unable to load upload parts")
			return
		}
		var parts []string
		for rows.Next() {
			var p string
			if re = rows.Scan(&p); re != nil {
				rows.Close()
				problem(w, 500, "internal", "unable to load upload parts")
				return
			}
			parts = append(parts, p)
		}
		rows.Close()
		if len(parts) != partCount {
			problem(w, 409, "conflict", "not all upload parts are present")
			return
		}
		composer, ok := s.Artifacts.(artifactstore.Composer)
		if !ok {
			problem(w, 503, "unavailable", "multipart composition is not configured")
			return
		}
		if _, re = composer.Compose(r.Context(), key, declared, parts, expected); re != nil {
			problem(w, 400, "invalid_request", "upload parts could not be composed")
			return
		}
	}
	f, obj, e := s.Artifacts.Open(r.Context(), key)
	if e != nil {
		problem(w, 400, "invalid_request", "stored object is missing")
		return
	}
	defer f.Close()
	k, _ := parseArtifactKind(kind)
	if k != artifact.Image && in.DurationMS <= 0 {
		problem(w, 400, "invalid_request", "duration is required for audio and video")
		return
	}
	result, e := artifact.Validate(f, artifact.Input{Kind: k, DeclaredType: declared, ExpectedSize: expected, ExpectedSHA256: in.SHA256, DurationMS: in.DurationMS}, mediaLimits(k, config))
	if e != nil {
		_, _ = s.DB.Exec(r.Context(), `UPDATE artifacts SET status='rejected' WHERE tenant_id=$1 AND id=$2`, tenant, aid)
		problem(w, 400, "invalid_request", e.Error())
		return
	}
	if obj.Size != expected {
		problem(w, 400, "invalid_request", "stored object size does not match reservation")
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		problem(w, 500, "internal", "unable to finalize artifact")
		return
	}
	defer tx.Rollback(r.Context())
	var attempt uuid.UUID
	e = tx.QueryRow(r.Context(), `SELECT va.id FROM verification_attempts va WHERE va.tenant_id=$1 AND va.occurrence_id=$2 AND va.requirement_id=$3 ORDER BY va.number DESC LIMIT 1`, tenant, in.OccurrenceID, in.RequirementID).Scan(&attempt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(r.Context(), `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) SELECT $1,$2,$3,$4,COALESCE(MAX(number),0)+1 FROM verification_attempts WHERE tenant_id=$2 AND occurrence_id=$3 AND requirement_id=$4 RETURNING id`, uuid.New(), tenant, in.OccurrenceID, in.RequirementID).Scan(&attempt)
	}
	if e != nil {
		problem(w, 409, "conflict", "verification attempt is unavailable")
		return
	}
	sub := uuid.New()
	idem := in.IdempotencyKey
	if idem == "" {
		idem = result.SHA256
	}
	var existing string
	e = tx.QueryRow(r.Context(), `INSERT INTO artifact_submissions(id,tenant_id,student_id,occurrence_id,requirement_id,attempt_id,artifact_id,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'submitted') ON CONFLICT(tenant_id,student_id,idempotency_key) DO UPDATE SET id=artifact_submissions.id RETURNING id`, sub, tenant, student, in.OccurrenceID, in.RequirementID, attempt, aid, idem).Scan(&existing)
	if e != nil {
		problem(w, 409, "conflict", "submission idempotency key is already used")
		return
	}
	sub = uuid.MustParse(existing)
	_, e = tx.Exec(r.Context(), `UPDATE artifacts SET status='finalized',detected_content_type=$1,sha256=$2,width=$3,height=$4,duration_ms=$5,finalized_at=now() WHERE tenant_id=$6 AND id=$7`, result.ContentType, result.SHA256, result.Width, result.Height, result.DurationMS, tenant, aid)
	if e != nil {
		problem(w, 500, "internal", "unable to finalize artifact")
		return
	}
	_, e = tx.Exec(r.Context(), `UPDATE artifact_upload_reservations SET status='finalized' WHERE tenant_id=$1 AND artifact_id=$2`, tenant, aid)
	if e != nil {
		problem(w, 500, "internal", "unable to close reservation")
		return
	}
	_, _ = tx.Exec(r.Context(), `INSERT INTO artifact_scans(tenant_id,artifact_id,scanner,status,detail) VALUES($1,$2,'none','not_configured','no malware scanner configured') ON CONFLICT(tenant_id,artifact_id) DO NOTHING`, tenant, aid)
	_, _ = tx.Exec(r.Context(), `INSERT INTO artifact_retention(tenant_id,artifact_id,retain_original_until,retain_derivatives_until) VALUES($1,$2,now()+interval '30 days',now()+interval '180 days') ON CONFLICT DO NOTHING`, tenant, aid)
	var requirementKind string
	_ = tx.QueryRow(r.Context(), `SELECT kind FROM verification_requirements WHERE tenant_id=$1 AND id=$2`, tenant, in.RequirementID).Scan(&requirementKind)
	if requirementKind == "agent_artifact_rubric" {
		_, e = tx.Exec(r.Context(), `INSERT INTO artifact_rubric_jobs(id,tenant_id,submission_id,rubric_snapshot) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,submission_id) DO NOTHING`, uuid.New(), tenant, sub, config)
		if e != nil {
			problem(w, 500, "internal", "unable to enqueue artifact review")
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		problem(w, 500, "internal", "unable to commit artifact")
		return
	}
	out := artifactOutput{ID: aid.String(), Kind: kind, ContentType: result.ContentType, Size: result.Size, SHA256: result.SHA256, Width: result.Width, Height: result.Height, DurationMS: result.DurationMS, Status: "finalized", SubmissionID: sub.String()}
	if k == artifact.Image {
		if thumb, tw, th, te := artifact.Thumbnail(result.Bytes, 640, 640); te == nil {
			dk := fmt.Sprintf("tenants/%s/artifacts/%s/derivatives/thumbnail", tenant, aid)
			if _, pe := s.Artifacts.Put(r.Context(), dk, "image/png", bytes.NewReader(thumb), int64(len(thumb))); pe == nil {
				_, _ = s.DB.Exec(r.Context(), `INSERT INTO artifact_derivatives(id,tenant_id,artifact_id,derivative_kind,object_key,content_type,byte_size,sha256,width,height) VALUES($1,$2,$3,'thumbnail',$4,'image/png',$5,$6,$7,$8) ON CONFLICT(tenant_id,artifact_id,derivative_kind) DO NOTHING`, uuid.New(), tenant, aid, dk, len(thumb), digestBytes(thumb), tw, th)
				out.DerivativeURL = "/student/artifacts/" + aid.String() + "/derivative/thumbnail"
			}
		}
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
	e = s.DB.QueryRow(r.Context(), `SELECT d.object_key,d.content_type FROM artifact_derivatives d JOIN artifacts a ON a.tenant_id=d.tenant_id AND a.id=d.artifact_id WHERE d.tenant_id=$1 AND a.student_id=$2 AND d.artifact_id=$3 AND d.derivative_kind=$4 AND a.status='finalized'`, tenant, student, aid, chi.URLParam(r, "kind")).Scan(&key, &ct)
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

func (s *Server) artifactState(ctx context.Context, tenant uuid.UUID, occurrence string, student *uuid.UUID) (map[string]any, error) {
	var reqID uuid.UUID
	var config []byte
	var status string
	query := `SELECT vr.id,vr.config,o.status FROM task_occurrences o JOIN verification_requirements vr ON vr.tenant_id=o.tenant_id AND vr.revision_id=o.revision_id WHERE o.tenant_id=$1 AND o.id=$2 AND vr.kind='agent_artifact_rubric'`
	args := []any{tenant, occurrence}
	if student != nil {
		query += ` AND o.student_id=$3`
		args = append(args, *student)
	}
	query += ` ORDER BY vr.ordinal LIMIT 1`
	if err := s.DB.QueryRow(ctx, query, args...).Scan(&reqID, &config, &status); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT sub.id,sub.artifact_id,a.kind,a.declared_content_type,a.byte_size,a.sha256,sub.status,sub.created_at FROM artifact_submissions sub JOIN artifacts a ON a.tenant_id=sub.tenant_id AND a.id=sub.artifact_id WHERE sub.tenant_id=$1 AND sub.occurrence_id=$2 AND sub.requirement_id=$3 ORDER BY sub.created_at`, tenant, occurrence, reqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	submissions := make([]map[string]any, 0)
	latest := ""
	for rows.Next() {
		var id, aid, kind, ct, sha, subStatus string
		var size int64
		var created time.Time
		if err := rows.Scan(&id, &aid, &kind, &ct, &size, &sha, &subStatus, &created); err != nil {
			return nil, err
		}
		latest = id
		mapped := map[string]string{"submitted": "evaluating", "evaluating": "evaluating", "accepted": "complete", "rejected": "rejected", "review": "review"}[subStatus]
		if mapped == "" {
			mapped = "queued"
		}
		submissions = append(submissions, map[string]any{"id": id, "artifactId": aid, "kind": kind, "mediaType": ct, "sizeBytes": size, "digest": sha, "status": mapped, "createdAt": created})
	}
	state := "ready"
	if len(submissions) > 0 {
		state = submissions[len(submissions)-1]["status"].(string)
	}
	var cfg any
	if len(config) > 0 {
		_ = json.Unmarshal(config, &cfg)
	}
	return map[string]any{"occurrenceId": occurrence, "requirementId": reqID.String(), "rubricRevision": reqID.String(), "config": cfg, "status": state, "submissions": submissions, "activeSubmissionId": latest}, rows.Err()
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
	_, err = s.DB.Exec(r.Context(), `UPDATE artifact_rubric_jobs j SET status='queued',available_at=now(),lease_owner=NULL,lease_until=NULL,updated_at=now() FROM artifact_submissions sub WHERE sub.tenant_id=j.tenant_id AND sub.id=j.submission_id AND sub.tenant_id=$1 AND sub.occurrence_id=$2`, tenant, occurrence)
	if err != nil {
		problem(w, 500, "internal", "unable to retry artifact review")
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
	err = s.DB.QueryRow(r.Context(), `SELECT d.object_key,d.content_type FROM artifact_derivatives d JOIN artifacts a ON a.tenant_id=d.tenant_id AND a.id=d.artifact_id JOIN task_occurrences o ON o.tenant_id=a.tenant_id AND o.id=$3 WHERE d.tenant_id=$1 AND d.artifact_id=$2`, sc.Tenant, aid, chi.URLParam(r, "occurrence")).Scan(&key, &ct)
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
