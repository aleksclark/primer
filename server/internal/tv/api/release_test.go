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

func TestAppReleaseSidecarIsOptionalAndDoesNotShareTasksCredentials(t *testing.T) {
	t.Parallel()
	apk := []byte("apk-bytes")
	dir := publishRelease(t, apk, "4")
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{ReleaseDir: dir})
	_, token := factory.PairedDevice(t, q)

	unsigned := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.True(t, unsigned.Available)
	assert.Empty(t, unsigned.PackageName)
	assert.Nil(t, unsigned.ManifestPayloadBase64, "current servers stay unsigned until an operator writes the sidecar")

	payload := "payload-b64"
	sig := "sig-b64"
	signer := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	minSdk := 28
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-manifest.json"), []byte(`{
  "packageName":"com.aleksclark.primer.tv",
  "versionName":"0.2.0",
  "signerSha256":"`+signer+`",
  "minSdk":28,
  "channel":"stable",
  "manifestPayloadBase64":"`+payload+`",
  "manifestSignature":"`+sig+`",
  "signingKeyId":"ed25519-v1"
}`), 0o600))

	signed := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.Equal(t, "com.aleksclark.primer.tv", signed.PackageName)
	require.NotNil(t, signed.ManifestPayloadBase64)
	assert.Equal(t, payload, *signed.ManifestPayloadBase64)
	require.NotNil(t, signed.ManifestSignature)
	assert.Equal(t, sig, *signed.ManifestSignature)
	require.NotNil(t, signed.SigningKeyID)
	assert.Equal(t, "ed25519-v1", *signed.SigningKeyID)
	require.NotNil(t, signed.MinSdk)
	assert.Equal(t, minSdk, *signed.MinSdk)
}

func TestAppReleaseMalformedSidecarIsNotSilentAbsence(t *testing.T) {
	t.Parallel()
	apk := []byte("apk-bytes")
	dir := publishRelease(t, apk, "5")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-manifest.json"), []byte("{not json"), 0o600))
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{ReleaseDir: dir})
	_, token := factory.PairedDevice(t, q)

	body := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.True(t, body.Available)
	assert.Nil(t, body.ManifestPayloadBase64, "malformed sidecar must not look signed")
	assert.Empty(t, body.PackageName)
}

func TestAppReleaseOversizedSidecarIsRejected(t *testing.T) {
	t.Parallel()
	apk := []byte("apk-bytes")
	dir := publishRelease(t, apk, "6")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-manifest.json"), make([]byte, 24*1024+1), 0o600))
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{ReleaseDir: dir})
	_, token := factory.PairedDevice(t, q)

	body := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.True(t, body.Available)
	assert.Nil(t, body.ManifestPayloadBase64)
}

func TestAppReleaseResolvesAncestorSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	real := filepath.Join(root, "real", "rel-1")
	require.NoError(t, os.MkdirAll(real, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(real, "primer-tv.apk"), []byte("apk-v1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(real, "version"), []byte("1\n"), 0o600))
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(filepath.Join(root, "real"), alias))
	h, q, _ := tvtestutil.API(t, tvtestutil.Options{ReleaseDir: filepath.Join(alias, "rel-1")})
	_, token := factory.PairedDevice(t, q)
	body := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.Equal(t, 1, body.VersionCode)
}

func TestAppReleaseResolvesSymlinkToImmutableDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	v1 := filepath.Join(root, "rel-1")
	v2 := filepath.Join(root, "rel-2")
	require.NoError(t, os.Mkdir(v1, 0o700))
	require.NoError(t, os.Mkdir(v2, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(v1, "primer-tv.apk"), []byte("apk-v1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(v1, "version"), []byte("1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(v2, "primer-tv.apk"), []byte("apk-v2"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(v2, "version"), []byte("2\n"), 0o600))
	link := filepath.Join(root, "current")
	require.NoError(t, os.Symlink(v1, link))

	h, q, _ := tvtestutil.API(t, tvtestutil.Options{ReleaseDir: link})
	_, token := factory.PairedDevice(t, q)
	body := decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.Equal(t, 1, body.VersionCode)
	apk := h.Get("/app/release/apk", "Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, apk.Code)
	assert.Equal(t, []byte("apk-v1"), apk.Body.Bytes())

	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(v2, link))
	body = decode[api.AppRelease](t, h.Get("/app/release", "Authorization: Bearer "+token).Body.Bytes())
	assert.Equal(t, 2, body.VersionCode)
	apk = h.Get("/app/release/apk", "Authorization: Bearer "+token)
	require.Equal(t, http.StatusOK, apk.Code)
	assert.Equal(t, []byte("apk-v2"), apk.Body.Bytes())
}

func TestAppReleaseRequiresAPairedDevice(t *testing.T) {
	t.Parallel()
	h, _, _ := tvtestutil.API(t, tvtestutil.Options{
		ReleaseDir: publishRelease(t, []byte("apk"), "1"),
	})

	assert.Equal(t, http.StatusUnauthorized, h.Get("/app/release").Code)
	assert.Equal(t, http.StatusUnauthorized, h.Get("/app/release/apk").Code)
}
