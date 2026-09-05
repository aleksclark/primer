package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"primer-tasks/internal/db"
)

func main() {
	dsn := os.Getenv("TASKS_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "TASKS_DATABASE_URL is required")
		os.Exit(2)
	}
	ctx := context.Background()
	if err := db.SafeDatabaseName(dsn); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		panic(e)
	}
	defer p.Close()
	if e = db.Migrate(ctx, p); e != nil {
		panic(e)
	}
}
