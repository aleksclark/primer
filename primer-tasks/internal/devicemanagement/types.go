package devicemanagement

import (
	"crypto/ed25519"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StudentPackageName  = "com.aleksclark.primer.student"
	EnrollmentQRScheme  = "primer-management"
	EnrollmentQRVersion = "v1"
)

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
	PublicOrigin          string
	BasePath              string
	ArtifactDir           string
	ReleasePublisherToken string
	ReleaseSigningKey     ed25519.PrivateKey
	MaxArtifactBytes      int64
}

type Enrollment struct {
	ID                 string    `json:"id" format:"uuid"`
	Code               string    `json:"code" minLength:"32" maxLength:"32"`
	ExpiresAt          time.Time `json:"expiresAt" format:"date-time"`
	QRPayload          string    `json:"qrPayload"`
	BaseURL            string    `json:"baseUrl" format:"uri"`
	ResponseLossPolicy string    `json:"responseLossPolicy" enum:"fresh-parent-enrollment"`
}

type IssueEnrollmentInput struct {
	Label            string `json:"label,omitempty" maxLength:"80"`
	ExpiresInMinutes int    `json:"expiresInMinutes,omitempty" minimum:"1" maximum:"60"`
}

type DeviceCapabilities struct {
	AndroidAPI        int      `json:"androidApi,omitempty" minimum:"28" maximum:"36"`
	SupportedABIs     []string `json:"supportedAbis,omitempty" maxItems:"8"`
	DeviceOwner       bool     `json:"deviceOwner,omitempty"`
	LockTaskSupported bool     `json:"lockTaskSupported,omitempty"`
}

type EnrollInput struct {
	Code             string             `json:"code" minLength:"32" maxLength:"32"`
	DeviceName       string             `json:"deviceName" minLength:"1" maxLength:"80"`
	DeviceModel      string             `json:"deviceModel,omitempty" maxLength:"80"`
	StableDeviceKey  string             `json:"stableDeviceKey,omitempty" minLength:"16" maxLength:"128"`
	EnrollmentKeyID  string             `json:"enrollmentKeyId,omitempty" minLength:"8" maxLength:"64"`
	EnrollmentPubKey string             `json:"enrollmentPublicKey,omitempty" minLength:"32" maxLength:"128"`
	Capabilities     DeviceCapabilities `json:"capabilities,omitempty"`
}

type EnrollResult struct {
	Token              string       `json:"token"`
	Device             Device       `json:"device"`
	Desired            DesiredState `json:"desired"`
	ResponseLossPolicy string       `json:"responseLossPolicy" enum:"fresh-parent-enrollment"`
}

type DeviceState string

const (
	DeviceActive         DeviceState = "active"
	DeviceQuarantined    DeviceState = "quarantined"
	DeviceRevoked        DeviceState = "revoked"
	DeviceDecommissioned DeviceState = "decommissioned"
)

type Device struct {
	ID                  string        `json:"id" format:"uuid"`
	DisplayName         string        `json:"displayName"`
	DeviceModel         string        `json:"deviceModel"`
	State               DeviceState   `json:"state" enum:"active,quarantined,revoked,decommissioned"`
	DesiredRevision     int64         `json:"desiredRevision" minimum:"0"`
	AppliedRevision     int64         `json:"appliedRevision" minimum:"0"`
	LastSeenAt          *time.Time    `json:"lastSeenAt,omitempty" format:"date-time"`
	CreatedAt           time.Time     `json:"createdAt" format:"date-time"`
	LatestReport        *PolicyReport `json:"latestReport,omitempty"`
	EnrollmentPublicKey string        `json:"enrollmentPublicKey,omitempty"`
	EnrollmentKeyID     string        `json:"enrollmentKeyId,omitempty"`
}

type DevicePage struct {
	Items []Device `json:"items" nullable:"false"`
}

