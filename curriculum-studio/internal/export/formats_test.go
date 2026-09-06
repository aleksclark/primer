package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFormatsValidationAndCalendarEscaping(t *testing.T) {
	_, _, err := FormatInfo("exe")
	require.Error(t, err)
	_, err = Render("pdf", Content{})
	require.Error(t, err)
	c := Content{Graph: &domain.PlanGraph{Revision: domain.PlanRevision{ID: uuid.New(), Title: "Plan"}}, Manifest: Manifest{CreatedAt: time.Now()}}
	_, err = Render("exe", c)
	require.Error(t, err)
	_, err = Render("csv", c)
	require.Error(t, err)
	empty, err := Render("ical", c)
	require.NoError(t, err)
	require.NotContains(t, string(empty), "BEGIN:VEVENT")
	for _, payload := range []string{`{`, `{}`, `{"start":"2026-09-01T10:00:00Z","end":"2026-09-01T09:00:00Z"}`} {
		c.Graph.SchedulingConstraints = []domain.SchedulingConstraint{{ID: uuid.New(), Kind: "calendar_window", Payload: json.RawMessage(payload)}}
		_, err = Render("ical", c)
		require.Error(t, err)
	}
	payload, err := json.Marshal(map[string]string{"start": "2026-09-01T09:00:00Z", "end": "2026-09-01T10:00:00Z", "title": strings.Repeat("Résumé ", 30) + "\nATTENDEE:untrusted"})
	require.NoError(t, err)
	c.Graph.SchedulingConstraints = []domain.SchedulingConstraint{{ID: uuid.New(), Kind: "testing_window", Payload: payload}}
	data, err := Render("ical", c)
	require.NoError(t, err)
	require.NotContains(t, string(data), "\r\nATTENDEE:")
	for _, line := range strings.Split(string(data), "\r\n") {
		require.LessOrEqual(t, len(line), 75)
	}
}
func TestFormatsCoverageUsesReportAndPreventsFormulaInjection(t *testing.T) {
	c := Content{Graph: &domain.PlanGraph{}, Report: &domain.ValidationReport{ID: uuid.New(), Status: "passed"}}
	data, err := Render("csv", c)
	require.NoError(t, err)
	require.Contains(t, string(data), "NO_FINDINGS")
	c.Findings = []domain.ValidationFinding{{Code: "warning", Message: "=HYPERLINK(\"https://example.test\")"}}
	data, err = Render("csv", c)
	require.NoError(t, err)
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	require.NoError(t, err)
	require.Equal(t, "'=HYPERLINK(\"https://example.test\")", rows[1][6])
}
