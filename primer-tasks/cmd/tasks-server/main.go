package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"primer-tasks/internal/api"
	"primer-tasks/internal/db"
	"syscall"
	"time"
)

func main() {
	env := os.Getenv("TASKS_ENV")
	if env == "" {
		env = "development"
	}
	if env == "production" && (os.Getenv("TASKS_AUTH_MODE") == "test" || os.Getenv("TASKS_ISSUER_URL") == "") {
		slog.Error("production requires live identity issuer")
		os.Exit(2)
	}
	if err := db.SafeDatabaseName(db.DSN()); err != nil {
		slog.Error("unsafe database", "error", err)
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, db.DSN())
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
	srv := &http.Server{Addr: envOr("TASKS_HOST", "127.0.0.1") + ":" + envOr("TASKS_PORT", "8080"), Handler: api.New(pool, env).Routes(), ReadHeaderTimeout: 10 * time.Second}
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