type ApprovedApp struct {
	PackageName  string `json:"packageName" minLength:"3" maxLength:"255" pattern:"^[a-zA-Z][a-zA-Z0-9_]*(\\.[a-zA-Z][a-zA-Z0-9_]*)+$"`
	Label        string `json:"label,omitempty" maxLength:"80"`
	SignerSHA256 string `json:"signerSha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	Required     bool   `json:"required,omitempty"`
}

type UserRestriction string

const (
	RestrictionNoFactoryReset UserRestriction = "no_factory_reset"
	RestrictionNoAddUser      UserRestriction = "no_add_user"
	RestrictionNoSafeBoot     UserRestriction = "no_safe_boot"
	RestrictionNoInstallApps  UserRestriction = "no_install_unknown_sources"
)

type LockTaskPolicy struct {
	Enabled        bool     `json:"enabled"`
	Packages       []string `json:"packages,omitempty" maxItems:"32"`
	AllowStatusBar bool     `json:"allowStatusBar,omitempty"`
	AllowKeyguard  bool     `json:"allowKeyguard,omitempty"`
	AllowOverview  bool     `json:"allowOverview,omitempty"`
}

type MaintenancePolicy struct {
	AllowParentUnlock bool `json:"allowParentUnlock"`
}

type Policy struct {
	ApprovedApps     []ApprovedApp     `json:"approvedApps" minItems:"1" maxItems:"64" nullable:"false"`
	RequiredPackages []ApprovedApp     `json:"requiredPackages,omitempty" maxItems:"16" nullable:"false"`
	UserRestrictions []UserRestriction `json:"userRestrictions,omitempty" maxItems:"16" nullable:"false" enum:"no_factory_reset,no_add_user,no_safe_boot,no_install_unknown_sources"`
	LockTask         LockTaskPolicy    `json:"lockTask"`
	Maintenance      MaintenancePolicy `json:"maintenance"`
}

type PolicyUpdateInput struct {
	BaseRevision int64  `json:"baseRevision" minimum:"0"`
	Policy       Policy `json:"policy"`
}

type PolicyRevision struct {
	ID        string    `json:"id" format:"uuid"`
	DeviceID  string    `json:"deviceId" format:"uuid"`
	Revision  int64     `json:"revision" minimum:"1"`
	Policy    Policy    `json:"policy"`
	CreatedAt time.Time `json:"createdAt" format:"date-time"`
}

type ControlStatus string

const (
	ControlRequested   ControlStatus = "requested"
	ControlApplied     ControlStatus = "applied"
	ControlPartial     ControlStatus = "partial"
	ControlFailed      ControlStatus = "failed"
	ControlUnsupported ControlStatus = "unsupported"
)

type ControlResult struct {
	Name    string        `json:"name" minLength:"1" maxLength:"80"`
	Desired string        `json:"desired" maxLength:"200"`
	Actual  string        `json:"actual" maxLength:"200"`
	Status  ControlStatus `json:"status" enum:"requested,applied,partial,failed,unsupported"`
	Error   string        `json:"error,omitempty" maxLength:"200"`
}

type PolicyReportStatus string

const (
	ReportRequested PolicyReportStatus = "requested"
	ReportApplied   PolicyReportStatus = "applied"
	ReportPartial   PolicyReportStatus = "partial"
	ReportFailed    PolicyReportStatus = "failed"
	ReportStale     PolicyReportStatus = "stale"
)

type PolicyReportInput struct {
	ReportID                string             `json:"reportId" format:"uuid"`
	PolicyRevision          int64              `json:"policyRevision" minimum:"0"`
	Status                  PolicyReportStatus `json:"status" enum:"requested,applied,partial,failed,stale"`
	InstalledStudentVersion string             `json:"installedStudentVersion,omitempty" maxLength:"32"`
	DeviceReportedAt        *time.Time         `json:"deviceReportedAt,omitempty" format:"date-time"`
	Controls                []ControlResult    `json:"controls,omitempty" maxItems:"64" nullable:"false"`
}

type PolicyReport struct {
	ID                      string             `json:"id" format:"uuid"`
	ReportID                string             `json:"reportId" format:"uuid"`
	DeviceID                string             `json:"deviceId" format:"uuid"`
	PolicyRevision          int64              `json:"policyRevision" minimum:"0"`
	Status                  PolicyReportStatus `json:"status" enum:"requested,applied,partial,failed,stale"`
	Stale                   bool               `json:"stale"`
	InstalledStudentVersion string             `json:"installedStudentVersion,omitempty"`
	Controls                []ControlResult    `json:"controls,omitempty" nullable:"false"`
	ReceivedAt              time.Time          `json:"receivedAt" format:"date-time"`
}

type RecoveryKind string

const (
	RecoveryMaintenanceLease RecoveryKind = "maintenance_lease"
	RecoveryRotateCode       RecoveryKind = "rotate_recovery_code"
)

// RecoveryEnvelope is a provisional opaque holder. Native encryption is not
// specified here; a later Tink/HPKE profile will replace alg/nonce/ciphertext
// semantics without changing this route. Do not treat this as a complete AEAD
// protocol: there is no sender ephemeral key, KDF, AAD, or intent-id binding yet.
type RecoveryEnvelope struct {
	KeyID      string `json:"keyId" minLength:"8" maxLength:"64"`
	Alg        string `json:"alg" enum:"X25519-ChaCha20Poly1305"`
	Nonce      string `json:"nonce" minLength:"16" maxLength:"64"`
	Ciphertext string `json:"ciphertext" minLength:"16" maxLength:"4096"`
}

type RecoveryIntentInput struct {
	Kind                   RecoveryKind      `json:"kind" enum:"maintenance_lease,rotate_recovery_code"`
	DeliveryExpiresMinutes int               `json:"deliveryExpiresMinutes,omitempty" minimum:"1" maximum:"60"`
	LeaseExpiresMinutes    int               `json:"leaseExpiresMinutes,omitempty" minimum:"1" maximum:"30"`
	Envelope               *RecoveryEnvelope `json:"envelope,omitempty"`
	ParentAcknowledged     bool              `json:"parentAcknowledged,omitempty"`
}

type RecoveryIntent struct {
	ID                 string            `json:"id" format:"uuid"`
	DeviceID           string            `json:"deviceId" format:"uuid"`
	Kind               RecoveryKind      `json:"kind" enum:"maintenance_lease,rotate_recovery_code"`
	Status             string            `json:"status" enum:"pending,applied,expired,revoked"`
	DeliveryExpiresAt  time.Time         `json:"deliveryExpiresAt" format:"date-time"`
	LeaseExpiresAt     *time.Time        `json:"leaseExpiresAt,omitempty" format:"date-time"`
	Envelope           *RecoveryEnvelope `json:"envelope,omitempty"`
	ParentAcknowledged bool              `json:"parentAcknowledged"`
	CreatedAt          time.Time         `json:"createdAt" format:"date-time"`
}

type RecoveryConfirmInput struct {
	ReportID string `json:"reportId" format:"uuid"`
}

type DesiredState struct {
	Device         Device           `json:"device"`
	PolicyRevision *PolicyRevision  `json:"policyRevision,omitempty"`
	Recovery       []RecoveryIntent `json:"recovery" nullable:"false"`
	ReleaseTargets []ReleaseTarget  `json:"releaseTargets" nullable:"false"`
}

type StateChangeInput struct {
	Reason string `json:"reason,omitempty" maxLength:"200"`
}

type ReleaseTarget struct {
	ID            string `json:"id" format:"uuid"`
	ReleaseID     string `json:"releaseId" format:"uuid"`
	PackageName   string `json:"packageName"`
	Channel       string `json:"channel"`
	Status        string `json:"status" enum:"queued,downloading,verifying,installing,confirmed,blocked,failed"`
	VersionCode   int64  `json:"versionCode" minimum:"1"`
	VersionName   string `json:"versionName"`
	SHA256        string `json:"sha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	ByteSize      int64  `json:"byteSize" minimum:"1"`
	TargetVersion int64  `json:"targetVersion" minimum:"1"`
	MinSdk        int    `json:"minSdk,omitempty"`
	SignerSHA256  string `json:"signerSha256,omitempty" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
}
