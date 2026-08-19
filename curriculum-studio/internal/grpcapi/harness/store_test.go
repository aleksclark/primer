package harness_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/grpcapi/harness"
)

func TestDeliverEventRecordsEnvelopeHeadersAndAttempt(t *testing.T) {
	store := harness.New()
	event := harness.FixtureEvent("evt_c9", v1.EventType_EVENT_TYPE_PLAN_REVISION_PUBLISHED, "ws_1", "prev_1")
	var got http.Header
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(receiver.Close)

	delivery, err := store.DeliverEvent(t.Context(), "wh_1", receiver.URL, event, []byte("secret"))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, delivery.HTTPStatus)
	require.Equal(t, "evt_c9", delivery.EventID)
	require.Equal(t, "plan_revision.published", got.Get("X-Curriculum-Studio-Event-Type"))
	require.Equal(t, "evt_c9", got.Get("X-Curriculum-Studio-Event-Id"))
	require.NotEmpty(t, got.Get("X-Curriculum-Studio-Signature"))
	require.Len(t, store.Deliveries(), 1)
}
