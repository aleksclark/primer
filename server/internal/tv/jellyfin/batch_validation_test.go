package jellyfin_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchMetadataRejectsIncompleteOrMalformedResponses(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":  `{"TotalRecordCount":2,"Items":[{"Id":"a","Type":"Movie"}]}`,
		"duplicate":  `{"TotalRecordCount":2,"Items":[{"Id":"a","Type":"Movie"},{"Id":"a","Type":"Movie"}]}`,
		"missing-id": `{"TotalRecordCount":1,"Items":[{"Id":"","Type":"Movie"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			items, err := c.ItemsByID(context.Background(), []string{"a", "b"})
			require.Error(t, err, "an incomplete response cannot authorize orphaning requested media")
			require.Nil(t, items)
		})
	}
}
