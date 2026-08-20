package api

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"primer-tasks/internal/jobs"
)

func TestExternalSecretResolverFailsClosedInProductionAndBindsDevelopmentVersion(t *testing.T) {
	oldSecret, oldRef, oldVersion, oldBundle := os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET"), os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_REF"), os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION"), os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE")
	t.Cleanup(func() {
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET", oldSecret)
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_REF", oldRef)
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION", oldVersion)
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE", oldBundle)
	})
	_ = os.Unsetenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE")
	_ = os.Unsetenv("TASKS_EXTERNAL_VERIFIER_SECRET")
	if externalSecretResolver("production") != nil {
		t.Fatal("production resolver enabled without secret")
	}
	_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET", "test-secret")
	_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_REF", "managed-ref")
	_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION", "v2")
	resolver := externalSecretResolver("test")
	if resolver == nil {
		t.Fatal("test resolver missing")
	}
	if secret, err := resolver.Resolve(context.Background(), "managed-ref", "v2"); err != nil || string(secret) != "test-secret" {
		t.Fatalf("managed secret=%q err=%v", secret, err)
	}
	if secret, err := resolver.Resolve(context.Background(), "managed-ref", "v1"); err == nil || secret != nil {
		t.Fatalf("unexpected old secret=%q err=%v", secret, err)
	}
	_ = os.Unsetenv("TASKS_EXTERNAL_VERIFIER_SECRET")
	_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE", `{"managed-ref:v1":"old-secret","managed-ref:v2":"new-secret"}`)
	rotated := externalSecretResolver("production")
	if rotated == nil {
		t.Fatal("versioned production resolver missing")
	}
	for version, expected := range map[string]string{"v1": "old-secret", "v2": "new-secret"} {
		secret, err := rotated.Resolve(context.Background(), "managed-ref", version)
		if err != nil || string(secret) != expected {
			t.Fatalf("versioned secret version=%s value=%q err=%v", version, secret, err)
		}
	}
	_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE", "not-json")
	if externalSecretResolver("production") != nil {
		t.Fatal("malformed production secret bundle accepted")
	}
}

func TestExternalVerifierHealthIsAbsentWithoutDatabase(t *testing.T) {
	if health := (&Server{}).externalVerifierHealth(context.Background()); health != nil {
		t.Fatalf("health without database=%+v", health)
	}
}

func TestExternalVerifierHealthReturnsNilWhenDatabaseIsUnavailable(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://tasks:tasks@127.0.0.1:1/does_not_exist?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if health := (&Server{DB: pool}).externalVerifierHealth(context.Background()); health != nil {
		t.Fatalf("unavailable database health=%+v", health)
	}
}

func TestStartExternalWorkerReturnsWhenSecretResolverIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	(&Server{}).StartExternalWorker(ctx)
	running := &Server{ExternalSecrets: jobs.StaticSecretResolver{}}
	running.StartExternalWorker(ctx)
	worker := jobs.NewExternalWorker(nil, nil, nil)
	if worker == nil || worker.Poll != 250*time.Millisecond {
		t.Fatalf("worker defaults=%+v", worker)
	}
}
