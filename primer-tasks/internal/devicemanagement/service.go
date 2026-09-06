package devicemanagement

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

var (
	packageNameRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)
	hex64RE       = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

var allowedRestrictions = map[UserRestriction]struct{}{
	RestrictionNoFactoryReset: {},
	RestrictionNoAddUser:      {},
	RestrictionNoSafeBoot:     {},
	RestrictionNoInstallApps:  {},
}

type txIface interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) pool() *pgxpool.Pool { return s.DB }

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

func marshalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: encode", ErrUnavailable)
	}
	return b, nil
}

func canonicalReport(in PolicyReportInput) ([]byte, error) {
	if in.Controls == nil {
		in.Controls = []ControlResult{}
	}
	return marshalJSON(in)
}

func parseUUID(v, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(v))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s must be a UUID", ErrInvalid, name)
	}
	return id, nil
}

func enrollmentBaseURL(origin, basePath string) (string, error) {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return "", fmt.Errorf("%w: public origin is required for enrollment QR", ErrUnavailable)
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%w: public origin must be an absolute URL", ErrInvalid)
	}
	path := strings.TrimRight(basePath, "/")
	if path != "" && path != "/tasks" {
		return "", fmt.Errorf("%w: unsupported mount path", ErrInvalid)
	}
	return origin + path, nil
}

func (s *Service) qrPayload(baseURL, code string) string {
	return EnrollmentQRScheme + ":" + EnrollmentQRVersion + ":" + baseURL + "/management-device/enroll#" + code
}

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.DisplayName, &d.DeviceModel, &d.State, &d.DesiredRevision, &d.AppliedRevision, &d.LastSeenAt, &d.CreatedAt, &d.EnrollmentPublicKey, &d.EnrollmentKeyID)
	return d, err
}

