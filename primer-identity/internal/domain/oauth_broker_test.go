package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

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
