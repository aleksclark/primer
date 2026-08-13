// Package ytdlp shells out to yt-dlp for YouTube channel/playlist downloads.
// Output paths are shaped for Jellyfin (Shows/<slug>/Season 01/...) and
// downloads are made idempotent via per-show --download-archive plus a
// ledger-driven finalize rename (no playlist_index in final names).
package ytdlp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultFormat caps quality at 1080p for the TV box, with progressive
// fallbacks. Prefer H.264/H.265 when YouTube offers them, but accept any
// progressive/mp4 stream rather than failing the whole download when the
// preferred codecs are blocked by SSAP or client experiments.
const DefaultFormat = `bv*[height<=1080][vcodec~='^(avc|hevc|h264|h265)']+ba/bv*[height<=1080]+ba/b[height<=1080]/b`

// DefaultMinDurationSeconds rejects YouTube Shorts-length clips by default.
const DefaultMinDurationSeconds = 59

// Runner executes yt-dlp. Tests substitute a fake.
type Runner interface {
	// Download pulls a URL into the library. archivePath makes re-runs no-ops
	// for already-seen videos.
	Download(ctx context.Context, opts DownloadOpts) error
}

// DownloadOpts configures one yt-dlp invocation.
type DownloadOpts struct {
	// URL is the channel or playlist URL.
	URL string
	// Slug is the manifest id; it becomes the show folder name.
	Slug string
	// OutputDir is the library root (Jellyfin-scanned).
	OutputDir string
	// ArchivePath is the --download-archive file (prefer PerShowArchivePath).
	ArchivePath string
	// Binary is the yt-dlp executable (default "yt-dlp").
	Binary string
	// Format overrides DefaultFormat.
	Format string
	// ExtraArgs are appended before the URL.
	ExtraArgs []string
	// CookiesPath, when set, is passed as --cookies.
	CookiesPath string
	// JSRuntime, when set, is passed as --js-runtimes (e.g. "node").
	JSRuntime string
	// MinDurationSeconds overrides the default match-filter duration floor.
	// Zero means DefaultMinDurationSeconds. Negative disables duration filter.
	MinDurationSeconds int
	// ExcludeShorts when non-nil false allows explicit /shorts URLs.
	// Nil or true rejects explicit /shorts tab URLs.
	ExcludeShorts *bool
	// ExcludeLive when non-nil false allows explicit /streams URLs.
	// Nil or true rejects explicit /streams tab URLs.
	ExcludeLive *bool
	// MatchFilter overrides the computed default when non-empty.
	MatchFilter string
	// ShowTitle is used by FinalizeStaging after download (optional here).
	ShowTitle string
	// SkipFinalize skips automatic FinalizeStaging after a successful run.
	SkipFinalize bool
}

// ExecRunner shells out to a real yt-dlp binary.
type ExecRunner struct{}

// Download runs yt-dlp with the Primer staging template and format cap.
func (ExecRunner) Download(ctx context.Context, opts DownloadOpts) error {
	if opts.URL == "" {
		return fmt.Errorf("ytdlp: url is required")
	}
	if opts.Slug == "" {
		return fmt.Errorf("ytdlp: slug is required")
	}
	if opts.OutputDir == "" {
		return fmt.Errorf("ytdlp: output dir is required")
	}
	if err := rejectTabURL(opts); err != nil {
		return err
	}
	binary := opts.Binary
	if binary == "" {
		binary = "yt-dlp"
	}
	if opts.Format == "" {
		opts.Format = DefaultFormat
	}
	// Default per-show archive when caller did not set one.
	if opts.ArchivePath == "" {
		opts.ArchivePath = PerShowArchivePath(opts.OutputDir, opts.Slug)
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("ytdlp: create output dir: %w", err)
	}
	if opts.ArchivePath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.ArchivePath), 0o755); err != nil {
			return fmt.Errorf("ytdlp: create archive dir: %w", err)
		}
	}
	// Ensure staging dir exists.
	if err := os.MkdirAll(StagingDir(opts.OutputDir, opts.Slug), 0o755); err != nil {
		return fmt.Errorf("ytdlp: create staging dir: %w", err)
	}

	args := BuildArgs(opts)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Sanitize: never dump full argv (cookies path etc.).
		return fmt.Errorf("ytdlp: download failed: %w", err)
	}
	if !opts.SkipFinalize {
		_, _ = FinalizeStaging(FinalizeOpts{
			OutputDir:   opts.OutputDir,
			Slug:        opts.Slug,
			ShowTitle:   opts.ShowTitle,
			MinDuration: effectiveMinDuration(opts.MinDurationSeconds),
		})
	}
	return nil
}

