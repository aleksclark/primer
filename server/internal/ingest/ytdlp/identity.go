package ytdlp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// youtubeIDRe matches a bare 11-char YouTube video id.
var youtubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// bracketedIDRe extracts the last [id] group from a basename.
var bracketedIDRe = regexp.MustCompile(`\[([A-Za-z0-9_-]+)\]`)

// PathMatches reports whether path is under Shows/<slug> with a path-boundary
// prefix (Shows/<slug>/ or Shows/<slug> end). It rejects Shows/<slug>-extra.
func PathMatches(path, slug string) bool {
	if path == "" || slug == "" {
		return false
	}
	// Normalize to forward slashes for stable matching across OS path forms.
	p := filepath.ToSlash(path)
	needle := "Shows/" + slug
	idx := strings.Index(p, needle)
	if idx < 0 {
		return false
	}
	// Ensure Shows/ is a path segment boundary (start or after /).
	if idx > 0 && p[idx-1] != '/' {
		return false
	}
	rest := p[idx+len(needle):]
	return rest == "" || strings.HasPrefix(rest, "/")
}

// ParseYouTubeID extracts an 11-char YouTube id from a filename containing [id].
// Invalid lengths or character sets are rejected.
func ParseYouTubeID(name string) (string, bool) {
	base := filepath.Base(name)
	matches := bracketedIDRe.FindAllStringSubmatch(base, -1)
	if len(matches) == 0 {
		return "", false
	}
	// Prefer the last bracket group (title may contain brackets; id is trailing).
	id := matches[len(matches)-1][1]
	if !youtubeIDRe.MatchString(id) {
		return "", false
	}
	return id, true
}

// StagingTemplate is the yt-dlp -o path for phase-A downloads (no SxxExx).
func StagingTemplate(outputDir, slug string) string {
	return filepath.Join(
		outputDir,
		"Shows",
		slug,
		"Season 01",
		"_staging",
		slug+` - %(title).80B [%(id)s].%(ext)s`,
	)
}

// FinalBasename builds the final media basename (no extension) after ledger assign.
func FinalBasename(slug string, season, episode int, title, id string) string {
	return fmt.Sprintf("%s - S%02dE%03d - %s [%s]", slug, season, episode, title, id)
}

// PerShowArchivePath is the per-slug --download-archive file.
func PerShowArchivePath(outputDir, slug string) string {
	return filepath.Join(outputDir, "Shows", slug, ".ytdlp-archive.txt")
}

// ShowDir returns Shows/<slug> under outputDir.
func ShowDir(outputDir, slug string) string {
	return filepath.Join(outputDir, "Shows", slug)
}

// SeasonDir returns Shows/<slug>/Season 01 under outputDir.
func SeasonDir(outputDir, slug string) string {
	return filepath.Join(ShowDir(outputDir, slug), "Season 01")
}

// StagingDir returns the phase-A staging directory.
func StagingDir(outputDir, slug string) string {
	return filepath.Join(SeasonDir(outputDir, slug), "_staging")
}

// LedgerPath returns the per-show .primer-index.json path.
func LedgerPath(outputDir, slug string) string {
	return filepath.Join(ShowDir(outputDir, slug), ".primer-index.json")
}
