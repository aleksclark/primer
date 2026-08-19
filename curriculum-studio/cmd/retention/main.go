package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	studiodb "github.com/aleksclark/primer/curriculum-studio/internal/db"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/google/uuid"
)

func main() {
	dryRun := flag.Bool("dry-run", true, "report eligible rows without deleting")
	workspace := flag.String("workspace", "", "workspace UUID to retain (required)")
	flag.Parse()
	id, err := uuid.Parse(*workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-workspace must be a UUID")
		os.Exit(2)
	}
	cfg, err := studiodb.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	pool, err := studiodb.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()
	counts, err := repo.NewRetentionRepo(pool).Retain(context.Background(), id, time.Now().UTC(), repo.DefaultRetentionPolicy(), *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("webhook_deliveries=%d idempotency_keys=%d audit_events=%d dry_run=%t\n", counts.WebhookDeliveries, counts.IdempotencyKeys, counts.AuditEvents, *dryRun)
}
