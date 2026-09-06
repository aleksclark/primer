package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/devicemanagement"
)

func TestReleasePublisherParentDistinctionAndInvalidArtifacts(t *testing.T) {
	if _, err := exec.LookPath("aapt2"); err != nil {
		if os.Getenv("ANDROID_HOME") != "" {
			t.Setenv("PATH", os.Getenv("ANDROID_HOME")+"/build-tools/35.0.0:"+os.Getenv("PATH"))
		}
	}
	if _, err := exec.LookPath("aapt2"); err != nil {
		t.Setenv("PATH", "/opt/android-sdk/build-tools/35.0.0:"+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath("aapt2"); err != nil || exec.Command("aapt2", "version").Run() != nil {
		t.Fatalf("aapt2 required for APK publication tests: %v", err)
	}
	if _, err := exec.LookPath("apksigner"); err != nil {
		t.Fatal("apksigner required for APK publication tests")
	}

	pool := integrationPool(t)
	_, _ = seedIntegration(t, pool)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = pub
	dir := t.TempDir()
	s := NewWithAuth(pool, "test", AuthConfig{
		SessionSecret:         []byte("release-secret"),
		IssuerSecret:          []byte("release-issuer"),
		PublicOrigin:          "https://tasks.test",
		ReleasePublisherToken: "publisher-secret",
		ReleaseSigningKey:     priv,
		ArtifactDir:           dir,
	})
	h := s.Routes()

	issue := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments", "parent-a", `{"label":"A16"}`)
	var enrollment devicemanagement.Enrollment
	if err := json.Unmarshal(issue.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	enroll := requestJSON(t, h, http.MethodPost, "/management-device/enroll", "", `{"code":"`+enrollment.Code+`","deviceName":"Student A16"}`)
	var enrolled devicemanagement.EnrollResult
	if err := json.Unmarshal(enroll.Body.Bytes(), &enrolled); err != nil || enrolled.Token == "" {
		t.Fatalf("enroll = %d %s", enroll.Code, enroll.Body.String())
	}

	apk := buildSignedAPK(t, "com.aleksclark.primer.student", 13, "1.0.13")
	rel, err := s.Management.PublishAPK(t.Context(), "operator", apk, "stable")
	if err != nil {
		t.Fatalf("publish valid apk: %v", err)
	}
	if rel.ManifestSignature == "" || rel.SHA256 == "" {
		t.Fatalf("unsigned manifest: %+v", rel)
	}

	if rec := requestJSON(t, h, http.MethodGet, "/managed-releases", "parent-a", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), rel.ID) {
		t.Fatalf("parent list releases = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/managed-releases", "parent-b", ""); rec.Code != 200 {
		t.Fatalf("parent B list = %d", rec.Code)
	}

	junk := filepath.Join(t.TempDir(), "junk.apk")
	if err := os.WriteFile(junk, []byte("not an apk"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Management.PublishAPK(t.Context(), "operator", junk, "stable"); err == nil {
		t.Fatal("invalid apk published")
	}

	unsigned := buildUnsignedAPK(t, "com.aleksclark.primer.student", 14, "1.0.14")
	if _, err := s.Management.PublishAPK(t.Context(), "operator", unsigned, "stable"); err == nil {
		t.Fatal("unsigned apk published")
	}

	untrusted := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("release-secret"), IssuerSecret: []byte("release-issuer"), ArtifactDir: dir})
	if _, err := untrusted.Management.PublishAPK(t.Context(), "operator", apk, "stable"); err == nil {
		t.Fatal("publish succeeded without trust root")
	}

	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", `{"releaseId":"`+rel.ID+`"}`); rec.Code != 200 {
		t.Fatalf("parent target = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-b", `{"releaseId":"`+rel.ID+`"}`); rec.Code != 404 {
		t.Fatalf("cross-household target = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearerJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", enrolled.Token, `{"releaseId":"`+rel.ID+`"}`); rec.Code != 401 {
		t.Fatalf("management bearer published/targeted as parent = %d %s", rec.Code, rec.Body.String())
	}

	desired := requestBearer(t, h, http.MethodGet, "/management-device/desired", enrolled.Token)
	if desired.Code != 200 || !strings.Contains(desired.Body.String(), `"status":"queued"`) {
		t.Fatalf("desired targets = %d %s", desired.Code, desired.Body.String())
	}
	bytes := requestBearer(t, h, http.MethodGet, "/management-device/artifacts/"+rel.ID, enrolled.Token)
	if bytes.Code != 200 || len(bytes.Body.Bytes()) == 0 {
		t.Fatalf("artifact = %d", bytes.Code)
	}

	issueB := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments", "parent-b", `{"label":"other"}`)
	var enrollmentB devicemanagement.Enrollment
	_ = json.Unmarshal(issueB.Body.Bytes(), &enrollmentB)
	enrollB := requestJSON(t, h, http.MethodPost, "/management-device/enroll", "", `{"code":"`+enrollmentB.Code+`","deviceName":"Other"}`)
	var enrolledB devicemanagement.EnrollResult
	_ = json.Unmarshal(enrollB.Body.Bytes(), &enrolledB)
	if rec := requestBearer(t, h, http.MethodGet, "/management-device/artifacts/"+rel.ID, enrolledB.Token); rec.Code != 403 && rec.Code != 404 {
		t.Fatalf("cross-household artifact = %d %s", rec.Code, rec.Body.String())
	}

	reportID := uuid.NewString()
	receiptBody := `{"reportId":"` + reportID + `","status":"downloading","targetVersion":1}`
	receipt := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, receiptBody)
	if receipt.Code != 200 {
		t.Fatalf("receipt = %d %s", receipt.Code, receipt.Body.String())
	}
	replay := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, receiptBody)
	if replay.Code != 200 || !strings.Contains(replay.Body.String(), `"status":"downloading"`) {
		t.Fatalf("receipt replay = %d %s", replay.Code, replay.Body.String())
	}
	changed := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+reportID+`","status":"confirmed","targetVersion":1}`)
	if changed.Code != 409 {
		t.Fatalf("changed receipt payload = %d %s", changed.Code, changed.Body.String())
	}
	stale := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+uuid.NewString()+`","status":"confirmed","targetVersion":99}`)
	if stale.Code != 409 {
		t.Fatalf("stale receipt = %d %s", stale.Code, stale.Body.String())
	}
}