func deviceSelect() string {
	return `id,display_name,device_model,state,desired_revision,applied_revision,last_seen_at,created_at,enrollment_public_key,enrollment_key_id`
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
	baseURL, err := enrollmentBaseURL(s.PublicOrigin, s.BasePath)
	if err != nil {
		return Enrollment{}, err
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
	meta, err := marshalJSON(map[string]any{"enrollmentId": id.String(), "baseUrl": baseURL})
	if err != nil {
		return Enrollment{}, err
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO management_enrollment_codes(id,tenant_id,created_by,label,code_hash,expires_at,base_url) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, tenantID, sc.ActorRef, strings.TrimSpace(in.Label), hashSecret(code), expires, baseURL); err != nil {
		return Enrollment{}, err
	}
	if err = insertAudit(ctx, tx, tenantID, nil, "parent", sc.ActorRef, "management.enrollment_issued", meta); err != nil {
		return Enrollment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Enrollment{}, err
	}
	return Enrollment{ID: id.String(), Code: code, ExpiresAt: expires, QRPayload: s.qrPayload(baseURL, code), BaseURL: baseURL, ResponseLossPolicy: "fresh-parent-enrollment"}, nil
}

func (s *Service) Enroll(ctx context.Context, in EnrollInput) (EnrollResult, error) {
	p := s.pool()
	if p == nil {
		return EnrollResult{}, ErrUnavailable
	}
	if len(strings.TrimSpace(in.Code)) != 32 || strings.TrimSpace(in.DeviceName) == "" {
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
	var enrollmentID, tenantID, label, baseURL string
	var expires time.Time
	var consumedAt, revokedAt, abandonedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id,tenant_id,label,expires_at,consumed_at,revoked_at,abandoned_at,base_url FROM management_enrollment_codes WHERE code_hash=$1 FOR UPDATE`, hashSecret(strings.TrimSpace(in.Code))).Scan(&enrollmentID, &tenantID, &label, &expires, &consumedAt, &revokedAt, &abandonedAt, &baseURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return EnrollResult{}, ErrGone
	}
	if err != nil {
		return EnrollResult{}, err
	}
	if revokedAt != nil || consumedAt != nil || abandonedAt != nil || !expires.After(s.now()) {
		return EnrollResult{}, ErrGone
	}
	name := strings.TrimSpace(in.DeviceName)
	deviceID := uuid.New()
	credentialID := uuid.New()
	enrollmentUUID := uuid.MustParse(enrollmentID)
	tenantUUID := uuid.MustParse(tenantID)
	caps, err := marshalJSON(in.Capabilities)
	if err != nil {
		return EnrollResult{}, err
	}
	var stableHash any
	if strings.TrimSpace(in.StableDeviceKey) != "" {
		stableHash = hashSecret(strings.TrimSpace(in.StableDeviceKey))
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_devices(id,tenant_id,enrollment_id,display_name,device_model,stable_device_key_hash,capabilities,enrollment_public_key,enrollment_key_id,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`, deviceID, tenantUUID, enrollmentUUID, name, strings.TrimSpace(in.DeviceModel), stableHash, caps, strings.TrimSpace(in.EnrollmentPubKey), strings.TrimSpace(in.EnrollmentKeyID)); err != nil {
		return EnrollResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_device_credentials(id,tenant_id,device_id,token_hash) VALUES($1,$2,$3,$4)`, credentialID, tenantUUID, deviceID, hashSecret(token)); err != nil {
		return EnrollResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_enrollment_codes SET consumed_at=now(), consumed_device_id=$1 WHERE tenant_id=$2 AND id=$3`, deviceID, tenantUUID, enrollmentUUID); err != nil {
		return EnrollResult{}, err
	}
	meta, err := marshalJSON(map[string]any{"enrollmentId": enrollmentID, "responseLossPolicy": "fresh-parent-enrollment"})
	if err != nil {
		return EnrollResult{}, err
	}
	if err = insertAudit(ctx, tx, tenantUUID, &deviceID, "management_device", deviceID.String(), "management.device_enrolled", meta); err != nil {
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
	return EnrollResult{Token: token, Device: d, Desired: desired, ResponseLossPolicy: "fresh-parent-enrollment"}, nil
}

func (s *Service) AbandonEnrollment(ctx context.Context, sc Scope, enrollmentID string) error {
	id, err := parseUUID(enrollmentID, "enrollmentId")
	if err != nil {
		return err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var consumedDevice *uuid.UUID
	var consumedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT consumed_device_id, consumed_at FROM management_enrollment_codes WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, id).Scan(&consumedDevice, &consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_enrollment_codes SET abandoned_at=now(), revoked_at=COALESCE(revoked_at,now()) WHERE tenant_id=$1 AND id=$2`, tenantID, id); err != nil {
		return err
	}
	if consumedDevice != nil {
		if _, err = tx.Exec(ctx, `UPDATE management_devices SET state='revoked', revoked_at=COALESCE(revoked_at,now()), updated_at=now() WHERE tenant_id=$1 AND id=$2 AND state IN ('active','quarantined')`, tenantID, *consumedDevice); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE management_device_credentials SET revoked_at=COALESCE(revoked_at,now()) WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, tenantID, *consumedDevice); err != nil {
			return err
		}
	}
	meta, err := marshalJSON(map[string]any{"enrollmentId": id.String(), "hadCredential": consumedAt != nil})
	if err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, tenantID, consumedDevice, "parent", sc.ActorRef, "management.enrollment_abandoned", meta); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AuthenticateDevice(ctx context.Context, bearer string) (DeviceScope, error) {
	p := s.pool()
	if p == nil || strings.TrimSpace(bearer) == "" {
		return DeviceScope{}, ErrUnauthorized
	}
	var sc DeviceScope
	err := p.QueryRow(ctx, `SELECT c.tenant_id,c.device_id FROM management_device_credentials c JOIN management_devices d ON d.tenant_id=c.tenant_id AND d.id=c.device_id WHERE c.token_hash=$1 AND c.revoked_at IS NULL AND d.state='active'`, hashSecret(strings.TrimSpace(bearer))).Scan(&sc.TenantID, &sc.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceScope{}, ErrUnauthorized
	}
	if err != nil {
		return DeviceScope{}, err
	}
	if _, err = p.Exec(ctx, `UPDATE management_device_credentials SET last_used_at=now() WHERE token_hash=$1`, hashSecret(strings.TrimSpace(bearer))); err != nil {
		return DeviceScope{}, err
	}
	if _, err = p.Exec(ctx, `UPDATE management_devices SET last_seen_at=now(), updated_at=now() WHERE tenant_id=$1 AND id=$2`, sc.TenantID, sc.DeviceID); err != nil {
		return DeviceScope{}, err
	}
	return sc, nil
}

func (s *Service) ListDevices(ctx context.Context, sc Scope) ([]Device, error) {
	rows, err := s.pool().Query(ctx, `SELECT `+deviceSelect()+` FROM management_devices WHERE tenant_id=$1 ORDER BY created_at DESC`, sc.TenantID)
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
		if err = s.attachLatestReport(ctx, sc.TenantID, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) GetDevice(ctx context.Context, sc Scope, id string) (Device, error) {
	if _, err := parseUUID(id, "deviceId"); err != nil {
		return Device{}, err
	}
	d, err := scanDevice(s.pool().QueryRow(ctx, `SELECT `+deviceSelect()+` FROM management_devices WHERE tenant_id=$1 AND id=$2`, sc.TenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	if err = s.attachLatestReport(ctx, sc.TenantID, &d); err != nil {
		return Device{}, err
	}
	return d, nil
}

func (s *Service) attachLatestReport(ctx context.Context, tenantID string, d *Device) error {
	rep, err := s.latestPolicyReport(ctx, tenantID, d.ID)
	if err != nil {
		return err
	}
	d.LatestReport = rep
	return nil
}

func (s *Service) UpdatePolicy(ctx context.Context, sc Scope, deviceID string, in PolicyUpdateInput) (PolicyRevision, error) {
	if err := ValidatePolicy(in.Policy); err != nil {
		return PolicyRevision{}, err
	}
	id, err := parseUUID(deviceID, "deviceId")
	if err != nil {
		return PolicyRevision{}, err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return PolicyRevision{}, err
	}
	policyJSON, err := marshalJSON(in.Policy)
	if err != nil {
		return PolicyRevision{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return PolicyRevision{}, err
	}
	defer tx.Rollback(ctx)
	var current int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT desired_revision,state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, id).Scan(&current, &state); errors.Is(err, pgx.ErrNoRows) {
		return PolicyRevision{}, ErrNotFound
	} else if err != nil {
		return PolicyRevision{}, err
	}
	if state != string(DeviceActive) {
		return PolicyRevision{}, ErrForbidden
	}
	if current != in.BaseRevision {
		return PolicyRevision{}, fmt.Errorf("%w: stale policy base revision", ErrConflict)
	}
	rev := current + 1
	revisionID := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO management_policy_revisions(id,tenant_id,device_id,revision,created_by,policy) VALUES($1,$2,$3,$4,$5,$6)`, revisionID, tenantID, id, rev, sc.ActorRef, policyJSON); err != nil {
		return PolicyRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_devices SET desired_revision=$1,updated_at=now() WHERE tenant_id=$2 AND id=$3`, rev, tenantID, id); err != nil {
		return PolicyRevision{}, err
	}
	meta, err := marshalJSON(map[string]any{"revision": rev})
	if err != nil {
		return PolicyRevision{}, err
	}
	if err = insertAudit(ctx, tx, tenantID, &id, "parent", sc.ActorRef, "management.policy_revised", meta); err != nil {
		return PolicyRevision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PolicyRevision{}, err
	}
	return PolicyRevision{ID: revisionID.String(), DeviceID: deviceID, Revision: rev, Policy: in.Policy, CreatedAt: s.now()}, nil
}

func ValidatePolicy(p Policy) error {
	if len(p.ApprovedApps) == 0 {
		return fmt.Errorf("%w: at least Student must be approved", ErrInvalid)
	}
	seen := map[string]string{}
	student := false
	check := func(app ApprovedApp, required bool) error {
		pkg := app.PackageName
		if pkg != strings.TrimSpace(pkg) || !packageNameRE.MatchString(pkg) {
			return fmt.Errorf("%w: invalid package name", ErrInvalid)
		}
		if !hex64RE.MatchString(app.SignerSHA256) {
			return fmt.Errorf("%w: signerSha256 must be 64 lowercase hex chars", ErrInvalid)
		}
		if prev, ok := seen[pkg]; ok && prev != app.SignerSHA256 {
			return fmt.Errorf("%w: conflicting signer for %s", ErrInvalid, pkg)
		}
		seen[pkg] = app.SignerSHA256
		if pkg == StudentPackageName {
			student = true
			if !required && !app.Required {
				return fmt.Errorf("%w: Student must remain required", ErrInvalid)
			}
		}
		return nil
	}
	for _, app := range p.ApprovedApps {
		if err := check(app, app.Required); err != nil {
			return err
		}
	}
	for _, app := range p.RequiredPackages {
		if err := check(app, true); err != nil {
			return err
		}
	}
	if !student {
		return fmt.Errorf("%w: policy must retain Student", ErrInvalid)
	}
	if signer, ok := seen[StudentPackageName]; !ok || signer == "" {
		return fmt.Errorf("%w: Student signer pin is required", ErrInvalid)
	}
	seenRestriction := map[UserRestriction]struct{}{}
	for _, r := range p.UserRestrictions {
		if _, ok := allowedRestrictions[r]; !ok {
			return fmt.Errorf("%w: unsupported user restriction", ErrInvalid)
		}
		if _, dup := seenRestriction[r]; dup {
			return fmt.Errorf("%w: duplicate user restriction", ErrInvalid)
		}
		seenRestriction[r] = struct{}{}
	}
	for _, pkg := range p.LockTask.Packages {
		if !packageNameRE.MatchString(pkg) {
			return fmt.Errorf("%w: invalid lock-task package", ErrInvalid)
		}
	}
	if p.LockTask.Enabled {
		found := false
		for _, pkg := range p.LockTask.Packages {
			if pkg == StudentPackageName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: lock-task must retain Student", ErrInvalid)
		}
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
	return DesiredState{Device: d, PolicyRevision: policy, Recovery: recovery, ReleaseTargets: []ReleaseTarget{}}, nil
}

func (s *Service) ReportPolicy(ctx context.Context, sc DeviceScope, in PolicyReportInput) (PolicyReport, error) {
	reportID, err := parseUUID(in.ReportID, "reportId")
	if err != nil {
		return PolicyReport{}, err
	}
	if in.PolicyRevision < 0 || !validPolicyStatus(string(in.Status)) {
		return PolicyReport{}, ErrInvalid
	}
	payload, err := canonicalReport(in)
	if err != nil {
		return PolicyReport{}, err
	}
	deviceID, err := uuid.Parse(sc.DeviceID)
	if err != nil {
		return PolicyReport{}, err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return PolicyReport{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return PolicyReport{}, err
	}
	defer tx.Rollback(ctx)
	var desired int64
	if err = tx.QueryRow(ctx, `SELECT desired_revision FROM management_devices WHERE tenant_id=$1 AND id=$2 AND state='active' FOR UPDATE`, tenantID, deviceID).Scan(&desired); errors.Is(err, pgx.ErrNoRows) {
		return PolicyReport{}, ErrUnauthorized
	} else if err != nil {
		return PolicyReport{}, err
	}
	if in.PolicyRevision > desired {
		return PolicyReport{}, ErrConflict
	}
	var existingID uuid.UUID
	var existingPayload []byte
	var existing PolicyReport
	err = tx.QueryRow(ctx, `SELECT id,report_id,device_id,revision,status,stale,installed_student_version,report,received_at FROM management_policy_reports WHERE tenant_id=$1 AND device_id=$2 AND report_id=$3`, tenantID, deviceID, reportID).Scan(&existingID, &existing.ReportID, &existing.DeviceID, &existing.PolicyRevision, &existing.Status, &existing.Stale, &existing.InstalledStudentVersion, &existingPayload, &existing.ReceivedAt)
	if err == nil {
		var stored PolicyReportInput
		if err = json.Unmarshal(existingPayload, &stored); err != nil {
			return PolicyReport{}, err
		}
		storedJSON, err := canonicalReport(stored)
		if err != nil {
			return PolicyReport{}, err
		}
		if !bytes.Equal(storedJSON, payload) {
			return PolicyReport{}, fmt.Errorf("%w: report payload changed", ErrConflict)
		}
		existing.Controls = stored.Controls
		existing.ID = existingID.String()
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
		status = ReportStale
	}
	id := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO management_policy_reports(id,tenant_id,device_id,report_id,revision,status,installed_student_version,device_reported_at,stale,report) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, tenantID, deviceID, reportID, in.PolicyRevision, status, in.InstalledStudentVersion, in.DeviceReportedAt, stale, payload); err != nil {
		return PolicyReport{}, err
	}
	if !stale && status == ReportApplied {
		if _, err = tx.Exec(ctx, `UPDATE management_devices SET applied_revision=$1,latest_report_id=$2,last_seen_at=now(),updated_at=now() WHERE tenant_id=$3 AND id=$4 AND applied_revision<=$1`, in.PolicyRevision, id, tenantID, deviceID); err != nil {
			return PolicyReport{}, err
		}
	} else if _, err = tx.Exec(ctx, `UPDATE management_devices SET latest_report_id=$1,last_seen_at=now(),updated_at=now() WHERE tenant_id=$2 AND id=$3`, id, tenantID, deviceID); err != nil {
		return PolicyReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PolicyReport{}, err
	}
	return s.GetPolicyReport(ctx, sc, in.ReportID)
}

func validPolicyStatus(v string) bool {
	switch PolicyReportStatus(v) {
	case ReportRequested, ReportApplied, ReportPartial, ReportFailed, ReportStale:
		return true
	default:
		return false
	}
}

func (s *Service) GetPolicyReport(ctx context.Context, sc DeviceScope, reportID string) (PolicyReport, error) {
	return scanPolicyReport(s.pool().QueryRow(ctx, `SELECT id,report_id,device_id,revision,status,stale,installed_student_version,report,received_at FROM management_policy_reports WHERE tenant_id=$1 AND device_id=$2 AND report_id=$3`, sc.TenantID, sc.DeviceID, reportID))
}

func (s *Service) latestPolicyReport(ctx context.Context, tenantID, deviceID string) (*PolicyReport, error) {
	rep, err := scanPolicyReport(s.pool().QueryRow(ctx, `SELECT id,report_id,device_id,revision,status,stale,installed_student_version,report,received_at FROM management_policy_reports WHERE tenant_id=$1 AND device_id=$2 ORDER BY received_at DESC LIMIT 1`, tenantID, deviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rep, nil
}

func scanPolicyReport(row pgx.Row) (PolicyReport, error) {
	var out PolicyReport
	var raw []byte
	err := row.Scan(&out.ID, &out.ReportID, &out.DeviceID, &out.PolicyRevision, &out.Status, &out.Stale, &out.InstalledStudentVersion, &raw, &out.ReceivedAt)
	if err != nil {
		return PolicyReport{}, err
	}
	var in PolicyReportInput
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &in)
		out.Controls = in.Controls
	}
	if out.Controls == nil {
		out.Controls = []ControlResult{}
	}
	return out, nil
}

func allowedTransition(from, to string) bool {
	switch from {
	case string(DeviceActive):
		return to == string(DeviceQuarantined) || to == string(DeviceRevoked)
	case string(DeviceQuarantined):
		return to == string(DeviceRevoked)
	default:
		return false
	}
}

func (s *Service) ChangeDeviceState(ctx context.Context, sc Scope, deviceID, state, reason string) (Device, error) {
	if state != string(DeviceQuarantined) && state != string(DeviceRevoked) && state != string(DeviceDecommissioned) {
		return Device{}, ErrInvalid
	}
	id, err := parseUUID(deviceID, "deviceId")
	if err != nil {
		return Device{}, err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return Device{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback(ctx)
	var current string
	if err = tx.QueryRow(ctx, `SELECT state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, id).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	} else if err != nil {
		return Device{}, err
	}
	if current == state {
		if err = tx.Commit(ctx); err != nil {
			return Device{}, err
		}
		return s.GetDevice(ctx, sc, deviceID)
	}
	if !allowedTransition(current, state) {
		return Device{}, fmt.Errorf("%w: illegal device state transition", ErrConflict)
	}
	col := "revoked_at"
	if state == string(DeviceQuarantined) {
		col = "quarantined_at"
	} else if state == string(DeviceDecommissioned) {
		col = "decommissioned_at"
	}
	if _, err = tx.Exec(ctx, `UPDATE management_devices SET state=$1,`+col+`=now(),updated_at=now() WHERE tenant_id=$2 AND id=$3`, state, tenantID, id); err != nil {
		return Device{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_device_credentials SET revoked_at=now() WHERE tenant_id=$1 AND device_id=$2 AND revoked_at IS NULL`, tenantID, id); err != nil {
		return Device{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_recovery_intents SET status='revoked' WHERE tenant_id=$1 AND device_id=$2 AND status='pending'`, tenantID, id); err != nil {
		return Device{}, err
	}
	meta, err := marshalJSON(map[string]any{"reason": reason, "from": current, "to": state})
	if err != nil {
		return Device{}, err
	}
	if err = insertAudit(ctx, tx, tenantID, &id, "parent", sc.ActorRef, "management.device_"+state, meta); err != nil {
		return Device{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, sc, deviceID)
}

func (s *Service) CreateRecoveryIntent(ctx context.Context, sc Scope, deviceID string, in RecoveryIntentInput) (RecoveryIntent, error) {
	if in.Kind != RecoveryMaintenanceLease && in.Kind != RecoveryRotateCode {
		return RecoveryIntent{}, ErrInvalid
	}
	id, err := parseUUID(deviceID, "deviceId")
	if err != nil {
		return RecoveryIntent{}, err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return RecoveryIntent{}, err
	}
	deliveryMins := in.DeliveryExpiresMinutes
	if deliveryMins <= 0 {
		deliveryMins = 15
	}
	if deliveryMins > 60 {
		return RecoveryIntent{}, fmt.Errorf("%w: delivery window exceeds one hour", ErrInvalid)
	}
	var leaseMins int
	if in.Kind == RecoveryMaintenanceLease {
		leaseMins = in.LeaseExpiresMinutes
		if leaseMins <= 0 {
			leaseMins = 15
		}
		if leaseMins > 30 {
			return RecoveryIntent{}, fmt.Errorf("%w: maintenance lease exceeds 30 minutes", ErrInvalid)
		}
	} else if in.LeaseExpiresMinutes != 0 {
		return RecoveryIntent{}, fmt.Errorf("%w: rotation has no maintenance lease", ErrInvalid)
	}
	if in.Kind == RecoveryRotateCode {
		if in.Envelope == nil || in.Envelope.Alg != "X25519-ChaCha20Poly1305" || in.Envelope.KeyID == "" || in.Envelope.Nonce == "" || in.Envelope.Ciphertext == "" {
			return RecoveryIntent{}, fmt.Errorf("%w: rotation requires X25519-ChaCha20Poly1305 envelope", ErrInvalid)
		}
		if !in.ParentAcknowledged {
			return RecoveryIntent{}, fmt.Errorf("%w: parent must acknowledge one-time custody before rotation", ErrInvalid)
		}
	} else if in.Envelope != nil {
		return RecoveryIntent{}, fmt.Errorf("%w: maintenance lease has no recovery envelope", ErrInvalid)
	}
	envelopeJSON, err := marshalJSON(in.Envelope)
	if err != nil {
		return RecoveryIntent{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return RecoveryIntent{}, err
	}
	defer tx.Rollback(ctx)
	var state, keyID string
	if err = tx.QueryRow(ctx, `SELECT state,enrollment_key_id FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, id).Scan(&state, &keyID); errors.Is(err, pgx.ErrNoRows) {
		return RecoveryIntent{}, ErrNotFound
	} else if err != nil {
		return RecoveryIntent{}, err
	}
	if state != string(DeviceActive) {
		return RecoveryIntent{}, ErrForbidden
	}
	if in.Kind == RecoveryRotateCode && keyID != "" && in.Envelope.KeyID != keyID {
		return RecoveryIntent{}, fmt.Errorf("%w: envelope keyId does not match enrolled device key", ErrInvalid)
	}
	intentID := uuid.New()
	delivery := s.now().Add(time.Duration(deliveryMins) * time.Minute)
	var lease any
	if leaseMins > 0 {
		t := s.now().Add(time.Duration(leaseMins) * time.Minute)
		lease = t
	}
	var parentAck any
	if in.ParentAcknowledged {
		parentAck = s.now()
	}
	var out RecoveryIntent
	var envelopeRaw []byte
	var leaseOut *time.Time
	err = tx.QueryRow(ctx, `INSERT INTO management_recovery_intents(id,tenant_id,device_id,kind,created_by,expires_at,delivery_expires_at,lease_expires_at,envelope,parent_acknowledged_at) VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8,$9) RETURNING id,device_id,kind,status,delivery_expires_at,lease_expires_at,envelope,parent_acknowledged_at IS NOT NULL,created_at`, intentID, tenantID, id, in.Kind, sc.ActorRef, delivery, lease, envelopeJSON, parentAck).Scan(&out.ID, &out.DeviceID, &out.Kind, &out.Status, &out.DeliveryExpiresAt, &leaseOut, &envelopeRaw, &out.ParentAcknowledged, &out.CreatedAt)
	if err != nil {
		return RecoveryIntent{}, err
	}
	out.LeaseExpiresAt = leaseOut
	if len(envelopeRaw) > 0 && string(envelopeRaw) != "null" && string(envelopeRaw) != "{}" {
		var env RecoveryEnvelope
		if err = json.Unmarshal(envelopeRaw, &env); err == nil && env.KeyID != "" {
			out.Envelope = &env
		}
	}
	meta, err := marshalJSON(map[string]any{"kind": in.Kind, "deliveryExpiresAt": delivery})
	if err != nil {
		return RecoveryIntent{}, err
	}
	if err = insertAudit(ctx, tx, tenantID, &id, "parent", sc.ActorRef, "management.recovery_intent_created", meta); err != nil {
		return RecoveryIntent{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RecoveryIntent{}, err
	}
	return out, nil
}

func (s *Service) PendingRecovery(ctx context.Context, tenantID, deviceID string) ([]RecoveryIntent, error) {
	rows, err := s.pool().Query(ctx, `SELECT id,device_id,kind,status,delivery_expires_at,lease_expires_at,envelope,parent_acknowledged_at IS NOT NULL,created_at FROM management_recovery_intents WHERE tenant_id=$1 AND device_id=$2 AND status='pending' AND delivery_expires_at>now() ORDER BY created_at`, tenantID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecoveryIntent{}
	for rows.Next() {
		var x RecoveryIntent
		var raw []byte
		if err = rows.Scan(&x.ID, &x.DeviceID, &x.Kind, &x.Status, &x.DeliveryExpiresAt, &x.LeaseExpiresAt, &raw, &x.ParentAcknowledged, &x.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 && string(raw) != "null" && string(raw) != "{}" {
			var env RecoveryEnvelope
			if json.Unmarshal(raw, &env) == nil && env.KeyID != "" {
				x.Envelope = &env
			}
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) ConfirmRecoveryIntent(ctx context.Context, sc DeviceScope, intentID, reportID string) error {
	id, err := parseUUID(intentID, "recoveryId")
	if err != nil {
		return err
	}
	rid, err := parseUUID(reportID, "reportId")
	if err != nil {
		return err
	}
	deviceID, err := uuid.Parse(sc.DeviceID)
	if err != nil {
		return err
	}
	tenantID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deviceID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthorized
	} else if err != nil {
		return err
	}
	if state != string(DeviceActive) {
		return ErrUnauthorized
	}
	var status string
	var appliedReport *uuid.UUID
	var delivery time.Time
	err = tx.QueryRow(ctx, `SELECT status,applied_report_id,delivery_expires_at FROM management_recovery_intents WHERE tenant_id=$1 AND device_id=$2 AND id=$3 FOR UPDATE`, tenantID, deviceID, id).Scan(&status, &appliedReport, &delivery)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "applied" && appliedReport != nil && *appliedReport == rid {
		return tx.Commit(ctx)
	}
	if status == "applied" {
		return fmt.Errorf("%w: recovery already confirmed with a different report", ErrConflict)
	}
	if status != "pending" || !delivery.After(s.now()) {
		return ErrNotFound
	}
	tag, err := tx.Exec(ctx, `UPDATE management_recovery_intents SET status='applied',applied_report_id=$1,applied_at=now() WHERE tenant_id=$2 AND device_id=$3 AND id=$4 AND status='pending' AND delivery_expires_at>now()`, rid, tenantID, deviceID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func insertAudit(ctx context.Context, tx txIface, tenantID uuid.UUID, deviceID *uuid.UUID, actorKind, actorRef, action string, metadata []byte) error {
	_, err := tx.Exec(ctx, `INSERT INTO management_audit_records(tenant_id,device_id,actor_kind,actor_ref,action,metadata) VALUES($1,$2,$3,$4,$5,$6)`, tenantID, deviceID, actorKind, actorRef, action, metadata)
	return err
}
