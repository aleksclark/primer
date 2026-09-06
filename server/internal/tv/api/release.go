package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// releaseTag groups the app distribution operations in the spec.
const releaseTag = "App Release"

// apkFilename is the fixed name the release APK is published under. The box
// has no Play Store, so the server is the whole distribution channel.
const apkFilename = "primer-tv.apk"

// versionFilename holds the version code of the published APK. Version codes
// are monotonic, so the client only needs to compare integers.
const versionFilename = "version"

// sidecarFilename is an operator-authored signed ReleaseManifest envelope
// next to the APK. TV never talks to Tasks for this; there is no shared DB
// or credential. Missing sidecar is a normal current-server state and the
// client must fail closed rather than install on unsigned metadata.
const sidecarFilename = "release-manifest.json"

// maxSidecarBytes bounds the operator sidecar. The Android envelope cap is
// 16KiB of base64url payload; this leaves room for the JSON wrapper without
// an unbounded ReadFile.
const maxSidecarBytes = 24 * 1024

// AppRelease describes the APK currently published for sideloading.
type AppRelease struct {
	Available             bool    `json:"available" doc:"Whether a release is published at all."`
	VersionCode           int     `json:"versionCode" doc:"Android versionCode of the published APK. Higher means newer."`
	SizeBytes             int64   `json:"sizeBytes" doc:"Size of the APK, so the client can show progress."`
	SHA256                string  `json:"sha256" doc:"Hex digest of the APK, for verifying the download."`
	DownloadURL           string  `json:"downloadUrl" doc:"Path to fetch the APK from."`
	PackageName           string  `json:"packageName,omitempty" doc:"Claimed package; client accepts only the verified payload."`
	VersionName           *string `json:"versionName,omitempty"`
	SignerSHA256          *string `json:"signerSha256,omitempty"`
	MinSdk                *int    `json:"minSdk,omitempty"`
	Channel               *string `json:"channel,omitempty"`
	ManifestPayloadBase64 *string `json:"manifestPayloadBase64,omitempty" doc:"Exact signed ReleaseManifest bytes, base64url."`
	ManifestSignature     *string `json:"manifestSignature,omitempty" doc:"Ed25519 signature of those bytes, base64url."`
	SigningKeyID          *string `json:"signingKeyId,omitempty" doc:"Must be ed25519-v1."`
}

// releaseSidecar is the operator-authored file beside primer-tv.apk.
type releaseSidecar struct {
	PackageName           string `json:"packageName"`
	VersionName           string `json:"versionName"`
	SignerSHA256          string `json:"signerSha256"`
	MinSdk                int    `json:"minSdk"`
	Channel               string `json:"channel"`
	ManifestPayloadBase64 string `json:"manifestPayloadBase64"`
	ManifestSignature     string `json:"manifestSignature"`
	SigningKeyID          string `json:"signingKeyId"`
}

type releaseOutput struct {
	Body AppRelease
}

type apkOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Body               []byte
}

// registerAppRelease wires the self-update endpoints.
//
// Both are device-authenticated rather than admin-authenticated: the box is
// what needs to fetch them, and it only ever holds a device token. They are
// deliberately not public, so an unpaired client on the LAN cannot pull the
// build.
func (s *Server) registerAppRelease() {
	huma.Register(s.api, s.deviceOp(huma.Operation{
		OperationID: "get-app-release",
		Method:      http.MethodGet,
		Path:        "/app/release",
		Summary:     "Latest published app version",
		Description: "What the client compares against its own versionCode to decide whether to update.",
		Tags:        []string{releaseTag},
	}), s.getAppRelease)

	huma.Register(s.api, s.deviceOp(huma.Operation{
		OperationID: "download-app-apk",
		Method:      http.MethodGet,
		Path:        "/app/release/apk",
		Summary:     "Download the app",
		Description: "The published APK, for sideloading onto a device with no app store.",
		Tags:        []string{releaseTag},
		Errors:      []int{http.StatusNotFound},
	}), s.downloadAPK)
}

func (s *Server) getAppRelease(ctx context.Context, _ *struct{}) (*releaseOutput, error) {
	release, err := s.readRelease()
	if err != nil {
		return nil, err
	}
	return &releaseOutput{Body: *release}, nil
}

func (s *Server) downloadAPK(ctx context.Context, _ *struct{}) (*apkOutput, error) {
	dir, err := s.resolvedReleaseDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, apkFilename))
	if err != nil {
		return nil, huma.Error404NotFound("no app release is published")
	}
	return &apkOutput{
		ContentType:        "application/vnd.android.package-archive",
		ContentDisposition: `attachment; filename="` + apkFilename + `"`,
		Body:               data,
	}, nil
}

