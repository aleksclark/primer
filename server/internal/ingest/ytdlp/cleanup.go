package ytdlp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// orphanFormatRe matches yt-dlp intermediate format streams like .f137.mp4 / .f251.webm
// without being a final .mkv.
var orphanFormatRe = regexp.MustCompile(`\.f[0-9]+\.(mp4|webm)$`)

// CleanupShowDir removes incomplete/orphan download debris under Shows/<slug>.
// Keeps complete *[id].mkv (and matching sidecars with [id]).
// Returns the number of paths removed.
func CleanupShowDir(outputDir, slug string) (int, error) {
	root := ShowDir(outputDir, slug)
	st, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("ytdlp: cleanup stat: %w", err)
	}
	if !st.IsDir() {
		return 0, nil
	}

	// Build set of basenames (no ext) that have a complete [id].mkv
	complete := map[string]struct{}{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(strings.ToLower(name), ".mkv") {
			if _, ok := ParseYouTubeID(name); ok {
				base := strings.TrimSuffix(name, filepath.Ext(name))
				complete[base] = struct{}{}
			}
		}
		return nil
	})

	removed := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		lower := strings.ToLower(name)

		// Never delete ledger/archive/show nfo
		if name == ".primer-index.json" || name == ".ytdlp-archive.txt" || name == "tvshow.nfo" {
			return nil
		}

		del := false
		switch {
		case strings.HasSuffix(lower, ".part"):
			del = true
		case strings.HasSuffix(lower, ".ytdl"):
			del = true
		case strings.Contains(lower, ".part-frag"):
			del = true
		case orphanFormatRe.MatchString(lower):
			// orphan f### stream: delete unless a matching complete [id].mkv shares stem
			// clip.f137.mp4 — no matching complete mkv of same stem "clip"
			stem := orphanFormatStem(name)
			if !hasCompletePrefix(complete, stem) {
				del = true
			}
		case isThumbWithoutID(name):
			del = true
		case isS01E000WithoutID(name):
			del = true
		}

		if !del {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("ytdlp: cleanup remove %s: %w", path, err)
		}
		removed++
		return nil
	})
	if err != nil {
		return removed, err
	}
	return removed, nil
}

func orphanFormatStem(name string) string {
	// "clip.f137.mp4" -> "clip"
	lower := strings.ToLower(name)
	loc := orphanFormatRe.FindStringIndex(lower)
	if loc == nil {
		return name
	}
	return name[:loc[0]]
}

func hasCompletePrefix(complete map[string]struct{}, stem string) bool {
	if stem == "" {
		return false
	}
	for base := range complete {
		if base == stem || strings.HasPrefix(base, stem) {
			return true
		}
	}
	return false
}

func isThumbWithoutID(name string) bool {
	lower := strings.ToLower(name)
	if !(strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".webp") || strings.HasSuffix(lower, ".png")) {
		return false
	}
	_, ok := ParseYouTubeID(name)
	return !ok
}

func isS01E000WithoutID(name string) bool {
	// Legacy EC placeholders: S01E000 in name, no [id]
	if !strings.Contains(strings.ToUpper(name), "S01E000") {
		return false
	}
	_, ok := ParseYouTubeID(name)
	return !ok
}
