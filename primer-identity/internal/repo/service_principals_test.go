package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestServicePrincipalWritersRejectInvalidInputsBeforeDatabase(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := CreateServiceCredential(ctx, nil, ServiceCredentialInput{})
	require.Error(t, err)
	_, err = RotateServiceCredential(ctx, nil, uuid.Nil, ServiceCredentialInput{}, now)
	require.Error(t, err)
	require.Error(t, RevokeServiceCredential(ctx, nil, uuid.Nil, now))
	require.Error(t, DisableServicePrincipal(ctx, nil, uuid.Nil, now))
	_, err = CreateServiceGrant(ctx, nil, uuid.Nil, uuid.Nil, "", "", nil, now, now)
	require.Error(t, err)
}
