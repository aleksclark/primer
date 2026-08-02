package jellyfin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
)

// itemsJSON is a canned /Items response with one fully populated item.
const itemsJSON = `{
  "Items": [
    {
      "Id": "abc123",
      "Name": "Bill Nye: Inertia",
      "SortName": "Bill Nye Inertia",
      "Overview": "Objects in motion.",
      "Type": "Episode",
      "RunTimeTicks": 13500000000,
      "Container": "mkv",
      "ImageTags": {"Primary": "tag-1"},
      "MediaStreams": [
        {"Type": "Video", "Codec": "h264"},
        {"Type": "Video", "Codec": "ignored"},
        {"Type": "Audio", "Codec": "aac"},
        {"Type": "Subtitle", "Codec": "srt"}
      ]
    }
  ]
}`

func newClient(t *testing.T, handler http.Handler) (*jellyfin.HTTPClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := jellyfin.New(jellyfin.Options{
		BaseURL:    srv.URL,
		APIKey:     "secret-key",
		UserID:     "user-1",
		HTTPClient: srv.Client(),
	})
	require.NoError(t, err)
	return client, srv
}

func TestNewValidatesBaseURL(t *testing.T) {
	t.Parallel()

	_, err := jellyfin.New(jellyfin.Options{})
	assert.Error(t, err, "base url is required")

	client, err := jellyfin.New(jellyfin.Options{BaseURL: "http://example.test/"})
	require.NoError(t, err)
	assert.NotContains(t, client.StreamURL("x"), "//Videos", "trailing slash should be trimmed")
}

func TestPing(t *testing.T) {
	t.Parallel()
	var gotPath, gotToken string
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Emby-Token")
		_, _ = w.Write([]byte(`{"Version":"10.9.0"}`))
	}))

	require.NoError(t, client.Ping(context.Background()))
	assert.Equal(t, "/System/Info", gotPath)
	assert.Equal(t, "secret-key", gotToken, "the API key authenticates the request")
}

func TestPingPropagatesFailure(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	assert.Error(t, client.Ping(context.Background()))
}

func TestBrowse(t *testing.T) {
	t.Parallel()
	var gotQuery url.Values
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(itemsJSON))
	}))

	items, err := client.Browse(context.Background(), jellyfin.BrowseParams{
		ParentID:   "lib-1",
		SearchTerm: "nye",
		Limit:      10,
		StartIndex: 5,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, "abc123", item.ID)
	assert.Equal(t, "Bill Nye: Inertia", item.Name)
	assert.Equal(t, "Bill Nye Inertia", item.SortName)
	assert.Equal(t, "Episode", item.Type)
	assert.Equal(t, 1350*time.Second, item.Runtime, "RunTimeTicks convert to a duration")
	assert.Equal(t, 1350, item.RuntimeSeconds())
	assert.Equal(t, "mkv", item.Container)
	assert.Equal(t, "h264", item.VideoCodec, "first video stream wins")
	assert.Equal(t, "aac", item.AudioCodec)
	assert.Equal(t, "tag-1", item.ImageTag)

	assert.Equal(t, "lib-1", gotQuery.Get("ParentId"))
	assert.Equal(t, "nye", gotQuery.Get("SearchTerm"))
	assert.Equal(t, "10", gotQuery.Get("Limit"))
	assert.Equal(t, "5", gotQuery.Get("StartIndex"))
	assert.Equal(t, "user-1", gotQuery.Get("UserId"))
	assert.Equal(t, "true", gotQuery.Get("Recursive"))
}

func TestBrowseOmitsEmptyParams(t *testing.T) {
	t.Parallel()
	var gotQuery url.Values
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))

	items, err := client.Browse(context.Background(), jellyfin.BrowseParams{})
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Empty(t, gotQuery.Get("ParentId"))
	assert.Empty(t, gotQuery.Get("SearchTerm"))
	assert.Empty(t, gotQuery.Get("Limit"))
	assert.Empty(t, gotQuery.Get("StartIndex"))
}

func TestBrowseErrors(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, err := client.Browse(context.Background(), jellyfin.BrowseParams{})
	assert.Error(t, err)

	malformed, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	_, err = malformed.Browse(context.Background(), jellyfin.BrowseParams{})
	assert.Error(t, err, "a malformed body is an error")
}

func TestItem(t *testing.T) {
	t.Parallel()
	var gotQuery url.Values
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(itemsJSON))
	}))

	item, err := client.Item(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", item.ID)
	assert.Equal(t, "abc123", gotQuery.Get("Ids"))
}

func TestItemNotFound(t *testing.T) {
	t.Parallel()

	empty, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))
	_, err := empty.Item(context.Background(), "missing")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound, "an empty result means the item is gone")

	// An empty ID short-circuits without a request.
	_, err = empty.Item(context.Background(), "")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound)

	notFound, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err = notFound.Item(context.Background(), "missing")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound)
}

