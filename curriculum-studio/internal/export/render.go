// Package export renders small, deterministic planning artifacts. Storage and
// job metadata remain owned by the Studio export repository.
package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

func Markdown(g *domain.PlanGraph) []byte {
	if g == nil {
		return []byte("# Curriculum\n\n")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", g.Revision.Title)
	if len(g.Objectives) > 0 {
		b.WriteString("## Objectives\n\n")
		for _, o := range g.Objectives {
			fmt.Fprintf(&b, "- **%s** — %s\n", o.Title, o.Description)
		}
		b.WriteString("\n")
	}
	if len(g.Outcomes) > 0 {
		b.WriteString("## Outcomes\n\n")
		for _, o := range g.Outcomes {
			fmt.Fprintf(&b, "- **%s** — %s\n", o.Title, o.Description)
		}
		b.WriteString("\n")
	}
	if len(g.Units) > 0 {
		b.WriteString("## Scope and sequence\n\n")
		for _, u := range g.Units {
			fmt.Fprintf(&b, "%d. %s\n", u.Position+1, u.Title)
		}
	}
	return []byte(b.String())
}

// PDF emits a deliberately small, valid single-page PDF without adding a
// heavyweight renderer dependency. The textual stream is escaped and the
// artifact is sufficient for download/archival; richer typography can land in
// the later document-export phase.
func PDF(g *domain.PlanGraph) []byte {
	text := strings.ReplaceAll(strings.ReplaceAll(string(Markdown(g)), "\\", "\\\\"), "(", "\\(")
	text = strings.ReplaceAll(text, ")", "\\)")
	stream := fmt.Sprintf("BT /F1 11 Tf 50 760 Td (%s) Tj ET", strings.ReplaceAll(text, "\n", ") Tj 0 -14 Td ("))
	objects := []string{"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n", "2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n", "3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>endobj\n", "4 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n", fmt.Sprintf("5 0 obj<</Length %d>>stream\n%s\nendstream\nendobj\n", len(stream), stream)}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for _, o := range objects {
		offsets = append(offsets, b.Len())
		b.WriteString(o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}
