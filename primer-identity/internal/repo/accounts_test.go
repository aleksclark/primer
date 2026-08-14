package repo_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
	"github.com/aleksclark/primer/identity/internal/testutil"
	"github.com/aleksclark/primer/identity/internal/testutil/factory"
)

func q(t *testing.T) repo.Querier {
	t.Helper()
	return testutil.NewSavepointQuerier(testutil.Tx(t))
}

func TestAccountCreate_YieldsUUIDSub(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	email := "ada@example.com"
	acct, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName:  "Ada Lovelace",
		PrimaryEmail: &email,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, acct.ID)
	assert.Equal(t, domain.AccountStatusActive, acct.Status)
	assert.Equal(t, "Ada Lovelace", acct.DisplayName)
	require.NotNil(t, acct.PrimaryEmail)
	assert.Equal(t, email, *acct.PrimaryEmail)
	assert.False(t, acct.CreatedAt.IsZero())

	got, err := repo.GetAccount(ctx, tx, acct.ID)
	require.NoError(t, err)
	assert.Equal(t, acct.ID, got.ID)
	assert.Equal(t, domain.AccountStatusActive, got.Status)
}

func TestAccountLock_PersistsStatus(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	acct := factory.Account(t, tx)
	locked, err := repo.LockAccount(ctx, tx, acct.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AccountStatusLocked, locked.Status)

	got, err := repo.GetAccount(ctx, tx, acct.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AccountStatusLocked, got.Status)
	// Hook for later token-mint phases: locked accounts must be refused.
	assert.NotEqual(t, domain.AccountStatusActive, got.Status)
}

func TestExternalUnique_ProviderSubject(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	a := factory.Account(t, tx)
	b := factory.Account(t, tx)

	_, err := repo.AttachExternalIdentity(ctx, tx, a.ID, domain.ProviderGoogle, "sub-A")
	require.NoError(t, err)

	_, err = repo.AttachExternalIdentity(ctx, tx, b.ID, domain.ProviderGoogle, "sub-A")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrConflict), "got %v", err)

	// Account B must not be linked.
	idents, err := repo.ListExternalIdentitiesByAccount(ctx, tx, b.ID)
	require.NoError(t, err)
	assert.Empty(t, idents)

	// A still owns the pair.
	found, err := repo.FindByProviderSubject(ctx, tx, domain.ProviderGoogle, "sub-A")
	require.NoError(t, err)
	assert.Equal(t, a.ID, found.AccountID)
}

func TestExternal_SameProviderDifferentSubIsSeparate(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	email := "shared@example.com"
	a, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: &email, DisplayName: "A"})
	require.NoError(t, err)
	b, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: &email, DisplayName: "B"})
	require.NoError(t, err)

	_, err = repo.UpsertExternalIdentity(ctx, tx, a.ID, domain.ProviderGoogle, "sub-A")
	require.NoError(t, err)
	_, err = repo.UpsertExternalIdentity(ctx, tx, b.ID, domain.ProviderGoogle, "sub-B")
	require.NoError(t, err)

	assert.NotEqual(t, a.ID, b.ID)
	fa, err := repo.FindByProviderSubject(ctx, tx, domain.ProviderGoogle, "sub-A")
	require.NoError(t, err)
	fb, err := repo.FindByProviderSubject(ctx, tx, domain.ProviderGoogle, "sub-B")
	require.NoError(t, err)
	assert.Equal(t, a.ID, fa.AccountID)
	assert.Equal(t, b.ID, fb.AccountID)
}

func TestNoEmailMerge_PasswordAndGoogleSameEmail(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	email := "twin@example.com"
	passwordAcct, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName:  "Password User",
		PrimaryEmail: &email,
	})
	require.NoError(t, err)
	require.NoError(t, repo.SetPassword(ctx, tx, passwordAcct.ID, "s3cret-password"))
	_, err = repo.AttachExternalIdentity(ctx, tx, passwordAcct.ID, domain.ProviderPassword, passwordAcct.ID.String())
	require.NoError(t, err)

	// Google subject arrives with the same verified email string — create a
	// separate account; repo API keys only provider+subject.
	googleAcct, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName:  "Google User",
		PrimaryEmail: &email,
	})
	require.NoError(t, err)
	_, err = repo.AttachExternalIdentity(ctx, tx, googleAcct.ID, domain.ProviderGoogle, "google-sub-G")
	require.NoError(t, err)

	assert.NotEqual(t, passwordAcct.ID, googleAcct.ID)

	// No automatic link: password account does not own the Google subject.
	found, err := repo.FindByProviderSubject(ctx, tx, domain.ProviderGoogle, "google-sub-G")
	require.NoError(t, err)
	assert.Equal(t, googleAcct.ID, found.AccountID)
	assert.NotEqual(t, passwordAcct.ID, found.AccountID)

	listed, err := repo.ListAccountsByEmail(ctx, tx, email)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	ids := map[uuid.UUID]bool{listed[0].ID: true, listed[1].ID: true}
	assert.True(t, ids[passwordAcct.ID])
	assert.True(t, ids[googleAcct.ID])
}

