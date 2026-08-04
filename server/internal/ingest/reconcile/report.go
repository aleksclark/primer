package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report accumulates one run's outcomes for the markdown status file.
type Report struct {
	StartedAt time.Time
	DryRun    bool

	Resolved        []string // id → provider written
	AlreadyResolved []string
	ReviewQueued    []string
	ReviewApplied   []string

	AcquiredMovies   []string
	AcquiredSeries   []string
	AcquiredYouTube  []string
	AlreadyHeld      []string
	AwaitingDownload []string
	ManualQueue      []string

	JellyfinRefreshed bool
	TVSynced          bool
	Imported          []string
	Updated           []string
	SkippedExcluded   []string
	NotInJellyfin     []string

	// Manifest catalog tracking (TV server content_manifest_entries).
	ManifestSynced   string
	AttemptsRecorded []string
	MarkedPresent    []string
	FailedQueue      []string

	Errors []string
}

// NewReport starts an empty report.
func NewReport(dryRun bool) *Report {
	return &Report{StartedAt: time.Now().UTC(), DryRun: dryRun}
}

// Markdown renders the report as a human-readable status document.
func (r *Report) Markdown() string {
	var b strings.Builder
	mode := "apply"
	if r.DryRun {
		mode = "plan"
	}
	fmt.Fprintf(&b, "# content-ingest %s — %s\n\n", mode, r.StartedAt.Format(time.RFC3339))

	section(&b, "Resolved (provider IDs written to manifest)", r.Resolved)
	section(&b, "Already resolved", r.AlreadyResolved)
	section(&b, "Review applied from review.yaml", r.ReviewApplied)
	section(&b, "Queued for human review", r.ReviewQueued)

	section(&b, "Acquired — movies (Radarr)", r.AcquiredMovies)
	section(&b, "Acquired — series (Sonarr)", r.AcquiredSeries)
	section(&b, "Acquired — YouTube (yt-dlp)", r.AcquiredYouTube)
	section(&b, "Already in library / archive", r.AlreadyHeld)
	section(&b, "Awaiting download", r.AwaitingDownload)
	section(&b, "Manual rip queue", r.ManualQueue)
	section(&b, "Failed (human intervention)", r.FailedQueue)

	fmt.Fprintf(&b, "## Sync\n\n")
	fmt.Fprintf(&b, "- TV content-manifest catalog: %s\n", dash(r.ManifestSynced))
	fmt.Fprintf(&b, "- Jellyfin library refresh: %s\n", yn(r.JellyfinRefreshed))
	fmt.Fprintf(&b, "- TV server metadata sync: %s\n\n", yn(r.TVSynced))

	section(&b, "Acquisition attempts recorded", r.AttemptsRecorded)
	section(&b, "Marked present in TV catalog", r.MarkedPresent)
	section(&b, "Imported into TV server", r.Imported)
	section(&b, "Updated classification", r.Updated)
	section(&b, "Skipped (excluded episodes)", r.SkippedExcluded)
	section(&b, "Not yet in Jellyfin", r.NotInJellyfin)

	if len(r.Errors) > 0 {
		section(&b, "Errors", r.Errors)
	}

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| Metric | Count |\n|---|---|\n")
	fmt.Fprintf(&b, "| Resolved | %d |\n", len(r.Resolved))
	fmt.Fprintf(&b, "| Review queue | %d |\n", len(r.ReviewQueued))
	fmt.Fprintf(&b, "| Acquired | %d |\n", len(r.AcquiredMovies)+len(r.AcquiredSeries)+len(r.AcquiredYouTube))
	fmt.Fprintf(&b, "| Awaiting download | %d |\n", len(r.AwaitingDownload))
	fmt.Fprintf(&b, "| Marked present | %d |\n", len(r.MarkedPresent))
	fmt.Fprintf(&b, "| Imported | %d |\n", len(r.Imported))
	fmt.Fprintf(&b, "| Updated | %d |\n", len(r.Updated))
	fmt.Fprintf(&b, "| Manual rip queue | %d |\n", len(r.ManualQueue))
	fmt.Fprintf(&b, "| Failed (human) | %d |\n", len(r.FailedQueue))
	fmt.Fprintf(&b, "| Errors | %d |\n", len(r.Errors))
	return b.String()
}

func section(b *strings.Builder, title string, items []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(items) == 0 {
		fmt.Fprintf(b, "_(none)_\n\n")
		return
	}
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	fmt.Fprintf(b, "\n")
}

func yn(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func dash(s string) string {
	if s == "" {
		return "no"
	}
	return s
}

// WriteMarkdown writes the report under dir as ingest-YYYYMMDD-HHMMSS.md.
func (r *Report) WriteMarkdown(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}
	name := fmt.Sprintf("ingest-%s.md", r.StartedAt.Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(r.Markdown()), 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}
