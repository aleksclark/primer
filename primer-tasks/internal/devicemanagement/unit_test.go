package devicemanagement

import (
	"encoding/base64"
	"os"
	"path/filepath"
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

func TestEnrollmentKeyAndRecoveryEnvelope(t *testing.T) {
	if _, err := enrollmentKeyIDFromPublic("%%%%"); err == nil {
		t.Fatal("invalid keyset accepted")
	}
	pub := base64Raw("{\"primaryKeyId\":1}")
	id, err := enrollmentKeyIDFromPublic(pub)
	if err != nil || len(id) != 64 {
		t.Fatalf("key id = %q %v", id, err)
	}
	if err := validateRecoveryEnvelope(nil); err == nil {
		t.Fatal("nil envelope accepted")
	}
	if err := validateRecoveryEnvelope(&RecoveryEnvelope{KeyID: id, Alg: RecoveryHPKEAlg, Ciphertext: base64Raw("cipher-cipher-cipher-cipher")}); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledAppsRequireSelfStudent(t *testing.T) {
	if err := validateInstalledApps([]InstalledApp{{PackageName: StudentPackageName, SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}); err == nil {
		t.Fatal("missing self accepted")
	}
	if err := validateInstalledApps([]InstalledApp{{PackageName: StudentPackageName, SignerSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Self: true}}); err != nil {
		t.Fatal(err)
	}
}

func base64Raw(v string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(v))
}
