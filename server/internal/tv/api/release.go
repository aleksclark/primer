package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

// AppRelease describes the APK currently published for sideloading.
type AppRelease struct {
	Available   bool   `json:"available" doc:"Whether a release is published at all."`
	VersionCode int    `json:"versionCode" doc:"Android versionCode of the published APK. Higher means newer."`
	SizeBytes   int64  `json:"sizeBytes" doc:"Size of the APK, so the client can show progress."`
	SHA256      string `json:"sha256" doc:"Hex digest of the APK, for verifying the download."`
	DownloadURL string `json:"downloadUrl" doc:"Path to fetch the APK from."`
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
	path, err := s.apkPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, huma.Error404NotFound("no app release is published")
	}
	return &apkOutput{
		ContentType:        "application/vnd.android.package-archive",
		ContentDisposition: `attachment; filename="` + apkFilename + `"`,
		Body:               data,
	}, nil
}

// apkPath resolves the published APK, refusing when releases are switched off.
func (s *Server) apkPath() (string, error) {
	if s.releaseDir == "" {
		return "", huma.Error404NotFound("app releases are not configured on this server")
	}
	return filepath.Join(s.releaseDir, apkFilename), nil
}

// readRelease describes the published APK. A missing release is reported as
// "none available" rather than an error: a server that has never had an APK
// uploaded is a normal state, and the client should simply not offer an update.
func (s *Server) readRelease() (*AppRelease, error) {
	if s.releaseDir == "" {
		return &AppRelease{Available: false}, nil
	}
	path := filepath.Join(s.releaseDir, apkFilename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AppRelease{Available: false}, nil
		}
		return nil, huma.Error500InternalServerError("cannot read the published release")
	}

	version, err := s.readVersionCode()
	if err != nil {
		return nil, err
	}

	sum, err := fileSHA256(path)
	if err != nil {
		return nil, huma.Error500InternalServerError("cannot digest the published release")
	}

	return &AppRelease{
		Available:   true,
		VersionCode: version,
		SizeBytes:   info.Size(),
		SHA256:      sum,
		DownloadURL: "/api/v1/app/release/apk",
	}, nil
}

// readVersionCode reads the published version code. It is kept in a plain file
// beside the APK so publishing is a copy and a write, with no build tooling on
// the server.
func (s *Server) readVersionCode() (int, error) {
	raw, err := os.ReadFile(filepath.Join(s.releaseDir, versionFilename))
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
