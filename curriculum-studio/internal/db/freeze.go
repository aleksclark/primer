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

// LiveMarkerPath is the optional ops marker file that classifies an env as live.
const LiveMarkerFilename = "STUDIO_MIGRATIONS_LIVE"

// IsLiveEnv reports whether migrations are live via env flag or marker file path.
func IsLiveEnv(envTruthy bool, markerPath string) bool {
	if envTruthy {
		return true
	}
	if markerPath == "" {
		return false
	}
	_, err := os.Stat(markerPath)
	return err == nil
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
