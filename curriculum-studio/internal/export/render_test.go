package export

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func TestMarkdownAndPDFContainPlanContent(t *testing.T) {
	g := &domain.PlanGraph{Revision: domain.PlanRevision{Title: "Grade 6 Mathematics"}, Objectives: []domain.Objective{{Title: "Reason precisely"}}}
	md := Markdown(g)
	require.Contains(t, string(md), "Grade 6 Mathematics")
	pdf := PDF(g)
	require.True(t, bytes.HasPrefix(pdf, []byte("%PDF-1.4")))
	require.NotEmpty(t, pdf)
}