// resolvedReleaseDir follows TV_RELEASE_DIR once so metadata and the APK of
// one request come from the same immutable directory. A later symlink swap
// may still race a subsequent download; the Android client must then fail
// integrity and retry rather than install mixed bytes.
func (s *Server) resolvedReleaseDir() (string, error) {
	if s.releaseDir == "" {
		return "", huma.Error404NotFound("app releases are not configured on this server")
	}
	info, err := os.Lstat(s.releaseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", huma.Error404NotFound("app releases are not configured on this server")
		}
		return "", huma.Error500InternalServerError("cannot read the published release")
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return s.releaseDir, nil
	}
	target, err := filepath.EvalSymlinks(s.releaseDir)
	if err != nil {
		return "", huma.Error500InternalServerError("cannot read the published release")
	}
	return target, nil
}

func isNotFound(err error) bool {
	var humaErr huma.StatusError
	return errors.As(err, &humaErr) && humaErr.GetStatus() == http.StatusNotFound
}

// readRelease describes the published APK. A missing release is reported as
// "none available" rather than an error: a server that has never had an APK
// uploaded is a normal state, and the client should simply not offer an update.
func (s *Server) readRelease() (*AppRelease, error) {
	dir, err := s.resolvedReleaseDir()
	if err != nil {
		if isNotFound(err) {
			return &AppRelease{Available: false}, nil
		}
		return nil, err
	}
	path := filepath.Join(dir, apkFilename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AppRelease{Available: false}, nil
		}
		return nil, huma.Error500InternalServerError("cannot read the published release")
	}

	version, err := readVersionCode(dir)
	if err != nil {
		return nil, err
	}

	sum, err := fileSHA256(path)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot digest the published release")
	}

	release := &AppRelease{
		Available:   true,
		VersionCode: version,
		SizeBytes:   info.Size(),
		SHA256:      sum,
		DownloadURL: "/api/v1/app/release/apk",
	}
	if err := applySidecar(release, filepath.Join(dir, sidecarFilename)); err != nil {
		slog.Error("tv release sidecar is present but unusable; serving unsigned metadata", "error", err)
	}
	return release, nil
}

// applySidecar copies operator-authored signed-manifest fields when present.
// A missing file is the normal legacy state. A present but oversized,
// unreadable, or malformed sidecar is logged and left unsigned so it is not
// silently indistinguishable from absence. The Android client still refuses
// to install unsigned metadata.
func applySidecar(release *AppRelease, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", sidecarFilename, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", sidecarFilename)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("%s is empty", sidecarFilename)
	}
	if info.Size() > maxSidecarBytes {
		return fmt.Errorf("%s exceeds %d bytes", sidecarFilename, maxSidecarBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", sidecarFilename, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxSidecarBytes+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", sidecarFilename, err)
	}
	if int64(len(raw)) > maxSidecarBytes {
		return fmt.Errorf("%s exceeds %d bytes", sidecarFilename, maxSidecarBytes)
	}
	var side releaseSidecar
	if err := json.Unmarshal(raw, &side); err != nil {
		return fmt.Errorf("parse %s: %w", sidecarFilename, err)
	}
	if strings.TrimSpace(side.ManifestPayloadBase64) == "" || strings.TrimSpace(side.ManifestSignature) == "" || strings.TrimSpace(side.SigningKeyID) == "" {
		return fmt.Errorf("%s is missing signed-manifest fields", sidecarFilename)
	}
	if len(side.ManifestPayloadBase64) > 16_384 {
		return fmt.Errorf("%s payload exceeds 16384 base64url characters", sidecarFilename)
	}
	release.PackageName = strings.TrimSpace(side.PackageName)
	if v := strings.TrimSpace(side.VersionName); v != "" {
		release.VersionName = &v
	}
	if v := strings.TrimSpace(side.SignerSHA256); v != "" {
		release.SignerSHA256 = &v
	}
	if side.MinSdk > 0 {
		minSdk := side.MinSdk
		release.MinSdk = &minSdk
	}
	if v := strings.TrimSpace(side.Channel); v != "" {
		release.Channel = &v
	}
	payload := strings.TrimSpace(side.ManifestPayloadBase64)
	sig := strings.TrimSpace(side.ManifestSignature)
	keyID := strings.TrimSpace(side.SigningKeyID)
	release.ManifestPayloadBase64 = &payload
	release.ManifestSignature = &sig
	release.SigningKeyID = &keyID
	return nil
}

// readVersionCode reads the published version code. It is kept in a plain file
// beside the APK so publishing is a copy and a write, with no build tooling on
// the server.
func readVersionCode(dir string) (int, error) {
	raw, err := os.ReadFile(filepath.Join(dir, versionFilename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, huma.Error500InternalServerError("cannot read the published version")
	}
	version, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, huma.Error500InternalServerError(fmt.Sprintf("published version is not a number: %v", err))
	}
	return version, nil
}

// fileSHA256 digests a file without holding it all in memory.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
