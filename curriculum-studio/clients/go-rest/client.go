// Package gorest is the committed façade over the generated Studio authoring
// REST client. Models and operation methods come only from generated/.
package gorest

import (
	"context"
	"errors"
	"net/http"

	"github.com/aleksclark/primer/curriculum-studio/clients/go-rest/generated"
)

type bearerSource struct{ token string }

func (b bearerSource) BearerAuth(context.Context, generated.OperationName) (generated.BearerAuth, error) {
	if b.token == "" {
		return generated.BearerAuth{}, errors.New("studio REST bearer token is required")
	}
	return generated.BearerAuth{Token: b.token}, nil
}

// Client embeds the generated operation client without redefining any wire
// models. Use generated response unions to inspect typed success/error bodies.
type Client struct{ *generated.Client }

// NewClient constructs a client for an emitted Studio OpenAPI server.
func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	inner, err := generated.NewClient(baseURL, bearerSource{token: token}, generated.WithClient(httpClient))
	if err != nil {
		return nil, err
	}
	return &Client{Client: inner}, nil
}
