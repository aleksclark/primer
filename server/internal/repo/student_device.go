package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/authutil"
	"github.com/aleksclark/primer/server/internal/domain"
)

// PairingCodeTTL is how long a parent-issued pairing code remains valid.
const PairingCodeTTL = 15 * time.Minute

// CreatePairingCode stores a one-time pairing code for a student and returns
// the plaintext code once. Single-family: any authenticated parent may issue
// codes for any student.
func CreatePairingCode(ctx context.Context, q Querier, studentID string, createdBy *string, now time.Time) (code string, row *domain.StudentDevicePairingCode, err error) {
	code, err = authutil.NewPairingCode()
	if err != nil {
		return "", nil, err
	}
	hash := authutil.HashToken(code)
	expires := now.Add(PairingCodeTTL)
	const sqlStr = `
INSERT INTO student_device_pairing_codes (student_id, code_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, student_id, code_hash, created_by, expires_at, used_at, created_at`
	rows, err := q.Query(ctx, sqlStr, studentID, hash, createdBy, expires)
	if err != nil {
		return "", nil, fmt.Errorf("insert pairing code: %w", err)
	}
	pc, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.StudentDevicePairingCode])
	if err != nil {
		return "", nil, fmt.Errorf("scan pairing code: %w", err)
	}
	return code, &pc, nil
}

// ClaimStudentPairingCode exchanges a pairing code for a device token.
func ClaimStudentPairingCode(ctx context.Context, q Querier, code, deviceName string, now time.Time) (token string, device *domain.StudentDevice, err error) {
	token, tokenHash, err := authutil.NewToken()
	if err != nil {
		return "", nil, err
	}
	codeHash := authutil.HashToken(code)

	err = WithTx(ctx, q, func(tx Querier) error {
		const claimSQL = `
UPDATE student_device_pairing_codes
SET used_at = $2
WHERE code_hash = $1
  AND used_at IS NULL
  AND expires_at > $2
RETURNING id, student_id, code_hash, created_by, expires_at, used_at, created_at`
		rows, err := tx.Query(ctx, claimSQL, codeHash, now)
		if err != nil {
			return fmt.Errorf("claim pairing code: %w", err)
		}
		pc, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.StudentDevicePairingCode])
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("scan claimed code: %w", err)
		}

		dev, err := StudentDevices.Create(ctx, tx, map[string]any{
			"student_id":   pc.StudentID,
			"name":         deviceName,
			"token_hash":   tokenHash,
			"last_seen_at": now,
		})
		if err != nil {
			return err
		}
		device = dev
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return token, device, nil
}

// StudentDeviceByToken finds an unrevoked device by plaintext token.
func StudentDeviceByToken(ctx context.Context, q Querier, token string) (*domain.StudentDevice, error) {
	hash := authutil.HashToken(token)
	const sqlStr = `
SELECT id, student_id, name, token_hash, last_seen_at, revoked_at,
       capabilities, capabilities_reported_at, created_at, updated_at
FROM student_devices
WHERE token_hash = $1 AND token_hash <> '' AND revoked_at IS NULL
LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, hash)
	if err != nil {
		return nil, fmt.Errorf("query student device: %w", err)
	}
	dev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.StudentDevice])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan student device: %w", err)
	}
	return &dev, nil
}

// StoreDeviceCapabilities records the latest capability report from a device.
func StoreDeviceCapabilities(ctx context.Context, q Querier, deviceID string, caps map[string]any, now time.Time) error {
	if caps == nil {
		caps = map[string]any{}
	}
	raw, err := json.Marshal(caps)
	if err != nil {
		return fmt.Errorf("marshal device capabilities: %w", err)
	}
	_, err = q.Exec(ctx, `
UPDATE student_devices
SET capabilities = $2::jsonb,
    capabilities_reported_at = $3,
    last_seen_at = $3,
    updated_at = now()
WHERE id = $1`, deviceID, string(raw), now)
	if err != nil {
		return fmt.Errorf("store device capabilities: %w", err)
	}
	return nil
}

// LatestDeviceCapabilitiesForStudent returns the newest non-empty capability
// report across the student's unrevoked devices (for eligibility).
func LatestDeviceCapabilitiesForStudent(ctx context.Context, q Querier, studentID string) (map[string]any, *time.Time, error) {
	const sqlStr = `
SELECT capabilities, capabilities_reported_at
FROM student_devices
WHERE student_id = $1
  AND revoked_at IS NULL
  AND capabilities_reported_at IS NOT NULL
ORDER BY capabilities_reported_at DESC
LIMIT 1`
	var caps map[string]any
	var reported *time.Time
	err := q.QueryRow(ctx, sqlStr, studentID).Scan(&caps, &reported)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("latest device capabilities: %w", err)
	}
	return caps, reported, nil
}

// TouchStudentDevice updates last_seen_at.
func TouchStudentDevice(ctx context.Context, q Querier, deviceID string, now time.Time) error {
	_, err := q.Exec(ctx,
		`UPDATE student_devices SET last_seen_at = $2, updated_at = now() WHERE id = $1`,
		deviceID, now)
	if err != nil {
		return fmt.Errorf("touch student device: %w", err)
	}
	return nil
}

// RevokeStudentDevice marks a device revoked.
func RevokeStudentDevice(ctx context.Context, q Querier, deviceID string, now time.Time) (*domain.StudentDevice, error) {
	const sqlStr = `
UPDATE student_devices
SET revoked_at = $2, token_hash = '', updated_at = now()
WHERE id = $1 AND revoked_at IS NULL
RETURNING id, student_id, name, token_hash, last_seen_at, revoked_at,
          capabilities, capabilities_reported_at, created_at, updated_at`
	rows, err := q.Query(ctx, sqlStr, deviceID, now)
	if err != nil {
		return nil, fmt.Errorf("revoke device: %w", err)
	}
	dev, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.StudentDevice])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan revoked device: %w", err)
	}
	return &dev, nil
}

// ListStudentDevices returns all devices for the household (single-family),
// optionally filtered by student. Includes revoked devices for parent diagnostics.
func ListStudentDevices(ctx context.Context, q Querier, studentID string) ([]domain.StudentDevice, error) {
	const allSQL = `
SELECT id, student_id, name, token_hash, last_seen_at, revoked_at,
       capabilities, capabilities_reported_at, created_at, updated_at
FROM student_devices
ORDER BY last_seen_at DESC NULLS LAST, created_at DESC`
	const byStudentSQL = `
SELECT id, student_id, name, token_hash, last_seen_at, revoked_at,
       capabilities, capabilities_reported_at, created_at, updated_at
FROM student_devices
WHERE student_id = $1
ORDER BY last_seen_at DESC NULLS LAST, created_at DESC`
	var rows pgx.Rows
	var err error
	if studentID == "" {
		rows, err = q.Query(ctx, allSQL)
	} else {
		rows, err = q.Query(ctx, byStudentSQL, studentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list student devices: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.StudentDevice])
	if err != nil {
		return nil, fmt.Errorf("scan student devices: %w", err)
	}
	return items, nil
}
