package api_test

import (
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

// viewing is a well-formed ingest body for an educational programme.
func viewing(ref string, extra objMap) objMap {
	body := objMap{
		"source":         "tv",
		"sourceRef":      ref,
		"mediaTitle":     "Bill Nye: Inertia",
		"class":          "educational",
		"subjectTags":    []string{"science", "physics"},
		"standardCodes":  []string{"TN.SCI.6.PS.2"},
		"watchedSeconds": 1500,
		"occurredOn":     "2031-04-15",
	}
	maps.Copy(body, extra)
	return body
}

func TestInstructionLogIngestRecordsInstructionalTime(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	resp := h.Post("/instruction-logs/ingest", viewing("session-1", nil))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	body := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, true, body["created"])

	log := body["log"].(objMap)
	assert.Equal(t, "tv", log["source"])
	assert.Equal(t, "session-1", log["sourceRef"])
	assert.Equal(t, "Bill Nye: Inertia", log["mediaTitle"])
	assert.Equal(t, "educational", log["class"])
	assert.Equal(t, float64(1500), log["watchedSeconds"])
	assert.Equal(t, []any{"science", "physics"}, log["subjectTags"])
	assert.Equal(t, []any{"TN.SCI.6.PS.2"}, log["standardCodes"])
	assert.Contains(t, log["occurredOn"], "2031-04-15",
		"the calendar day the producer reported is the day stored")
}

func TestInstructionLogIngestIsIdempotent(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	first := h.Post("/instruction-logs/ingest", viewing("session-repeat", nil))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	firstID := decode[objMap](t, first.Body.Bytes())["log"].(objMap)["id"]

	// A producer that never saw our answer retries with a bigger number. The
	// hours must not grow.
	second := h.Post("/instruction-logs/ingest", viewing("session-repeat", objMap{"watchedSeconds": 9999}))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	body := decode[objMap](t, second.Body.Bytes())
	assert.Equal(t, false, body["created"])
	log := body["log"].(objMap)
	assert.Equal(t, firstID, log["id"], "the retry matched the existing log")
	assert.Equal(t, float64(1500), log["watchedSeconds"], "the retry did not rewrite the recorded time")

	page := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, h.Get("/instruction-logs?filter=source:tv").Body.Bytes())
	assert.Equal(t, 1, page.TotalCount, "two posts, one log")
}

func TestInstructionLogIngestRefusesEntertainment(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	resp := h.Post("/instruction-logs/ingest", viewing("session-fun", objMap{"class": "entertainment"}))
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code,
		"entertainment viewing is not instructional time and must not inflate the hours")

	page := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, h.Get("/instruction-logs").Body.Bytes())
	assert.Zero(t, page.TotalCount)
}

func TestInstructionLogIngestValidation(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"sourceRef": ""})).Code,
		"an idempotency key is mandatory")
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"watchedSeconds": 0})).Code,
		"a viewing with no watch time is not instructional time")
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"occurredOn": "15/04/2031"})).Code)
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"occurredOn": "2031-13-99"})).Code,
		"a well-shaped but impossible day is refused")
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"mediaTitle": ""})).Code)
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.Post("/instruction-logs/ingest", viewing("s", objMap{"studentId": "not-a-uuid"})).Code)
}

func TestInstructionLogIngestAttributesAStudent(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)
	student := factory.Student(t, tx)

	resp := h.Post("/instruction-logs/ingest", viewing("session-student", objMap{"studentId": student.ID}))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	log := decode[objMap](t, resp.Body.Bytes())["log"].(objMap)
	assert.Equal(t, student.ID, log["studentId"])

	// An unknown student is a bad reference, not a server error.
	unknown := h.Post("/instruction-logs/ingest",
		viewing("session-ghost", objMap{"studentId": "00000000-0000-0000-0000-000000000000"}))
	assert.Equal(t, http.StatusUnprocessableEntity, unknown.Code)
}

func TestInstructionLogIngestRequiresTheServiceTokenWhenConfigured(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t, testutil.Options{ServiceToken: "s3cret"})

	assert.Equal(t, http.StatusUnauthorized, h.Post("/instruction-logs/ingest", viewing("s", nil)).Code)
	assert.Equal(t, http.StatusUnauthorized,
		h.Post("/instruction-logs/ingest", "X-Service-Token: wrong", viewing("s", nil)).Code)
	assert.Equal(t, http.StatusCreated,
		h.Post("/instruction-logs/ingest", "X-Service-Token: s3cret", viewing("s", nil)).Code)

	// The parent's admin UI shares the unauthenticated surface of every other
	// LMS resource; only the machine ingest carries a credential.
	assert.Equal(t, http.StatusOK, h.Get("/instruction-logs").Code)
}

func TestInstructionLogCRUD(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	resp := h.Post("/instruction-logs", objMap{
		"source":         "manual",
		"mediaTitle":     "Apollo 13 (with commentary)",
		"class":          "mixed",
		"subjectTags":    []string{"science", "history"},
		"watchedSeconds": 3600,
		"occurredOn":     "2031-04-16T00:00:00Z",
		"notes":          "Watched on DVD; discussed afterwards.",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	created := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "manual", created["source"])
	assert.Empty(t, created["sourceRef"], "hand-entered rows carry no idempotency key")

	id := created["id"].(string)
	updated := h.Patch("/instruction-logs/"+id, objMap{"watchedSeconds": 3000})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Equal(t, float64(3000), decode[objMap](t, updated.Body.Bytes())["watchedSeconds"])

	// Hand entry repeats freely: the idempotency index only binds producers
	// that supply a reference.
	again := h.Post("/instruction-logs", objMap{
		"source": "manual", "mediaTitle": "Apollo 13 (with commentary)",
		"class": "mixed", "watchedSeconds": 3600,
	})
	assert.Equal(t, http.StatusCreated, again.Code, again.Body.String())

	// Entertainment is refused here too, not merely on the ingest.
	bad := h.Post("/instruction-logs", objMap{
		"source": "manual", "mediaTitle": "Cartoons", "class": "entertainment", "watchedSeconds": 600,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Code)

	assert.Equal(t, http.StatusNoContent, h.Delete("/instruction-logs/"+id).Code)
	assert.Equal(t, http.StatusNotFound, h.Get("/instruction-logs/"+id).Code)

	factory.InstructionLog(t, tx, factory.Override{"media_title": "Cosmos: Standing Up in the Milky Way"})
	found := decode[struct {
		TotalCount int      `json:"totalCount"`
		Items      []objMap `json:"items"`
	}](t, h.Get("/instruction-logs?q=Milky+Way&sort=occurred_on&dir=desc").Body.Bytes())
	require.Equal(t, 1, found.TotalCount)
	assert.Equal(t, "Cosmos: Standing Up in the Milky Way", found.Items[0]["mediaTitle"])
}

func TestInstructionLogIngestAcceptsAnUntaggedViewing(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	// Tags are optional: a programme nobody has classified yet still counts as
	// time watched, and the columns are NOT NULL arrays either way.
	resp := h.Post("/instruction-logs/ingest", objMap{
		"sourceRef":      "session-untagged",
		"mediaTitle":     "Nova: Hunting the Elements",
		"class":          "mixed",
		"watchedSeconds": 3000,
		"occurredOn":     "2031-04-17",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	log := decode[objMap](t, resp.Body.Bytes())["log"].(objMap)
	assert.Equal(t, "tv", log["source"], "the source defaults to the TV channel")
	assert.Equal(t, []any{}, log["subjectTags"])
	assert.Equal(t, []any{}, log["standardCodes"])
	assert.Empty(t, log["notes"])
}
