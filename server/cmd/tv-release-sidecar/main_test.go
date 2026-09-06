package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanonicalManifestMatchesGoFieldOrder(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(manifest{
		PackageName:   "com.aleksclark.primer.tv",
		Channel:       "stable",
		VersionCode:   2,
		VersionName:   `Primer "N+1" 测试`,
		MinSdk:        28,
		SupportedABIs: []string{"arm64-v8a"},
		SignerSHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SHA256:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ByteSize:      12,
	})
	require.NoError(t, err)
	require.Equal(t, `{"packageName":"com.aleksclark.primer.tv","channel":"stable","versionCode":2,"versionName":"Primer \"N+1\" 测试","minSdk":28,"supportedAbis":["arm64-v8a"],"signerSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","byteSize":12}`, string(payload))
	require.LessOrEqual(t, len(base64.RawURLEncoding.EncodeToString(payload)), maxPayloadB64)
}

func TestLoadKeyRejectsMismatchedSeedAndPublic(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	got, err := loadKey(fmtHex(priv))
	require.NoError(t, err)
	require.True(t, priv.Equal(got))
	_, err = loadKey(base64.RawURLEncoding.EncodeToString(priv))
	require.NoError(t, err)
	bad := append([]byte{}, priv...)
	bad[len(bad)-1] ^= 0xff
	_, err = loadKey(fmtHex(bad))
	require.Error(t, err)
	_, err = loadKey("")
	require.Error(t, err)
	_, err = loadKey("abc")
	require.Error(t, err)
}

func TestRefuseExistingOutAndSourceDest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.apk")
	require.NoError(t, os.WriteFile(src, []byte("apk"), 0o600))
	existing := filepath.Join(dir, "already")
	require.NoError(t, os.Mkdir(existing, 0o700))
	require.Error(t, refuseExistingOut(existing, src, ""))
	require.Error(t, refuseExistingOut(src, src, ""))
	live := filepath.Join(dir, "live")
	require.NoError(t, os.Mkdir(live, 0o700))
	require.Error(t, refuseExistingOut(live, src, live))
	fresh := filepath.Join(dir, "fresh")
	require.NoError(t, refuseExistingOut(fresh, src, live))
}

func TestSnapshotRejectsNonRegularAndSameSizeReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "snap.apk")
	_, err := snapshotRegularFile(dir, dest, maxAPKBytes)
	require.Error(t, err)

	src := filepath.Join(dir, "src.apk")
	require.NoError(t, os.WriteFile(src, []byte("same-size-bytes"), 0o600))
	n, err := snapshotRegularFile(src, dest, maxAPKBytes)
	require.NoError(t, err)
	require.Equal(t, int64(len("same-size-bytes")), n)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, []byte("same-size-bytes"), got)

	require.NoError(t, os.WriteFile(src, []byte("replaced-bytes!"), 0o600))
	again := filepath.Join(dir, "snap2.apk")
	n, err = snapshotRegularFile(src, again, maxAPKBytes)
	require.NoError(t, err)
	require.Equal(t, int64(len("replaced-bytes!")), n)
	got, err = os.ReadFile(again)
	require.NoError(t, err)
	require.Equal(t, []byte("replaced-bytes!"), got)
	original, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, []byte("same-size-bytes"), original, "first snapshot must stay the original bytes")
}

func TestParseBadgingEmptyNativeCodeIsUniversal(t *testing.T) {
	t.Parallel()
	meta, err := parseBadging("package: name='com.aleksclark.primer.tv' versionCode='2' versionName='0.2.0'\nsdkVersion:'28'\n")
	require.NoError(t, err)
	require.Empty(t, meta.SupportedABIs)
	require.NotNil(t, meta.SupportedABIs)
	_, err = parseBadging("package: name='com.aleksclark.primer.tv' versionCode='2' versionCodeMajor='1' versionName='0.2.0'\nsdkVersion:'28'\n")
	require.Error(t, err)
}

func TestParseSignerRequiresExactCertificateLine(t *testing.T) {
	t.Parallel()
	digest, err := parseSignerSHA256("Signer #1 certificate SHA-256 digest: aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99\nNumber of signers: 1\n")
	require.NoError(t, err)
	require.Equal(t, "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899", digest)
	_, err = parseSignerSHA256("SHA-256: aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899\n")
	require.Error(t, err)
	_, err = parseSignerSHA256("Signer #1 certificate SHA-256 digest: aa\nSigner #2 certificate SHA-256 digest: bb\nNumber of signers: 2\n")
	require.Error(t, err)
}

func TestRenameNoReplaceDoesNotClobberExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "dest")
	require.NoError(t, os.Mkdir(src, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "keep"), []byte("old"), 0o600))
	err := renameNoReplace(src, dest)
	require.Error(t, err)
	got, err := os.ReadFile(filepath.Join(dest, "keep"))
	require.NoError(t, err)
	require.Equal(t, []byte("old"), got)
	_, err = os.Stat(src)
	require.NoError(t, err, "source staging dir must remain after a refused rename")
}

func TestSnapshotFIFODoesNotBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	dest := filepath.Join(dir, "snap.apk")
	done := make(chan error, 1)
	go func() {
		_, err := snapshotRegularFile(fifo, dest, maxAPKBytes)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		require.Contains(t, err.Error(), "not a regular file")
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO snapshot blocked instead of failing closed")
	}
}

func TestStageRefusesClobberAndSourceDest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.apk")
	require.NoError(t, os.WriteFile(src, []byte("apk"), 0o600))
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key := fmtHex(priv)
	existing := filepath.Join(dir, "out")
	require.NoError(t, os.Mkdir(existing, 0o700))
	require.Error(t, run(src, existing, "stable", key, ""))
	require.Error(t, run(src, src, "stable", key, ""))
	live := filepath.Join(dir, "live")
	require.NoError(t, os.Mkdir(live, 0o700))
	require.Error(t, run(src, live, "stable", key, live))
}

func mustDecode(v string) []byte {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		panic(err)
	}
	return b
}

func acceptanceRequested() bool {
	v := strings.TrimSpace(os.Getenv("TV_RELEASE_SIDECAR_ACCEPTANCE"))
	return v == "1" || strings.EqualFold(v, "true")
}

func requiredAPK(t *testing.T, wd string) string {
	t.Helper()
	if explicit := strings.TrimSpace(os.Getenv("TV_RELEASE_SIDECAR_APK")); explicit != "" {
		info, err := os.Stat(explicit)
		require.NoError(t, err, "TV_RELEASE_SIDECAR_APK is required for acceptance")
		require.True(t, info.Mode().IsRegular())
		return explicit
	}
	candidates := []string{
		filepath.Clean(filepath.Join(wd, "../../../android/app/build/outputs/apk/debug/app-debug.apk")),
		filepath.Clean(filepath.Join(wd, "../../android/app/build/outputs/apk/debug/app-debug.apk")),
	}
	for _, src := range candidates {
		if info, err := os.Stat(src); err == nil && info.Mode().IsRegular() {
			return src
		}
	}
	t.Fatalf("tv-release-sidecar-acceptance requires a signed TV APK (assemble :app:assembleDebug or set TV_RELEASE_SIDECAR_APK)")
	return ""
}

func lookTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	root := os.Getenv("ANDROID_HOME")
	if root == "" {
		root = "/opt/android-sdk"
	}
	matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", name))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func fmtHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}

func TestStageCLIProducesSignedSidecarFromInspectableAPK(t *testing.T) {
	if !acceptanceRequested() {
		t.Skip("ordinary unit job; run make tv-release-sidecar-acceptance for the required executable CLI gate")
	}
	aapt2 := lookTool("aapt2")
	apksigner := lookTool("apksigner")
	require.NotEmpty(t, aapt2, "aapt2 is required for tv-release-sidecar-acceptance")
	require.NotEmpty(t, apksigner, "apksigner is required for tv-release-sidecar-acceptance")
	wd, err := os.Getwd()
	require.NoError(t, err)
	src := requiredAPK(t, wd)
	dir := t.TempDir()
	copied := filepath.Join(dir, "in.apk")
	n, err := snapshotRegularFile(src, copied, maxAPKBytes)
	require.NoError(t, err)
	require.Greater(t, n, int64(0))
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	dest := filepath.Join(dir, "staging")
	t.Setenv("TV_AAPT2", aapt2)
	t.Setenv("TV_APKSIGNER", apksigner)
	require.NoError(t, run(copied, dest, "stable", fmtHex(priv), ""))
	require.Error(t, run(copied, dest, "stable", fmtHex(priv), ""))
	body, err := os.ReadFile(filepath.Join(dest, sidecarName))
	require.NoError(t, err)
	var doc sidecar
	require.NoError(t, json.Unmarshal(body, &doc))
	require.Equal(t, tvPackage, doc.PackageName)
	require.Equal(t, signingKeyID, doc.SigningKeyID)
	payload, err := base64.RawURLEncoding.DecodeString(doc.ManifestPayloadBase64)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(priv.Public().(ed25519.PublicKey), payload, mustDecode(doc.ManifestSignature)))
	require.True(t, strings.Contains(string(payload), `"packageName":"com.aleksclark.primer.tv"`))
	staged, err := os.ReadFile(filepath.Join(dest, apkName))
	require.NoError(t, err)
	original, err := os.ReadFile(copied)
	require.NoError(t, err)
	require.Equal(t, original, staged)
}
