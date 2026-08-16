package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func TestValidateBrokerTransactionInputRejectsInvalidProtocolBounds(t *testing.T) {
	valid := validBrokerTransactionInput()
	if err := ValidateCreateBrokerTransactionInput(valid); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*CreateBrokerTransactionInput)
	}{
		{"state hash", func(in *CreateBrokerTransactionInput) { in.StateHash = make([]byte, 31) }},
		{"cookie hash", func(in *CreateBrokerTransactionInput) { in.BrokerCookieHash = make([]byte, 31) }},
		{"sealed short", func(in *CreateBrokerTransactionInput) { in.StateSealed = make([]byte, 29) }},
		{"sealed long", func(in *CreateBrokerTransactionInput) { in.StateSealed = make([]byte, 1054) }},
		{"state length", func(in *CreateBrokerTransactionInput) { in.StateLength = 1025 }},
		{"pkce", func(in *CreateBrokerTransactionInput) { in.PKCEChallenge = strings.Repeat("=", 43) }},
		{"scope ordering", func(in *CreateBrokerTransactionInput) { in.RequestedScopes = []string{"z", "a"} }},
		{"scope duplicate", func(in *CreateBrokerTransactionInput) { in.RequestedScopes = []string{"a", "a"} }},
		{"expiry", func(in *CreateBrokerTransactionInput) {
			in.ExpiresAt = in.CreatedAt.Add(10*time.Minute + time.Nanosecond)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validBrokerTransactionInput()
			tc.mutate(&in)
			if err := ValidateCreateBrokerTransactionInput(in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCanonicalScopesSortsWithoutDeduping(t *testing.T) {
	got := CanonicalScopes([]string{"z", "a", "m"})
	if strings.Join(got, ",") != "a,m,z" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateProviderMemberSessionIDBounds(t *testing.T) {
	if err := ValidateProviderMemberSessionID("sess-1"); err != nil {
		t.Fatalf("valid id rejected: %v", err)
	}
	if err := ValidateProviderMemberSessionID(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty id = %v, want ErrInvalid", err)
	}
	if err := ValidateProviderMemberSessionID("sess\x00id"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("control id = %v, want ErrInvalid", err)
	}
	if err := ValidateProviderMemberSessionID(strings.Repeat("s", 256)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("256-rune id = %v, want ErrInvalid", err)
	}
	overBytes := strings.Repeat("é", 513)
	if utf8.RuneCountInString(overBytes) <= 255 && len(overBytes) <= 1024 {
		t.Fatal("fixture must exceed the 1024-byte snapshot bound")
	}
	if err := ValidateProviderMemberSessionID(overBytes); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlong-byte id = %v, want ErrInvalid", err)
	}
}

func TestValidateProviderSessionAssociationRequiresBoundedMemberSession(t *testing.T) {
	assoc := validAssociation()
	if err := ValidateProviderSessionAssociation(assoc); err != nil {
		t.Fatalf("valid association rejected: %v", err)
	}
	assoc.ID = uuid.Nil
	if err := ValidateProviderSessionAssociation(assoc); err != nil {
		t.Fatalf("create-time nil association id rejected: %v", err)
	}
	assoc.ProviderMemberSessionID = strings.Repeat("s", 256)
	if err := ValidateProviderSessionAssociation(assoc); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized member session = %v, want ErrInvalid", err)
	}
}

func TestValidateOAuthGrantRejectsServiceAndIncompleteHumanSubjects(t *testing.T) {
	grant := validHumanGrant()
	if err := ValidateOAuthGrant(grant); err != nil {
		t.Fatalf("valid human grant rejected: %v", err)
	}
	grant.SubjectClass = "service"
	grant.ServicePrincipalID = ptrUUID(uuid.New())
	grant.AccountID = nil
	grant.ProviderSessionAssociationID = nil
	if err := ValidateOAuthGrant(grant); !errors.Is(err, ErrInvalid) {
		t.Fatalf("service grant = %v, want ErrInvalid", err)
	}
}

func TestValidateAuthorizationCodeRejectsConsumptionMarkerAndOverlongLifetime(t *testing.T) {
	code := validAuthorizationCode()
	if err := ValidateAuthorizationCode(code); err != nil {
		t.Fatalf("valid code rejected: %v", err)
	}
	now := time.Now().UTC()
	consumed := now
	code.ConsumedAt = &consumed
	if err := ValidateAuthorizationCode(code); !errors.Is(err, ErrInvalid) {
		t.Fatalf("consumed code = %v, want ErrInvalid", err)
	}
	code = validAuthorizationCode()
	code.IssuedAt = now
	code.ExpiresAt = now.Add(61 * time.Second)
	if err := ValidateAuthorizationCode(code); !errors.Is(err, ErrInvalid) {
		t.Fatalf("61s lifetime = %v, want ErrInvalid", err)
	}
}

func validBrokerTransactionInput() CreateBrokerTransactionInput {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return CreateBrokerTransactionInput{
		OAuthClientID: uuid.New(), RedirectID: uuid.New(), StateHash: make([]byte, 32),
		StatePepperVersion: 1, StateSealed: make([]byte, 30), StateKeyVersion: 1, StateLength: 1,
		PKCEChallenge: strings.Repeat("A", 43), PKCEMethod: "S256", RequestedScopes: []string{"openid"},
		ResourceURI: "https://resource.example", Audience: "audience", BrokerCookieHash: make([]byte, 32),
		BrokerCookiePepperVersion: 1, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	}
}

func validAssociation() ProviderSessionAssociation {
	return ProviderSessionAssociation{
		ID: uuid.New(), AccountID: uuid.New(), StytchMappingID: uuid.New(),
		Provider: "stytch_b2b", ProviderProjectID: "proj", ProviderOrganizationID: "org",
		ProviderMemberID: "member", ProviderMemberSessionID: "sess-1",
		ProviderExpiresAt: time.Now().UTC().Add(time.Hour), Status: "active",
		LastValidatedAt: time.Now().UTC(),
	}
}

func validHumanGrant() OAuthGrant {
	accountID := uuid.New()
	assocID := uuid.New()
	now := time.Now().UTC()
	return OAuthGrant{
		ID: uuid.New(), AccountID: &accountID, OAuthClientID: uuid.New(),
		ProviderSessionAssociationID: &assocID, ResourceURI: "https://resource.example",
		Audience: "audience", Scopes: []string{"openid"}, SubjectClass: "human",
		Status: "active", GrantedAt: now, NotAfter: now.Add(time.Hour),
	}
}

func validAuthorizationCode() OAuthAuthorizationCode {
	now := time.Now().UTC()
	return OAuthAuthorizationCode{
		ID: uuid.New(), CodeHash: make([]byte, 32), PepperVersion: 1,
		GrantID: uuid.New(), BrokerTransactionID: uuid.New(), OAuthClientID: uuid.New(),
		RedirectURI: "https://bff.example/callback", ResourceURI: "https://resource.example",
		Audience: "audience", Scopes: []string{"openid"}, PKCEChallenge: strings.Repeat("A", 43),
		PKCEMethod: "S256", IssuedAt: now, ExpiresAt: now.Add(60 * time.Second),
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
