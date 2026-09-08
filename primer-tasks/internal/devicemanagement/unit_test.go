package devicemanagement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrollmentBaseURLAndQR(t *testing.T) {
	if _, err := enrollmentBaseURL("", ""); err == nil {
		t.Fatal("empty origin accepted")
	}
	if _, err := enrollmentBaseURL("not-a-url", ""); err == nil {
		t.Fatal("relative origin accepted")
	}
	if _, err := enrollmentBaseURL("https://tasks.test", "/other"); err == nil {
		t.Fatal("unsupported mount accepted")
	}
	got, err := enrollmentBaseURL("https://tasks.test/", "/tasks")
	if err != nil || got != "https://tasks.test/tasks" {
		t.Fatalf("base URL = %q %v", got, err)
	}
	s := &Service{}
	if q := s.qrPayload(got, "ABC"); q != "primer-management:v1:https://tasks.test/tasks/management-device/enroll#ABC" {
		t.Fatalf("qr = %s", q)
	}
}

func TestAllowedTransitionsAndStatuses(t *testing.T) {
	if allowedTransition("active", "quarantined") != true || allowedTransition("quarantined", "revoked") != true || allowedTransition("revoked", "active") {
		t.Fatal("monotonic device transitions changed")
	}
	if !validPolicyStatus("applied") || validPolicyStatus("installed") {
		t.Fatal("policy status enum changed")
	}
	if !validReleaseStatus("queued") || !validReleaseStatus("confirmed") || validReleaseStatus("success") {
		t.Fatal("release status enum changed")
	}
}

func TestCanonicalReportAndParseUUID(t *testing.T) {
	if _, err := parseUUID("nope", "reportId"); err == nil {
		t.Fatal("invalid uuid accepted")
	}
	b, err := canonicalReport(PolicyReportInput{ReportID: "r", Status: ReportApplied})
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}

func TestParseBadgingAndSigner(t *testing.T) {
	meta, err := parseBadging("package: name='com.aleksclark.primer.student' versionCode='13' versionName='1.0'\nminSdkVersion:'28'\nnative-code: 'arm64-v8a' 'armeabi-v7a'\n")
	if err != nil || meta.PackageName != StudentPackageName || meta.VersionCode != 13 || meta.MinSdk != 28 {
		t.Fatalf("badging = %+v %v", meta, err)
	}
	if _, err = parseBadging("package: name='bad' versionCode='1'\n"); err == nil {
		t.Fatal("invalid package accepted")
	}
	sig, err := parseSignerSHA256("Signer #1 certificate SHA-256 digest: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || len(sig) != 64 {
		t.Fatalf("signer = %q %v", sig, err)
	}
	if _, err = parseSignerSHA256("no digest"); err == nil {
		t.Fatal("missing signer accepted")
	}
}

