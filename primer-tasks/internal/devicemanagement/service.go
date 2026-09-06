package devicemanagement

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrGone         = errors.New("gone")
	ErrConflict     = errors.New("conflict")
	ErrInvalid      = errors.New("invalid")
	ErrUnavailable  = errors.New("unavailable")
)

var packageNameRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)

func (s *Service) pool() *pgxpool.Pool {
	return s.DB
}

func (s *Service) listReleaseTargets(ctx context.Context, tenantID, deviceID string) ([]ReleaseTarget, error) {
	return []ReleaseTarget{}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func secretHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

func secretToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashSecret(v string) []byte { h := sha256.Sum256([]byte(v)); return h[:] }

func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.DisplayName, &d.DeviceModel, &d.State, &d.DesiredRevision, &d.AppliedRevision, &d.LastSeenAt, &d.CreatedAt)
	return d, err
}

func (s *Service) IssueEnrollment(ctx context.Context, sc Scope, in IssueEnrollmentInput) (Enrollment, error) {
	p := s.pool()
	if p == nil {
		return Enrollment{}, ErrUnavailable
	}
	mins := in.ExpiresInMinutes
	if mins <= 0 {
		mins = 15
	}
	if mins > 60 {
		return Enrollment{}, fmt.Errorf("%w: enrollment expiry exceeds one hour", ErrInvalid)
	}
	code, err := secretHex(16)
	if err != nil {
		return Enrollment{}, ErrUnavailable
	}
	id := uuid.New()
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return Enrollment{}, err
	}
	expires := s.now().Add(time.Duration(mins) * time.Minute)
	if _, err = p.Exec(ctx, `INSERT INTO management_enrollment_codes(id,tenant_id,created_by,label,code_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, id, tenantID, sc.ActorRef, strings.TrimSpace(in.Label), hashSecret(code), expires); err != nil {
		return Enrollment{}, err
	}
	_ = s.audit(ctx, sc.TenantID, "", "parent", sc.ActorRef, "management.enrollment_issued", map[string]any{"enrollmentId": id.String()})
	return Enrollment{ID: id.String(), Code: code, ExpiresAt: expires, QRPayload: "primer-management:v1:" + code}, nil
}

func (s *Service) Enroll(ctx context.Context, in EnrollInput) (EnrollResult, error) {
	p := s.pool()
	if p == nil {
		return EnrollResult{}, ErrUnavailable
	}
	if strings.TrimSpace(in.Code) == "" || strings.TrimSpace(in.DeviceName) == "" {
		return EnrollResult{}, fmt.Errorf("%w: code and deviceName are required", ErrInvalid)
	}
	token, err := secretToken()
	if err != nil {
		return EnrollResult{}, ErrUnavailable
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		return EnrollResult{}, err
	}
	defer tx.Rollback(ctx)
	var enrollmentID, tenantID, label string
	var expires time.Time
	var consumedAt, revokedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id,tenant_id,label,expires_at,consumed_at,revoked_at FROM management_enrollment_codes WHERE code_hash=$1 FOR UPDATE`, hashSecret(strings.TrimSpace(in.Code))).Scan(&enrollmentID, &tenantID, &label, &expires, &consumedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EnrollResult{}, ErrGone
	}
	if err != nil {
		return EnrollResult{}, err
	}
	if revokedAt != nil || consumedAt != nil || !expires.After(s.now()) {
		return EnrollResult{}, ErrGone
	}
	name := strings.TrimSpace(in.DeviceName)
	if name == "" && label != "" {
		name = label
	}
	deviceID := uuid.New()
	credentialID := uuid.New()
	enrollmentUUID, err := uuid.Parse(enrollmentID)
	if err != nil {
		return EnrollResult{}, err
	}
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return EnrollResult{}, err
	}
	caps := in.Capabilities
	if caps == nil {
		caps = map[string]any{}
	}
	var stableHash any
	if strings.TrimSpace(in.StableDeviceKey) != "" {
		stableHash = hashSecret(strings.TrimSpace(in.StableDeviceKey))
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_devices(id,tenant_id,enrollment_id,display_name,device_model,stable_device_key_hash,capabilities,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$7,now())`, deviceID, tenantUUID, enrollmentUUID, name, strings.TrimSpace(in.DeviceModel), stableHash, jsonBytes(caps)); err != nil {
		return EnrollResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_device_credentials(id,tenant_id,device_id,token_hash) VALUES($1,$2,$3,$4)`, credentialID, tenantUUID, deviceID, hashSecret(token)); err != nil {
		return EnrollResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_enrollment_codes SET consumed_at=now(), consumed_device_id=$1 WHERE tenant_id=$2 AND id=$3`, deviceID, tenantUUID, enrollmentUUID); err != nil {
		return EnrollResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_audit_records(tenant_id,device_id,actor_kind,actor_ref,action,metadata) VALUES($1,$2,'management_device',$3,'management.device_enrolled',$4)`, tenantUUID, deviceID, deviceID.String(), jsonBytes(map[string]any{"enrollmentId": enrollmentID})); err != nil {
		return EnrollResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EnrollResult{}, err
	}
	d, err := s.GetDevice(ctx, Scope{TenantID: tenantID, ActorRef: "system"}, deviceID.String())
	if err != nil {
		return EnrollResult{}, err
	}
	desired, err := s.DesiredState(ctx, DeviceScope{TenantID: tenantID, DeviceID: deviceID.String()})
	if err != nil {
		return EnrollResult{}, err
	}
	return EnrollResult{Token: token, Device: d, Desired: desired}, nil
}

