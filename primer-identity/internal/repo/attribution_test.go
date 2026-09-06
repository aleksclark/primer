package repo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type diagnosticPoisonError struct{}

func (diagnosticPoisonError) Error() string {
	panic("diagnostic classification must not inspect arbitrary error text")
}

func TestFailureAttributionClosedClassesNeverUseErrorText(t *testing.T) {
	require.Equal(t, "unknown", repo.ClassifyFailure(diagnosticPoisonError{}).String())
	marker := "DO_NOT_LOG_artifact_cookie_jwt_account_client_sql_value"
	for code, want := range map[string]string{"40001": "serialization_conflict", "40P01": "deadlock", "23505": "conflict", "23503": "not_found", "23514": "invalid", "53300": "capacity", "57014": "query_canceled", marker: "unknown"} {
		err := &pgconn.PgError{Code: code, Message: marker, Detail: marker, ConstraintName: marker, TableName: marker, Where: marker}
		class := repo.ClassifyFailure(err).String()
		require.Equal(t, want, class)
		if strings.Contains(class, marker) {
			t.Fatal("diagnostic included unsafe error metadata")
		}
	}
	require.Equal(t, "unknown", repo.ClassifyFailure(errors.New(marker+" SQLSTATE 40001")).String(), "legacy retry text detection is not a trusted diagnostic classifier")
	require.Equal(t, "unknown", repo.FailureClass(255).String())
}

func TestFailureAttributionSerializableExhaustionAndRecovery(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	attempts := 0
	marker := "DO_NOT_LOG_" + uuid.NewString()
	err := repo.WithSerializableRetry(ctx, pool, func(tx pgx.Tx) error {
		attempts++
		_, e := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{DisplayName: marker})
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `DO $$ BEGIN RAISE EXCEPTION 'sensitive diagnostic test marker' USING ERRCODE='40001'; END $$`)
		return e
	})
	require.Error(t, err)
	require.Equal(t, 8, attempts, "budget must remain exactly eight")
	info := repo.RetryFailureDetails(err)
	require.Equal(t, "serialization_conflict", info.Class().String())
	require.Equal(t, 8, info.Attempts())
	require.Equal(t, 8, info.Limit())
	require.True(t, info.Exhausted())
	var pe *pgconn.PgError
	require.True(t, errors.As(err, &pe), "wrapping must preserve typed cause for existing retry control flow")
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE display_name=$1`, marker).Scan(&count))
	require.Zero(t, count, "every failed transaction rolled back")
	attempts = 0
	err = repo.WithSerializableRetry(ctx, pool, func(tx pgx.Tx) error {
		attempts++
		if attempts == 1 {
			return &pgconn.PgError{Code: "40001", Message: marker}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Zero(t, repo.RetryFailureDetails(err).Attempts(), "a recovered error is not terminal attribution")
	cause := errors.New(marker)
	attempts = 0
	err = repo.WithSerializableRetry(ctx, pool, func(tx pgx.Tx) error { attempts++; return cause })
	require.True(t, errors.Is(err, cause))
	require.Equal(t, 1, attempts)
	info = repo.RetryFailureDetails(err)
	require.Equal(t, "unknown", info.Class().String())
	require.Equal(t, 1, info.Attempts())
	require.False(t, info.Exhausted())
}
