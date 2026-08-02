package primer_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/primer"
)

// ingestServer stands in for the LMS ingest endpoint, recording what arrived.
type ingestServer struct {
	*httptest.Server

	Requests []primer.InstructionLog
	Tokens   []string
}

// newIngestServer serves the given status and body for every ingest.
func newIngestServer(t *testing.T, status int, body string) *ingestServer {
	t.Helper()
	s := &ingestServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, primer.IngestPath, r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		s.Tokens = append(s.Tokens, r.Header.Get(primer.ServiceTokenHeader))

		raw, _ := io.ReadAll(r.Body)
		var log primer.InstructionLog
		require.NoError(t, json.Unmarshal(raw, &log))
		s.Requests = append(s.Requests, log)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

// sampleLog is a well-formed instruction log.
var sampleLog = primer.InstructionLog{
	Source:         primer.SourceTV,
	SourceRef:      "session-1",
	MediaTitle:     "Bill Nye: Inertia",
	Class:          "educational",
	SubjectTags:    []string{"science"},
	StandardCodes:  []string{"TN.SCI.6.PS.2"},
	WatchedSeconds: 1500,
	OccurredOn:     "2031-04-15",
}

func TestNewRequiresConfiguration(t *testing.T) {
	t.Parallel()

	_, err := primer.New(primer.Options{})
	assert.ErrorIs(t, err, primer.ErrNotConfigured,
		"an absent LMS switches reporting off rather than failing")

	_, err = primer.New(primer.Options{BaseURL: "primer.example"})
	assert.Error(t, err, "a base url without a scheme cannot be posted to")

	_, err = primer.New(primer.Options{BaseURL: "://nope"})
	assert.Error(t, err)
}

func TestClientIngestPostsTheLog(t *testing.T) {
	t.Parallel()
	srv := newIngestServer(t, http.StatusCreated, `{"log":{"id":"log-7"},"created":true}`)

	client, err := primer.New(primer.Options{
		BaseURL:      srv.URL + "/",
		ServiceToken: "s3cret",
	})
	require.NoError(t, err)

	result, err := client.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.Equal(t, "log-7", result.LogID)
	assert.True(t, result.Created)

	require.Len(t, srv.Requests, 1)
	assert.Equal(t, sampleLog, srv.Requests[0], "the log crosses the wire unchanged")
	assert.Equal(t, []string{"s3cret"}, srv.Tokens, "the service token authenticates the call")
}

func TestClientIngestTreatsADuplicateAsSuccess(t *testing.T) {
	t.Parallel()
	srv := newIngestServer(t, http.StatusOK, `{"log":{"id":"log-7"},"created":false}`)

	client, err := primer.New(primer.Options{BaseURL: srv.URL})
	require.NoError(t, err)

	result, err := client.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.False(t, result.Created, "a replay is reported, not treated as a failure")
	assert.Empty(t, srv.Tokens[0], "no token configured means none is sent")
}

func TestClientIngestSurfacesRefusalsAndGarbage(t *testing.T) {
	t.Parallel()

	refused := newIngestServer(t, http.StatusUnprocessableEntity, `{"detail":"class must be educational"}`)
	client, err := primer.New(primer.Options{BaseURL: refused.URL})
	require.NoError(t, err)
	_, err = client.Ingest(context.Background(), sampleLog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session-1", "the error names the viewing that was refused")
	assert.Contains(t, err.Error(), "class must be educational")

	garbage := newIngestServer(t, http.StatusCreated, `not json`)
	client, err = primer.New(primer.Options{BaseURL: garbage.URL})
	require.NoError(t, err)
	_, err = client.Ingest(context.Background(), sampleLog)
	assert.Error(t, err)
}

func TestClientIngestSurfacesTransportFailure(t *testing.T) {
	t.Parallel()
	srv := newIngestServer(t, http.StatusCreated, `{}`)
	url := srv.URL
	srv.Close()

	client, err := primer.New(primer.Options{BaseURL: url, Timeout: time.Second})
	require.NoError(t, err)
	_, err = client.Ingest(context.Background(), sampleLog)
	assert.Error(t, err, "an LMS that is down is an error the reporter can retry, not a panic")
}

func TestClientIngestHonoursContextCancellation(t *testing.T) {
	t.Parallel()
	srv := newIngestServer(t, http.StatusCreated, `{"log":{"id":"x"},"created":true}`)

	client, err := primer.New(primer.Options{BaseURL: srv.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Ingest(ctx, sampleLog)
	assert.Error(t, err)
}

func TestFakeIngesterMirrorsTheIdempotencyContract(t *testing.T) {
	t.Parallel()
	fake := primer.NewFake()

	first, err := fake.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.True(t, first.Created)

	second, err := fake.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.False(t, second.Created)
	assert.Equal(t, first.LogID, second.LogID)

	other := sampleLog
	other.SourceRef = "session-2"
	third, err := fake.Ingest(context.Background(), other)
	require.NoError(t, err)
	assert.True(t, third.Created)

	assert.Len(t, fake.Accepted(), 2)
	assert.Equal(t, 3, fake.Calls)

	fake.Err = assert.AnError
	_, err = fake.Ingest(context.Background(), sampleLog)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestZeroValueFakeIsUsable(t *testing.T) {
	t.Parallel()
	// A fake built as a struct literal rather than through NewFake still has to
	// deduplicate, so a caller cannot accidentally lose the contract.
	var fake primer.Fake

	first, err := fake.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.True(t, first.Created)

	second, err := fake.Ingest(context.Background(), sampleLog)
	require.NoError(t, err)
	assert.False(t, second.Created)
}
