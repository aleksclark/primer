package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"primer-tasks/internal/config"
	"primer-tasks/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx := context.Background()
	if err := db.SafeDatabaseName(cfg.DatabaseURL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	p, e := pgxpool.New(ctx, cfg.DatabaseURL)
	if e != nil {
		panic(e)
	}
	defer p.Close()
	if e = db.Migrate(ctx, p); e != nil {
		panic(e)
	}
}
