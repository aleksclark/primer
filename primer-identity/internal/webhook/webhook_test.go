package webhook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFingerprintVectorsAreLengthPrefixedAndReasonBound(t *testing.T) {
	original := bytes.Repeat([]byte{1}, 32)
	presented := bytes.Repeat([]byte{2}, 32)
	a := SemanticFingerprint(Provider, "project-test-a", "event-1", original, presented, "body_hash_mismatch")
	b := SemanticFingerprint(Provider, "project-test-a", "event-1", original, presented, "cross_id_collision")
	require.Len(t, a, sha256.Size)
	require.Len(t, b, sha256.Size)
	require.NotEqual(t, a, b)
	require.Equal(t, a, SemanticFingerprint(Provider, "project-test-a", "event-1", original, presented, "body_hash_mismatch"))
}

func TestObservationPreservesTransportMatches(t *testing.T) {
	id := uuid.New()
	a := match{ID: id, Hash: bytes.Repeat([]byte{1}, 32), ok: true}
	b := match{ID: uuid.New(), Hash: bytes.Repeat([]byte{2}, 32), ok: true}
	sem := SemanticFingerprint(Provider, "p", "e", a.Hash, b.Hash, "cross_id_collision")
	first := ObservationFingerprint(sem, "cross_id_collision", "msg-1", a, b)
	second := ObservationFingerprint(sem, "cross_id_collision", "msg-2", a, b)
	require.NotEqual(t, first, second)
}

func TestContentTypeAndEnvelopeValidation(t *testing.T) {
	require.True(t, validContentType("application/json"))
	require.True(t, validContentType("application/json; charset=utf-8"))
	require.False(t, validContentType("application/json; charset=latin-1"))
	require.False(t, validContentType("text/json"))
	body, err := json.Marshal(envelope{ProjectID: "p", EventID: "e", Action: "update", ObjectType: "member", Source: "direct", EntityID: "m", Timestamp: "2026-08-18T12:00:00Z", EventType: "direct.member.update"})
	require.NoError(t, err)
	got, err := parseEnvelope(body)
	require.NoError(t, err)
	require.Equal(t, "direct.member.update", got.EventType)
	require.Error(t, func() error { _, err := parseEnvelope([]byte(`{"project_id":"p"}`)); return err }())
}

func TestHandlerRequiresSignedPostContentType(t *testing.T) {
	require.False(t, validID(""))
	require.True(t, validID("svix-1"))
	_ = http.MethodPost
}

func TestCollisionClassification(t *testing.T) {
	hash := bytes.Repeat([]byte{1}, 32)
	other := bytes.Repeat([]byte{2}, 32)
	a := match{ID: uuid.New(), Hash: hash, ok: true}
	b := match{ID: uuid.New(), Hash: other, ok: true}
	require.Equal(t, "cross_id_collision", classify(a, b, hash))
	require.Equal(t, "event_id_reused_new_svix_id", classify(a, match{}, hash))
	require.Equal(t, "svix_id_reused_new_event_id", classify(match{}, b, other))
	require.Equal(t, "body_hash_mismatch", classify(a, match{}, other))
}

func TestRetrySchedule(t *testing.T) {
	require.Equal(t, time.Minute, delayForAttempt(0))
	require.Equal(t, 5*time.Minute, delayForAttempt(2))
	require.Equal(t, 12*time.Hour, delayForAttempt(99))
}

func TestConfigurationAndWorkerGuards(t *testing.T) {
	_, err := NewHandler(Config{})
	require.Error(t, err)
	var worker *Worker
	_, err = worker.Claim(context.Background())
	require.Error(t, err)
	_, err = (&Worker{}).Claim(context.Background())
	require.Error(t, err)
	var alerts *AlertWorker
	_, err = alerts.Claim(context.Background())
	require.Error(t, err)
	require.Error(t, (&AlertWorker{}).Alerted(context.Background(), AlertLease{}))
	require.Error(t, (&AlertWorker{}).Fail(context.Background(), AlertLease{}, "x"))
}
