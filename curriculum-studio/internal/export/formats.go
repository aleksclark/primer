package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gomutex/godocx"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// Manifest is download provenance, not the Primer protobuf integration bundle.
type Manifest struct {
	ExportID             uuid.UUID  `json:"export_id"`
	PlanRevisionID       uuid.UUID  `json:"plan_revision_id"`
	MaterializationRunID *uuid.UUID `json:"materialization_run_id,omitempty"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	Format               string     `json:"format"`
	ContentType          string     `json:"content_type"`
	Checksum             string     `json:"sha256,omitempty"`
}
type Content struct {
	Graph    *domain.PlanGraph
	Items    []domain.MaterializedItem
	Report   *domain.ValidationReport
	Findings []domain.ValidationFinding
	Manifest Manifest
}

func FormatInfo(format string) (extension, contentType string, err error) {
	switch format {
	case domain.ExportFormatMarkdown:
		return "md", "text/markdown; charset=utf-8", nil
	case domain.ExportFormatPDF:
		return "pdf", "application/pdf", nil
	case domain.ExportFormatDOCX:
		return "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	case domain.ExportFormatCSV:
		return "csv", "text/csv; charset=utf-8", nil
	case domain.ExportFormatJSON:
		return "json", "application/json", nil
	case domain.ExportFormatICal:
		return "ics", "text/calendar; charset=utf-8", nil
	default:
		return "", "", fmt.Errorf("unsupported export format %q", format)
	}
}
func Render(format string, c Content) ([]byte, error) {
	if c.Graph == nil {
		return nil, fmt.Errorf("plan graph is required")
	}
	switch format {
	case domain.ExportFormatMarkdown:
		return documentText(c), nil
	case domain.ExportFormatPDF:
		return pdfText(string(documentText(c))), nil
	case domain.ExportFormatDOCX:
		doc, err := godocx.NewDocument()
		if err != nil {
			return nil, err
		}
		defer doc.Close()
		for _, line := range strings.Split(string(documentText(c)), "\n") {
			doc.AddParagraph(line)
		}
		var b bytes.Buffer
		if err = doc.Write(&b); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	case domain.ExportFormatCSV:
		return coverageCSV(c)
	case domain.ExportFormatJSON:
		// Deliberate download subset. No learner profile, input snapshot, provider
		// credentials, or claim of SessionSpec/MaterializationBundle protobuf parity.
		return json.MarshalIndent(struct {
			Schema     string            `json:"schema"`
			Scope      string            `json:"scope"`
			Provenance Manifest          `json:"provenance"`
			Plan       *domain.PlanGraph `json:"plan"`
			Items      []downloadItem    `json:"items"`
		}{"studio.download.v1", "Planning download subset; Primer integration uses CurriculumIntegrationService, not this JSON format.", c.Manifest, c.Graph, downloadItems(c.Items)}, "", "  ")
	case domain.ExportFormatICal:
		return calendar(c)
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

type downloadItem struct {
	ID    uuid.UUID       `json:"id"`
	Kind  string          `json:"kind"`
	Title string          `json:"title"`
	Body  json.RawMessage `json:"body"`
}

func downloadItems(items []domain.MaterializedItem) []downloadItem {
	out := make([]downloadItem, 0, len(items))
	for _, i := range items {
		out = append(out, downloadItem{i.ID, i.Kind, i.Title, i.Body})
	}
	return out
}
func documentText(c Content) []byte {
	var b bytes.Buffer
	b.Write(Markdown(c.Graph))
	for _, p := range c.Graph.Projects {
		fmt.Fprintf(&b, "\n## Project: %s\n\n%s\n", p.Title, p.Description)
	}
	for _, i := range c.Items {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", i.Title, i.Kind, i.Body)
	}
	return b.Bytes()
}
func coverageCSV(c Content) ([]byte, error) {
	if c.Report == nil {
		return nil, fmt.Errorf("coverage export requires a validation report")
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"report_id", "status", "severity", "code", "node_kind", "node_id", "message"})
	if len(c.Findings) == 0 {
		_ = w.Write([]string{c.Report.ID.String(), c.Report.Status, "info", "NO_FINDINGS", "revision", c.Graph.Revision.ID.String(), "No validation findings"})
	}
	for _, f := range c.Findings {
		node := ""
		if f.NodeID != nil {
			node = f.NodeID.String()
		}
		row := []string{c.Report.ID.String(), c.Report.Status, f.Severity, f.Code, f.NodeKind, node, f.Message}
		// Prevent formula execution when opened in a spreadsheet.
		for i, v := range row {
			if strings.ContainsAny(stringFirst(v), "=+-@\t\r") {
				row[i] = "'" + v
			}
		}
		_ = w.Write(row)
	}
	w.Flush()
	return b.Bytes(), w.Error()
}
func stringFirst(s string) string {
	if s == "" {
		return ""
	}
	return s[:1]
}

// Calendar windows use explicit RFC3339 start/end payloads. Undated planning
// policies are not invented appointments. An empty calendar is valid.
func calendar(c Content) ([]byte, error) {
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Primer//Curriculum Studio//EN", "CALSCALE:GREGORIAN"}
	for _, sc := range c.Graph.SchedulingConstraints {
		if sc.Kind != "calendar_window" && sc.Kind != "testing_window" && sc.Kind != "blackout" {
			continue
		}
		var v struct{ Start, End, Title string }
		if err := json.Unmarshal(sc.Payload, &v); err != nil {
			return nil, fmt.Errorf("invalid calendar constraint %s", sc.ID)
		}
		start, err := time.Parse(time.RFC3339, v.Start)
		if err != nil {
			return nil, fmt.Errorf("calendar constraint %s requires RFC3339 start", sc.ID)
		}
		end, err := time.Parse(time.RFC3339, v.End)
		if err != nil || !end.After(start) {
			return nil, fmt.Errorf("calendar constraint %s requires end after start", sc.ID)
		}
		if v.Title == "" {
			v.Title = c.Graph.Revision.Title + " — " + sc.Kind
		}
		lines = append(lines, "BEGIN:VEVENT", "UID:"+sc.ID.String()+"@curriculum-studio", "DTSTAMP:"+c.Manifest.CreatedAt.UTC().Format("20060102T150405Z"), "DTSTART:"+start.UTC().Format("20060102T150405Z"), "DTEND:"+end.UTC().Format("20060102T150405Z"), "SUMMARY:"+icalEscape(v.Title), "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")
	var b strings.Builder
	for _, line := range lines {
		for len(line) > 75 {
			n := 75
			for !utf8.RuneStart(line[n]) {
				n--
			}
			b.WriteString(line[:n] + "\r\n")
			line = " " + line[n:]
		}
		b.WriteString(line + "\r\n")
	}
	return []byte(b.String()), nil
}
func icalEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\r\n", "\\n", "\n", "\\n", "\r", "\\n", ";", "\\;", ",", "\\,").Replace(s)
}
