// tasks-bootstrap is a privileged one-shot operator command, never an HTTP route.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/db"
)

func main() {
	var in db.ParentLink
	flag.StringVar(&in.Issuer, "issuer", "", "exact verified Clerk issuer (not email)")
	flag.StringVar(&in.Subject, "subject", "", "Clerk user subject from approved sign-in")
	flag.StringVar(&in.TenantID, "tenant-id", "", "existing local household UUID")
	flag.StringVar(&in.ActorRef, "actor-ref", "", "existing local parent subject_ref")
	flag.BoolVar(&in.CreateHousehold, "create-household", false, "explicitly provision initial household/admin membership")
	flag.StringVar(&in.HouseholdName, "household-name", "", "name for explicitly created household")
	flag.Parse()
	dsn := os.Getenv("TASKS_DATABASE_URL")
	if dsn == "" {
		fail("TASKS_DATABASE_URL is required")
	}
	if db.SafeDatabaseName(dsn) != nil {
		fail("unsafe Tasks database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fail("bootstrap database configuration invalid")
	}
	defer pool.Close()
	if err = db.BootstrapParent(ctx, pool, in); err != nil {
		fail("bootstrap refused; verify explicit mapping, active membership, and applied migrations")
	}
	fmt.Println("parent identity linked; existing local IDs preserved")
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(2) }