func TestStreamURLForcesDirectPlay(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.NotFoundHandler())

	raw := client.StreamURL("abc 123")
	parsed, err := url.Parse(raw)
	require.NoError(t, err)

	assert.Equal(t, "/Videos/abc%20123/stream", parsed.EscapedPath(), "item IDs are path-escaped")
	assert.Equal(t, "true", parsed.Query().Get("static"), "static=true disables transcoding")
	assert.Equal(t, "secret-key", parsed.Query().Get("api_key"))
}

func TestImageURL(t *testing.T) {
	t.Parallel()
	client, srv := newClient(t, http.NotFoundHandler())

	assert.Equal(t, srv.URL+"/Items/abc/Images/Primary?tag=t1", client.ImageURL("abc", "Primary", "t1"))
	assert.Equal(t, srv.URL+"/Items/abc/Images/Primary", client.ImageURL("abc", "", ""),
		"an empty type defaults to Primary and an empty tag is omitted")
	assert.Equal(t, srv.URL+"/Items/abc/Images/Backdrop", client.ImageURL("abc", "Backdrop", ""))
}

func TestFetchImage(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/Items/abc/Images/Primary", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))

	data, contentType, err := client.FetchImage(context.Background(), "abc", "Primary", "t1")
	require.NoError(t, err)
	assert.Equal(t, []byte("png-bytes"), data)
	assert.Equal(t, "image/png", contentType)
}

func TestFetchImageDefaultsContentType(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil
		_, _ = w.Write([]byte{0x01})
	}))

	_, contentType, err := client.FetchImage(context.Background(), "abc", "Primary", "")
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", contentType)
}

func TestFetchImageErrors(t *testing.T) {
	t.Parallel()

	notFound, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, _, err := notFound.FetchImage(context.Background(), "abc", "Primary", "")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound)

	broken, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, _, err = broken.FetchImage(context.Background(), "abc", "Primary", "")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, jellyfin.ErrNotFound)
}

func TestRequestsHonourContextCancellation(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(itemsJSON))
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Browse(ctx, jellyfin.BrowseParams{})
	assert.Error(t, err)
	_, _, err = client.FetchImage(ctx, "abc", "Primary", "")
	assert.Error(t, err)
	assert.Error(t, client.Ping(ctx))
}

func TestFakeClientBehavesLikeTheRealOne(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(
		jellyfin.Item{ID: "a", Name: "Apollo 13", Runtime: 2 * time.Hour},
		jellyfin.Item{ID: "b", Name: "Bill Nye", Runtime: time.Hour},
	)

	require.NoError(t, fake.Ping(context.Background()))

	all, err := fake.Browse(context.Background(), jellyfin.BrowseParams{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
	assert.Equal(t, 1, fake.BrowseCalls)

	// Case-insensitive search
	found, err := fake.Browse(context.Background(), jellyfin.BrowseParams{SearchTerm: "bill"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "b", found[0].ID)

	// Paging
	paged, err := fake.Browse(context.Background(), jellyfin.BrowseParams{StartIndex: 1, Limit: 1})
	require.NoError(t, err)
	require.Len(t, paged, 1)
	assert.Equal(t, "b", paged[0].ID)

	beyond, err := fake.Browse(context.Background(), jellyfin.BrowseParams{StartIndex: 99})
	require.NoError(t, err)
	assert.Empty(t, beyond)

	item, err := fake.Item(context.Background(), "a")
	require.NoError(t, err)
	assert.Equal(t, "Apollo 13", item.Name)
	assert.Equal(t, 7200, item.RuntimeSeconds())

	_, err = fake.Item(context.Background(), "missing")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound)

	assert.Contains(t, fake.StreamURL("a"), "static=true")
	assert.Contains(t, fake.ImageURL("a", "", "t"), "/Images/Primary")
	assert.Contains(t, fake.ImageURL("a", "Backdrop", ""), "/Images/Backdrop")

	data, contentType, err := fake.FetchImage(context.Background(), "a", "Primary", "")
	require.NoError(t, err)
	assert.Equal(t, []byte("fake-image"), data)
	assert.Equal(t, "image/jpeg", contentType)

	_, _, err = fake.FetchImage(context.Background(), "missing", "Primary", "")
	assert.ErrorIs(t, err, jellyfin.ErrNotFound)
}

func TestFakeClientSurfacesConfiguredError(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(jellyfin.Item{ID: "a"})
	fake.Err = assert.AnError

	assert.ErrorIs(t, fake.Ping(context.Background()), assert.AnError)
	_, err := fake.Browse(context.Background(), jellyfin.BrowseParams{})
	assert.ErrorIs(t, err, assert.AnError)
	_, err = fake.Item(context.Background(), "a")
	assert.ErrorIs(t, err, assert.AnError)
	_, _, err = fake.FetchImage(context.Background(), "a", "Primary", "")
	assert.ErrorIs(t, err, assert.AnError)
}
