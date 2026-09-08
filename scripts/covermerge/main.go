package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type manifestLaunch struct {
	ID           string `json:"id"`
	Dir          string `json:"dir"`
	BinarySHA256 string `json:"binarySha256"`
	ExitClass    string `json:"exitClass"`
	Cover        bool   `json:"cover"`
}

type runManifest struct {
	RunID         string   `json:"runId"`
	SourceSHA     string   `json:"sourceSha"`
	BinarySHA256  string   `json:"binarySha256"`
	CoverMode     string   `json:"coverMode"`
	CoverPkg      string   `json:"coverPkg"`
	Race          bool     `json:"race"`
	GoVersion     string   `json:"goVersion"`
	GOOS          string   `json:"goos"`
	GOARCH        string   `json:"goarch"`
	GOFLAGS       string   `json:"goflags"`
	GOWORK        string   `json:"gowork"`
	Packages      []string `json:"packages"`
	ParentProfile string   `json:"parentProfile"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: covermerge: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: covermerge <run-root> <parent-profile> <merged-profile>")
	}
	root, parentPath, mergedPath := args[0], args[1], args[2]
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("coverage run root is empty")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("coverage run root missing: %s", root)
	}
	manifest, err := loadManifest(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	if manifest.ParentProfile != "" && filepath.Clean(manifest.ParentProfile) != filepath.Clean(parentPath) {
		return fmt.Errorf("manifest parent profile %s does not match %s", manifest.ParentProfile, parentPath)
	}
	parent, err := parseProfile(parentPath)
	if err != nil {
		return err
	}
	parentTotal, parentCovered := profileTotals(parent)
	if manifest.CoverMode != "" && parent.Mode != manifest.CoverMode {
		return fmt.Errorf("parent mode %s does not match manifest %s", parent.Mode, manifest.CoverMode)
	}
	launches, err := loadLaunchMetas(root, manifest)
	if err != nil {
		return err
	}
	childAdded := 0
	normalSeen := 0
	for _, launch := range launches {
		switch launch.ExitClass {
		case "normal":
			normalSeen++
			if !launch.Cover {
				return fmt.Errorf("launch %s expected coverage but was not instrumented", launch.ID)
			}
			if launch.BinarySHA256 != "" && manifest.BinarySHA256 != "" && launch.BinarySHA256 != manifest.BinarySHA256 {
				return fmt.Errorf("launch %s binary hash %s does not match run %s", launch.ID, launch.BinarySHA256, manifest.BinarySHA256)
			}
			childPath, err := convertLaunch(launch.Dir, launch.ID, manifest)
			if err != nil {
				return err
			}
			child, err := parseProfile(childPath)
			if err != nil {
				return err
			}
			merged, added, err := mergeChildIntoParent(parent, child, "primer-tasks/internal/")
			if err != nil {
				return err
			}
			parent = merged
			childAdded += added
		case "sigkill":
			continue
		default:
			return fmt.Errorf("launch %s has unsupported exit class %q", launch.ID, launch.ExitClass)
		}
	}
	if normalSeen == 0 {
		return fmt.Errorf("no normal-exit child launches recorded")
	}
	if err := writeProfile(mergedPath, parent); err != nil {
		return err
	}
	mergedTotal, mergedCovered := profileTotals(parent)
	if mergedTotal != parentTotal {
		return fmt.Errorf("merged denominator %d differs from parent %d", mergedTotal, parentTotal)
	}
	fmt.Printf("parent-only: %s%% (%d/%d statements)\n", formatPercent(parentCovered, parentTotal), parentCovered, parentTotal)
	fmt.Printf("child-added statements: %d\n", childAdded)
	fmt.Printf("merged: %s%% (%d/%d statements)\n", formatPercent(mergedCovered, mergedTotal), mergedCovered, mergedTotal)
	return nil
}

func loadManifest(path string) (runManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return runManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return runManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if strings.TrimSpace(manifest.RunID) == "" {
		return runManifest{}, fmt.Errorf("manifest runId is empty")
	}
	if strings.TrimSpace(manifest.CoverMode) == "" {
		return runManifest{}, fmt.Errorf("manifest coverMode is empty")
	}
	return manifest, nil
}
