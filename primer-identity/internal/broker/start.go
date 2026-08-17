package broker

import (
	"context"

	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/aleksclark/primer/identity/internal/secrethash"
)

// BoundCookie reports whether the presented broker cookie is bound to a live
// transaction. Every failure is the same non-oracular unbound error.
func (s *Service) BoundCookie(ctx context.Context, cookieValue string) error {
	_, err := s.liveBindingForCookie(ctx, cookieValue)
	return err
}

// StartInput is the bounded application start request. HTTP later consumes it
// without persisting email, connection, or organization selectors.
type StartInput struct {
	Method         brokerprovider.Method
	EmailAddress   string
	OrganizationID string
	ConnectionID   string
}

// Start begins a provider authentication attempt after confirming the broker
// cookie is bound. It does not persist provider handles or artifacts.
func (s *Service) Start(ctx context.Context, cookieValue string, in StartInput) (brokerprovider.StartResult, error) {
	if err := s.BoundCookie(ctx, cookieValue); err != nil {
		return brokerprovider.StartResult{}, err
	}
	return s.provider.StartLogin(ctx, brokerprovider.StartRequest{
		Method:         in.Method,
		EmailAddress:   in.EmailAddress,
		OrganizationID: in.OrganizationID,
		ConnectionID:   in.ConnectionID,
	})
}

// StartMethod begins a provider authentication attempt after confirming the
// broker cookie is bound. It does not persist provider handles or artifacts.
func (s *Service) StartMethod(ctx context.Context, cookieValue string, method brokerprovider.Method) (brokerprovider.StartResult, error) {
	return s.Start(ctx, cookieValue, StartInput{Method: method})
}

func (s *Service) liveBindingForCookie(ctx context.Context, cookieValue string) (liveBinding, error) {
	if cookieValue == "" || len(cookieValue) > maxCookieValueLen {
		return liveBinding{}, ErrUnboundCallback
	}
	cookieHash, err := secrethash.Hash(secrethash.Peppers(s.secrets.BrokerCookiePeppers),
		s.secrets.BrokerCookieActiveVersion, cookieHashContext, []byte(cookieValue))
	if err != nil {
		return liveBinding{}, ErrUnboundCallback
	}
	return s.loadLiveBinding(ctx, cookieHash)
}
