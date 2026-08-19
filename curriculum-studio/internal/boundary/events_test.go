package boundary_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/boundary"
)

func TestEventTypeWireNameUsesGeneratedClosedEnum(t *testing.T) {
	for _, tc := range []struct {
		typ  v1.EventType
		wire string
	}{
		{v1.EventType_EVENT_TYPE_CURRICULUM_CREATED, "curriculum.created"},
		{v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED, "plan_revision.published"},
		{v1.EventType_EVENT_TYPE_MATERIALIZATION_REQUESTED, "materialization.requested"},
		{v1.EventType_EVENT_TYPE_MATERIALIZATION_READY, "materialization.ready"},
		{v1.EventType_EVENT_TYPE_MATERIALIZATION_FAILED, "materialization.failed"},
		{v1.EventType_EVENT_TYPE_MATERIALIZED_ITEM_SUPERSEDED, "materialized_item.superseded"},
		{v1.EventType_EVENT_TYPE_PLAN_CHANGE_PROPOSED, "plan_change.proposed"},
	} {
		got, err := boundary.EventTypeWireName(tc.typ)
		require.NoError(t, err)
		require.Equal(t, tc.wire, got)
	}
	_, err := boundary.EventTypeWireName(v1.EventType_EVENT_TYPE_UNSPECIFIED)
	require.Error(t, err)
}

func TestEventDeliveryHeadersIncludeEnvelopeAndHMAC(t *testing.T) {
	event := &v1.DomainEvent{
		Id:         "evt_1",
		Type:       v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED,
		OccurredAt: timestamppb.Now(),
	}
	payload := []byte(`{"id":"evt_1"}`)
	secret := []byte("test-secret")
	headers, err := boundary.EventDeliveryHeaders(event, payload, secret)
	require.NoError(t, err)
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "evt_1", headers.Get(boundary.EventIDHeader))
	require.Equal(t, "plan_revision.published", headers.Get(boundary.EventTypeHeader))

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	require.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), headers.Get(boundary.EventSignatureHeader))
}
