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
	oldSecret, oldRef, oldVersion := os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET"), os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_REF"), os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION")
	t.Cleanup(func() {
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET", oldSecret)
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_REF", oldRef)
		_ = os.Setenv("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION", oldVersion)
	})
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