func (s *Service) AuthenticateDevice(ctx context.Context, bearer string) (DeviceScope, error) {
	p := s.pool()
	if p == nil || strings.TrimSpace(bearer) == "" {
		return DeviceScope{}, ErrUnauthorized
	}
	var sc DeviceScope
	err := p.QueryRow(ctx, `SELECT c.tenant_id,c.device_id FROM management_device_credentials c JOIN management_devices d ON d.tenant_id=c.tenant_id AND d.id=c.device_id WHERE c.token_hash=$1 AND c.revoked_at IS NULL AND d.revoked_at IS NULL AND d.state='active'`, hashSecret(strings.TrimSpace(bearer))).Scan(&sc.TenantID, &sc.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceScope{}, ErrUnauthorized
	}
	if err != nil {
		return DeviceScope{}, err
	}
	_, _ = p.Exec(ctx, `UPDATE management_device_credentials SET last_used_at=now() WHERE token_hash=$1`, hashSecret(strings.TrimSpace(bearer)))
	_, _ = p.Exec(ctx, `UPDATE management_devices SET last_seen_at=now(), updated_at=now() WHERE tenant_id=$1 AND id=$2`, sc.TenantID, sc.DeviceID)
	return sc, nil
}

func (s *Service) ListDevices(ctx context.Context, sc Scope) ([]Device, error) {
	rows, err := s.pool().Query(ctx, `SELECT id,display_name,device_model,state,desired_revision,applied_revision,last_seen_at,created_at FROM management_devices WHERE tenant_id=$1 ORDER BY created_at DESC`, sc.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) GetDevice(ctx context.Context, sc Scope, id string) (Device, error) {
	d, err := scanDevice(s.pool().QueryRow(ctx, `SELECT id,display_name,device_model,state,desired_revision,applied_revision,last_seen_at,created_at FROM management_devices WHERE tenant_id=$1 AND id=$2`, sc.TenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (s *Service) UpdatePolicy(ctx context.Context, sc Scope, deviceID string, in PolicyUpdateInput) (PolicyRevision, error) {
	if err := ValidatePolicy(in.Policy); err != nil {
		return PolicyRevision{}, err
	}
	p := s.pool()
	tx, err := p.Begin(ctx)
	if err != nil {
		return PolicyRevision{}, err
	}
	defer tx.Rollback(ctx)
	var current int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT desired_revision,state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.TenantID, deviceID).Scan(&current, &state); errors.Is(err, pgx.ErrNoRows) {
		return PolicyRevision{}, ErrNotFound
	} else if err != nil {
		return PolicyRevision{}, err
	}
	if state != "active" {
		return PolicyRevision{}, ErrForbidden
	}
	if current != in.BaseRevision {
		return PolicyRevision{}, fmt.Errorf("%w: stale policy base revision", ErrConflict)
	}
	rev := current + 1
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO management_policy_revisions(id,tenant_id,device_id,revision,created_by,policy) VALUES($1,$2,$3,$4,$5,$6)`, id, sc.TenantID, deviceID, rev, sc.ActorRef, jsonBytes(in.Policy)); err != nil {
		return PolicyRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_devices SET desired_revision=$1,updated_at=now() WHERE tenant_id=$2 AND id=$3`, rev, sc.TenantID, deviceID); err != nil {
		return PolicyRevision{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_audit_records(tenant_id,device_id,actor_kind,actor_ref,action,metadata) VALUES($1,$2,'parent',$3,'management.policy_revised',$4)`, sc.TenantID, deviceID, sc.ActorRef, jsonBytes(map[string]any{"revision": rev})); err != nil {
		return PolicyRevision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PolicyRevision{}, err
	}
	return PolicyRevision{ID: id, DeviceID: deviceID, Revision: rev, Policy: in.Policy, CreatedAt: s.now()}, nil
}

func ValidatePolicy(p Policy) error {
	if len(p.ApprovedApps) == 0 {
		return fmt.Errorf("%w: at least Student must be approved", ErrInvalid)
	}
	seenStudent := false
	for _, app := range append(append([]ApprovedApp{}, p.ApprovedApps...), p.RequiredPackages...) {
		pkg := strings.TrimSpace(app.PackageName)
		if !packageNameRE.MatchString(pkg) {
			return fmt.Errorf("%w: invalid package name", ErrInvalid)
		}
		if strings.TrimSpace(app.SignerSHA256) == "" || strings.Contains(app.SignerSHA256, "*") {
			return fmt.Errorf("%w: signer pin is required", ErrInvalid)
		}
		if strings.EqualFold(pkg, StudentPackageName) {
			seenStudent = true
		}
	}
	if !seenStudent {
		return fmt.Errorf("%w: policy must retain Student", ErrInvalid)
	}
	return nil
}

func (s *Service) LatestPolicy(ctx context.Context, tenantID, deviceID string) (*PolicyRevision, error) {
	var pr PolicyRevision
	var raw []byte
	err := s.pool().QueryRow(ctx, `SELECT id,device_id,revision,policy,created_at FROM management_policy_revisions WHERE tenant_id=$1 AND device_id=$2 ORDER BY revision DESC LIMIT 1`, tenantID, deviceID).Scan(&pr.ID, &pr.DeviceID, &pr.Revision, &raw, &pr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &pr.Policy); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (s *Service) DesiredState(ctx context.Context, sc DeviceScope) (DesiredState, error) {
	d, err := s.GetDevice(ctx, Scope{TenantID: sc.TenantID, ActorRef: "device"}, sc.DeviceID)
	if err != nil {
		return DesiredState{}, err
	}
	policy, err := s.LatestPolicy(ctx, sc.TenantID, sc.DeviceID)
	if err != nil {
		return DesiredState{}, err
	}
	recovery, err := s.PendingRecovery(ctx, sc.TenantID, sc.DeviceID)
	if err != nil {
		return DesiredState{}, err
	}
	targets, err := s.listReleaseTargets(ctx, sc.TenantID, sc.DeviceID)
	if err != nil {
		return DesiredState{}, err
	}
	return DesiredState{Device: d, PolicyRevision: policy, Recovery: recovery, ReleaseTargets: targets}, nil
}

func (s *Service) ReportPolicy(ctx context.Context, sc DeviceScope, in PolicyReportInput) (PolicyReport, error) {
	if _, err := uuid.Parse(in.ReportID); err != nil {
		return PolicyReport{}, fmt.Errorf("%w: reportId must be a UUID", ErrInvalid)
	}
	if in.PolicyRevision < 0 || !validPolicyStatus(in.Status) {
		return PolicyReport{}, ErrInvalid
	}
	p := s.pool()
	tx, err := p.Begin(ctx)
	if err != nil {
		return PolicyReport{}, err
	}
	defer tx.Rollback(ctx)
	var desired int64
	if err = tx.QueryRow(ctx, `SELECT desired_revision FROM management_devices WHERE tenant_id=$1 AND id=$2 AND state='active' FOR UPDATE`, sc.TenantID, sc.DeviceID).Scan(&desired); errors.Is(err, pgx.ErrNoRows) {
		return PolicyReport{}, ErrUnauthorized
	} else if err != nil {
		return PolicyReport{}, err
	}
	if in.PolicyRevision > desired {
		return PolicyReport{}, ErrConflict
	}
	var existing PolicyReport
	err = tx.QueryRow(ctx, `SELECT id,report_id,device_id,revision,status,stale,received_at FROM management_policy_reports WHERE tenant_id=$1 AND device_id=$2 AND report_id=$3`, sc.TenantID, sc.DeviceID, in.ReportID).Scan(&existing.ID, &existing.ReportID, &existing.DeviceID, &existing.PolicyRevision, &existing.Status, &existing.Stale, &existing.ReceivedAt)
	if err == nil {
		if _, err = tx.Exec(ctx, `UPDATE management_devices SET last_seen_at=now(),updated_at=now() WHERE tenant_id=$1 AND id=$2`, sc.TenantID, sc.DeviceID); err != nil {
			return PolicyReport{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return PolicyReport{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PolicyReport{}, err
	}
	stale := in.PolicyRevision < desired
	status := in.Status
	if stale {
		status = "stale"
	}
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO management_policy_reports(id,tenant_id,device_id,report_id,revision,status,installed_student_version,device_reported_at,stale,report) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, sc.TenantID, sc.DeviceID, in.ReportID, in.PolicyRevision, status, in.InstalledStudentVersion, in.DeviceReportedAt, stale, jsonBytes(in)); err != nil {
		return PolicyReport{}, err
	}
	if !stale && status == "applied" {
		if _, err = tx.Exec(ctx, `UPDATE management_devices SET applied_revision=$1,last_seen_at=now(),updated_at=now() WHERE tenant_id=$2 AND id=$3 AND applied_revision<=$1`, in.PolicyRevision, sc.TenantID, sc.DeviceID); err != nil {
			return PolicyReport{}, err
		}
	} else if _, err = tx.Exec(ctx, `UPDATE management_devices SET last_seen_at=now(),updated_at=now() WHERE tenant_id=$1 AND id=$2`, sc.TenantID, sc.DeviceID); err != nil {
		return PolicyReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PolicyReport{}, err
	}
	return s.GetPolicyReport(ctx, sc, in.ReportID)
}

func validPolicyStatus(v string) bool {
	switch v {
	case "requested", "applied", "partial", "failed", "stale":
		return true
	default:
		return false
	}
}

func (s *Service) GetPolicyReport(ctx context.Context, sc DeviceScope, reportID string) (PolicyReport, error) {
	var out PolicyReport
	err := s.pool().QueryRow(ctx, `SELECT id,report_id,device_id,revision,status,stale,received_at FROM management_policy_reports WHERE tenant_id=$1 AND device_id=$2 AND report_id=$3`, sc.TenantID, sc.DeviceID, reportID).Scan(&out.ID, &out.ReportID, &out.DeviceID, &out.PolicyRevision, &out.Status, &out.Stale, &out.ReceivedAt)
	return out, err
}

func (s *Service) ChangeDeviceState(ctx context.Context, sc Scope, deviceID, state, reason string) (Device, error) {
	if state != "quarantined" && state != "revoked" && state != "decommissioned" {
		return Device{}, ErrInvalid
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback(ctx)
	var col string
	switch state {
	case "quarantined":
		col = "quarantined_at"
	case "revoked":
		col = "revoked_at"
	case "decommissioned":
		col = "decommissioned_at"
	}
	cmd, err := tx.Exec(ctx, `UPDATE management_devices SET state=$1,`+col+`=now(),updated_at=now() WHERE tenant_id=$2 AND id=$3`, state, sc.TenantID, deviceID)
	if err != nil {
		return Device{}, err
	}
	if cmd.RowsAffected() == 0 {
		return Device{}, ErrNotFound
	}
	_, _ = tx.Exec(ctx, `UPDATE management_device_credentials SET revoked_at=now() WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, sc.TenantID, deviceID)
	_, _ = tx.Exec(ctx, `INSERT INTO management_audit_records(tenant_id,device_id,actor_kind,actor_ref,action,metadata) VALUES($1,$2,'parent',$3,$4,$5)`, sc.TenantID, deviceID, sc.ActorRef, "management.device_"+state, jsonBytes(map[string]any{"reason": reason}))
	if err = tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, sc, deviceID)
}

func (s *Service) CreateRecoveryIntent(ctx context.Context, sc Scope, deviceID string, in RecoveryIntentInput) (RecoveryIntent, error) {
	if in.Kind != "maintenance_lease" && in.Kind != "rotate_recovery_code" {
		return RecoveryIntent{}, ErrInvalid
	}
	mins := in.ExpiresInMinutes
	if mins <= 0 {
		mins = 30
	}
	if mins > 24*60 {
		return RecoveryIntent{}, fmt.Errorf("%w: recovery intent too long", ErrInvalid)
	}
	if in.Kind == "rotate_recovery_code" && strings.TrimSpace(in.MaterialCiphertext) == "" {
		return RecoveryIntent{}, fmt.Errorf("%w: staged rotation requires encrypted material", ErrInvalid)
	}
	if _, err := s.GetDevice(ctx, sc, deviceID); err != nil {
		return RecoveryIntent{}, err
	}
	id := uuid.NewString()
	expires := s.now().Add(time.Duration(mins) * time.Minute)
	var materialHash any
	if in.MaterialCiphertext != "" {
		materialHash = hashSecret(in.MaterialCiphertext)
	}
	var out RecoveryIntent
	err := s.pool().QueryRow(ctx, `INSERT INTO management_recovery_intents(id,tenant_id,device_id,kind,created_by,expires_at,material_ciphertext,material_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,device_id,kind,status,expires_at,material_ciphertext,created_at`, id, sc.TenantID, deviceID, in.Kind, sc.ActorRef, expires, in.MaterialCiphertext, materialHash).Scan(&out.ID, &out.DeviceID, &out.Kind, &out.Status, &out.ExpiresAt, &out.MaterialCiphertext, &out.CreatedAt)
	if err == nil {
		_ = s.audit(ctx, sc.TenantID, deviceID, "parent", sc.ActorRef, "management.recovery_intent_created", map[string]any{"kind": in.Kind})
	}
	return out, err
}

func (s *Service) PendingRecovery(ctx context.Context, tenantID, deviceID string) ([]RecoveryIntent, error) {
	rows, err := s.pool().Query(ctx, `SELECT id,device_id,kind,status,expires_at,material_ciphertext,created_at FROM management_recovery_intents WHERE tenant_id=$1 AND device_id=$2 AND status='pending' AND expires_at>now() ORDER BY created_at`, tenantID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecoveryIntent{}
	for rows.Next() {
		var x RecoveryIntent
		if err = rows.Scan(&x.ID, &x.DeviceID, &x.Kind, &x.Status, &x.ExpiresAt, &x.MaterialCiphertext, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) ConfirmRecoveryIntent(ctx context.Context, sc DeviceScope, intentID, reportID string) error {
	cmd, err := s.pool().Exec(ctx, `UPDATE management_recovery_intents SET status='applied',applied_report_id=$1,applied_at=now() WHERE tenant_id=$2 AND device_id=$3 AND id=$4 AND status='pending' AND expires_at>now()`, reportID, sc.TenantID, sc.DeviceID, intentID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) audit(ctx context.Context, tenantID, deviceID, actorKind, actorRef, action string, metadata map[string]any) error {
	_, err := s.pool().Exec(ctx, `INSERT INTO management_audit_records(tenant_id,device_id,actor_kind,actor_ref,action,metadata) VALUES($1,$2,$3,$4,$5,$6)`, tenantID, emptyUUIDNil(deviceID), actorKind, actorRef, action, jsonBytes(metadata))
	return err
}

func emptyUUIDNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
