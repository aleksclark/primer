package devicemanagement

import (
	"crypto/ed25519"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const StudentPackageName = "com.aleksclark.primer.student"

type Scope struct {
	TenantID string
	ActorRef string
}

type DeviceScope struct {
	TenantID string
	DeviceID string
}

type Service struct {
	DB                    *pgxpool.Pool
	Now                   func() time.Time
	ArtifactDir           string
	ReleasePublisherToken string
	ReleaseSigningKey     ed25519.PrivateKey
	MaxArtifactBytes      int64
}

type Enrollment struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt" format:"date-time"`
	QRPayload string    `json:"qrPayload"`
}

type IssueEnrollmentInput struct {
	Label            string `json:"label,omitempty"`
	ExpiresInMinutes int    `json:"expiresInMinutes,omitempty"`
}

type EnrollInput struct {
	Code            string         `json:"code"`
	DeviceName      string         `json:"deviceName"`
	DeviceModel     string         `json:"deviceModel,omitempty"`
	StableDeviceKey string         `json:"stableDeviceKey,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
}

type EnrollResult struct {
	Token   string       `json:"token"`
	Device  Device       `json:"device"`
	Desired DesiredState `json:"desired"`
}

type Device struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"displayName"`
	DeviceModel     string     `json:"deviceModel"`
	State           string     `json:"state"`
	DesiredRevision int64      `json:"desiredRevision"`
	AppliedRevision int64      `json:"appliedRevision"`
	LastSeenAt      *time.Time `json:"lastSeenAt,omitempty" format:"date-time"`
	CreatedAt       time.Time  `json:"createdAt" format:"date-time"`
}

type DevicePage struct {
	Items []Device `json:"items" nullable:"false"`
}

type ApprovedApp struct {
	PackageName  string `json:"packageName"`
	Label        string `json:"label,omitempty"`
	SignerSHA256 string `json:"signerSha256"`
	Required     bool   `json:"required,omitempty"`
}

type Policy struct {
	ApprovedApps     []ApprovedApp  `json:"approvedApps" nullable:"false"`
	RequiredPackages []ApprovedApp  `json:"requiredPackages,omitempty" nullable:"false"`
	UserRestrictions []string       `json:"userRestrictions,omitempty" nullable:"false"`
	LockTask         map[string]any `json:"lockTask,omitempty"`
	Maintenance      map[string]any `json:"maintenance,omitempty"`
}

type PolicyUpdateInput struct {
	BaseRevision int64  `json:"baseRevision"`
	Policy       Policy `json:"policy"`
}

type PolicyRevision struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"deviceId"`
	Revision  int64     `json:"revision"`
	Policy    Policy    `json:"policy"`
	CreatedAt time.Time `json:"createdAt" format:"date-time"`
}

type ControlResult struct {
	Name    string `json:"name"`
	Desired string `json:"desired"`
	Actual  string `json:"actual"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type PolicyReportInput struct {
	ReportID                string          `json:"reportId"`
	PolicyRevision          int64           `json:"policyRevision"`
	Status                  string          `json:"status"`
	InstalledStudentVersion string          `json:"installedStudentVersion,omitempty"`
	DeviceReportedAt        *time.Time      `json:"deviceReportedAt,omitempty" format:"date-time"`
	Controls                []ControlResult `json:"controls,omitempty" nullable:"false"`
}

type PolicyReport struct {
	ID             string    `json:"id"`
	ReportID       string    `json:"reportId"`
	DeviceID       string    `json:"deviceId"`
	PolicyRevision int64     `json:"policyRevision"`
	Status         string    `json:"status"`
	Stale          bool      `json:"stale"`
	ReceivedAt     time.Time `json:"receivedAt" format:"date-time"`
}

type RecoveryIntentInput struct {
	Kind               string `json:"kind"`
	ExpiresInMinutes   int    `json:"expiresInMinutes,omitempty"`
	MaterialCiphertext string `json:"materialCiphertext,omitempty"`
}

type RecoveryIntent struct {
	ID                 string    `json:"id"`
	DeviceID           string    `json:"deviceId"`
	Kind               string    `json:"kind"`
	Status             string    `json:"status"`
	ExpiresAt          time.Time `json:"expiresAt" format:"date-time"`
	MaterialCiphertext string    `json:"materialCiphertext,omitempty"`
	CreatedAt          time.Time `json:"createdAt" format:"date-time"`
}

type RecoveryConfirmInput struct {
	ReportID string `json:"reportId"`
}

type DesiredState struct {
	Device         Device           `json:"device"`
	PolicyRevision *PolicyRevision  `json:"policyRevision,omitempty"`
	Recovery       []RecoveryIntent `json:"recovery" nullable:"false"`
	ReleaseTargets []ReleaseTarget  `json:"releaseTargets" nullable:"false"`
}

type StateChangeInput struct {
	Reason string `json:"reason,omitempty"`
}

// ReleaseTarget is the durable per-device rollout intent. Phase 4 returns an
// empty list; Phase 5 fills it from management_release_targets.
type ReleaseTarget struct {
	ID            string `json:"id"`
	ReleaseID     string `json:"releaseId"`
	PackageName   string `json:"packageName"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	VersionCode   int64  `json:"versionCode"`
	VersionName   string `json:"versionName"`
	SHA256        string `json:"sha256"`
	ByteSize      int64  `json:"byteSize"`
	TargetVersion int64  `json:"targetVersion"`
	MinSdk        int    `json:"minSdk,omitempty"`
	SignerSHA256  string `json:"signerSha256,omitempty"`
}
