package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/verification"
)

// lockManualDeviceAuthority is deliberately separate from browser dialogue
// authority. No device token is admitted to student REST dialogue or its socket.
// Lock order matches archive: student, then credential, then occurrence progress.
func lockManualDeviceAuthority(ctx context.Context, tx pgx.Tx, r *http.Request, student uuid.UUID) (verification.StudentAuthority, error) {
	var a verification.StudentAuthority
	a.StudentID = student.String()
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return a, verification.ErrDialogueRevoked
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" || len(token) > 512 {
		return a, verification.ErrDialogueRevoked
	}
	if err := tx.QueryRow(ctx, `SELECT tenant_id FROM students WHERE id=$1 AND archived_at IS NULL FOR SHARE`, student).Scan(&a.TenantID); err != nil {
		return a, verification.ErrDialogueRevoked
	}
	var device string
	if err := tx.QueryRow(ctx, `SELECT id FROM student_devices WHERE tenant_id=$1 AND student_id=$2 AND token_hash=$3 AND revoked_at IS NULL FOR SHARE`, a.TenantID, student, hash(token)).Scan(&device); err != nil {
		return a, verification.ErrDialogueRevoked
	}
	return a, nil
}
