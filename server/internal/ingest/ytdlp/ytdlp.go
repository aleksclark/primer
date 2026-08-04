// Package ytdlp shells out to yt-dlp for YouTube channel/playlist downloads.
// Output paths are shaped for Jellyfin (Shows/<slug>/Season 01/...) and
// downloads are made idempotent via --download-archive.
package ytdlp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultFormat caps quality at 1080p for the TV box, with progressive
// fallbacks. Prefer H.264/H.265 when YouTube offers them, but accept any
// progressive/mp4 stream rather than failing the whole download when the
// preferred codecs are blocked by SSAP or client experiments.
const DefaultFormat = `bv*[height<=1080][vcodec~='^(avc|hevc|h264|h265)']+ba/bv*[height<=1080]+ba/b[height<=1080]/b`

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
	// ArchivePath is the --download-archive file.
	ArchivePath string
	// Binary is the yt-dlp executable (default "yt-dlp").
	Binary string
	// Format overrides DefaultFormat.
	Format string
	// PlaylistFilter, when set, is passed as --match-filter for playlist title.
	// yt-dlp's match-filter is limited; for playlist narrowing we pass the
	// playlist URL directly when Filters.Playlists is set upstream.
	ExtraArgs []string
}

// ExecRunner shells out to a real yt-dlp binary.
type ExecRunner struct{}

// Download runs yt-dlp with the Primer output template and format cap.
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
	binary := opts.Binary
	if binary == "" {
		binary = "yt-dlp"
	}
	format := opts.Format
	if format == "" {
		format = DefaultFormat
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("ytdlp: create output dir: %w", err)
	}
	if opts.ArchivePath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.ArchivePath), 0o755); err != nil {
			return fmt.Errorf("ytdlp: create archive dir: %w", err)
		}
	}

	// Shows/<slug>/Season 01/<slug> - S01E%(playlist_index)03d - %(title)s.%(ext)s
	// Season 01 is a deliberate simplification: channel dumps land in one season
	// so Jellyfin treats them as a series; the slug embeds the manifest id for
	// path-prefix matching on import.
	outTemplate := filepath.Join(
		opts.OutputDir,
		"Shows",
		opts.Slug,
		"Season 01",
		opts.Slug+` - S01E%(playlist_index)03d - %(title).200B.%(ext)s`,
	)

	downloadURL := NormalizeYouTubeURL(opts.URL)
	args := []string{
		"--ignore-errors",
		"--no-abort-on-error",
		"--no-overwrites",
		"--embed-metadata",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--merge-output-format", "mkv",
		// Prefer android/web clients over tv clients that hit SSAP experiments.
		"--extractor-args", "youtube:player_client=android,web",
		"-f", format,
		"-o", outTemplate,
	}
	if opts.ArchivePath != "" {
		args = append(args, "--download-archive", opts.ArchivePath)
	}
	args = append(args, opts.ExtraArgs...)
	args = append(args, downloadURL)

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ytdlp: %s %s: %w", binary, strings.Join(args, " "), err)
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
// content in Jellyfin (Shows/<slug>).
func PathPrefix(slug string) string {
	return filepath.Join("Shows", slug)
}
