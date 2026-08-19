package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"primer-tasks/internal/db"
)

func main() {
	ctx := context.Background()
	if err := db.SafeDatabaseName(db.DSN()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	p, e := pgxpool.New(ctx, db.DSN())
	if e != nil {
		panic(e)
	}
	defer p.Close()
	if e = db.Migrate(ctx, p); e != nil {
		panic(e)
	}
}
