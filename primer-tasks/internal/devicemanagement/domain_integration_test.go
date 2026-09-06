package devicemanagement

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	tasksdb "primer-tasks/internal/db"
)

func domainPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	dsn := os.Getenv("TASKS_TEST_DATABASE_URL")
	if dsn == "" {
		if err := exec.Command("docker", "info").Run(); err != nil {
			if os.Getenv("PRIMER_TASKS_COVERAGE_GATE") == "1" {
				t.Fatalf("docker required: %v", err)
			}
			t.Skipf("docker unavailable: %v", err)
		}
		container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("primer_tasks_mgmt_test"),
			tcpostgres.WithUsername("tasks"),
			tcpostgres.WithPassword("tasks"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })
		dsn, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tasksdb.SafeDatabaseName(dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := tasksdb.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestRecoveryIdempotencyPauseAndStateBranches(t *testing.T) {
	pool := domainPool(t)
	ctx := context.Background()
	tenant := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'A')`, tenant); err != nil {
		t.Fatal(err)
	}
	svc := &Service{DB: pool, PublicOrigin: "https://tasks.test"}
	sc := Scope{TenantID: tenant.String(), ActorRef: "parent-a"}
	enroll, err := svc.IssueEnrollment(ctx, sc, IssueEnrollmentInput{Label: "A16"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.IssueEnrollment(ctx, sc, IssueEnrollmentInput{ExpiresInMinutes: 61}); err == nil {
		t.Fatal("long enrollment accepted")
	}
	pub := base64.RawURLEncoding.EncodeToString([]byte(`{"primaryKeyId":1}`))
	keyID, err := enrollmentKeyIDFromPublic(pub)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := svc.Enroll(ctx, EnrollInput{Code: enroll.Code, DeviceName: "Student", EnrollmentPubKey: pub, EnrollmentKeyID: keyID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Enroll(ctx, EnrollInput{Code: enroll.Code, DeviceName: "Student"}); err != ErrGone {
		t.Fatalf("replay enroll = %v", err)
	}
	req := uuid.NewString()
	env := &RecoveryEnvelope{KeyID: keyID, Alg: RecoveryHPKEAlg, Ciphertext: base64.RawURLEncoding.EncodeToString([]byte("cipher-cipher-cipher-cipher"))}
	first, err := svc.CreateRecoveryIntent(ctx, sc, claimed.Device.ID, RecoveryIntentInput{RequestID: req, Kind: RecoveryRotateCode, Envelope: env, ParentAcknowledged: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != req {
		t.Fatalf("intent id = %s", first.ID)
	}
	again, err := svc.CreateRecoveryIntent(ctx, sc, claimed.Device.ID, RecoveryIntentInput{RequestID: req, Kind: RecoveryRotateCode, Envelope: env, ParentAcknowledged: true})
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotent recovery = %+v %v", again, err)
	}
	if _, err = svc.CreateRecoveryIntent(ctx, sc, claimed.Device.ID, RecoveryIntentInput{RequestID: req, Kind: RecoveryMaintenanceLease}); err == nil {
		t.Fatal("changed recovery payload accepted")
	}
	pending, err := svc.PendingRecovery(ctx, sc.TenantID, claimed.Device.ID)
	if err != nil || len(pending) == 0 {
		t.Fatalf("pending = %v %v", pending, err)
	}
	lease, err := svc.CreateRecoveryIntent(ctx, sc, claimed.Device.ID, RecoveryIntentInput{RequestID: uuid.NewString(), Kind: RecoveryMaintenanceLease, LeaseExpiresMinutes: 10})
	if err != nil || lease.LeaseExpiresAt == nil {
		t.Fatalf("lease = %+v %v", lease, err)
	}
	if lease.LeaseExpiresAt.Sub(svc.now()) > 30*time.Minute {
		t.Fatal("lease exceeded 30 minutes")
	}
	ds, err := svc.AuthenticateDevice(ctx, claimed.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ConfirmRecoveryIntent(ctx, ds, first.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err = svc.ConfirmRecoveryIntent(ctx, ds, first.ID, uuid.NewString()); err == nil {
		t.Fatal("changed confirm accepted")
	}
	history, err := svc.RecoveryHistory(ctx, sc, claimed.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	seenApplied, seenPending := false, false
	for _, item := range history {
		switch item.Status {
		case "applied":
			seenApplied = true
		case "pending":
			seenPending = true
		}
	}
	if !seenApplied || !seenPending {
		t.Fatalf("parent recovery history missing applied/pending: %+v", history)
	}
	if _, err = pool.Exec(ctx, `UPDATE management_recovery_intents SET delivery_expires_at=now()-interval '1 minute' WHERE id=$1 AND status='pending'`, lease.ID); err != nil {
		t.Fatal(err)
	}
	history, err = svc.RecoveryHistory(ctx, sc, claimed.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	seenExpired := false
	for _, item := range history {
		if item.ID == lease.ID && item.Status == "expired" {
			seenExpired = true
		}
	}
	if !seenExpired {
		t.Fatalf("expired intent not visible to parent: %+v", history)
	}
	pendingAfter, err := svc.PendingRecovery(ctx, sc.TenantID, claimed.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range pendingAfter {
		if item.ID == lease.ID {
			t.Fatal("expired intent still pending for device")
		}
	}
	if _, err = svc.ChangeDeviceState(ctx, sc, claimed.Device.ID, "quarantined", "check"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ChangeDeviceState(ctx, sc, claimed.Device.ID, "quarantined", "check"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ChangeDeviceState(ctx, sc, claimed.Device.ID, "revoked", "lost"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CreateRecoveryIntent(ctx, sc, claimed.Device.ID, RecoveryIntentInput{RequestID: uuid.NewString(), Kind: RecoveryMaintenanceLease}); err == nil {
		t.Fatal("recovery after revoke accepted")
	}
	if err = svc.AbandonEnrollment(ctx, sc, enroll.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPublishTargetReceiptAndPause(t *testing.T) {
	requireAPKTools(t)
	pool := domainPool(t)
	ctx := context.Background()
	tenant := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,'A')`, tenant); err != nil {
		t.Fatal(err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{DB: pool, PublicOrigin: "https://tasks.test", ArtifactDir: t.TempDir(), ReleaseSigningKey: priv}
	sc := Scope{TenantID: tenant.String(), ActorRef: "parent-a"}
	enroll, err := svc.IssueEnrollment(ctx, sc, IssueEnrollmentInput{})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := svc.Enroll(ctx, EnrollInput{Code: enroll.Code, DeviceName: "Student"})
	if err != nil {
		t.Fatal(err)
	}
	apk := buildSignedAPK(t, StudentPackageName, 21, "1.0.21")
	rel, err := svc.PublishAPK(ctx, "operator", apk, "stable")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SetReleaseTarget(ctx, sc, claimed.Device.ID, ReleaseTargetInput{ReleaseID: rel.ID}); err == nil {
		t.Fatal("target without policy accepted")
	}
	_, err = svc.UpdatePolicy(ctx, sc, claimed.Device.ID, PolicyUpdateInput{Policy: Policy{ApprovedApps: []ApprovedApp{{PackageName: StudentPackageName, SignerSHA256: rel.SignerSHA256, Required: true}}, LockTask: LockTaskPolicy{Enabled: true, Packages: []string{StudentPackageName}}, Maintenance: MaintenancePolicy{AllowParentUnlock: true}}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.SetReleaseTarget(ctx, sc, claimed.Device.ID, ReleaseTargetInput{ReleaseID: rel.ID, BaseTargetVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.SetReleaseTarget(ctx, sc, claimed.Device.ID, ReleaseTargetInput{ReleaseID: rel.ID, BaseTargetVersion: 0})
	if err != nil || again.TargetVersion != target.TargetVersion {
		t.Fatalf("idempotent target = %+v %v", again, err)
	}
	ds := DeviceScope{TenantID: sc.TenantID, DeviceID: claimed.Device.ID}
	if _, err = svc.DeviceRelease(ctx, ds, rel.ID); err != nil {
		t.Fatal(err)
	}
	code := int64(21)
	if _, err = svc.ReportRelease(ctx, ds, ReleaseReceiptInput{ReportID: uuid.NewString(), TargetID: target.ID, Status: "confirmed", TargetVersion: 1, InstalledVersionCode: &code}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ReportRelease(ctx, ds, ReleaseReceiptInput{ReportID: uuid.NewString(), TargetID: target.ID, Status: "downloading", TargetVersion: 1}); err == nil {
		t.Fatal("regress confirmed accepted")
	}
	paused, err := svc.PauseRelease(ctx, "operator", rel.ID)
	if err != nil || paused.Status != "paused" {
		t.Fatalf("pause = %+v %v", paused, err)
	}
	if _, _, err = svc.ArtifactBytes(ctx, ds, rel.ID); err == nil {
		t.Fatal("paused artifact still delivered")
	}
}

func requireAPKTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("aapt2"); err != nil {
		t.Setenv("PATH", "/opt/android-sdk/build-tools/35.0.0:"+os.Getenv("PATH"))
	}
	if _, err := exec.LookPath("aapt2"); err != nil {
		t.Fatalf("aapt2 required: %v", err)
	}
}

func buildSignedAPK(t *testing.T, pkg string, version int64, name string) string {
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
	link := exec.Command("aapt2", "link", "-o", apk, "-I", "/opt/android-sdk/platforms/android-34/android.jar", "--manifest", manifest)
	if out, err := link.CombinedOutput(); err != nil {
		t.Fatalf("aapt2 link: %v %s", err, out)
	}
	keystore := filepath.Join(dir, "release.jks")
	cmd := exec.Command("keytool", "-genkeypair", "-keystore", keystore, "-storepass", "android", "-keypass", "android", "-alias", "release", "-keyalg", "RSA", "-keysize", "2048", "-validity", "10000", "-dname", "CN=PrimerRelease")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("keytool: %v %s", err, out)
	}
	signed := apk + ".signed.apk"
	sign := exec.Command("apksigner", "sign", "--ks", keystore, "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", signed, apk)
	if out, err := sign.CombinedOutput(); err != nil {
		t.Fatalf("apksigner: %v %s", err, out)
	}
	return signed
}
