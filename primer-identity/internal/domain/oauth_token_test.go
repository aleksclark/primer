package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateClientAssertionReplayRejectsReplayBounds(t *testing.T) {
	valid := validClientAssertionReplay()
	if err := ValidateClientAssertionReplay(valid); err != nil {
		t.Fatalf("valid replay rejected: %v", err)
	}
	// Retention may extend through JWT exp + AssertionClockSkew (parser accept window).
	retain := validClientAssertionReplay()
	retain.ExpiresAt = retain.IssuedAt.Add(MaxAssertionTTL + AssertionClockSkew)
	retain.ConsumedAt = retain.IssuedAt.Add(MaxAssertionTTL + AssertionClockSkew)
	if err := ValidateClientAssertionReplay(retain); err != nil {
		t.Fatalf("skew retention window rejected: %v", err)
	}
	// consumed_at may sit after JWT exp while still inside retention.
	late := validClientAssertionReplay()
	jwtExp := late.IssuedAt.Add(2 * time.Minute)
	late.ExpiresAt = jwtExp.Add(AssertionClockSkew)
	late.ConsumedAt = jwtExp.Add(59 * time.Second)
	if err := ValidateClientAssertionReplay(late); err != nil {
		t.Fatalf("late skew consumption rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*ClientAssertionReplay)
	}{
		{"jti hash", func(in *ClientAssertionReplay) { in.JTIHash = make([]byte, 31) }},
		{"endpoint", func(in *ClientAssertionReplay) { in.EndpointKind = "introspect" }},
		{"audience", func(in *ClientAssertionReplay) { in.Audience = "" }},
		{"audience control", func(in *ClientAssertionReplay) { in.Audience = "https://x\x00/token" }},
		{"lifetime", func(in *ClientAssertionReplay) {
			in.IssuedAt = valid.IssuedAt
			in.ExpiresAt = valid.IssuedAt.Add(MaxAssertionTTL + AssertionClockSkew + time.Second)
		}},
		{"zero lifetime", func(in *ClientAssertionReplay) { in.ExpiresAt = in.IssuedAt }},
		{"consumed after retention", func(in *ClientAssertionReplay) {
			in.ConsumedAt = in.ExpiresAt.Add(time.Second)
		}},
		{"consumed before iat skew", func(in *ClientAssertionReplay) {
			in.ConsumedAt = in.IssuedAt.Add(-AssertionClockSkew - time.Second)
		}},
		{"client", func(in *ClientAssertionReplay) { in.OAuthClientID = uuid.Nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validClientAssertionReplay()
			tc.mutate(&in)
			if err := ValidateClientAssertionReplay(in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateInitialRefreshIssuanceRejectsCapsAndHash(t *testing.T) {
	valid := validInitialRefresh()
	if err := ValidateInitialRefreshIssuance(valid); err != nil {
		t.Fatalf("valid issuance rejected: %v", err)
	}
	now := valid.Family.LastRotatedAt
	cases := []struct {
		name   string
		mutate func(*InitialRefreshIssuance)
	}{
		{"hash", func(in *InitialRefreshIssuance) { in.Token.TokenHash = make([]byte, 31) }},
		{"pepper", func(in *InitialRefreshIssuance) { in.Token.PepperVersion = 0 }},
		{"sequence", func(in *InitialRefreshIssuance) { in.Token.Sequence = 1 }},
		{"consumed", func(in *InitialRefreshIssuance) {
			ts := now
			in.Token.ConsumedAt = &ts
		}},
		{"idle after absolute", func(in *InitialRefreshIssuance) {
			in.Family.IdleExpiresAt = in.Family.AbsoluteExpiresAt.Add(time.Hour)
			in.Token.ExpiresAt = in.Family.IdleExpiresAt
		}},
		{"absolute after grant", func(in *InitialRefreshIssuance) {
			in.Family.AbsoluteExpiresAt = in.GrantNotAfter.Add(time.Hour)
			in.Family.IdleExpiresAt = in.Family.AbsoluteExpiresAt.Add(-time.Hour)
			in.Token.ExpiresAt = in.Family.IdleExpiresAt
		}},
		{"absolute after provider", func(in *InitialRefreshIssuance) {
			in.Family.AbsoluteExpiresAt = in.ProviderExpiresAt.Add(time.Hour)
			in.Family.IdleExpiresAt = now.Add(24 * time.Hour)
			in.Token.ExpiresAt = in.Family.IdleExpiresAt
		}},
		{"idle ceiling", func(in *InitialRefreshIssuance) {
			in.Family.IdleExpiresAt = now.Add(MaxRefreshIdle + time.Hour)
			in.Token.ExpiresAt = in.Family.IdleExpiresAt
		}},
		{"absolute ceiling", func(in *InitialRefreshIssuance) {
			in.Family.AbsoluteExpiresAt = now.Add(MaxRefreshAbsolute + time.Hour)
			in.GrantNotAfter = in.Family.AbsoluteExpiresAt
			in.ProviderExpiresAt = in.Family.AbsoluteExpiresAt
			in.Token.ExpiresAt = in.Family.IdleExpiresAt
		}},
		{"status", func(in *InitialRefreshIssuance) { in.Family.Status = RefreshFamilyStatusRevoked }},
		{"public client id", func(in *InitialRefreshIssuance) { in.ClientID = in.Family.OAuthClientID.String() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInitialRefresh()
			tc.mutate(&in)
			if err := ValidateInitialRefreshIssuance(in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateTokenIssuanceAuditCopiesEvidenceWithoutSecrets(t *testing.T) {
	valid := validTokenIssuanceAudit()
	if err := ValidateTokenIssuanceAudit(valid); err != nil {
		t.Fatalf("valid audit rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*TokenIssuanceAudit)
	}{
		{"jti", func(in *TokenIssuanceAudit) { in.JTIHash = make([]byte, 16) }},
		{"code id without hash", func(in *TokenIssuanceAudit) {
			id := uuid.New()
			in.AuthorizationCodeID = &id
			in.AuthorizationCodeHash = nil
		}},
		{"internal client uuid", func(in *TokenIssuanceAudit) { in.ClientID = uuid.NewString() }},
		{"subject", func(in *TokenIssuanceAudit) { in.SubjectRef = "acct-1" }},
		{"kid control", func(in *TokenIssuanceAudit) { in.Kid = "kid\x00" }},
		{"outcome", func(in *TokenIssuanceAudit) { in.Outcome = "signed" }},
		{"ttl", func(in *TokenIssuanceAudit) { in.ExpiresAt = in.IssuedAt.Add(16 * time.Minute) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validTokenIssuanceAudit()
			tc.mutate(&in)
			if err := ValidateTokenIssuanceAudit(in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateOAuthClientKeyRejectsPrivateMaterialAndProfile(t *testing.T) {
	valid := validOAuthClientKey()
	if err := ValidateOAuthClientKey(valid); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	priv := validOAuthClientKey()
	priv.JWKJSON = []byte(`{"kty":"EC","crv":"P-256","x":"a","y":"b","d":"private"}`)
	if err := ValidateOAuthClientKey(priv); !errors.Is(err, ErrInvalid) {
		t.Fatalf("private jwk = %v, want ErrInvalid", err)
	}
	badAlg := validOAuthClientKey()
	badAlg.Alg = "RS256"
	if err := ValidateOAuthClientKey(badAlg); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rs256 = %v, want ErrInvalid", err)
	}
}

func TestValidateClaimAuthorizationCodeBinding(t *testing.T) {
	valid := validClaimAuthorizationCode()
	if err := ValidateClaimAuthorizationCode(valid); err != nil {
		t.Fatalf("valid claim rejected: %v", err)
	}
	bad := validClaimAuthorizationCode()
	bad.CodeHash = make([]byte, 8)
	if err := ValidateClaimAuthorizationCode(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short hash = %v, want ErrInvalid", err)
	}
	bad = validClaimAuthorizationCode()
	bad.CodeVerifier = "short"
	if err := ValidateClaimAuthorizationCode(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short verifier = %v, want ErrInvalid", err)
	}
}

func TestValidateClaimAuthorizationCodeRequiresBoundedUTCNow(t *testing.T) {
	valid := validClaimAuthorizationCode()
	if err := ValidateClaimAuthorizationCode(valid); err != nil {
		t.Fatalf("valid claim rejected: %v", err)
	}
	zero := valid
	zero.Now = time.Time{}
	if err := ValidateClaimAuthorizationCode(zero); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero now = %v, want ErrInvalid", err)
	}
	local := valid
	local.Now = time.Date(2026, 8, 17, 16, 0, 0, 0, time.FixedZone("CST", -6*3600))
	if err := ValidateClaimAuthorizationCode(local); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-UTC now = %v, want ErrInvalid", err)
	}
}

func TestBindingMatchesRejectsClientRedirectResourceAudienceAndPKCE(t *testing.T) {
	code := validIB2AuthorizationCode()
	claim := validClaimAuthorizationCode()
	claim.OAuthClientID = code.OAuthClientID
	claim.RedirectURI = code.RedirectURI
	claim.ResourceURI = code.ResourceURI
	claim.Audience = code.Audience
	if err := BindingMatches(code, claim); err != nil {
		t.Fatalf("matching bindings rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*ClaimAuthorizationCodeInput)
	}{
		{"client", func(in *ClaimAuthorizationCodeInput) { in.OAuthClientID = uuid.New() }},
		{"redirect", func(in *ClaimAuthorizationCodeInput) { in.RedirectURI = "https://other.example/callback" }},
		{"resource", func(in *ClaimAuthorizationCodeInput) { in.ResourceURI = "https://other.example/resource" }},
		{"audience", func(in *ClaimAuthorizationCodeInput) { in.Audience = "other-audience" }},
		{"pkce", func(in *ClaimAuthorizationCodeInput) { in.CodeVerifier = strings.Repeat("b", 43) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := claim
			tc.mutate(&in)
			if err := BindingMatches(code, in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateConsumeBindingsRequireActiveGrantAndProvider(t *testing.T) {
	in, claim := validConsumeBindings()
	if err := ValidateConsumeBindings(in, claim); err != nil {
		t.Fatalf("valid consume bindings rejected: %v", err)
	}
	expired := in
	expired.Now = in.Grant.NotAfter.Add(time.Second)
	if err := ValidateConsumeBindings(expired, claim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expired grant = %v, want ErrInvalid", err)
	}
	revoked := in
	revoked.Grant.Status = "revoked"
	if err := ValidateConsumeBindings(revoked, claim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revoked grant = %v, want ErrInvalid", err)
	}
	wrongProvider := in
	wrongProvider.Association.ID = uuid.New()
	if err := ValidateConsumeBindings(wrongProvider, claim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("provider mismatch = %v, want ErrInvalid", err)
	}
	wrongGrantClient := in
	wrongGrantClient.Grant.OAuthClientID = uuid.New()
	if err := ValidateConsumeBindings(wrongGrantClient, claim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("grant client = %v, want ErrInvalid", err)
	}
	zeroNow := in
	zeroNow.Now = time.Time{}
	if err := ValidateConsumeBindings(zeroNow, claim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero consume now = %v, want ErrInvalid", err)
	}
}

func TestValidateConsumedAuthorizationCodeAndFormatHelpers(t *testing.T) {
	code := validIB2AuthorizationCode()
	if err := ValidateConsumedAuthorizationCode(code); err != nil {
		t.Fatalf("valid consumed code rejected: %v", err)
	}
	code.CodeHash = make([]byte, 8)
	if err := ValidateConsumedAuthorizationCode(code); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short hash = %v, want ErrInvalid", err)
	}
	if got := FormatScopes([]string{"profile", "openid"}); got != "openid profile" {
		t.Fatalf("canonical scopes = %q", got)
	}
	if err := Must32("jti_hash", make([]byte, 16)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Must32 = %v, want ErrInvalid", err)
	}
	internal := uuid.New()
	if err := FormatClientBinding(internal.String(), internal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("internal uuid client = %v, want ErrInvalid", err)
	}
	if err := FormatClientBinding("public-client", internal); err != nil {
		t.Fatalf("public client rejected: %v", err)
	}
}

func TestVerifyS256PKCEAcceptsExactChallenge(t *testing.T) {
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if err := VerifyS256PKCE(verifier, challenge); err != nil {
		t.Fatalf("valid pkce rejected: %v", err)
	}
	if err := VerifyS256PKCE(verifier+"x", challenge); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatch = %v, want ErrInvalid", err)
	}
	if err := VerifyS256PKCE("+++", challenge); !errors.Is(err, ErrInvalid) {
		t.Fatalf("grammar = %v, want ErrInvalid", err)
	}
}

func TestHumanSubjectRefUsesAccountID(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := HumanSubjectRef(id)
	if got != "identity:11111111-1111-1111-1111-111111111111" {
		t.Fatalf("got %q", got)
	}
}

func validClientAssertionReplay() ClientAssertionReplay {
	now := time.Now().UTC().Truncate(time.Second)
	return ClientAssertionReplay{
		OAuthClientID: uuid.New(),
		EndpointKind:  AssertionEndpointToken,
		JTIHash:       make([]byte, 32),
		Audience:      "https://identity.example/oauth/token",
		IssuedAt:      now,
		ExpiresAt:     now.Add(2 * time.Minute),
		ConsumedAt:    now,
	}
}

func validInitialRefresh() InitialRefreshIssuance {
	now := time.Now().UTC().Truncate(time.Second)
	return InitialRefreshIssuance{
		Family: OAuthRefreshFamily{
			GrantID: uuid.New(), OAuthClientID: uuid.New(),
			ResourceURI: "https://resource.example", Status: RefreshFamilyStatusActive,
			AbsoluteExpiresAt: now.Add(90 * 24 * time.Hour), IdleExpiresAt: now.Add(14 * 24 * time.Hour),
			LastRotatedAt: now,
		},
		Token: OAuthRefreshToken{
			TokenHash: make([]byte, 32), PepperVersion: 1, Sequence: 0,
			IssuedAt: now, ExpiresAt: now.Add(14 * 24 * time.Hour),
		},
		ClientID:          "public-client",
		GrantNotAfter:     now.Add(120 * 24 * time.Hour),
		ProviderExpiresAt: now.Add(100 * 24 * time.Hour),
	}
}

func validTokenIssuanceAudit() TokenIssuanceAudit {
	now := time.Now().UTC().Truncate(time.Second)
	hash := make([]byte, 32)
	codeID := uuid.New()
	return TokenIssuanceAudit{
		GrantID: uuid.New(), AuthorizationCodeID: &codeID, AuthorizationCodeHash: hash,
		SubjectRef: HumanSubjectRef(uuid.New()), ClientID: "public-client",
		ResourceURI: "https://resource.example", Audience: "audience",
		Scopes: []string{"openid"}, JTIHash: hash, Kid: "key-1",
		IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute), Outcome: IssuanceOutcomeCommitted,
	}
}

func validOAuthClientKey() OAuthClientKey {
	return OAuthClientKey{
		OAuthClientID: uuid.New(), Kid: "client-key-1",
		JWKJSON: []byte(`{"kty":"EC","crv":"P-256","x":"abc","y":"def"}`),
		Alg:     "ES256", Use: "sig", Enabled: true,
	}
}

func validClaimAuthorizationCode() ClaimAuthorizationCodeInput {
	return ClaimAuthorizationCodeInput{
		CodeHash: make([]byte, 32), OAuthClientID: uuid.New(),
		RedirectURI: "https://bff.example/callback", ResourceURI: "https://resource.example",
		Audience: "audience", CodeVerifier: strings.Repeat("a", 43),
		Now: time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC),
	}
}

func validIB2AuthorizationCode() OAuthAuthorizationCode {
	now := time.Now().UTC().Truncate(time.Second)
	sum := sha256.Sum256([]byte(strings.Repeat("a", 43)))
	return OAuthAuthorizationCode{
		GrantID: uuid.New(), OAuthClientID: uuid.New(),
		CodeHash: make([]byte, 32), PepperVersion: 1,
		RedirectURI: "https://bff.example/callback", ResourceURI: "https://resource.example",
		Audience: "audience", Scopes: []string{"openid"},
		PKCEChallenge: base64.RawURLEncoding.EncodeToString(sum[:]), PKCEMethod: "S256",
		IssuedAt: now, ExpiresAt: now.Add(30 * time.Second),
	}
}

func validConsumeBindings() (ConsumeBindings, ClaimAuthorizationCodeInput) {
	code := validIB2AuthorizationCode()
	accountID := uuid.New()
	assocID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	in := ConsumeBindings{
		Code: code,
		Grant: OAuthGrant{
			ID: code.GrantID, AccountID: &accountID, OAuthClientID: code.OAuthClientID,
			ProviderSessionAssociationID: &assocID, ResourceURI: code.ResourceURI, Audience: code.Audience,
			Scopes: []string{"openid"}, SubjectClass: "human", Status: "active",
			GrantedAt: now, NotAfter: now.Add(24 * time.Hour),
		},
		Association: ProviderSessionAssociation{
			ID: assocID, AccountID: accountID, StytchMappingID: uuid.New(), Provider: "stytch_b2b",
			ProviderProjectID: "proj", ProviderOrganizationID: "org", ProviderMemberID: "member",
			ProviderMemberSessionID: "sess", ProviderExpiresAt: now.Add(time.Hour), Status: "active",
			LastValidatedAt: now,
		},
		Now: now,
	}
	claim := ClaimAuthorizationCodeInput{
		CodeHash: code.CodeHash, OAuthClientID: code.OAuthClientID,
		RedirectURI: code.RedirectURI, ResourceURI: code.ResourceURI,
		Audience: code.Audience, CodeVerifier: strings.Repeat("a", 43),
		Now: now,
	}
	return in, claim
}
