package jellyfin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aleksclark/primer/server/internal/tv/jellyfin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionAdditionBatchesLargeLibraries(t *testing.T) {
	var batches [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		assert.LessOrEqual(t, len(ids), 100, "bound request URI for reverse proxies")
		assert.Less(t, len(r.RequestURI), 4096)
		batches = append(batches, ids)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	c, err := jellyfin.New(jellyfin.Options{BaseURL: server.URL})
	require.NoError(t, err)
	var ids []string
	for i := 0; i < 225; i++ {
		ids = append(ids, fmt.Sprintf("%032d", i))
	}
	require.NoError(t, c.AddToCollection(context.Background(), "collection", ids))
	require.Len(t, batches, 3)
	var added []string
	for _, b := range batches {
		added = append(added, b...)
	}
	assert.Equal(t, ids, added)
}
