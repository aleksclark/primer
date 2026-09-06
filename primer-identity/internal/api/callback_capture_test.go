package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aleksclark/primer/identity/internal/api"
	"github.com/aleksclark/primer/identity/internal/broker"
	"github.com/aleksclark/primer/identity/internal/brokerprovider"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCallbackAttributionCaptureIsolationAndRelease(t *testing.T) {
	svc := newBrokerService(t, scripted(t, successFixture(uniqueLabel("unused"), brokerprovider.MethodEmailMagicLink)))
	handler, _ := newBrokerAPI(t, svc)
	a, b := httptest.NewServer(handler), httptest.NewServer(handler)
	defer a.Close()
	defer b.Close()
	rid := uuid.NewString()
	ca, releaseA := api.CaptureBrokerCallbackForTest(strings.TrimPrefix(a.URL, "http://"), rid)
	defer releaseA()
	cb, releaseB := api.CaptureBrokerCallbackForTest(strings.TrimPrefix(b.URL, "http://"), rid)
	defer releaseB()
	request := func(server *httptest.Server, binding bool) {
		path := "/broker/stytch/callback"
		if binding {
			path += "?token=not-a-fixture&type=discovery_magic_link"
		}
		req, err := http.NewRequest("GET", server.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set("X-Request-ID", rid)
		req.Host = strings.TrimPrefix(b.URL, "http://") // Host must not select the capture.
		if binding {
			req.Header.Set("Cookie", broker.BrokerCookieName+"=not-a-bound-cookie")
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 400, resp.StatusCode)
	}
	request(a, false)
	da, ok := ca.Snapshot()
	require.True(t, ok)
	require.Equal(t, "input", da.Stage())
	_, ok = cb.Snapshot()
	require.False(t, ok)
	request(b, true)
	db, ok := cb.Snapshot()
	require.True(t, ok)
	require.Equal(t, "binding", db.Stage())
	releaseB()
	request(b, false)
	db, _ = cb.Snapshot()
	require.Equal(t, "binding", db.Stage(), "released sink must not receive later requests")
	next, releaseNext := api.CaptureBrokerCallbackForTest(strings.TrimPrefix(b.URL, "http://"), rid)
	defer releaseNext()
	releaseB() // stale cleanup must not remove the replacement capture
	request(b, false)
	dn, ok := next.Snapshot()
	require.True(t, ok)
	require.Equal(t, "input", dn.Stage())
}
