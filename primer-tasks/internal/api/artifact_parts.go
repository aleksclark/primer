package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) uploadArtifactPart(w http.ResponseWriter, r *http.Request, student uuid.UUID) {
	tenant, err := s.artifactTenant(r.Context(), student)
	if err != nil {
		problem(w, 401, "revoked", "student session required")
		return
	}
	aid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		problem(w, 404, "not_found", "artifact not found")
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "part"))
	if err != nil || n < 1 {
		problem(w, 400, "invalid_request", "part number is invalid")
		return
	}
	var finalKey string
	var total int64
	var count int
	var expires time.Time
	err = s.DB.QueryRow(r.Context(), `SELECT a.object_key,a.byte_size,u.part_count,u.expires_at FROM artifacts a JOIN artifact_upload_reservations u ON u.tenant_id=a.tenant_id AND u.artifact_id=a.id WHERE a.tenant_id=$1 AND a.student_id=$2 AND a.id=$3 AND u.status='reserved' AND u.expires_at>now()`, tenant, student, aid).Scan(&finalKey, &total, &count, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "not_found", "upload reservation not found")
		return
	}
	if err != nil {
		problem(w, 500, "internal", "unable to load reservation")
		return
	}
	if n > count {
		problem(w, 400, "invalid_request", "part number is outside reservation")
		return
	}
	if r.ContentLength < 0 || r.ContentLength == 0 {
		problem(w, 400, "invalid_request", "part size is required")
		return
	}
	partKey := fmt.Sprintf("%s.part-%d", finalKey, n)
	r.Body = http.MaxBytesReader(w, r.Body, total+1)
	obj, err := s.Artifacts.Put(r.Context(), partKey, r.Header.Get("Content-Type"), r.Body, r.ContentLength)
	if err != nil {
		problem(w, 400, "invalid_request", "part could not be stored")
		return
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO artifact_upload_parts(tenant_id,reservation_id,part_number,object_key,byte_size,uploaded_at) SELECT u.tenant_id,u.id,$3,$4,$5,now() FROM artifact_upload_reservations u WHERE u.tenant_id=$1 AND u.artifact_id=$2 AND u.status='reserved' ON CONFLICT(tenant_id,reservation_id,part_number) DO UPDATE SET object_key=EXCLUDED.object_key,byte_size=EXCLUDED.byte_size,uploaded_at=now()`, tenant, aid, n, partKey, obj.Size)
	if err != nil {
		problem(w, 500, "internal", "unable to record upload part")
		return
	}
	jsonOK(w, map[string]any{"status": "uploaded", "part": n, "size": obj.Size, "expiresAt": expires})
}
