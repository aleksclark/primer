package ytdlp

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxPlotRunes = 4000

// EpisodeNFO holds fields for an episode NFO sidecar.
type EpisodeNFO struct {
	Title      string
	ShowTitle  string
	Season     int
	Episode    int
	Plot       string
	UploadDate string // YYYYMMDD
	RuntimeSec int
	YouTubeID  string
	Slug       string
}

// WriteShowNFO writes Shows/<slug>/tvshow.nfo and returns its path.
func WriteShowNFO(outputDir, slug, title, channelID string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("ytdlp: show nfo: slug required")
	}
	if title == "" {
		title = slug
	}
	dir := ShowDir(outputDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ytdlp: show nfo dir: %w", err)
	}
	path := filepath.Join(dir, "tvshow.nfo")
	plot := fmt.Sprintf("YouTube channel curated by Primer. Slug %s.", slug)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>`)
	b.WriteByte('\n')
	b.WriteString("<tvshow>\n")
	// Leave initial metadata import unlocked: Jellyfin can otherwise freeze
	// filename-derived names before reading the NFO. The dedicated source
	// library disables remote providers; TV curator locks are independent.
	writeTag(&b, "lockdata", "false")
	writeTag(&b, "title", title)
	writeTag(&b, "originaltitle", title)
	writeTag(&b, "sorttitle", title)
	writeTag(&b, "plot", plot)
	b.WriteString(`  <uniqueid type="primer-slug" default="true">`)
	b.WriteString(xmlEscape(slug))
	b.WriteString("</uniqueid>\n")
	b.WriteString(`  <uniqueid type="youtube-channel">`)
	b.WriteString(xmlEscape(channelID))
	b.WriteString("</uniqueid>\n")
	b.WriteString("  <premiered></premiered>\n")
	writeTag(&b, "status", "Continuing")
	b.WriteString("</tvshow>\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("ytdlp: write show nfo: %w", err)
	}
	return path, nil
}

// WriteEpisodeNFO writes an episode NFO next to mediaPath (replacing extension with .nfo).
func WriteEpisodeNFO(mediaPath string, meta EpisodeNFO) (string, error) {
	if mediaPath == "" {
		return "", fmt.Errorf("ytdlp: episode nfo: media path required")
	}
	ext := filepath.Ext(mediaPath)
	path := strings.TrimSuffix(mediaPath, ext) + ".nfo"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("ytdlp: episode nfo dir: %w", err)
	}
	plot := truncateRunes(meta.Plot, maxPlotRunes)
	aired := FormatUploadDate(meta.UploadDate)
	runtimeMin := meta.RuntimeSec / 60
	if meta.RuntimeSec > 0 && runtimeMin < 1 {
		runtimeMin = 1
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>`)
	b.WriteByte('\n')
	b.WriteString("<episodedetails>\n")
	writeTag(&b, "lockdata", "false")
	writeTag(&b, "title", meta.Title)
	writeTag(&b, "showtitle", meta.ShowTitle)
	writeTag(&b, "season", fmt.Sprintf("%d", meta.Season))
	writeTag(&b, "episode", fmt.Sprintf("%d", meta.Episode))
	writeTag(&b, "plot", plot)
	writeTag(&b, "premiered", aired)
	writeTag(&b, "aired", aired)
	writeTag(&b, "runtime", fmt.Sprintf("%d", runtimeMin))
	b.WriteString(`  <uniqueid type="youtube" default="true">`)
	b.WriteString(xmlEscape(meta.YouTubeID))
	b.WriteString("</uniqueid>\n")
	b.WriteString(`  <uniqueid type="primer-slug">`)
	b.WriteString(xmlEscape(meta.Slug))
	b.WriteString("</uniqueid>\n")
	b.WriteString("</episodedetails>\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("ytdlp: write episode nfo: %w", err)
	}
	return path, nil
}

// FormatUploadDate converts YYYYMMDD to YYYY-MM-DD. Invalid input returns "".
func FormatUploadDate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return ""
	}
	for _, c := range yyyymmdd {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return yyyymmdd[0:4] + "-" + yyyymmdd[4:6] + "-" + yyyymmdd[6:8]
}

func writeTag(b *strings.Builder, name, value string) {
	b.WriteString("  <")
	b.WriteString(name)
	b.WriteString(">")
	b.WriteString(xmlEscape(value))
	b.WriteString("</")
	b.WriteString(name)
	b.WriteString(">\n")
}

func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		// EscapeText only fails on invalid UTF-8; replace bad runes.
		return strings.ToValidUTF8(s, "")
	}
	return b.String()
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
