package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Materialize writes the fixture tree into destDir (created if needed).
// Paths are validated with contracts.SafeRelPath / JoinUnder.
func Materialize(destDir string, fixtures []contracts.FixtureEntry) error {
	if destDir == "" {
		return fmt.Errorf("destination directory is required")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	// Parents before children: sort by path depth then path.
	sorted := append([]contracts.FixtureEntry(nil), fixtures...)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := pathDepth(sorted[i].Path)
		dj := pathDepth(sorted[j].Path)
		if di != dj {
			return di < dj
		}
		return sorted[i].Path < sorted[j].Path
	})
	for _, f := range sorted {
		if err := materializeOne(destDir, f); err != nil {
			return err
		}
	}
	return nil
}

func pathDepth(p string) int {
	n := 0
	for _, c := range p {
		if c == '/' {
			n++
		}
	}
	return n
}

func materializeOne(root string, f contracts.FixtureEntry) error {
	full, err := contracts.JoinUnder(root, f.Path)
	if err != nil {
		return fmt.Errorf("fixture %q: %w", f.Path, err)
	}
	mode := os.FileMode(0o644)
	if f.Type == contracts.FixtureDirectory {
		mode = 0o755
	}
	if f.Mode != "" {
		m, err := contracts.ParseMode(f.Mode)
		if err != nil {
			return fmt.Errorf("fixture %q: %w", f.Path, err)
		}
		mode = os.FileMode(m)
	}
	switch f.Type {
	case contracts.FixtureDirectory:
		if err := os.MkdirAll(full, mode); err != nil {
			return fmt.Errorf("mkdir %s: %w", f.Path, err)
		}
		if err := os.Chmod(full, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", f.Path, err)
		}
	case contracts.FixtureFile:
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir parent for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(full, []byte(f.Content), mode); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
		// WriteFile applies umask; force the declared mode.
		if err := os.Chmod(full, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", f.Path, err)
		}
	default:
		return fmt.Errorf("fixture %q: unknown type %q", f.Path, f.Type)
	}
	return nil
}
