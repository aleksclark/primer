package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"primer-tasks/internal/devicemanagement"
)

const (
	studentSigner = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tvSigner      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestManagementEnrollmentPolicyIsolationReplayAndCAS(t *testing.T) {
	pool := integrationPool(t)
	alice, bob := seedIntegration(t, pool)
	s := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("management-secret"), IssuerSecret: []byte("management-issuer"), PublicOrigin: "https://tasks.test"})
	h := s.Routes()

	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments", "", `{"label":"A16"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated enrollment = %d %s", rec.Code, rec.Body.String())
	}

	issueA := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments", "parent-a", `{"label":"A16","expiresInMinutes":15}`)
	if issueA.Code != http.StatusCreated {
		t.Fatalf("issue enrollment A = %d %s", issueA.Code, issueA.Body.String())
	}
	var enrollmentA devicemanagement.Enrollment
	if err := json.Unmarshal(issueA.Body.Bytes(), &enrollmentA); err != nil || enrollmentA.Code == "" {
		t.Fatalf("enrollment A body: %s", issueA.Body.String())
	}
	if enrollmentA.BaseURL != "https://tasks.test" || !strings.Contains(enrollmentA.QRPayload, enrollmentA.BaseURL+"/management-device/enroll#") || enrollmentA.ResponseLossPolicy != "fresh-parent-enrollment" {
		t.Fatalf("enrollment QR/baseURL contract: %+v", enrollmentA)
	}

	pairRec := requestJSON(t, h, http.MethodPost, "/students/"+alice+"/pairing", "parent-a", "")
	code, _ := pairingResponse(t, pairRec)
	devicePair := requestJSON(t, h, http.MethodPost, "/device/pair", "", `{"code":"`+code+`"}`)
	var tasksDevice DevicePair
	if err := json.Unmarshal(devicePair.Body.Bytes(), &tasksDevice); err != nil || tasksDevice.Token == "" {
		t.Fatalf("tasks pair = %d %s", devicePair.Code, devicePair.Body.String())
	}

	enrollBody := `{"code":"` + enrollmentA.Code + `","deviceName":"Student A16","deviceModel":"SM-S166V"}`
	enrollA := requestJSON(t, h, http.MethodPost, "/management-device/enroll", "", enrollBody)
	if enrollA.Code != http.StatusCreated {
		t.Fatalf("enroll A = %d %s", enrollA.Code, enrollA.Body.String())
	}
	var enrolled devicemanagement.EnrollResult
	if err := json.Unmarshal(enrollA.Body.Bytes(), &enrolled); err != nil || enrolled.Token == "" {
		t.Fatalf("enroll result: %s", enrollA.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/management-device/enroll", "", enrollBody); rec.Code != http.StatusGone {
		t.Fatalf("enrollment replay = %d %s", rec.Code, rec.Body.String())
	}

	if rec := requestBearer(t, h, http.MethodGet, "/device/profile", enrolled.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("management bearer accepted as Tasks device = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodGet, "/students", enrolled.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("management bearer accepted as parent = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodGet, "/management-device/desired", tasksDevice.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("Tasks bearer accepted as management = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/management-device/desired", "parent-a", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("parent cookie accepted as management device = %d %s", rec.Code, rec.Body.String())
	}
	raw := httptest.NewRequest(http.MethodGet, "/management-device/desired", nil)
	raw.Header.Set("Authorization", enrolled.Token)
	rawRec := httptest.NewRecorder()
	h.ServeHTTP(rawRec, raw)
	if rawRec.Code != http.StatusUnauthorized {
		t.Fatalf("raw token without Bearer scheme accepted = %d %s", rawRec.Code, rawRec.Body.String())
	}

	listA := requestJSON(t, h, http.MethodGet, "/managed-devices", "parent-a", "")
	if listA.Code != http.StatusOK || !strings.Contains(listA.Body.String(), enrolled.Device.ID) {
		t.Fatalf("list A = %d %s", listA.Code, listA.Body.String())
	}
	listB := requestJSON(t, h, http.MethodGet, "/managed-devices", "parent-b", "")
	if listB.Code != http.StatusOK || strings.Contains(listB.Body.String(), enrolled.Device.ID) {
		t.Fatalf("cross-household list leak = %d %s", listB.Code, listB.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/managed-devices/"+enrolled.Device.ID, "parent-b", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-household get = %d %s", rec.Code, rec.Body.String())
	}

	unsafe := `{"baseRevision":0,"policy":{"approvedApps":[{"packageName":"com.example.other","signerSha256":"` + studentSigner + `"}]}}`
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", unsafe); rec.Code != http.StatusBadRequest {
		t.Fatalf("policy without Student = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", `{"baseRevision":0,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","signerSha256":"deadbeef","required":true}]}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("short signer accepted = %d %s", rec.Code, rec.Body.String())
	}
	policyBody := `{"baseRevision":0,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","label":"Student","signerSha256":"` + studentSigner + `","required":true}],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}}}`
	policy := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", policyBody)
	if policy.Code != http.StatusOK {
		t.Fatalf("policy v1 = %d %s body=%s", policy.Code, policy.Body.String(), policyBody)
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", policyBody); rec.Code != http.StatusConflict {
		t.Fatalf("stale CAS = %d %s", rec.Code, rec.Body.String())
	}

	var wg sync.WaitGroup
	codes := make(chan int, 2)
	next := `{"baseRevision":1,"policy":{"approvedApps":[{"packageName":"com.aleksclark.primer.student","signerSha256":"` + studentSigner + `","required":true},{"packageName":"com.aleksclark.primer.tv","signerSha256":"` + tvSigner + `"}],"lockTask":{"enabled":true,"packages":["com.aleksclark.primer.student"]},"maintenance":{"allowParentUnlock":true}}}`
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/policy", "parent-a", next).Code
		}()
	}
	wg.Wait()
	close(codes)
	ok, conflict := 0, 0
	for code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("concurrent revision status %d", code)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrent CAS ok=%d conflict=%d", ok, conflict)
	}

	desired := requestBearer(t, h, http.MethodGet, "/management-device/desired", enrolled.Token)
	if desired.Code != http.StatusOK || !strings.Contains(desired.Body.String(), `"desiredRevision":2`) {
		t.Fatalf("desired = %d %s", desired.Code, desired.Body.String())
	}

	staleReport := `{"reportId":"` + uuid.NewString() + `","policyRevision":1,"status":"applied"}`
	stale := requestBearerJSON(t, h, http.MethodPost, "/management-device/reports", enrolled.Token, staleReport)
	if stale.Code != http.StatusOK || !strings.Contains(stale.Body.String(), `"stale":true`) {
		t.Fatalf("stale report = %d %s", stale.Code, stale.Body.String())
	}
	got := requestJSON(t, h, http.MethodGet, "/managed-devices/"+enrolled.Device.ID, "parent-a", "")
	if !strings.Contains(got.Body.String(), `"appliedRevision":0`) {
		t.Fatalf("stale ack overwrote applied revision: %s", got.Body.String())
	}

	reportID := uuid.NewString()
	appliedBody := `{"reportId":"` + reportID + `","policyRevision":2,"status":"applied","installedStudentVersion":"1"}`
	applied := requestBearerJSON(t, h, http.MethodPost, "/management-device/reports", enrolled.Token, appliedBody)
	if applied.Code != http.StatusOK || !strings.Contains(applied.Body.String(), `"stale":false`) {
		t.Fatalf("applied report = %d %s", applied.Code, applied.Body.String())
	}
	replay := requestBearerJSON(t, h, http.MethodPost, "/management-device/reports", enrolled.Token, appliedBody)
	if replay.Code != http.StatusOK {
		t.Fatalf("idempotent report replay = %d %s", replay.Code, replay.Body.String())
	}
	changed := requestBearerJSON(t, h, http.MethodPost, "/management-device/reports", enrolled.Token, `{"reportId":"`+reportID+`","policyRevision":2,"status":"failed"}`)
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed report payload = %d %s", changed.Code, changed.Body.String())
	}
	got = requestJSON(t, h, http.MethodGet, "/managed-devices/"+enrolled.Device.ID, "parent-a", "")
	if !strings.Contains(got.Body.String(), `"appliedRevision":2`) || !strings.Contains(got.Body.String(), `"installedStudentVersion":"1"`) {
		t.Fatalf("parent latest report missing: %s", got.Body.String())
	}

	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/recovery", "parent-a", `{"kind":"rotate_recovery_code","envelope":{"keyId":"device-key-1","alg":"X25519-ChaCha20Poly1305","nonce":"n1n1n1n1n1n1n1n1","ciphertext":"cipher-cipher-cipher"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("rotation without parent ack = %d %s", rec.Code, rec.Body.String())
	}
	recovery := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/recovery", "parent-a", `{"kind":"maintenance_lease","deliveryExpiresMinutes":15,"leaseExpiresMinutes":10}`)
	if recovery.Code != http.StatusCreated || !strings.Contains(recovery.Body.String(), `"status":"pending"`) {
		t.Fatalf("recovery intent = %d %s", recovery.Code, recovery.Body.String())
	}
	var intent devicemanagement.RecoveryIntent
	if err := json.Unmarshal(recovery.Body.Bytes(), &intent); err != nil {
		t.Fatal(err)
	}
	confirmID := uuid.NewString()
	confirm := requestBearerJSON(t, h, http.MethodPost, "/management-device/recovery/"+intent.ID+"/confirm", enrolled.Token, `{"reportId":"`+confirmID+`"}`)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm recovery = %d %s", confirm.Code, confirm.Body.String())
	}
	replayConfirm := requestBearerJSON(t, h, http.MethodPost, "/management-device/recovery/"+intent.ID+"/confirm", enrolled.Token, `{"reportId":"`+confirmID+`"}`)
	if replayConfirm.Code != http.StatusOK {
		t.Fatalf("idempotent confirm = %d %s", replayConfirm.Code, replayConfirm.Body.String())
	}
	changedConfirm := requestBearerJSON(t, h, http.MethodPost, "/management-device/recovery/"+intent.ID+"/confirm", enrolled.Token, `{"reportId":"`+uuid.NewString()+`"}`)
	if changedConfirm.Code != http.StatusConflict {
		t.Fatalf("changed confirm = %d %s", changedConfirm.Code, changedConfirm.Body.String())
	}

	restarted := NewWithAuth(pool, "test", AuthConfig{SessionSecret: []byte("management-secret"), IssuerSecret: []byte("management-issuer"), PublicOrigin: "https://tasks.test"}).Routes()
	afterRestart := requestBearer(t, restarted, http.MethodGet, "/management-device/desired", enrolled.Token)
	if afterRestart.Code != http.StatusOK || !strings.Contains(afterRestart.Body.String(), `"desiredRevision":2`) {
		t.Fatalf("restart durability = %d %s", afterRestart.Code, afterRestart.Body.String())
	}

	if rec := requestJSON(t, h, http.MethodDelete, "/students/"+alice, "parent-a", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("archive student = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodGet, "/managed-devices/"+enrolled.Device.ID, "parent-a", ""); rec.Code != http.StatusOK {
		t.Fatalf("management survived Tasks archive = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/revoke", "parent-a", `{"reason":"lost"}`); rec.Code != http.StatusOK {
		t.Fatalf("revoke management = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/"+enrolled.Device.ID+"/recovery", "parent-a", `{"kind":"maintenance_lease"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("recovery on revoked device = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestBearer(t, h, http.MethodGet, "/management-device/desired", enrolled.Token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked management credential still valid = %d %s", rec.Code, rec.Body.String())
	}
	if rec := requestJSON(t, h, http.MethodPost, "/managed-devices/enrollments/"+enrollmentA.ID+"/abandon", "parent-a", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("abandon consumed enrollment = %d %s", rec.Code, rec.Body.String())
	}

	_ = bob
}

func requestBearerJSON(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
