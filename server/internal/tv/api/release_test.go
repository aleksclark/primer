package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/tv/api"
	tvtestutil "github.com/aleksclark/primer/server/internal/tv/testutil"
	"github.com/aleksclark/primer/server/internal/tv/testutil/factory"
)

// publishRelease writes an APK and its version file into a fresh directory.
func publishRelease(t *testing.T, apk []byte, version string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "primer-tv.apk"), apk, 0o600))
	if version != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "version"), []byte(version), 0o600))
	}
	return dir
}

func TestAppReleaseDescribesThePublishedBuild(t *testing.T) {
	t.Parallel()
	apk := []byte("not really an apk, but bytes all the same")
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		ReleaseDir: publishRelease(t, apk, "7\n"),
	})
	_, token := factory.PairedDevice(t, q)

	resp := h.Get("/app/release", "Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	body := decode[api.AppRelease](t, resp.Body.Bytes())

	sum := sha256.Sum256(apk)
	assert.True(t, body.Available)
	assert.Equal(t, 7, body.VersionCode)
	assert.Equal(t, int64(len(apk)), body.SizeBytes)
	assert.Equal(t, hex.EncodeToString(sum[:]), body.SHA256, "the client verifies what it downloaded")
	assert.Equal(t, "/api/v1/app/release/apk", body.DownloadURL)
}

func TestAppReleaseDownloadServesTheAPK(t *testing.T) {
	t.Parallel()
	apk := []byte("apk-bytes")
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		ReleaseDir: publishRelease(t, apk, "3"),
	})
	_, token := factory.PairedDevice(t, q)

	resp := h.Get("/app/release/apk", "Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, apk, resp.Body.Bytes())
	assert.Equal(t, "application/vnd.android.package-archive", resp.Header().Get("Content-Type"))
}

func TestAppReleaseReportsNoBuildRatherThanFailing(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t)
	_, token := factory.PairedDevice(t, q)

	resp := h.Get("/app/release", "Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, resp.Code, "an unconfigured server is a normal state")
	body := decode[api.AppRelease](t, resp.Body.Bytes())
	assert.False(t, body.Available)
	assert.Zero(t, body.VersionCode)

	assert.Equal(t, http.StatusNotFound,
		h.Get("/app/release/apk", "Authorization: Bearer "+token).Code)
}

func TestAppReleaseWithoutAVersionFileReportsZero(t *testing.T) {
	t.Parallel()
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{
		ReleaseDir: publishRelease(t, []byte("apk"), ""),
	})
	_, token := factory.PairedDevice(t, q)

	body := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.True(t, body.Available)
	assert.Zero(t, body.VersionCode, "an unversioned build never looks newer than what is installed")
}

func TestAppReleaseRequiresAPairedDevice(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		ReleaseDir: publishRelease(t, []byte("apk"), "1"),
	})

	assert.Equal(t, http.StatusUnauthorized, h.Get("/app/release").Code)
	assert.Equal(t, http.StatusUnauthorized, h.Get("/app/release/apk").Code)
}
