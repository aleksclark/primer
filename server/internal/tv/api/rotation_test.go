package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// suggestionIDs pulls the media item IDs out of a suggestions response.
func suggestionIDs(in []api.RotationSuggestion) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.MediaItem.ID)
	}
	return out
}

type suggestionsBody struct {
	Suggestions []api.RotationSuggestion `json:"suggestions"`
}

func TestRotationSuggestsOnlyOfferableUnwatchedItems(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	now := time.Now().UTC()

	fresh := factory.MediaItem(t, q, factory.Override{"title": "Never offered"})

	onOffer := factory.MediaItem(t, q, factory.Override{"title": "Already out"})
	factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": onOffer.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(48 * time.Hour),
	})

	watchedItem := factory.MediaItem(t, q, factory.Override{"title": "Already seen"})
	spent := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": watchedItem.ID,
		"starts_at":     now.AddDate(0, 0, -10),
		"ends_at":       now.AddDate(0, 0, -3),
	})
	factory.PlayLedgerEntry(t, q, factory.Override{
		"media_item_id":          watchedItem.ID,
		"availability_window_id": spent.ID,
	})

	incompatible := factory.MediaItem(t, q, factory.Override{
		"title":          "Cannot direct play",
		"direct_play_ok": false,
	})
	orphaned := factory.MediaItem(t, q, factory.Override{
		"title":       "Gone from Jellyfin",
		"orphaned_at": now,
	})

	body := decode[suggestionsBody](t, h.Get("/rotation/suggestions").Body.Bytes())
	ids := suggestionIDs(body.Suggestions)

	assert.Contains(t, ids, fresh.ID)
	assert.NotContains(t, ids, onOffer.ID, "an item already on offer needs no rotation")
	assert.NotContains(t, ids, watchedItem.ID, "a spent viewing is not re-offered")
	assert.NotContains(t, ids, incompatible.ID, "the box cannot play it")
	assert.NotContains(t, ids, orphaned.ID, "Jellyfin no longer has it")
}

func TestRotateOffersSuggestionsAndClosesWhatWasOut(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	now := time.Now().UTC()

	stale := factory.MediaItem(t, q, factory.Override{"title": "Last week's film"})
	open := factory.AvailabilityWindow(t, q, factory.Override{
		"media_item_id": stale.ID,
		"starts_at":     now.Add(-time.Hour),
		"ends_at":       now.Add(72 * time.Hour),
	})
	factory.MediaItem(t, q, factory.Override{"title": "Waiting in the wings"})

	resp := h.Post("/rotation/rotate", objMap{"expireOpen": true, "limit": 5, "days": 7})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.RotateResponse](t, resp.Body.Bytes())

	assert.Equal(t, 1, body.Expired, "the open window is closed")
	require.NotEmpty(t, body.Opened, "the waiting item is offered")

	// The closed window really is closed, so the catalog no longer offers it.
	catalog := decode[struct {
		Items []objMap `json:"items"`
	}](t, h.Get("/availability-windows?filter=media_item_id:"+stale.ID).Body.Bytes())
	require.Len(t, catalog.Items, 1)
	assert.NotEqual(t, open.EndsAt, catalog.Items[0]["endsAt"])
}

func TestRotateOffersTheNamedItemsOnly(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	wanted := factory.MediaItem(t, q, factory.Override{"title": "Bill Nye: Inertia"})
	factory.MediaItem(t, q, factory.Override{"title": "Something else"})

	body := decode[api.RotateResponse](t, h.Post("/rotation/rotate", objMap{
		"mediaItemIds": []string{wanted.ID},
	}).Body.Bytes())

	require.Len(t, body.Opened, 1)
	assert.Equal(t, wanted.ID, body.Opened[0].MediaItemID)
	assert.Zero(t, body.Expired, "nothing is closed unless asked")
}

func TestRotateIsRepeatable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	item := factory.MediaItem(t, q)

	first := decode[api.RotateResponse](t, h.Post("/rotation/rotate", objMap{
		"mediaItemIds": []string{item.ID},
	}).Body.Bytes())
	require.Len(t, first.Opened, 1)

	second := decode[api.RotateResponse](t, h.Post("/rotation/rotate", objMap{
		"mediaItemIds": []string{item.ID},
	}).Body.Bytes())
	assert.Empty(t, second.Opened, "an item already on offer is not offered twice")
}

func TestRotatedItemsBecomePlayable(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)

	item := factory.MediaItem(t, q, factory.Override{"class": domain.ClassEducational})
	_, token := factory.PairedDevice(t, q)

	before := decode[struct {
		Items []objMap `json:"items"`
	}](t, h.Get("/catalog", "Authorization: Bearer "+token).Body.Bytes())
	assert.Empty(t, before.Items)

	require.Equal(t, http.StatusOK,
		h.Post("/rotation/rotate", objMap{"mediaItemIds": []string{item.ID}}).Code)

	after := decode[struct {
		Items []objMap `json:"items"`
	}](t, h.Get("/catalog", "Authorization: Bearer "+token).Body.Bytes())
	require.Len(t, after.Items, 1, "a rotated item is immediately watchable")
}

func TestRotationRequiresTheAdminKey(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{AdminKey: "secret"})

	assert.Equal(t, http.StatusUnauthorized, h.Get("/rotation/suggestions").Code)
	assert.Equal(t, http.StatusUnauthorized, h.Post("/rotation/rotate", objMap{}).Code)
}