func buildSignedAPK(t *testing.T, pkg string, version int64, name string) string {
	t.Helper()
	unsigned := buildUnsignedAPK(t, pkg, version, name)
	keystore := filepath.Join(t.TempDir(), "release.jks")
	cmd := exec.Command("keytool", "-genkeypair", "-keystore", keystore, "-storepass", "android", "-keypass", "android", "-alias", "release", "-keyalg", "RSA", "-keysize", "2048", "-validity", "10000", "-dname", "CN=PrimerRelease")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("keytool: %v %s", err, out)
	}
	signed := unsigned + ".signed.apk"
	sign := exec.Command("apksigner", "sign", "--ks", keystore, "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", signed, unsigned)
	if out, err := sign.CombinedOutput(); err != nil {
		t.Fatalf("apksigner sign: %v %s", err, out)
	}
	return signed
}

func buildUnsignedAPK(t *testing.T, pkg string, version int64, name string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "AndroidManifest.xml")
	body := `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="` + pkg + `" android:versionCode="` + strconv.FormatInt(version, 10) + `" android:versionName="` + name + `">
  <uses-sdk android:minSdkVersion="28" android:targetSdkVersion="34"/>
  <application android:label="Student" android:hasCode="false"/>
</manifest>`
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	androidJar := "/opt/android-sdk/platforms/android-34/android.jar"
	apk := filepath.Join(dir, "app.apk")
	link := exec.Command("aapt2", "link", "-o", apk, "-I", androidJar, "--manifest", manifest)
	if out, err := link.CombinedOutput(); err != nil {
		t.Fatalf("aapt2 link: %v %s", err, out)
	}
	return apk
}
