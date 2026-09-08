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
	"sync"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/devicemanagement"
)

func requireAPKTools(t *testing.T) {
	t.Helper()
	sdk := androidSDKRoot()
	if sdk != "" {
		for _, tools := range androidBuildToolDirs(sdk) {
			t.Setenv("PATH", tools+":"+os.Getenv("PATH"))
		}
	}
	if _, err := exec.LookPath("aapt2"); err != nil || exec.Command("aapt2", "version").Run() != nil {
		t.Fatalf("aapt2 required for APK publication tests: %v", err)
	}
	if _, err := exec.LookPath("apksigner"); err != nil {
		t.Fatal("apksigner required for APK publication tests")
	}
}

func androidSDKRoot() string {
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	if _, err := os.Stat("/opt/android-sdk"); err == nil {
		return "/opt/android-sdk"
	}
	return ""
}

func androidBuildToolDirs(sdk string) []string {
	root := filepath.Join(sdk, "build-tools")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].IsDir() {
			dirs = append(dirs, filepath.Join(root, entries[i].Name()))
		}
	}
	return dirs
}

func androidJar(t *testing.T) string {
	t.Helper()
	sdk := androidSDKRoot()
	if sdk == "" {
		t.Fatal("Android SDK root is required for APK publication tests")
	}
	for _, api := range []string{"34", "35", "33"} {
		path := filepath.Join(sdk, "platforms", "android-"+api, "android.jar")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	matches, _ := filepath.Glob(filepath.Join(sdk, "platforms", "android-*", "android.jar"))
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	t.Fatalf("android.jar not found under %s", sdk)
	return ""
}

func TestReleasePublisherParentDistinctionAndInvalidArtifacts(t *testing.T) {
	requireAPKTools(t)

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
	if same, err := s.Management.PublishAPK(t.Context(), "operator", apk, "stable"); err != nil || same.ID != rel.ID {
		t.Fatalf("identical publish retry = %+v %v", same, err)
	}

	policy := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", `{"baseRevision":0,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","signerSha256":"`+rel.SignerSHA256+`","required":true}],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}}}`)
	if policy.Code != 200 {
		t.Fatalf("approve student = %d %s", policy.Code, policy.Body.String())
	}
	targetBody := `{"releaseId":"` + rel.ID + `","baseTargetVersion":0}`
	target := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", targetBody)
	if target.Code != 200 {
		t.Fatalf("parent target = %d %s", target.Code, target.Body.String())
	}
	var studentTarget devicemanagement.ReleaseTarget
	if err := json.Unmarshal(target.Body.Bytes(), &studentTarget); err != nil || studentTarget.ID == "" {
		t.Fatal(err)
	}
	retry := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", targetBody)
	if retry.Code != 200 {
		t.Fatalf("idempotent target = %d %s", retry.Code, retry.Body.String())
	}
	var retryTarget devicemanagement.ReleaseTarget
	_ = json.Unmarshal(retry.Body.Bytes(), &retryTarget)
	if retryTarget.TargetVersion != studentTarget.TargetVersion {
		t.Fatalf("identical target bumped version %d -> %d", studentTarget.TargetVersion, retryTarget.TargetVersion)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-b", targetBody); rec.Code != 404 {
		t.Fatalf("cross-household target = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearerJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", enrolled.Token, targetBody); rec.Code != 401 {
		t.Fatalf("management bearer published/targeted as parent = %d %s", rec.Code, rec.Body.String())
	}

	tvAPK := buildSignedAPK(t, "com.aleksclark.primer.tv", 4, "1.0.4")
	tvRel, err := s.Management.PublishAPK(t.Context(), "operator", tvAPK, "stable")
	if err != nil {
		t.Fatalf("publish tv: %v", err)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", `{"releaseId":"`+tvRel.ID+`","baseTargetVersion":0}`); rec.Code != 400 {
		t.Fatalf("unapproved tv target = %d %s", rec.Code, rec.Body.String())
	}
	revise := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", `{"baseRevision":1,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","signerSha256":"`+rel.SignerSHA256+`","required":true},{"packageName":"com.aleksclark.primer.tv","signerSha256":"`+tvRel.SignerSHA256+`"}],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}}}`)
	if revise.Code != 200 {
		t.Fatalf("approve tv = %d %s", revise.Code, revise.Body.String())
	}
	tvTargetRec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", `{"releaseId":"`+tvRel.ID+`","baseTargetVersion":0}`)
	if tvTargetRec.Code != 200 {
		t.Fatalf("tv target = %d %s", tvTargetRec.Code, tvTargetRec.Body.String())
	}
	var tvTarget devicemanagement.ReleaseTarget
	if err := json.Unmarshal(tvTargetRec.Body.Bytes(), &tvTarget); err != nil {
		t.Fatal(err)
	}

	desired := requestBearer(t, h, http.MethodGet, "/management-device/desired", enrolled.Token)
	if desired.Code != 200 || !strings.Contains(desired.Body.String(), studentTarget.ID) || !strings.Contains(desired.Body.String(), tvTarget.ID) || !strings.Contains(desired.Body.String(), `"manifestPayloadBase64"`) {
		t.Fatalf("desired targets = %d %s", desired.Code, desired.Body.String())
	}
	meta := requestBearer(t, h, http.MethodGet, "/management-device/releases/"+rel.ID, enrolled.Token)
	if meta.Code != 200 || !strings.Contains(meta.Body.String(), `"manifestPayloadBase64"`) {
		t.Fatalf("device signed release = %d %s", meta.Code, meta.Body.String())
	}
	gotAPK := requestBearer(t, h, http.MethodGet, "/management-device/artifacts/"+rel.ID, enrolled.Token)
	wantBytes, err := os.ReadFile(apk)
	if err != nil {
		t.Fatal(err)
	}
	if gotAPK.Code != 200 || gotAPK.Header().Get("Content-Type") != "application/vnd.android.package-archive" || int64(len(gotAPK.Body.Bytes())) != int64(len(wantBytes)) || string(gotAPK.Body.Bytes()) != string(wantBytes) {
		t.Fatalf("artifact mismatch code=%d ct=%s len=%d want=%d", gotAPK.Code, gotAPK.Header().Get("Content-Type"), gotAPK.Body.Len(), len(wantBytes))
	}
	if rec := requestJSON(t, h, http.MethodGet, "/managed-releases/"+rel.ID+"/apk", "parent-a", ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "application/vnd.android.package-archive" || rec.Body.Len() != len(wantBytes) {
		t.Fatalf("parent apk = %d ct=%s len=%d", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
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

	studentReceipt := `{"reportId":"` + uuid.NewString() + `","targetId":"` + studentTarget.ID + `","status":"downloading","targetVersion":1}`
	if rec := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, studentReceipt); rec.Code != 200 {
		t.Fatalf("student receipt = %d %s", rec.Code, rec.Body.String())
	}
	tvReceipt := `{"reportId":"` + uuid.NewString() + `","targetId":"` + tvTarget.ID + `","status":"downloading","targetVersion":1}`
	if rec := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, tvReceipt); rec.Code != 200 {
		t.Fatalf("tv receipt = %d %s", rec.Code, rec.Body.String())
	}
	confirmNil := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+uuid.NewString()+`","targetId":"`+studentTarget.ID+`","status":"confirmed","targetVersion":1}`)
	if confirmNil.Code != 400 {
		t.Fatalf("confirmed without version = %d %s", confirmNil.Code, confirmNil.Body.String())
	}
	confirmOK := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+uuid.NewString()+`","targetId":"`+studentTarget.ID+`","status":"confirmed","targetVersion":1,"installedVersionCode":13}`)
	if confirmOK.Code != 200 {
		t.Fatalf("confirmed student = %d %s", confirmOK.Code, confirmOK.Body.String())
	}
	regress := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+uuid.NewString()+`","targetId":"`+studentTarget.ID+`","status":"downloading","targetVersion":1}`)
	if regress.Code != 409 {
		t.Fatalf("regress confirmed = %d %s", regress.Code, regress.Body.String())
	}
	stale := requestBearerJSON(t, h, http.MethodPost, "/management-device/release-receipts", enrolled.Token, `{"reportId":"`+uuid.NewString()+`","targetId":"`+studentTarget.ID+`","status":"confirmed","targetVersion":99,"installedVersionCode":13}`)
	if stale.Code != 409 {
		t.Fatalf("stale receipt = %d %s", stale.Code, stale.Body.String())
	}
	paused, err := s.Management.PauseRelease(t.Context(), "operator", rel.ID)
	if err != nil || paused.Status != "paused" {
		t.Fatalf("pause = %+v %v", paused, err)
	}
	if rec := requestBearer(t, h, http.MethodGet, "/management-device/artifacts/"+rel.ID, enrolled.Token); rec.Code != 403 && rec.Code != 404 {
		t.Fatalf("paused artifact still deliverable = %d %s", rec.Code, rec.Body.String())
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
	apk := filepath.Join(dir, "app.apk")
	link := exec.Command("aapt2", "link", "-o", apk, "-I", androidJar(t), "--manifest", manifest)
	if out, err := link.CombinedOutput(); err != nil {
		t.Fatalf("aapt2 link: %v %s", err, out)
	}
	return apk
}

