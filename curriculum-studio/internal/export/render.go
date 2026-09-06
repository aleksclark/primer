// Package export renders small, deterministic planning artifacts. Storage and
// job metadata remain owned by the Studio export repository.
package export

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/text/encoding/charmap"

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

// PDF emits a basic paginated Helvetica document with WinAnsi encoding.
// DOCX/Markdown are preferred for text outside the WinAnsi character set.
func PDF(g *domain.PlanGraph) []byte { return pdfText(string(Markdown(g))) }

func pdfText(content string) []byte {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		runes := []rune(strings.ReplaceAll(line, "\t", "    "))
		for len(runes) > 80 {
			lines = append(lines, string(runes[:80]))
			runes = runes[80:]
		}
		lines = append(lines, string(runes))
	}
	pages := (len(lines) + 47) / 48
	kids := make([]string, pages)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 4+i*2)
	}
	objects := []string{"<</Type/Catalog/Pages 2 0 R>>", fmt.Sprintf("<</Type/Pages/Count %d/Kids[%s]>>", pages, strings.Join(kids, " ")), "<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>"}
	for i := 0; i < pages; i++ {
		var stream strings.Builder
		stream.WriteString("BT /F1 11 Tf 50 760 Td\n")
		for _, line := range lines[i*48 : min((i+1)*48, len(lines))] {
			stream.WriteByte('(')
			for _, r := range line {
				b, ok := charmap.Windows1252.EncodeRune(r)
				if !ok || b < 32 {
					b = '?'
				}
				if b == '(' || b == ')' || b == '\\' {
					stream.WriteByte('\\')
				}
				stream.WriteByte(b)
			}
			stream.WriteString(") Tj 0 -14 Td\n")
		}
		stream.WriteString("ET")
		objects = append(objects, fmt.Sprintf("<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 3 0 R>>>>/Contents %d 0 R>>", 5+i*2), fmt.Sprintf("<</Length %d>>stream\n%s\nendstream", stream.Len(), stream.String()))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj%s endobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}
