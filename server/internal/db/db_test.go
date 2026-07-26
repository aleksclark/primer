package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectInvalidURL(t *testing.T) {
	t.Parallel()
	_, err := Connect(context.Background(), "not-a-url://%%%")
	assert.Error(t, err)
}

func TestConnectUnreachable(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Connect(ctx, "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable")
	assert.Error(t, err)
}

func TestMigrateInvalidURL(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Migrate(ctx, "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable")
	assert.Error(t, err)
}

func TestMigrateDownInvalidURL(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := MigrateDown(ctx, "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable")
	assert.Error(t, err)
}
