package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type dialogueSourceFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
	Tracked bool   `json:"tracked"`
}
type dialogueSourceManifest struct {
	Worktree                   string               `json:"worktree"`
	GitDir                     string               `json:"gitDir"`
	HEAD                       string               `json:"head"`
	Branch                     string               `json:"branch"`
	BuildDir                   string               `json:"buildDir"`
	ModuleDir                  string               `json:"moduleDir"`
	ModulePath                 string               `json:"modulePath"`
	GoMod                      string               `json:"goMod"`
	TargetDir                  string               `json:"targetDir"`
	TargetImport               string               `json:"targetImport"`
	ParentTestExecutableSHA256 string               `json:"parentTestExecutableSha256"`
	InputDigest                string               `json:"inputDigest"`
	Dirty                      bool                 `json:"dirty"`
	DirtyInputDigest           string               `json:"dirtyInputDigest"`
	RuntimeFiles               []string             `json:"runtimeFiles"`
	Files                      []dialogueSourceFile `json:"files"`
}
type dialogueSourceBinding struct {
	Manifest                  string `json:"manifest"`
	ManifestSHA256            string `json:"manifestSha256"`
	Worktree                  string `json:"worktree"`
	HEAD                      string `json:"head"`
	InputDigest               string `json:"inputDigest"`
	Dirty                     bool   `json:"dirty"`
	DirtyInputDigest          string `json:"dirtyInputDigest"`
	StableAroundBuild         bool   `json:"stableAroundBuild"`
	EmbeddedVCSClassification string `json:"embeddedVcsClassification"`
}

