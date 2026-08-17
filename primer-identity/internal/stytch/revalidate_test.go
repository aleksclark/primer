package stytch_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stytchauth/stytch-go/v18/stytch/b2b/sessions"

	"github.com/aleksclark/primer/identity/internal/stytch"
)

type revalidator interface {
	RevalidateMemberSession(context.Context, string, string, string, string) (stytch.SessionSnapshot, error)
}

func TestPinnedSessionsSDKSurface(t *testing.T) {
	params := reflect.TypeOf(sessions.GetParams{})
	require.Equal(t, 2, params.NumField())
	for i, name := range []string{"OrganizationID", "MemberID"} {
		f := params.Field(i)
		require.Equal(t, name, f.Name)
		require.Equal(t, reflect.TypeOf(""), f.Type)
	}
	response, ok := reflect.TypeOf(sessions.GetResponse{}).FieldByName("MemberSessions")
	require.True(t, ok)
	require.Equal(t, reflect.TypeOf([]sessions.MemberSession{}), response.Type)
}

func TestRevalidateMakesOneExactUnpaginatedGetAndReturnsBoundedSnapshot(t *testing.T) {
	expires := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/b2b/sessions", r.URL.Path)
		require.Equal(t, "org", r.URL.Query().Get("organization_id"))
		require.Equal(t, "member", r.URL.Query().Get("member_id"))
		_, _ = w.Write([]byte(`{"member_sessions":[{"member_session_id":"session","organization_id":"org","member_id":"member","expires_at":"` + expires + `"}]}`))
	}))
	defer server.Close()
	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	r, ok := client.(revalidator)
	require.True(t, ok)
	snapshot, err := r.RevalidateMemberSession(context.Background(), "project-test-example", "org", "member", "session")
	require.NoError(t, err)
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, "session", snapshot.ProviderMemberSessionID)
	require.True(t, snapshot.Active)
}

func TestRevalidateClassifiesDefinitiveAndUnavailableFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"missing", http.StatusOK, `{"member_sessions":[]}`, stytch.ErrDefinitive},
		{"expired", http.StatusOK, `{"member_sessions":[{"member_session_id":"session","organization_id":"org","member_id":"member","expires_at":"2000-01-01T00:00:00Z"}]}`, stytch.ErrDefinitive},
		{"rate", http.StatusTooManyRequests, `{}`, stytch.ErrProviderUnavailable},
		{"server", http.StatusBadGateway, `{}`, stytch.ErrProviderUnavailable},
		{"wrong tuple", http.StatusOK, `{"member_sessions":[{"member_session_id":"session","organization_id":"other","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}]}`, stytch.ErrProviderUnavailable},
		{"duplicate", http.StatusOK, `{"member_sessions":[{"member_session_id":"session","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"},{"member_session_id":"session","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}]}`, stytch.ErrProviderUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
			require.NoError(t, err)
			r := client.(revalidator)
			_, err = r.RevalidateMemberSession(context.Background(), "project-test-example", "org", "member", "session")
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestRevalidateRejectsSessionBoundsAndHonorsDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { time.Sleep(3 * time.Second) }))
	defer server.Close()
	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.(revalidator).RevalidateMemberSession(context.Background(), "project-test-example", "org", "member", "session")
	require.True(t, errors.Is(err, stytch.ErrProviderUnavailable))
}

func TestRevalidateOverflowMalformedAndWrongProjectAreUnavailableWithoutLeaks(t *testing.T) {
	overflow := `{"member_sessions":[`
	for i := 0; i < 257; i++ {
		if i > 0 {
			overflow += ","
		}
		overflow += `{"member_session_id":"session-` + itoa(i) + `","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}`
	}
	overflow += `]}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{"overflow", overflow},
		{"malformed id", `{"member_sessions":[{"member_session_id":"session\u0001","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}]}`},
		{"missing expires", `{"member_sessions":[{"member_session_id":"session","organization_id":"org","member_id":"member"}]}`},
		{"other present", `{"member_sessions":[{"member_session_id":"other","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.URL.Query().Get("cursor"))
				require.Empty(t, r.URL.Query().Get("limit"))
				require.Empty(t, r.URL.Query().Get("page"))
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
			require.NoError(t, err)
			_, err = client.(revalidator).RevalidateMemberSession(context.Background(), "project-test-example", "org", "member", "session")
			if tc.name == "other present" {
				require.ErrorIs(t, err, stytch.ErrDefinitive)
				require.Equal(t, stytch.ErrDefinitive.Error(), err.Error())
			} else {
				require.ErrorIs(t, err, stytch.ErrProviderUnavailable)
				require.Equal(t, stytch.ErrProviderUnavailable.Error(), err.Error())
			}
			require.NotContains(t, err.Error(), "session-")
			require.NotContains(t, err.Error(), "organization_id")
		})
	}
}

func TestRevalidateRejectsOversizedBodyAndDoesNotAcceptAfterCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"member_sessions":[{"member_session_id":"session","organization_id":"org","member_id":"member","expires_at":"2099-01-01T00:00:00Z"}`))
		_, _ = w.Write(bytes.Repeat([]byte(" "), 1<<20))
		_, _ = w.Write([]byte(`]}`))
	}))
	defer server.Close()
	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.(revalidator).RevalidateMemberSession(context.Background(), "project-test-example", "org", "member", "session")
	require.ErrorIs(t, err, stytch.ErrProviderUnavailable)
}

func TestRevalidateRejectsWrongProjectBeforeOutboundCall(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	client, err := stytch.NewWithHTTPClient(adapterConfig(server.URL), server.Client())
	require.NoError(t, err)
	_, err = client.(revalidator).RevalidateMemberSession(context.Background(), "project-test-other", "org", "member", "session")
	require.ErrorIs(t, err, stytch.ErrProviderUnavailable)
	require.Zero(t, calls)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
