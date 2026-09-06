package export

import (
	"bytes"
	"strings"
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

func TestPDFPaginatesAndEscapes(t *testing.T) {
	pdf := pdfText(strings.Repeat("A line (with parentheses)\\backslash\n", 100))
	require.Contains(t, string(pdf), "/Count 3")
	require.Contains(t, string(pdf), `\(with parentheses\)\\backslash`)
	require.Contains(t, string(pdf), "%%EOF")
}