// No inherited worktree selector may redirect these read-only Git operations.
// The Go1.26.6 VCS discovery bug is NOT hidden by rewriting embedded VCS values.
func dialogueBuildEnvironment() []string {
	var out []string
	for _, value := range os.Environ() {
		name := strings.SplitN(value, "=", 2)[0]
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM", "GIT_OPTIONAL_LOCKS", "GOFLAGS", "GOWORK":
			continue
		}
		out = append(out, value)
	}
	return append(out, "GOWORK=off", "GOFLAGS=-mod=readonly", "GIT_OPTIONAL_LOCKS=0")
}
func dialogueGit(dir string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = dialogueBuildEnvironment()
	out, err := command.Output()
	if err != nil {
		return "", errors.New("source Git metadata unavailable")
	}
	return strings.TrimSpace(string(out)), nil
}
func dialogueFileHash(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, f)
	return hex.EncodeToString(hash.Sum(nil)), n, err
}
func dialogueWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func dialogueManifestFiles(root string, paths map[string]bool, tracked map[string]bool) ([]dialogueSourceFile, error) {
	files := make([]dialogueSourceFile, 0, len(paths))
	for path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		if !dialogueWithin(root, resolved) {
			return nil, errors.New("source input escapes the owned worktree")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		relative = filepath.ToSlash(relative)
		digest, size, err := dialogueFileHash(path)
		if err != nil {
			return nil, err
		}
		files = append(files, dialogueSourceFile{Path: relative, SHA256: digest, Bytes: size, Tracked: tracked[relative]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
func dialogueInputDigest(files []dialogueSourceFile) string {
	encoded, _ := json.Marshal(files)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", sum)
}
func sameDialogueInputs(before, after dialogueSourceManifest) error {
	if !reflect.DeepEqual(before, after) {
		return errors.New("checkout/build inputs changed during child compilation or qualification")
	}
	return nil
}

// Capture the actual selected main-module closure AND a conservative superset
// of all cmd/internal Go/tests/SQL plus module checksums. The filesystem walk
// includes untracked files; selected go:embed files cover non-Go/SQL assets.
// No diff-only manifest may omit new dialogue source or incoming migrations.
func captureDialogueSource(buildDir string, race bool) (manifest dialogueSourceManifest, err error) {
	buildDir, err = filepath.Abs(buildDir)
	if err != nil {
		return manifest, err
	}
	buildDir, err = filepath.EvalSymlinks(buildDir)
	if err != nil {
		return manifest, err
	}
	root, err := dialogueGit(buildDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return manifest, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return manifest, err
	}
	if buildDir != filepath.Join(root, "primer-tasks") {
		return manifest, errors.New("Tasks child build directory is not the owned checkout module")
	}
	if expected := os.Getenv("PASEO_AGENT_CWD"); expected != "" {
		if resolved, e := filepath.EvalSymlinks(expected); e != nil || resolved != root {
			return manifest, errors.New("source worktree differs from the assigned agent workspace")
		}
	}
	manifest.Worktree, manifest.BuildDir = root, buildDir
	manifest.HEAD, err = dialogueGit(root, "rev-parse", "HEAD")
	if err != nil {
		return manifest, err
	}
	manifest.Branch, err = dialogueGit(root, "branch", "--show-current")
	if err != nil {
		return manifest, err
	}
	manifest.GitDir, err = dialogueGit(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return manifest, err
	}
	manifest.ParentTestExecutableSHA256, _, err = dialogueFileHash("/proc/self/exe")
	if err != nil {
		return manifest, err
	}
	args := []string{"list", "-deps", "-json"}
	if race {
		args = append(args, "-race")
	}
	args = append(args, "./cmd/tasks-server")
	command := exec.Command("go", args...)
	command.Dir = buildDir
	command.Env = dialogueBuildEnvironment()
	out, err := command.Output()
	if err != nil {
		return manifest, errors.New("actual Tasks target/module resolution failed")
	}
	type module struct {
		Path, Dir, GoMod string
		Main             bool
	}
	type pkg struct {
		Dir, ImportPath                                                                                                     string
		DepOnly                                                                                                             bool
		Module                                                                                                              *module
		GoFiles, CgoFiles, CFiles, CXXFiles, MFiles, HFiles, FFiles, SFiles, SwigFiles, SwigCXXFiles, SysoFiles, EmbedFiles []string
	}
	paths := map[string]bool{}
	runtimePaths := map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var p pkg
		err = decoder.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
		if p.Module == nil || !p.Module.Main {
			continue
		}
		if p.Module.Dir != buildDir || p.Module.Path != "primer-tasks" {
			return manifest, errors.New("child selected a foreign main module")
		}
		if !p.DepOnly {
			manifest.TargetDir, manifest.TargetImport, manifest.ModuleDir, manifest.ModulePath, manifest.GoMod = p.Dir, p.ImportPath, p.Module.Dir, p.Module.Path, p.Module.GoMod
		}
		for _, set := range [][]string{p.GoFiles, p.CgoFiles, p.CFiles, p.CXXFiles, p.MFiles, p.HFiles, p.FFiles, p.SFiles, p.SwigFiles, p.SwigCXXFiles, p.SysoFiles, p.EmbedFiles} {
			for _, file := range set {
				path := filepath.Join(p.Dir, file)
				paths[path] = true
				relative, e := filepath.Rel(root, path)
				if e != nil {
					return manifest, e
				}
				runtimePaths[filepath.ToSlash(relative)] = true
			}
		}
	}
	if manifest.TargetImport != "primer-tasks/cmd/tasks-server" || manifest.TargetDir != filepath.Join(buildDir, "cmd", "tasks-server") || manifest.GoMod != filepath.Join(buildDir, "go.mod") {
		return manifest, errors.New("resolved child target/module does not match Tasks source")
	}
	for _, directory := range []string{"cmd", "internal"} {
		err = filepath.WalkDir(filepath.Join(buildDir, directory), func(path string, entry fs.DirEntry, walkError error) error {
			if walkError != nil {
				return walkError
			}
			if !entry.IsDir() {
				paths[path] = true
			}
			return nil
		})
		if err != nil {
			return manifest, err
		}
	}
	paths[filepath.Join(buildDir, "go.mod")], paths[filepath.Join(buildDir, "go.sum")] = true, true
	trackedList, err := dialogueGit(root, "ls-files", "-z", "--", "primer-tasks")
	if err != nil {
		return manifest, err
	}
	tracked := map[string]bool{}
	for _, path := range strings.Split(trackedList, "\x00") {
		tracked[path] = true
	}
	manifest.Files, err = dialogueManifestFiles(root, paths, tracked)
	if err != nil {
		return manifest, err
	}
	for path := range runtimePaths {
		manifest.RuntimeFiles = append(manifest.RuntimeFiles, path)
	}
	sort.Strings(manifest.RuntimeFiles)
	status, err := dialogueGit(root, "status", "--porcelain=v1", "--untracked-files=all", "--", "primer-tasks/cmd", "primer-tasks/internal", "primer-tasks/go.mod", "primer-tasks/go.sum")
	if err != nil {
		return manifest, err
	}
	manifest.Dirty = status != ""
	for _, file := range manifest.Files {
		manifest.Dirty = manifest.Dirty || !file.Tracked
	}
	manifest.InputDigest = dialogueInputDigest(manifest.Files)
	if manifest.Dirty {
		sum := sha256.Sum256([]byte(manifest.HEAD + "\n" + manifest.InputDigest))
		manifest.DirtyInputDigest = fmt.Sprintf("%x", sum)
	}
	return manifest, nil
}
func classifyDialogueStamp(build dialogueChildBuild, source dialogueSourceManifest) string {
	if build.Revision == source.HEAD && build.Modified == source.Dirty {
		return "matches-explicit-worktree-metadata"
	}
	if build.Revision == "" {
		return "absent-not-source-authority"
	}
	return "mismatch-not-source-authority"
}
func skipChildProcessRaceRepeat(t *testing.T) {
	t.Helper()
	if os.Getenv("PRIMER_TASKS_RACE_REPEAT") == "1" {
		t.Skip("child-process Tasks server fixtures are covered by count=1 tests; 10-repeat race stays on in-process packages")
	}
}

func buildTasksDialogueChild(t *testing.T, binary string) (dialogueChildBuild, dialogueSourceManifest, dialogueSourceBinding) {
	t.Helper()
	skipChildProcessRaceRepeat(t)
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	before, err := captureDialogueSource(dir, dialogueParentRace)
	if err != nil {
		t.Fatal(err)
	}
	build := buildDialogueChildInDir(t, binary, "./cmd/tasks-server", dialogueParentRace, dir)
	after, err := captureDialogueSource(dir, dialogueParentRace)
	if err != nil {
		t.Fatal(err)
	}
	if err = sameDialogueInputs(before, after); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "tmp", "p4-child-proof")
	if err = os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(folder, "source-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(encoded); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	binding := dialogueSourceBinding{Manifest: f.Name(), ManifestSHA256: fmt.Sprintf("%x", digest), Worktree: after.Worktree, HEAD: after.HEAD, InputDigest: after.InputDigest, Dirty: after.Dirty, DirtyInputDigest: after.DirtyInputDigest, StableAroundBuild: true, EmbeddedVCSClassification: classifyDialogueStamp(build, after)}
	return build, after, binding
}
func (h *publicDialogueHarness) verifyChildSource() {
	h.t.Helper()
	// Runtime resolution was captured on BOTH sides of compilation. During
	// execution, rehash the conservative whole cmd/internal input superset,
	// including new/untracked assets, without repeatedly resolving SDK packages.
	now := h.source
	var err error
	now.HEAD, err = dialogueGit(now.Worktree, "rev-parse", "HEAD")
	if err != nil {
		h.t.Fatal(err)
	}
	now.Branch, err = dialogueGit(now.Worktree, "branch", "--show-current")
	if err != nil {
		h.t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, directory := range []string{"cmd", "internal"} {
		if err = filepath.WalkDir(filepath.Join(now.BuildDir, directory), func(path string, entry fs.DirEntry, walkError error) error {
			if walkError != nil {
				return walkError
			}
			if !entry.IsDir() {
				paths[path] = true
			}
			return nil
		}); err != nil {
			h.t.Fatal(err)
		}
	}
	paths[filepath.Join(now.BuildDir, "go.mod")], paths[filepath.Join(now.BuildDir, "go.sum")] = true, true
	trackedList, err := dialogueGit(now.Worktree, "ls-files", "-z", "--", "primer-tasks")
	if err != nil {
		h.t.Fatal(err)
	}
	tracked := map[string]bool{}
	for _, path := range strings.Split(trackedList, "\x00") {
		tracked[path] = true
	}
	now.Files, err = dialogueManifestFiles(now.Worktree, paths, tracked)
	if err != nil {
		h.t.Fatal(err)
	}
	now.InputDigest = dialogueInputDigest(now.Files)
	status, err := dialogueGit(now.Worktree, "status", "--porcelain=v1", "--untracked-files=all", "--", "primer-tasks/cmd", "primer-tasks/internal", "primer-tasks/go.mod", "primer-tasks/go.sum")
	if err != nil {
		h.t.Fatal(err)
	}
	now.Dirty = status != ""
	for _, file := range now.Files {
		now.Dirty = now.Dirty || !file.Tracked
	}
	now.DirtyInputDigest = ""
	if now.Dirty {
		sum := sha256.Sum256([]byte(now.HEAD + "\n" + now.InputDigest))
		now.DirtyInputDigest = fmt.Sprintf("%x", sum)
	}
	if err = sameDialogueInputs(h.source, now); err != nil {
		h.t.Fatal(err)
	}
}
func verifyDialogueRunningExecutable(pid int, expected string) (string, error) {
	actual, _, err := dialogueFileHash(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", errors.New("actual child executable unavailable")
	}
	if actual != expected {
		return actual, errors.New("running child executable differs from the qualified build")
	}
	return actual, nil
}

func (h *publicDialogueHarness) prepareChildCoverageLaunch(env *[]string) {
	h.t.Helper()
	root := os.Getenv("PRIMER_TASKS_CHILD_COVER_ROOT")
	h.coverDir, h.coverLaunchID = "", ""
	if root == "" {
		return
	}
	if !h.build.Cover {
		h.t.Fatal("coverage gate launched an uninstrumented Tasks server child")
	}
	id := fmt.Sprintf("%s-%d", strings.ReplaceAll(h.t.Name(), "/", "_"), time.Now().UnixNano())
	dir := filepath.Join(root, "launches", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		h.t.Fatal(err)
	}
	h.coverLaunchID, h.coverDir = id, dir
	filtered := (*env)[:0]
	for _, value := range *env {
		if strings.HasPrefix(value, "GOCOVERDIR=") {
			continue
		}
		filtered = append(filtered, value)
	}
	*env = append(filtered, "GOCOVERDIR="+dir)
}

func (h *publicDialogueHarness) recordChildCoverageLaunch(exit dialogueChildExit) {
	h.t.Helper()
	if h.coverDir == "" {
		return
	}
	class := "normal"
	switch {
	case exit.CrashRequested && !exit.BeforeSignal && !exit.TimedOut:
		class = "sigkill"
	case exit.WaitError != nil || exit.BeforeSignal || exit.TimedOut || exit.RaceReport:
		class = "failed"
	}
	meta := map[string]any{
		"id": h.coverLaunchID, "dir": h.coverDir, "runId": os.Getenv("PRIMER_TASKS_CHILD_COVER_RUN_ID"),
		"sourceSha": h.source.HEAD, "binarySha256": h.build.SHA256, "cover": h.build.Cover,
		"coverMode": childCoverageMode(), "coverPkg": os.Getenv("PRIMER_TASKS_CHILD_COVERPKG"),
		"exitClass": class, "crashRequested": exit.CrashRequested, "beforeSignal": exit.BeforeSignal,
		"timedOut": exit.TimedOut, "raceReport": exit.RaceReport,
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		h.t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(h.coverDir, "meta.json"), encoded, 0600); err != nil {
		h.t.Fatal(err)
	}
	if class == "normal" {
		if err = requireChildCoverageCounters(h.coverDir); err != nil {
			h.t.Errorf("normal child exit omitted coverage counters: %v", err)
		}
	}
}

func requireChildCoverageCounters(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var meta, counters bool
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(entry.Name(), "covmeta."):
			meta = info.Size() > 0
		case strings.HasPrefix(entry.Name(), "covcounters."):
			counters = info.Size() > 0
		}
	}
	if !meta || !counters {
		return errors.New("coverage metadata or counters missing")
	}
	return nil
}

func TestDialogueSourceManifestIncludesUntrackedInputsAndRejectsDrift(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "new_dialogue.go")
	migration := filepath.Join(root, "00010_dialogue.sql")
	embed := filepath.Join(root, "source.txt")
	for path, text := range map[string]string{source: "package fixture", migration: "SELECT 1;", embed: "curated version one"} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths := map[string]bool{source: true, migration: true, embed: true}
	files, err := dialogueManifestFiles(root, paths, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatal("untracked source/embed/migration omitted")
	}
	for _, file := range files {
		if file.Tracked {
			t.Fatal("untracked fixture mislabeled")
		}
	}
	original := dialogueSourceManifest{HEAD: "foundation", Files: files, InputDigest: dialogueInputDigest(files), RuntimeFiles: []string{"new_dialogue.go", "source.txt"}}
	if err = os.WriteFile(migration, []byte("SELECT 2;"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := dialogueManifestFiles(root, paths, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	candidate := original
	candidate.Files = changed
	candidate.InputDigest = dialogueInputDigest(changed)
	if sameDialogueInputs(original, candidate) == nil {
		t.Fatal("untracked migration mutation not detected")
	}
	candidate = original
	candidate.HEAD = "other"
	if sameDialogueInputs(original, candidate) == nil {
		t.Fatal("source HEAD change not detected")
	}
	candidate = original
	candidate.TargetDir = "foreign"
	if sameDialogueInputs(original, candidate) == nil {
		t.Fatal("foreign target selection not detected")
	}
	if classifyDialogueStamp(dialogueChildBuild{Revision: "ancestor"}, original) != "mismatch-not-source-authority" {
		t.Fatal("ancestor stamp accepted as checkout provenance")
	}
	if _, err = verifyDialogueRunningExecutable(os.Getpid(), strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong running executable digest accepted")
	}
	actual, _, err := dialogueFileHash("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = verifyDialogueRunningExecutable(os.Getpid(), actual); err != nil {
		t.Fatal(err)
	}
}

func TestChildCoverageLaunchIsolationAndNormalExitCounters(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PRIMER_TASKS_CHILD_COVER_ROOT", root)
	t.Setenv("PRIMER_TASKS_CHILD_COVER_RUN_ID", "run-test")
	t.Setenv("PRIMER_TASKS_CHILD_COVER_MODE", "atomic")
	t.Setenv("PRIMER_TASKS_CHILD_COVERPKG", "primer-tasks/internal/api")
	h := &publicDialogueHarness{t: t, build: dialogueChildBuild{Cover: true, SHA256: "abc"}, source: dialogueSourceManifest{HEAD: "src"}}
	env := []string{"GOCOVERDIR=/tmp/stale", "TASKS_ENV=test"}
	h.prepareChildCoverageLaunch(&env)
	if h.coverDir == "" || !strings.HasPrefix(h.coverDir, filepath.Join(root, "launches")) {
		t.Fatal("coverage launch directory was not allocated under the gate root")
	}
	var sawGOCOVER bool
	for _, value := range env {
		if strings.HasPrefix(value, "GOCOVERDIR=") {
			if sawGOCOVER || value != "GOCOVERDIR="+h.coverDir {
				t.Fatal("inherited GOCOVERDIR leaked into the child")
			}
			sawGOCOVER = true
		}
	}
	if !sawGOCOVER {
		t.Fatal("child GOCOVERDIR missing")
	}
	first := h.coverDir
	h.prepareChildCoverageLaunch(&env)
	if h.coverDir == first {
		t.Fatal("restart reused a previous coverage directory")
	}
	if err := os.WriteFile(filepath.Join(h.coverDir, "covmeta.x"), []byte("meta"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.coverDir, "covcounters.x"), []byte("counters"), 0600); err != nil {
		t.Fatal(err)
	}
	h.recordChildCoverageLaunch(dialogueChildExit{})
	raw, err := os.ReadFile(filepath.Join(h.coverDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"exitClass":"normal"`)) {
		t.Fatalf("normal exit metadata missing: %s", raw)
	}
}

func TestChildCoverageNormalExitWithoutCountersFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PRIMER_TASKS_CHILD_COVER_ROOT", root)
	if err := requireChildCoverageCounters(root); err == nil {
		t.Fatal("empty coverage directory accepted")
	}
	h := &publicDialogueHarness{t: t, build: dialogueChildBuild{Cover: true, SHA256: "abc"}, source: dialogueSourceManifest{HEAD: "src"}}
	env := []string{}
	h.prepareChildCoverageLaunch(&env)
	if err := requireChildCoverageCounters(h.coverDir); err == nil {
		t.Fatal("metadata-only normal exit was accepted")
	}
}

func TestObservedSigkillDoesNotRequireCoverageCounters(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PRIMER_TASKS_CHILD_COVER_ROOT", root)
	h := &publicDialogueHarness{t: t, build: dialogueChildBuild{Cover: true, SHA256: "abc"}, source: dialogueSourceManifest{HEAD: "src"}}
	env := []string{}
	h.prepareChildCoverageLaunch(&env)
	h.recordChildCoverageLaunch(dialogueChildExit{CrashRequested: true, SignalSent: true})
	raw, err := os.ReadFile(filepath.Join(h.coverDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"exitClass":"sigkill"`)) {
		t.Fatalf("sigkill class missing: %s", raw)
	}
}
