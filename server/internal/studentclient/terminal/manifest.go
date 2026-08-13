package terminal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

const (
	// MaxManifestEntries bounds workspace walk size.
	MaxManifestEntries = 500
	// MaxManifestFileBytes is the largest file fully digested.
	MaxManifestFileBytes = 256 * 1024
)

// CaptureManifest walks root and returns a relative-path digest manifest.
// Host paths and file contents are never stored — only path, type, mode, digest.
func CaptureManifest(root string) (contracts.WorkspaceManifest, error) {
	m := contracts.WorkspaceManifest{
		SchemaVersion: contracts.WorkspaceManifestSchemaVersion,
	}
	if root == "" {
		return m, nil
	}
	root = filepath.Clean(root)
	var entries []contracts.WorkspaceManifestEntry
	truncated := false

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if len(entries) >= MaxManifestEntries {
			truncated = true
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		ent := contracts.WorkspaceManifestEntry{
			Path: rel,
			Mode: fmt.Sprintf("%04o", info.Mode().Perm()),
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			ent.Type = contracts.PathTypeSymlink
		case info.IsDir():
			ent.Type = contracts.PathTypeDirectory
		default:
			ent.Type = contracts.PathTypeFile
			ent.Size = info.Size()
			if info.Size() <= MaxManifestFileBytes && info.Mode().IsRegular() {
				if sum, herr := hashFile(path); herr == nil {
					ent.SHA256 = sum
				}
			}
		}
		entries = append(entries, ent)
		return nil
	})
	if err != nil {
		return m, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	m.Entries = entries
	m.Truncated = truncated
	m.Digest = digestManifest(m)
	return m, nil
}

// WriteSetDiff returns relative paths whose type/mode/digest changed or appeared/disappeared.
func WriteSetDiff(before, after contracts.WorkspaceManifest) []string {
	byPath := func(m contracts.WorkspaceManifest) map[string]contracts.WorkspaceManifestEntry {
		out := make(map[string]contracts.WorkspaceManifestEntry, len(m.Entries))
		for _, e := range m.Entries {
			out[e.Path] = e
		}
		return out
	}
	a := byPath(before)
	b := byPath(after)
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for p, eb := range b {
		ea, ok := a[p]
		if !ok || ea.Type != eb.Type || ea.Mode != eb.Mode || ea.SHA256 != eb.SHA256 || ea.Size != eb.Size {
			add(p)
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			add(p)
		}
	}
	sort.Strings(out)
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, MaxManifestFileBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestManifest(m contracts.WorkspaceManifest) string {
	type wire struct {
		Entries   []contracts.WorkspaceManifestEntry `json:"entries"`
		Truncated bool                               `json:"truncated"`
	}
	raw, _ := json.Marshal(wire{Entries: m.Entries, Truncated: m.Truncated})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