func TestConcurrentRevokeAndRetarget(t *testing.T) {
	requireAPKTools(t)
	pool := integrationPool(t)
	_, _ = seedIntegration(t, pool)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("release-secret"), IssuerSecret: []byte("release-issuer"), PublicOrigin: "https://tasks.test", ReleaseSigningKey: priv, ArtifactDir: t.TempDir()})
	h := s.Routes()
	issue := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments", "parent-a", `{"label":"A16"}`)
	var enrollment devicemanagement.Enrollment
	_ = json.Unmarshal(issue.Body.Bytes(), &enrollment)
	enroll := requestJSON(t, h, http.MethodPost, "/management-device/enroll", "", `{"code":"`+enrollment.Code+`","deviceName":"Student A16"}`)
	var enrolled devicemanagement.EnrollResult
	_ = json.Unmarshal(enroll.Body.Bytes(), &enrolled)
	apk := buildSignedAPK(t, "com.aleksclark.primer.student", 15, "1.0.15")
	rel, err := s.Management.PublishAPK(t.Context(), "operator", apk, "stable")
	if err != nil {
		t.Fatal(err)
	}
	policy := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", `{"baseRevision":0,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","signerSha256":"`+rel.SignerSHA256+`","required":true}],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}}}`)
	if policy.Code != 200 {
		t.Fatalf("policy = %d %s", policy.Code, policy.Body.String())
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes <- requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/releases", "parent-a", `{"releaseId":"`+rel.ID+`","baseTargetVersion":0}`).Code
	}()
	go func() {
		defer wg.Done()
		codes <- requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/revoke", "parent-a", `{"reason":"lost"}`).Code
	}()
	wg.Wait()
	close(codes)
	ok := 0
	for code := range codes {
		if code == 200 || code == 403 || code == 409 {
			ok++
			continue
		}
		t.Fatalf("concurrent revoke/retarget status %d", code)
	}
	if ok != 2 {
		t.Fatal("concurrent revoke/retarget did not complete")
	}
}
