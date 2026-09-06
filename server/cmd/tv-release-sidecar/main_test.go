package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

func TestLoadKeyAcceptsHexAndBase64URL(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.Equal(t, ed25519.PrivateKeySize, len(loadKey(fmtHex(priv))))
	require.Equal(t, ed25519.PrivateKeySize, len(loadKey(base64.RawURLEncoding.EncodeToString(priv))))
	require.Nil(t, loadKey(""))
	require.Nil(t, loadKey("abc"))
}

func TestCopyFileWritesExactBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "dest")
	require.NoError(t, os.WriteFile(src, []byte("apk-bytes"), 0o600))
	require.NoError(t, copyFile(src, dest))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, []byte("apk-bytes"), got)
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
