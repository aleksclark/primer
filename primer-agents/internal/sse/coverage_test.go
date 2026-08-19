package sse_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/agents/internal/appservice"
	"github.com/aleksclark/primer/agents/internal/sse"
	"github.com/aleksclark/primer/agents/internal/testutil"
)

func TestSSEHeartbeatAndBoundedLifetime(t *testing.T) {
	pool := testutil.DB(t)
	svc := appservice.New(pool)
	ns := "coverage-sse-heartbeat-" + uuid.NewString()
	run, err := svc.CreateRun(context.Background(), appservice.CreateRunCmd{OwnerNamespace: ns, IdempotencyKey: "heartbeat", Profile: "tutor"})
	require.NoError(t, err)

	cfg := sse.DefaultConfig()
	cfg.PollInterval = 5 * time.Millisecond
	cfg.HeartbeatInterval = 5 * time.Millisecond
	cfg.MaxLifetime = 40 * time.Millisecond
	cfg.WriteTimeout = time.Second
	rec := newResponseRecorder()
	sse.Stream(context.Background(), rec, pool, run.ID, ns, 0, cfg)
	require.Contains(t, rec.body.String(), ": keepalive")
}

type responseRecorder struct {
	body strings.Builder
}

func newResponseRecorder() *responseRecorder            { return &responseRecorder{} }
func (r *responseRecorder) Header() http.Header         { return make(http.Header) }
func (r *responseRecorder) Write(p []byte) (int, error) { return r.body.Write(p) }
func (r *responseRecorder) WriteHeader(int)             {}
func (r *responseRecorder) Flush()                      {}