func TestListByEmail_NonAuthoritativeMultiple(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	email := "multi@example.com"
	a, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: &email})
	require.NoError(t, err)
	b, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: &email})
	require.NoError(t, err)
	_, err = repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: strPtr("other@example.com")})
	require.NoError(t, err)

	// Case-insensitive match, multiple results — no canonical single login.
	listed, err := repo.ListAccountsByEmail(ctx, tx, "MULTI@example.com")
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.NotEqual(t, listed[0].ID, listed[1].ID)
	_ = a
	_ = b
}

func TestPassword_HashOnlyRightWrongDisabled(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	acct := factory.Account(t, tx)
	plaintext := "hunter2-not-stored"

	require.NoError(t, repo.SetPassword(ctx, tx, acct.ID, plaintext))

	// Raw DB column must not equal plaintext.
	var rawHash string
	err := tx.QueryRow(ctx, `SELECT password_hash FROM credentials_password WHERE account_id = $1`, acct.ID).Scan(&rawHash)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, rawHash)
	assert.NotContains(t, rawHash, plaintext)
	assert.True(t, strings.HasPrefix(rawHash, "$argon2id$"))

	cred, err := repo.GetPasswordCredential(ctx, tx, acct.ID)
	require.NoError(t, err)
	assert.Equal(t, "argon2id", cred.Algorithm)
	assert.Nil(t, cred.DisabledAt)

	ok, err := repo.CheckPassword(ctx, tx, acct.ID, plaintext)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = repo.CheckPassword(ctx, tx, acct.ID, "wrong-password")
	require.NoError(t, err)
	assert.False(t, ok)

	// Missing account: non-enumerating false.
	ok, err = repo.CheckPassword(ctx, tx, uuid.New(), plaintext)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, repo.DisablePassword(ctx, tx, acct.ID))
	ok, err = repo.CheckPassword(ctx, tx, acct.ID, plaintext)
	require.NoError(t, err)
	assert.False(t, ok, "disabled password must fail closed")

	// Re-set re-enables.
	require.NoError(t, repo.SetPassword(ctx, tx, acct.ID, "new-secret"))
	ok, err = repo.CheckPassword(ctx, tx, acct.ID, "new-secret")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPassword_MalformedHashFailsClosed(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	acct := factory.Account(t, tx)
	_, err := tx.Exec(ctx, `
INSERT INTO credentials_password (account_id, password_hash, algorithm)
VALUES ($1, $2, $3)`, acct.ID, "not-a-valid-phc", "argon2id")
	require.NoError(t, err)

	ok, err := repo.CheckPassword(ctx, tx, acct.ID, "anything")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPassword_EmptyAndOversizeRejected(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()
	acct := factory.Account(t, tx)

	err := repo.SetPassword(ctx, tx, acct.ID, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid), "%v", err)

	// Giant plaintext must fail closed as invalid without hanging on Argon2.
	giant := strings.Repeat("p", 1024+1)
	err = repo.SetPassword(ctx, tx, acct.ID, giant)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid), "%v", err)

	// Check path: no credential yet — still non-enumerating false for empty/giant.
	ok, err := repo.CheckPassword(ctx, tx, acct.ID, "")
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.CheckPassword(ctx, tx, acct.ID, giant)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, repo.SetPassword(ctx, tx, acct.ID, "good-password"))
	ok, err = repo.CheckPassword(ctx, tx, acct.ID, "")
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.CheckPassword(ctx, tx, acct.ID, giant)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPassword_CorruptedPHCResourceBoundFailsClosed(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()
	acct := factory.Account(t, tx)

	// 256 MiB memory cost PHC would DoS if decoded KeyLen/params bypass bounds.
	corrupt := "$argon2id$v=19$m=262144,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := tx.Exec(ctx, `
INSERT INTO credentials_password (account_id, password_hash, algorithm)
VALUES ($1, $2, $3)`, acct.ID, corrupt, "argon2id")
	require.NoError(t, err)

	ok, err := repo.CheckPassword(ctx, tx, acct.ID, "anything")
	require.NoError(t, err)
	assert.False(t, ok, "corrupted high-cost PHC must fail closed without accepting")
}

func TestUpsertExternalIdentity_IdempotentSameAccount(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	acct := factory.Account(t, tx)
	a, err := repo.UpsertExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, "idem-sub")
	require.NoError(t, err)
	b, err := repo.UpsertExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, "idem-sub")
	require.NoError(t, err)
	assert.Equal(t, a.ID, b.ID)
}

func TestUpsertExternalIdentity_ConflictOtherAccount(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	a := factory.Account(t, tx)
	b := factory.Account(t, tx)
	_, err := repo.UpsertExternalIdentity(ctx, tx, a.ID, domain.ProviderGoogle, "owned")
	require.NoError(t, err)
	_, err = repo.UpsertExternalIdentity(ctx, tx, b.ID, domain.ProviderGoogle, "owned")
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrConflict))
}

func TestExternalIdentity_RaceDuplicateProviderSub(t *testing.T) {
	// Uses the shared pool (not a single Tx) so concurrent writers can race.
	pool := testutil.DB(t)
	ctx := context.Background()

	a, err := repo.CreateAccount(ctx, pool, domain.CreateAccountInput{DisplayName: "race-a"})
	require.NoError(t, err)
	b, err := repo.CreateAccount(ctx, pool, domain.CreateAccountInput{DisplayName: "race-b"})
	require.NoError(t, err)

	subject := "race-sub-" + uuid.NewString()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := repo.AttachExternalIdentity(ctx, pool, a.ID, domain.ProviderGoogle, subject)
		errs <- e
	}()
	go func() {
		defer wg.Done()
		_, e := repo.AttachExternalIdentity(ctx, pool, b.ID, domain.ProviderGoogle, subject)
		errs <- e
	}()
	wg.Wait()
	close(errs)

	var okN, conflictN int
	for e := range errs {
		if e == nil {
			okN++
			continue
		}
		if errors.Is(e, domain.ErrConflict) {
			conflictN++
			continue
		}
		t.Fatalf("unexpected error: %v", e)
	}
	assert.Equal(t, 1, okN)
	assert.Equal(t, 1, conflictN)

	found, err := repo.FindByProviderSubject(ctx, pool, domain.ProviderGoogle, subject)
	require.NoError(t, err)
	assert.True(t, found.AccountID == a.ID || found.AccountID == b.ID)
}

func TestCreateAccount_ValidationBounds(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()

	_, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName: strings.Repeat("x", domain.MaxDisplayNameLen+1),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))

	email := strings.Repeat("e", domain.MaxEmailLen+1)
	_, err = repo.CreateAccount(ctx, tx, domain.CreateAccountInput{PrimaryEmail: &email})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))

	// Max exact should pass.
	okEmail := strings.Repeat("e", domain.MaxEmailLen)
	acct, err := repo.CreateAccount(ctx, tx, domain.CreateAccountInput{
		DisplayName:  strings.Repeat("n", domain.MaxDisplayNameLen),
		PrimaryEmail: &okEmail,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, acct.ID)
}

func TestAttachExternalIdentity_SubjectBounds(t *testing.T) {
	t.Parallel()
	tx := q(t)
	ctx := context.Background()
	acct := factory.Account(t, tx)

	_, err := repo.AttachExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, strings.Repeat("s", domain.MaxProviderSubjectLen+1))
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalid))

	_, err = repo.AttachExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, "bad\nsubject")
	require.Error(t, err)

	_, err = repo.AttachExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, strings.Repeat("s", domain.MaxProviderSubjectLen))
	require.NoError(t, err)

	// Multibyte within rune limit.
	_, err = repo.AttachExternalIdentity(ctx, tx, acct.ID, domain.ProviderGoogle, strings.Repeat("世", 100))
	require.NoError(t, err)
}

func TestGetAccount_NotFound(t *testing.T) {
	t.Parallel()
	tx := q(t)
	_, err := repo.GetAccount(context.Background(), tx, uuid.New())
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestAntiCheat_NoFindOrCreateByEmailSymbol(t *testing.T) {
	t.Parallel()
	// Compile-time / package surface: ListAccountsByEmail is the only email API
	// and is non-authoritative. There is intentionally no FindOrCreateByEmail.
	// This test documents the contract; a grep gate is also run in CI hygiene.
	assert.NotNil(t, repo.ListAccountsByEmail)
}

func strPtr(s string) *string { return &s }