func TestInspectAPKRejectsNonAPK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.apk")
	if err := os.WriteFile(path, []byte("not an apk"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectAPK(path, 1024); err == nil {
		t.Fatal("junk apk accepted")
	}
	if _, err := InspectAPK(path, 3); err == nil {
		t.Fatal("oversized bound ignored")
	}
}

func TestToolPathAndMaxBytes(t *testing.T) {
	s := &Service{}
	if s.maxBytes() != defaultMaxArtifactBytes {
		t.Fatal("default max bytes")
	}
	s.MaxArtifactBytes = 12
	if s.maxBytes() != 12 {
		t.Fatal("override max bytes")
	}
	if p := toolPath("TASKS_AAPT2_MISSING", "aapt2"); p == "" {
		t.Fatal("tool path empty")
	}
}

func TestNowOverride(t *testing.T) {
	fixed := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	s := &Service{Now: func() time.Time { return fixed }}
	if !s.now().Equal(fixed) {
		t.Fatal("now override")
	}
}

func TestTrustKeyMissing(t *testing.T) {
	s := &Service{}
	if _, err := s.trustKey(); err == nil {
		t.Fatal("missing trust root accepted")
	}
}

func TestPublishAPKRequiresConfiguredArtifactDir(t *testing.T) {
	s := &Service{ReleaseSigningKey: make([]byte, 64)}
	ctx := context.Background()
	if _, err := s.PublishAPK(ctx, "publisher", "missing.apk", "stable"); err == nil {
		t.Fatal("missing artifact dir accepted")
	}
}

func TestEnrollmentKeyAndRecoveryEnvelope(t *testing.T) {
	if _, err := enrollmentKeyIDFromPublic("%%%%"); err == nil {
		t.Fatal("invalid keyset accepted")
	}
	pub := base64Raw("{\"primaryKeyId\":1}")
	id, err := enrollmentKeyIDFromPublic(pub)
	if err != nil || len(id) != 64 {
		t.Fatalf("key id = %q %v", id, err)
	}
	if _, err := enrollmentKeyIDFromPublic(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte("x"), 9000))); err == nil {
		t.Fatal("oversized keyset accepted")
	}
	if _, err := enrollmentKeyIDFromPublic(base64Raw("not-json")); err == nil {
		t.Fatal("non-json keyset accepted")
	}
	if err := validateRecoveryEnvelope(nil); err == nil {
		t.Fatal("nil envelope accepted")
	}
	if err := validateRecoveryEnvelope(&RecoveryEnvelope{KeyID: id, Alg: "other", Ciphertext: base64Raw("cipher-cipher-cipher-cipher")}); err == nil {
		t.Fatal("unsupported recovery alg accepted")
	}
	if err := validateRecoveryEnvelope(&RecoveryEnvelope{KeyID: "short", Alg: RecoveryHPKEAlg, Ciphertext: base64Raw("cipher-cipher-cipher-cipher")}); err == nil {
		t.Fatal("short key id accepted")
	}
	if err := validateRecoveryEnvelope(&RecoveryEnvelope{KeyID: id, Alg: RecoveryHPKEAlg, Ciphertext: base64Raw("short")}); err == nil {
		t.Fatal("short ciphertext accepted")
	}
	if err := validateRecoveryEnvelope(&RecoveryEnvelope{KeyID: id, Alg: RecoveryHPKEAlg, Ciphertext: base64Raw("cipher-cipher-cipher-cipher")}); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledAppsRequireSelfStudent(t *testing.T) {
	if err := validateInstalledApps([]InstalledApp{{PackageName: StudentPackageName, SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}); err == nil {
		t.Fatal("missing self accepted")
	}
	if err := validateInstalledApps([]InstalledApp{{PackageName: "bad", SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Self: true}}); err == nil {
		t.Fatal("invalid package accepted")
	}
	if err := validateInstalledApps([]InstalledApp{{PackageName: "com.example.tv", SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Self: true}}); err == nil {
		t.Fatal("non-student self accepted")
	}
	if err := validateInstalledApps([]InstalledApp{{PackageName: StudentPackageName, SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Self: true}}); err != nil {
		t.Fatal(err)
	}
}

func base64Raw(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
}

func TestNilDatabaseAndInvalidInputsFailClosed(t *testing.T) {
	s := &Service{}
	ctx := context.Background()
	sc := Scope{TenantID: "00000000-0000-0000-0000-0000000000a1", ActorRef: "parent-a"}
	if _, err := s.IssueEnrollment(ctx, sc, IssueEnrollmentInput{}); err == nil {
		t.Fatal("nil pool issued enrollment")
	}
	if _, err := s.IssueEnrollment(ctx, sc, IssueEnrollmentInput{ExpiresInMinutes: 61}); err == nil {
		t.Fatal("overlong enrollment accepted")
	}
	if _, err := s.Enroll(ctx, EnrollInput{}); err == nil {
		t.Fatal("nil pool enroll accepted")
	}
	if err := s.AbandonEnrollment(ctx, sc, "nope"); err == nil {
		t.Fatal("invalid enrollment id accepted")
	}
	if _, err := s.AuthenticateDevice(ctx, "token"); err == nil {
		t.Fatal("nil pool auth accepted")
	}
	if _, err := s.GetDevice(ctx, sc, "nope"); err == nil {
		t.Fatal("invalid device id accepted")
	}
	if _, err := s.UpdatePolicy(ctx, sc, "nope", PolicyUpdateInput{}); err == nil {
		t.Fatal("invalid policy device accepted")
	}
	if _, err := s.DesiredState(ctx, DeviceScope{TenantID: sc.TenantID, DeviceID: "nope"}); err == nil {
		t.Fatal("invalid desired-state device accepted")
	}
	if _, err := s.ReportPolicy(ctx, DeviceScope{TenantID: sc.TenantID, DeviceID: "nope"}, PolicyReportInput{}); err == nil {
		t.Fatal("invalid report device accepted")
	}
	if _, err := s.ChangeDeviceState(ctx, sc, "nope", "active", "reason"); err == nil {
		t.Fatal("invalid state change accepted")
	}
	if _, err := s.RecoveryHistory(ctx, sc, "nope"); err == nil {
		t.Fatal("invalid recovery history accepted")
	}
	if err := s.ConfirmRecoveryIntent(ctx, DeviceScope{TenantID: sc.TenantID, DeviceID: "nope"}, "nope", "nope"); err == nil {
		t.Fatal("invalid recovery confirm accepted")
	}
	if _, err := s.PublishAPK(ctx, "", "missing.apk", "stable"); err == nil {
		t.Fatal("empty publisher accepted")
	}
	if _, err := s.SetReleaseTarget(ctx, sc, "nope", ReleaseTargetInput{}); err == nil {
		t.Fatal("invalid release target accepted")
	}
	if _, err := s.PauseRelease(ctx, "", "nope"); err == nil {
		t.Fatal("empty pause publisher accepted")
	}
	if _, err := s.PauseRelease(ctx, "publisher", "nope"); err == nil {
		t.Fatal("invalid pause id accepted")
	}
	if _, err := s.Enroll(ctx, EnrollInput{Code: strings.Repeat("a", 32), DeviceName: "Ada"}); err == nil {
		t.Fatal("nil pool enroll with valid shape accepted")
	}
}

func TestSnapshotAndImmutablePublish(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.apk")
	data := []byte("apk-bytes-apk-bytes")
	if err := os.WriteFile(src, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotAPK(src, 3); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
	snap, err := snapshotAPK(src, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(snap)
	sum := sha256Of(data)
	dest := filepath.Join(dir, "app.apk")
	if err := publishImmutableAPK(snap, dest, sum, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	if err := publishImmutableAPK(snap, dest, sum, int64(len(data))); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.apk")
	if err := os.WriteFile(other, []byte("different-bytes-here"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishImmutableAPK(other, dest, sha256Of([]byte("different-bytes-here")), int64(len("different-bytes-here"))); err == nil {
		t.Fatal("clobber accepted")
	}
}

func sha256Of(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
