// Package factory provides Identity test data builders.
package factory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/aleksclark/primer/identity/internal/domain"
	"github.com/aleksclark/primer/identity/internal/repo"
)

var seq atomic.Uint64

// Account creates an active account with a unique display name/email.
func Account(t *testing.T, q repo.Querier, overrides ...func(*domain.CreateAccountInput)) *domain.Account {
	t.Helper()
	n := seq.Add(1)
	email := fmt.Sprintf("user%d@example.com", n)
	in := domain.CreateAccountInput{
		DisplayName:  fmt.Sprintf("User %d", n),
		PrimaryEmail: &email,
	}
	for _, o := range overrides {
		o(&in)
	}
	acct, err := repo.CreateAccount(context.Background(), q, in)
	if err != nil {
		t.Fatalf("factory.Account: %v", err)
	}
	return acct
}

// AccountWithPassword creates an account and sets a known password.
func AccountWithPassword(t *testing.T, q repo.Querier, password string, overrides ...func(*domain.CreateAccountInput)) *domain.Account {
	t.Helper()
	acct := Account(t, q, overrides...)
	if err := repo.SetPassword(context.Background(), q, acct.ID, password); err != nil {
		t.Fatalf("factory.AccountWithPassword: %v", err)
	}
	return acct
}

// ExternalIdentityFor attaches provider+subject to the given account.
// Empty subject gets a unique generated value.
func ExternalIdentityFor(t *testing.T, q repo.Querier, account *domain.Account, provider, subject string) *domain.ExternalIdentity {
	t.Helper()
	if subject == "" {
		n := seq.Add(1)
		subject = fmt.Sprintf("subject-%d", n)
	}
	ident, err := repo.AttachExternalIdentity(context.Background(), q, account.ID, provider, subject)
	if err != nil {
		t.Fatalf("factory.ExternalIdentityFor: %v", err)
	}
	return ident
}