// BuildArgs constructs the yt-dlp argv (without the binary). Exported for tests.
func BuildArgs(opts DownloadOpts) []string {
	format := opts.Format
	if format == "" {
		format = DefaultFormat
	}
	outTemplate := StagingTemplate(opts.OutputDir, opts.Slug)
	args := []string{
		"--ignore-errors",
		"--no-abort-on-error",
		"--no-overwrites",
		"--continue",
		"--no-write-playlist-metafiles",
		"--embed-metadata",
		"--write-info-json",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--merge-output-format", "mkv",
		"--extractor-args", "youtube:player_client=android,web",
	}
	mf := opts.MatchFilter
	if mf == "" {
		mf = DefaultMatchFilter(&opts)
	}
	if mf != "" {
		args = append(args, "--match-filter", mf)
	}
	args = append(args, "-f", format, "-o", outTemplate)
	if opts.ArchivePath != "" {
		args = append(args, "--download-archive", opts.ArchivePath)
	}
	if opts.CookiesPath != "" {
		args = append(args, "--cookies", opts.CookiesPath)
	}
	if opts.JSRuntime != "" {
		args = append(args, "--js-runtimes", opts.JSRuntime)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, NormalizeYouTubeURL(opts.URL))
	return args
}

// DefaultMatchFilter builds the default yt-dlp --match-filter expression.
// opts may be nil.
func DefaultMatchFilter(opts *DownloadOpts) string {
	minDur := DefaultMinDurationSeconds
	if opts != nil {
		minDur = effectiveMinDuration(opts.MinDurationSeconds)
	}
	parts := []string{
		"!is_live",
		"!was_live",
		"live_status!=is_upcoming",
	}
	if minDur >= 0 {
		parts = append(parts, "duration>"+strconv.Itoa(minDur))
	}
	return strings.Join(parts, " & ")
}

func effectiveMinDuration(v int) int {
	if v == 0 {
		return DefaultMinDurationSeconds
	}
	return v
}

func rejectTabURL(opts DownloadOpts) error {
	u := strings.ToLower(strings.TrimSpace(opts.URL))
	// Normalize path-ish checks on raw URL (before /videos rewrite).
	if strings.Contains(u, "/shorts") {
		if opts.ExcludeShorts != nil && !*opts.ExcludeShorts {
			return nil
		}
		return fmt.Errorf("ytdlp: explicit /shorts URL requires exclude_shorts=false")
	}
	if strings.Contains(u, "/streams") {
		if opts.ExcludeLive != nil && !*opts.ExcludeLive {
			return nil
		}
		return fmt.Errorf("ytdlp: explicit /streams URL requires exclude_live=false")
	}
	return nil
}

// NormalizeYouTubeURL points channel handles at the /videos tab so yt-dlp
// does not try (and 404 on) streams/shorts tabs that some channels lack.
func NormalizeYouTubeURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return u
	}
	// Already a specific tab, playlist, or watch URL.
	if strings.Contains(u, "/playlist") || strings.Contains(u, "list=") ||
		strings.Contains(u, "/watch") || strings.Contains(u, "/videos") ||
		strings.Contains(u, "/shorts") || strings.Contains(u, "/streams") {
		return u
	}
	// https://www.youtube.com/@Handle  →  .../@Handle/videos
	if strings.Contains(u, "youtube.com/@") {
		return strings.TrimRight(u, "/") + "/videos"
	}
	// https://www.youtube.com/c/Name or /channel/UC... → append /videos
	if strings.Contains(u, "youtube.com/c/") || strings.Contains(u, "youtube.com/channel/") ||
		strings.Contains(u, "youtube.com/user/") {
		return strings.TrimRight(u, "/") + "/videos"
	}
	return u
}

// FakeRunner records download calls for tests.
type FakeRunner struct {
	Calls []DownloadOpts
	Err   error
}

// Download records the call.
func (f *FakeRunner) Download(_ context.Context, opts DownloadOpts) error {
	f.Calls = append(f.Calls, opts)
	return f.Err
}

// PathPrefix returns the library-relative path prefix used to match yt-dlp
// content in Jellyfin (Shows/<slug>). Prefer PathMatches for boundary checks.
func PathPrefix(slug string) string {
	return filepath.Join("Shows", slug)
}
