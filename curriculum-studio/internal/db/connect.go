package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" database/sql driver for goose
)

// Connect opens a pgx pool to the given Studio database URL and pings it.
// Forbidden LMS/TV/Identity database names are always refused.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return ConnectWithConfig(ctx, Config{DatabaseURL: databaseURL, GuardForbiddenDBNames: true})
}

// ConnectWithConfig opens a pool using cfg.DatabaseURL and optional MaxConns.
// DSN isolation is always enforced so library callers cannot bypass CLI/config
// Validate by clearing GuardForbiddenDBNames.
func ConnectWithConfig(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, fmt.Errorf("database url is required")
	}
	if err := ValidateDatabaseURL(cfg.DatabaseURL); err != nil {
		return nil, err
	}
	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns > 0 {
		pcfg.MaxConns = cfg.MaxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("connect pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
