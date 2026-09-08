package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func loadLaunchMetas(root string, manifest runManifest) ([]manifestLaunch, error) {
	pattern := filepath.Join(root, "launches", "*", "meta.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("discover launches: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no child coverage launches under %s", root)
	}
	seen := map[string]bool{}
	var launches []manifestLaunch
	for _, metaPath := range matches {
		raw, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", metaPath, err)
		}
		var launch manifestLaunch
		if err := json.Unmarshal(raw, &launch); err != nil {
			return nil, fmt.Errorf("parse %s: %w", metaPath, err)
		}
		if launch.ID == "" {
			return nil, fmt.Errorf("%s: launch id is empty", metaPath)
		}
		if seen[launch.ID] {
			return nil, fmt.Errorf("duplicate launch id %s", launch.ID)
		}
		seen[launch.ID] = true
		if launch.Dir == "" {
			launch.Dir = filepath.Dir(metaPath)
		}
		if err := qualifyLaunch(root, manifest, launch, raw, launch.ID); err != nil {
			return nil, err
		}
		launches = append(launches, launch)
	}
	return launches, nil
}

func qualifyLaunch(root string, manifest runManifest, launch manifestLaunch, raw []byte, id string) error {
	resolvedDir, err := filepath.Abs(launch.Dir)
	if err != nil {
		return fmt.Errorf("launch %s directory unreadable: %w", id, err)
	}
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("launch %s directory escapes run root", id)
	}
	var extra struct {
		RunID     string `json:"runId"`
		CoverMode string `json:"coverMode"`
		CoverPkg  string `json:"coverPkg"`
		SourceSHA string `json:"sourceSha"`
	}
	if err := json.Unmarshal(raw, &extra); err != nil {
		return fmt.Errorf("parse launch qualification %s: %w", id, err)
	}
	if extra.RunID != "" && extra.RunID != manifest.RunID {
		return fmt.Errorf("launch %s run id %s does not match %s", id, extra.RunID, manifest.RunID)
	}
	if extra.SourceSHA != "" && manifest.SourceSHA != "" && extra.SourceSHA != manifest.SourceSHA {
		return fmt.Errorf("launch %s source %s does not match run %s", id, extra.SourceSHA, manifest.SourceSHA)
	}
	if extra.CoverMode != "" && manifest.CoverMode != "" && extra.CoverMode != manifest.CoverMode {
		return fmt.Errorf("launch %s cover mode %s does not match run %s", id, extra.CoverMode, manifest.CoverMode)
	}
	if extra.CoverPkg != "" && manifest.CoverPkg != "" && extra.CoverPkg != manifest.CoverPkg {
		return fmt.Errorf("launch %s coverpkg does not match the run", id)
	}
	return nil
}

func convertLaunch(dir, id string, manifest runManifest) (string, error) {
	if err := requireCounterData(dir, id); err != nil {
		return "", err
	}
	out := filepath.Join(dir, "profile.out")
	args := []string{"tool", "covdata", "textfmt", "-hw", "-i=" + dir, "-o=" + out}
	if pkg := strings.TrimSpace(os.Getenv("PRIMER_TASKS_COVER_PKG_FILTER")); pkg != "" {
		args = append(args, "-pkg="+pkg)
	} else {
		args = append(args, "-pkg=primer-tasks/internal/...")
	}
	cmd := exec.Command("go", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("launch %s covdata conversion failed: %v (%s)", id, err, strings.TrimSpace(string(output)))
	}
	info, err := os.Stat(out)
	if err != nil {
		return "", fmt.Errorf("launch %s conversion produced no profile: %w", id, err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("launch %s conversion produced an empty profile", id)
	}
	_ = manifest
	return out, nil
}

func requireCounterData(dir, id string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("launch %s coverage directory unreadable: %w", id, err)
	}
	var hasMeta, hasCounters bool
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("launch %s coverage artifact stat failed: %w", id, err)
		}
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "covmeta."):
			if info.Size() == 0 {
				return fmt.Errorf("launch %s has empty coverage metadata", id)
			}
			hasMeta = true
		case strings.HasPrefix(name, "covcounters."):
			if info.Size() == 0 {
				return fmt.Errorf("launch %s has empty coverage counters", id)
			}
			hasCounters = true
		}
	}
	if !hasMeta || !hasCounters {
		return fmt.Errorf("launch %s missing coverage counters after normal exit", id)
	}
	return nil
}
