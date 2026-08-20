package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"primer-tasks/internal/api"
	"primer-tasks/internal/artifactstore"
	"primer-tasks/internal/config"
	"primer-tasks/internal/db"
	"primer-tasks/internal/schedule"
	"syscall"
	"time"
)

func main() {
	// Validate all security and identity configuration before opening a pool or
	// running migrations. In particular, production test-auth failures cannot
	// touch a database as a side effect of startup.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid Tasks configuration", "error", err)
		os.Exit(2)
	}
	if err := db.SafeDatabaseName(cfg.DatabaseURL); err != nil {
		slog.Error("unsafe database", "error", err)
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	if err = db.Migrate(ctx, pool); err != nil {
		panic(err)
	}
	if err = seed(ctx, pool); err != nil {
		panic(err)
	}
	worker := schedule.NewWorker(pool)
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	go worker.Run(workerCtx)
	var tasksAPI *api.Server
	if envOr("TASKS_ARTIFACT_STORE", "filesystem") == "s3" {
		store, storeErr := artifactstore.NewS3(workerCtx, artifactstore.S3Config{
			Endpoint: envOr("TASKS_S3_ENDPOINT", "http://minio:9000"), PublicEndpoint: os.Getenv("TASKS_S3_PUBLIC_ENDPOINT"), Region: envOr("TASKS_S3_REGION", "us-east-1"),
			Bucket: envOr("TASKS_S3_BUCKET", "primer-tasks"), AccessKey: os.Getenv("TASKS_S3_ACCESS_KEY"), SecretKey: os.Getenv("TASKS_S3_SECRET_KEY"), SessionToken: os.Getenv("TASKS_S3_SESSION_TOKEN"), ForcePathStyle: envOr("TASKS_S3_FORCE_PATH_STYLE", "true") == "true",
		})
		if storeErr != nil {
			slog.Error("invalid artifact object store", "error", storeErr)
			os.Exit(2)
		}
		if envOr("TASKS_S3_CREATE_BUCKET", "false") == "true" {
			if bucketErr := store.EnsureBucket(workerCtx); bucketErr != nil {
				slog.Error("unable to initialize artifact bucket", "error", bucketErr)
				os.Exit(2)
			}
		}
		tasksAPI = api.NewWithStore(pool, cfg.Env, store)
	} else {
		tasksAPI = api.New(pool, cfg.Env)
	}
	tasksAPI.StartAgentWorker(workerCtx)
	tasksAPI.StartDialogueWorker(workerCtx)
	tasksAPI.StartArtifactWorker(workerCtx)
	tasksAPI.StartArtifactCleanup(workerCtx)
	tasksAPI.StartExternalWorker(workerCtx)
	srv := &http.Server{Addr: envOr("TASKS_HOST", "127.0.0.1") + ":" + envOr("TASKS_PORT", "8080"), Handler: tasksAPI.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("tasks server listening", "addr", srv.Addr)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			panic(e)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(c)
}
func envOr(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func seed(ctx context.Context, p *pgxpool.Pool) error {
	_, e := p.Exec(ctx, `INSERT INTO tenants(id,name) VALUES ('00000000-0000-0000-0000-0000000000a1','Household A'),('00000000-0000-0000-0000-0000000000b1','Household B') ON CONFLICT DO NOTHING; INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES ('00000000-0000-0000-0000-0000000000a1','parent-a','admin'),('00000000-0000-0000-0000-0000000000b1','parent-b','admin') ON CONFLICT DO NOTHING`)
	return e
}
