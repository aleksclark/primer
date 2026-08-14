package db

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
)

// BaselineManifestName is the checked-in freeze inventory file name.
const BaselineManifestName = "baseline_manifest.json"

// ManifestEntry records one frozen baseline migration file.
type ManifestEntry struct {
	File              string `json:"file"`
	SHA256            string `json:"sha256"`
	BaselineImmutable bool   `json:"baseline_immutable_after_live"`
}

// BaselineManifest is the freeze inventory for migrations 00001–00004.
type BaselineManifest struct {
	VersionTable string          `json:"version_table"`
	Schema       string          `json:"schema"`
	Migrations   []ManifestEntry `json:"migrations"`
}

// BaselineFiles are the immutable initial migration history files.
var BaselineFiles = []string{
	"00001_identity_and_catalogs.sql",
	"00002_plan_domain.sql",
	"00003_materialization_and_integration.sql",
	"00004_invariants.sql",
}

// BuildManifest hashes migration files under migrationsDir (directory containing *.sql).
func BuildManifest(migrationsDir string) (BaselineManifest, error) {
	m := BaselineManifest{
		VersionTable: VersionTable,
		Schema:       SchemaName,
		Migrations:   make([]ManifestEntry, 0, len(BaselineFiles)),
	}
	for _, name := range BaselineFiles {
		path := filepath.Join(migrationsDir, name)
		sum, err := fileSHA256(path)
		if err != nil {
			return BaselineManifest{}, fmt.Errorf("%s: %w", name, err)
		}
		m.Migrations = append(m.Migrations, ManifestEntry{
			File:              name,
			SHA256:            sum,
			BaselineImmutable: true,
		})
	}
	return m, nil
}

// WriteManifest writes the manifest as indented JSON.
func WriteManifest(path string, m BaselineManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// LoadManifest reads a baseline manifest JSON file.
func LoadManifest(path string) (BaselineManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BaselineManifest{}, err
	}
	var m BaselineManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return BaselineManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// VerifyManifest checks that frozen baseline files still match the manifest.
// When liveMarker is true (or liveMarkerPath exists), any hash mismatch is a hard failure.
// When not live, mismatches are still reported as errors so CI catches accidental edits
// after the freeze inventory is committed (post-freeze pre-live drift).
func VerifyManifest(migrationsDir string, manifest BaselineManifest, live bool) error {
	if len(manifest.Migrations) == 0 {
		return fmt.Errorf("manifest has no migrations")
	}
	byName := make(map[string]ManifestEntry, len(manifest.Migrations))
	for _, e := range manifest.Migrations {
		byName[e.File] = e
	}
	var errs []string
	for _, name := range BaselineFiles {
		want, ok := byName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("missing manifest entry for %s", name))
			continue
		}
		got, err := fileSHA256(filepath.Join(migrationsDir, name))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if !strings.EqualFold(got, want.SHA256) {
			msg := fmt.Sprintf("%s: sha256 drift (manifest=%s file=%s)", name, want.SHA256, got)
			if live || want.BaselineImmutable {
				msg += " — frozen baseline must not be rewritten; add 00005+ instead"
			}
			errs = append(errs, msg)
		}
	}
	// Detect unexpected edits to non-baseline files is out of scope; ensure no
	// baseline file is missing from disk beyond the loop above.
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("freeze check failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// LiveMarkerFilename is the optional ops marker that classifies an env as live.
// Any existing path at this name (regular file, directory, symlink-to-file/dir)
// is live. Broken symlinks (Stat error) are not live — only resolvable existing paths.
const LiveMarkerFilename = "STUDIO_MIGRATIONS_LIVE"

// LiveMarkerExists reports whether markerPath exists as any filesystem node.
// Uses os.Stat (follows symlinks): regular file, directory, and symlink-to-*
// that resolve all count as live. Broken symlinks and missing paths do not.
// Python freeze_inventory.live_marker_exists must match these semantics.
func LiveMarkerExists(markerPath string) bool {
	if markerPath == "" {
		return false
	}
	_, err := os.Stat(markerPath)
	return err == nil
}

// ClassifyLive is the single dual-signal live classifier for env value + marker path.
// Env uses Truthy (trim+lower); marker uses LiveMarkerExists (any existing path).
func ClassifyLive(envValue, markerPath string) bool {
	return Truthy(envValue) || LiveMarkerExists(markerPath)
}

// IsLiveEnv reports whether migrations are live via pre-parsed env flag or marker path.
// Prefer ClassifyLive when the raw env string is available so spellings stay centralized.
func IsLiveEnv(envTruthy bool, markerPath string) bool {
	if envTruthy {
		return true
	}
	return LiveMarkerExists(markerPath)
}

// GuardWriteFreeze refuses regenerating baseline_manifest.json when the
// environment is live-classified (STUDIO_MIGRATIONS_LIVE env and/or marker file).
// Check mode remains available; there is intentionally no break-glass rewrite flag.
func GuardWriteFreeze(envTruthy bool, markerPath string) error {
	if !IsLiveEnv(envTruthy, markerPath) {
		return nil
	}
	return fmt.Errorf("refusing write-freeze: migrations are live-classified (STUDIO_MIGRATIONS_LIVE env or %s marker); baseline rewrite is pre-live only — use -check-freeze / --check and add 00005+ instead", LiveMarkerFilename)
}

// HashFS hashes baseline files from an fs.FS with migrations/ prefix.
func HashFS(fsys fs.FS) (map[string]string, error) {
	out := make(map[string]string, len(BaselineFiles))
	for _, name := range BaselineFiles {
		path := "migrations/" + name
		f, err := fsys.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		sum, err := readerSHA256(f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		out[name] = sum
	}
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readerSHA256(f)
}

func readerSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
