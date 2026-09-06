package repo_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
)

func stytchInput(org, member, email, display string) domain.StytchMappingInput {
	return domain.StytchMappingInput{
		StytchPrincipal: domain.StytchPrincipal{
			ProjectID: "project-test", OrganizationID: org, MemberID: member,
		},
		DisplayName: display, PrimaryEmail: &email,
	}
}

func TestFindAccountIDByStytchTupleUsesExactNormalizedTuple(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-find", "member-find", "same@example.com", "Find me")

	acct, err := repo.ResolveOrCreateStytchMapping(ctx, pool, input)
	require.NoError(t, err)

	got, err := repo.FindAccountIDByStytchTuple(ctx, pool, input.StytchPrincipal)
	require.NoError(t, err)
	require.Equal(t, acct.ID, got)
}

func TestResolveOrCreateStytchTupleIsStableAcrossProfileChanges(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	first := stytchInput("org-stable", "member-stable", "first@example.com", "First")
	second := stytchInput("org-stable", "member-stable", "changed@example.com", "Changed")

	one, err := repo.ResolveOrCreateStytchMapping(ctx, pool, first)
	require.NoError(t, err)
	two, err := repo.ResolveOrCreateStytchMapping(ctx, pool, second)
	require.NoError(t, err)
	require.Equal(t, one.ID, two.ID, "profile metadata must never remap the tuple")

	accounts, err := repo.ListAccountsByEmail(ctx, pool, "changed@example.com")
	require.NoError(t, err)
	require.Empty(t, accounts, "email must not be used as a lookup or remapping key")
}

func TestResolveOrCreateStytchSameEmailDifferentOrganizationsCreatesDistinctAccounts(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	one, err := repo.ResolveOrCreateStytchMapping(ctx, pool, stytchInput("org-one", "same-member", "same@example.com", "Same"))
	require.NoError(t, err)
	two, err := repo.ResolveOrCreateStytchMapping(ctx, pool, stytchInput("org-two", "same-member", "same@example.com", "Same"))
	require.NoError(t, err)
	require.NotEqual(t, one.ID, two.ID)

	rows, err := pool.Query(ctx, `SELECT account_id FROM stytch_mappings WHERE project_id = $1 AND member_id = $2 ORDER BY organization_id`, "project-test", "same-member")
	require.NoError(t, err)
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, ids, 2)
	require.NotEqual(t, ids[0], ids[1])
}

func TestCreateStytchMappingConflictFailsClosed(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-conflict", "member-conflict", "conflict@example.com", "Conflict")
	first := testutil.Tx(t)
	acctOne, err := repo.CreateAccount(ctx, first, domain.CreateAccountInput{DisplayName: "one"})
	require.NoError(t, err)
	require.NoError(t, first.Commit(ctx))
	second := testutil.Tx(t)
	acctTwo, err := repo.CreateAccount(ctx, second, domain.CreateAccountInput{DisplayName: "two"})
	require.NoError(t, err)
	require.NoError(t, second.Commit(ctx))

	_, err = repo.CreateStytchMapping(ctx, pool, acctOne.ID, input.StytchPrincipal)
	require.NoError(t, err)
	_, err = repo.CreateStytchMapping(ctx, pool, acctTwo.ID, input.StytchPrincipal)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrConflict), fmt.Sprintf("expected conflict, got %v", err))
	got, err := repo.FindAccountIDByStytchTuple(ctx, pool, input.StytchPrincipal)
	require.NoError(t, err)
	require.Equal(t, acctOne.ID, got)
}

func TestResolveOrCreateStytchTupleConcurrentFirstLoginConvergesWithoutOrphans(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real PostgreSQL concurrency harness")
	}
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-race", "member-race", "race@example.com", "Race")
	const callers = 12

	start := make(chan struct{})
	results := make(chan *domain.Account, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			acct, err := repo.ResolveOrCreateStytchMapping(ctx, pool, input)
			if err != nil {
				errs <- err
				return
			}
			results <- acct
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var accounts []*domain.Account
	for acct := range results {
		accounts = append(accounts, acct)
	}
	for err := range errs {
		require.NoError(t, err)
	}
	require.Len(t, accounts, callers)
	for _, acct := range accounts[1:] {
		require.Equal(t, accounts[0].ID, acct.ID)
	}

	var mappingCount, accountCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stytch_mappings WHERE project_id=$1 AND organization_id=$2 AND member_id=$3`, input.ProjectID, input.OrganizationID, input.MemberID).Scan(&mappingCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE id IN (SELECT account_id FROM stytch_mappings WHERE project_id=$1 AND organization_id=$2 AND member_id=$3)`, input.ProjectID, input.OrganizationID, input.MemberID).Scan(&accountCount))
	require.Equal(t, 1, mappingCount)
	require.Equal(t, 1, accountCount)
	var sameMetadataAccounts int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE primary_email=$1`, "race@example.com").Scan(&sameMetadataAccounts))
	require.Equal(t, 1, sameMetadataAccounts, "losing first-login attempts must not leave orphan accounts")
}

func TestResolveOrCreateStytchMappingUsesSerializableAndRetriesExactlyOnce(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-serializable-retry-"+uuid.NewString(), "member-serializable-retry-"+uuid.NewString(), "", "Retry")
	input.PrimaryEmail = ptr("serializable-retry-" + uuid.NewString() + "@example.com")

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	sequenceName := "stytch_mapping_retry_seq_" + suffix
	functionName := "stytch_mapping_retry_fn_" + suffix
	triggerName := "stytch_mapping_retry_trigger_" + suffix
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE SEQUENCE %s START WITH 1`, sequenceName))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger
LANGUAGE plpgsql AS $function$
DECLARE
	attempt bigint;
