package main

import (
	"context"
	"git.clark.team/aleksclark/authstack/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"primer-tasks/internal/api"
	"primer-tasks/internal/config"
	"primer-tasks/internal/db"
	"primer-tasks/internal/parentauth"
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
	if cfg.Env != "production" {
		if err = db.Migrate(ctx, pool); err != nil {
			panic(err)
		}
		if cfg.AuthMode != "clerk" {
			if err = seed(ctx, pool); err != nil {
				panic(err)
			}
		}
	}
	app := api.New(pool, cfg.Env)
	app.BasePath = cfg.BasePath
	if cfg.AuthMode == "clerk" {
		app.ParentAuthenticator, err = parentauth.New(ctx, cfg.ClerkIssuer, cfg.ClerkJWKSURL)
		if err != nil {
			slog.Error("Clerk initialization failed", "error", err)
			os.Exit(2)
		}
		// Household authority is the local ledger, not Clerk organizations. A
		// standard Clerk session has no aud; bind exact issuer + browser azp.
		app.ParentPolicy = auth.AuthenticationPolicy{AcceptedCredentials: []auth.CredentialKind{auth.CredentialSession}, AuthorizedParties: []string{cfg.PublicOrigin}}
		if cfg.ClerkAudience != "" {
			app.ParentPolicy.Audiences = []string{cfg.ClerkAudience}
		}
	}
	worker := schedule.NewWorker(pool)
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	go worker.Run(workerCtx)
	app.StartAgentWorker(workerCtx)
	app.StartDialogueWorker(workerCtx)
	srv := &http.Server{Addr: envOr("TASKS_HOST", "127.0.0.1") + ":" + envOr("TASKS_PORT", "8080"), Handler: api.Mount(app.Routes(), cfg.BasePath, cfg.WebDir), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
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
