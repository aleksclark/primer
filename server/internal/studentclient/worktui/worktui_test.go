package worktui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkItemListInterface(t *testing.T) {
	t.Parallel()
	w := workItem{TitleText: "Basic navigation", State: "available", Slug: "basic-navigation"}
	assert.Equal(t, "Basic navigation", w.Title())
	assert.Equal(t, "basic-navigation · available", w.Description())
	assert.Contains(t, w.FilterValue(), "basic-navigation")
	assert.Contains(t, w.FilterValue(), "Basic navigation")
}

func TestFetchAndRenderWorkQueue(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/student/work", r.URL.Path)
		assert.Equal(t, "Bearer dev-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"assignment": map[string]any{"state": "available"},
					"activity":   map[string]any{"title": "Navigate", "slug": "basic-navigation"},
				},
				{
					"assignment": map[string]any{"state": "in_progress"},
					"activity":   map[string]any{"title": "", "slug": "typing-drills"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	m := NewModel(Options{BaseURL: srv.URL + "/", DeviceToken: "dev-token", HTTPClient: srv.Client()})
	require.NotNil(t, m.Init())

	msg := m.fetch()
	loaded, ok := msg.(loadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	require.Len(t, loaded.items, 2)
	assert.Equal(t, "Navigate", loaded.items[0].(workItem).TitleText)
	assert.Equal(t, "typing-drills", loaded.items[1].(workItem).TitleText) // falls back to slug

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(model)
	next, _ = m.Update(loaded)
	m = next.(model)
	assert.True(t, m.loaded)
	view := m.View().Content
	assert.Contains(t, view, "Navigate")
	assert.Contains(t, view, "typing-drills")

	// Empty queue path.
	m2 := NewModel(Options{BaseURL: srv.URL, DeviceToken: "x", HTTPClient: srv.Client()})
	m2.loaded = true
	m2.list.SetItems(nil)
	assert.Contains(t, m2.View().Content, "No assignments")
}

func TestFetchErrorAndQuit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	m := NewModel(Options{BaseURL: srv.URL, DeviceToken: "tok", HTTPClient: srv.Client()})
	msg := m.fetch()
	loaded := msg.(loadedMsg)
	require.Error(t, loaded.err)

	next, _ := m.Update(loaded)
	m = next.(model)
	assert.Contains(t, m.View().Content, "error:")
	assert.Contains(t, m.View().Content, "work status 403")

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	m = next.(model)
	assert.True(t, m.quitting)
	require.NotNil(t, cmd)
	assert.Equal(t, "", m.View().Content)
}

func TestFetchNetworkError(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{BaseURL: "http://127.0.0.1:1", DeviceToken: "tok"})
	msg := m.fetch()
	loaded := msg.(loadedMsg)
	require.Error(t, loaded.err)
}

func TestLoadingView(t *testing.T) {
	t.Parallel()
	m := NewModel(Options{})
	assert.Contains(t, m.View().Content, "Loading work queue")
	// list.Item interface satisfaction via workItem
	var _ list.Item = workItem{}
}