BEGIN
	IF NEW.primary_email = '%s' THEN
		IF current_setting('transaction_isolation') <> 'serializable' THEN
			RAISE EXCEPTION 'mapping transaction isolation was %%', current_setting('transaction_isolation') USING ERRCODE = '55000';
		END IF;
		attempt := nextval('%s');
		IF attempt = 1 THEN
			RAISE EXCEPTION 'forced mapping serialization retry' USING ERRCODE = '40001';
		END IF;
	END IF;
	RETURN NEW;
END
$function$`, functionName, *input.PrimaryEmail, sequenceName))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON accounts FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON accounts`, triggerName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP SEQUENCE IF EXISTS %s`, sequenceName))
	})

	account, err := repo.ResolveOrCreateStytchMapping(ctx, pool, input)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, account.ID)

	var attempts int64
	require.NoError(t, pool.QueryRow(ctx, fmt.Sprintf(`SELECT last_value FROM %s`, sequenceName)).Scan(&attempts))
	require.Equal(t, int64(2), attempts, "one forced serialization failure must cause exactly one retry")
	var accountCount, mappingCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE primary_email = $1`, *input.PrimaryEmail).Scan(&accountCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stytch_mappings WHERE project_id = $1 AND organization_id = $2 AND member_id = $3`, input.ProjectID, input.OrganizationID, input.MemberID).Scan(&mappingCount))
	require.Equal(t, 1, accountCount)
	require.Equal(t, 1, mappingCount)
}

func TestResolveOrCreateStytchMappingExhaustsThreeRetryableAttemptsAndRollsBack(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-serializable-exhausted-"+uuid.NewString(), "member-serializable-exhausted-"+uuid.NewString(), "", "Exhausted")
	input.PrimaryEmail = ptr("serializable-exhausted-" + uuid.NewString() + "@example.com")
	sequenceName := installStytchMappingTrigger(t, pool, input, "40P01", true)

	_, err := repo.ResolveOrCreateStytchMapping(ctx, pool, input)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "40P01", pgErr.Code)
	info := repo.RetryFailureDetails(err)
	require.Equal(t, "deadlock", info.Class().String())
	require.Equal(t, 3, info.Attempts())
	require.Equal(t, 3, info.Limit())
	require.True(t, info.Exhausted())

	var attempts int64
	require.NoError(t, pool.QueryRow(ctx, fmt.Sprintf(`SELECT last_value FROM %s`, sequenceName)).Scan(&attempts))
	require.Equal(t, int64(3), attempts)
	var accountCount, mappingCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE primary_email = $1`, *input.PrimaryEmail).Scan(&accountCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stytch_mappings WHERE project_id = $1 AND organization_id = $2 AND member_id = $3`, input.ProjectID, input.OrganizationID, input.MemberID).Scan(&mappingCount))
	require.Equal(t, 0, accountCount, "exhausted attempts must not leave an account orphan")
	require.Equal(t, 0, mappingCount)
}

func TestResolveOrCreateStytchMappingDoesNotRetryNonretryableSQLState(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	input := stytchInput("org-nonretryable-"+uuid.NewString(), "member-nonretryable-"+uuid.NewString(), "", "Nonretryable")
	input.PrimaryEmail = ptr("nonretryable-" + uuid.NewString() + "@example.com")
	sequenceName := installStytchMappingTrigger(t, pool, input, "23514", true)

	_, err := repo.ResolveOrCreateStytchMapping(ctx, pool, input)
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalid)
	info := repo.RetryFailureDetails(err)
	require.Equal(t, "invalid", info.Class().String())
	require.Equal(t, 1, info.Attempts())
	require.False(t, info.Exhausted())

	var attempts int64
	require.NoError(t, pool.QueryRow(ctx, fmt.Sprintf(`SELECT last_value FROM %s`, sequenceName)).Scan(&attempts))
	require.Equal(t, int64(1), attempts, "nonretryable SQLSTATE must not be retried")
	var accountCount, mappingCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE primary_email = $1`, *input.PrimaryEmail).Scan(&accountCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM stytch_mappings WHERE project_id = $1 AND organization_id = $2 AND member_id = $3`, input.ProjectID, input.OrganizationID, input.MemberID).Scan(&mappingCount))
	require.Equal(t, 0, accountCount)
	require.Equal(t, 0, mappingCount)
}

func installStytchMappingTrigger(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, input domain.StytchMappingInput, sqlState string, alwaysFail bool) string {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	sequenceName := "stytch_mapping_retry_seq_" + suffix
	functionName := "stytch_mapping_retry_fn_" + suffix
	triggerName := "stytch_mapping_retry_trigger_" + suffix
	_, err := pool.Exec(context.Background(), fmt.Sprintf(`CREATE SEQUENCE %s START WITH 1`, sequenceName))
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), fmt.Sprintf(`
CREATE FUNCTION %s() RETURNS trigger
LANGUAGE plpgsql AS $function$
DECLARE
	attempt bigint;
BEGIN
	IF NEW.primary_email = '%s' THEN
		IF current_setting('transaction_isolation') <> 'serializable' THEN
			RAISE EXCEPTION 'mapping transaction isolation was %%', current_setting('transaction_isolation') USING ERRCODE = '55000';
		END IF;
		attempt := nextval('%s');
		IF %t OR attempt = 1 THEN
			RAISE EXCEPTION 'forced mapping failure' USING ERRCODE = '%s';
		END IF;
	END IF;
	RETURN NEW;
END
$function$`, functionName, *input.PrimaryEmail, sequenceName, alwaysFail, sqlState))
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON accounts FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON accounts`, triggerName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`DROP SEQUENCE IF EXISTS %s`, sequenceName))
	})
	return sequenceName
}

func ptr(value string) *string { return &value }
