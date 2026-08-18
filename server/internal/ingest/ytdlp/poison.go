package ytdlp

import (
	"fmt"
	"os"
	"path/filepath"
)

// RemoveRootShowNFO deletes ONLY {outputDir}/tvshow.nfo (poison Primer Plano NFO).
// It must never touch Shows/<slug>/tvshow.nfo.
func RemoveRootShowNFO(outputDir string) error {
	if outputDir == "" {
		return fmt.Errorf("ytdlp: remove root nfo: output dir required")
	}
	path := filepath.Join(outputDir, "tvshow.nfo")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ytdlp: remove root tvshow.nfo: %w", err)
	}
	return nil
}
