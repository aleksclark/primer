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
        {"Type": "Video", "Codec": "h264"},
        {"Type": "Audio", "Codec": "aac"},
        {"Type": "Audio", "Codec": "eac3"},
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
	assert.Equal(t, "h264", item.VideoCodec, "duplicate video codecs collapse to the unique set")
	assert.Equal(t, "aac,eac3", item.AudioCodec, "every audio stream is retained for direct-play checks")
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
	assert.ErrorIs(t, fake.RefreshLibrary(context.Background()), assert.AnError)
	_, err = fake.ScanRunning(context.Background())
	assert.ErrorIs(t, err, assert.AnError)
}

func TestRefreshAndScanRunning(t *testing.T) {
	t.Parallel()
	var refreshHits int
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/Refresh":
			assert.Equal(t, http.MethodPost, r.Method)
			refreshHits++
			w.WriteHeader(http.StatusNoContent)
		case "/ScheduledTasks":
			_, _ = w.Write([]byte(`[
				{"Name":"Scan Media Library","Key":"RefreshLibrary","State":"Running"},
				{"Name":"Other","Key":"x","State":"Idle"}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	require.NoError(t, client.RefreshLibrary(context.Background()))
	assert.Equal(t, 1, refreshHits)
	running, err := client.ScanRunning(context.Background())
	require.NoError(t, err)
	assert.True(t, running)
}

func TestScanRunningIdle(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Name":"Cleanup","Key":"clean","State":"Idle"}]`))
	}))
	running, err := client.ScanRunning(context.Background())
	require.NoError(t, err)
	assert.False(t, running)
}

func TestBrowseProviderAndPath(t *testing.T) {
	t.Parallel()
	var gotQuery url.Values
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		// Simulate a Jellyfin that IGNORES AnyProviderIdEquals and returns
		// unrelated library items — the client must filter them out.
		_, _ = w.Write([]byte(`{
			"Items": [
				{
					"Id": "ep1", "Name": "Ep1", "Type": "Episode",
					"Path": "/media/Shows/paul-sellers/Season 01/x.mkv",
					"ProviderIds": {"Tmdb": "603", "Tvdb": "79165"},
					"IndexNumber": 1, "ParentIndexNumber": 1, "SeriesName": "Paul Sellers",
					"SeriesId": "series-ps",
					"RunTimeTicks": 100000000
				},
				{
					"Id": "other", "Name": "3 Ninjas", "Type": "Movie",
					"Path": "/media/Movies/3 Ninjas.mkv",
					"ProviderIds": {"Tmdb": "1"}
				},
				{
					"Id": "no-provider", "Name": "Orphan", "Type": "Movie",
					"Path": "/media/Movies/Orphan.mkv"
				}
			]
		}`))
	}))

	items, err := client.Browse(context.Background(), jellyfin.BrowseParams{
		AnyProviderIDEquals: "Tmdb=603",
		PathContains:        "paul-sellers",
		IncludeProviderIDs:  true,
		IncludePath:         true,
		Limit:               20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1, "unrelated library items must be dropped client-side")
	assert.Equal(t, "ep1", items[0].ID)
	assert.Equal(t, "S01E01", items[0].EpisodeKey())
	assert.Equal(t, "603", items[0].ProviderID("tmdb"))
	assert.Equal(t, "Tmdb=603", gotQuery.Get("AnyProviderIdEquals"))
	assert.Contains(t, gotQuery.Get("Fields"), "ProviderIds")
	assert.Contains(t, gotQuery.Get("Fields"), "Path")
}

func TestBrowseDropsProviderMismatches(t *testing.T) {
	t.Parallel()
	client, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Whole-library dump with no matching provider id.
		_, _ = w.Write([]byte(`{
			"Items": [
				{"Id":"a","Name":"3 Ninjas","Type":"Movie","ProviderIds":{"Tmdb":"11234"}},
				{"Id":"b","Name":"50 First Dates","Type":"Movie","ProviderIds":{"Tmdb":"1824"}},
				{"Id":"c","Name":"Babylon 5","Type":"Episode","ProviderIds":{"Tvdb":"70726"}}
			]
		}`))
	}))
	items, err := client.Browse(context.Background(), jellyfin.BrowseParams{
		AnyProviderIDEquals: "Tvdb=79165",
		IncludeProviderIDs:  true,
		Limit:               500,
	})
	require.NoError(t, err)
	assert.Empty(t, items, "provider filter must not accept foreign library rows")
}

func TestProviderMatch(t *testing.T) {
	t.Parallel()
	it := jellyfin.Item{ProviderIds: map[string]string{"Tmdb": "603", "Tvdb": "79165"}}
	assert.True(t, jellyfin.ProviderMatch(it, "Tmdb=603"))
	assert.True(t, jellyfin.ProviderMatch(it, "tmdb=603"))
	assert.True(t, jellyfin.ProviderMatch(it, "Tvdb=79165|Tmdb=999"))
	assert.False(t, jellyfin.ProviderMatch(it, "Tmdb=1"))
	assert.False(t, jellyfin.ProviderMatch(it, "Tvdb=1"))
	assert.True(t, jellyfin.ProviderMatch(it, "603"), "bare value matches any key")
	assert.False(t, jellyfin.ProviderMatch(jellyfin.Item{}, "Tmdb=603"))
}

func TestFakeBrowseProviderPathAndAdmin(t *testing.T) {
	t.Parallel()
	fake := jellyfin.NewFake(
		jellyfin.Item{
			ID: "a", Name: "A", Path: "/media/Shows/slug/x.mkv",
			ProviderIds: map[string]string{"Tmdb": "9"}, Type: "Episode",
			ParentIndexNumber: 1, IndexNumber: 2,
		},
		jellyfin.Item{ID: "b", Name: "B", Path: "/other", ProviderIds: map[string]string{"Tvdb": "1"}},
	)
	byPath, err := fake.Browse(context.Background(), jellyfin.BrowseParams{PathContains: "Shows/slug"})
	require.NoError(t, err)
	require.Len(t, byPath, 1)

	byProv, err := fake.Browse(context.Background(), jellyfin.BrowseParams{AnyProviderIDEquals: "Tmdb=9"})
	require.NoError(t, err)
	require.Len(t, byProv, 1)

	byBare, err := fake.Browse(context.Background(), jellyfin.BrowseParams{AnyProviderIDEquals: "9"})
	require.NoError(t, err)
	require.Len(t, byBare, 1)

	require.NoError(t, fake.RefreshLibrary(context.Background()))
	assert.Equal(t, 1, fake.RefreshCalls)
	fake.Scanning = true
	running, err := fake.ScanRunning(context.Background())
	require.NoError(t, err)
	assert.True(t, running)
	assert.Equal(t, "S01E02", byPath[0].EpisodeKey())
	assert.Equal(t, "", jellyfin.Item{Type: "Movie"}.EpisodeKey())
}
