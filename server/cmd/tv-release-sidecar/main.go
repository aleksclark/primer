// Command tv-release-sidecar writes an operator-authored signed
// ReleaseManifest envelope beside a Primer TV APK. It does not talk to
// Tasks, open a database, or use a Tasks token. Do not publish anything live
// from this command; it only produces an immutable staging directory.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	apkName       = "primer-tv.apk"
	versionName   = "version"
	sidecarName   = "release-manifest.json"
	tvPackage     = "com.aleksclark.primer.tv"
	signingKeyID  = "ed25519-v1"
	maxAPKBytes   = 256 << 20
	maxPayloadB64 = 16_384
)

var (
	hexSHA256RE   = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	packageNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	signerLineRE  = regexp.MustCompile(`(?i)^Signer(?: #1)? certificate SHA-256 digest:\s*([0-9a-f: ]+)$`)
	nativeCodeRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type manifest struct {
	PackageName   string   `json:"packageName"`
	Channel       string   `json:"channel"`
	VersionCode   int64    `json:"versionCode"`
	VersionName   string   `json:"versionName"`
	MinSdk        int      `json:"minSdk"`
	SupportedABIs []string `json:"supportedAbis"`
	SignerSHA256  string   `json:"signerSha256"`
	SHA256        string   `json:"sha256"`
	ByteSize      int64    `json:"byteSize"`
}

type sidecar struct {
	PackageName           string `json:"packageName"`
	VersionName           string `json:"versionName"`
	SignerSHA256          string `json:"signerSha256"`
	MinSdk                int    `json:"minSdk"`
	Channel               string `json:"channel"`
	ManifestPayloadBase64 string `json:"manifestPayloadBase64"`
	ManifestSignature     string `json:"manifestSignature"`
	SigningKeyID          string `json:"signingKeyId"`
}

type apkMeta struct {
	PackageName   string
	VersionCode   int64
	VersionName   string
	MinSdk        int
	SupportedABIs []string
	SignerSHA256  string
	SHA256        string
	ByteSize      int64
}

func main() {
	apk := flag.String("apk", "", "path to the signed Primer TV APK")
	out := flag.String("out", "", "new empty staging directory; refused if it already exists")
	channel := flag.String("channel", "stable", "release channel recorded in the signed payload")
	flag.Parse()
	if err := run(*apk, *out, *channel, os.Getenv("TV_RELEASE_SIGNING_KEY"), os.Getenv("TV_RELEASE_DIR")); err != nil {
		fail(err.Error())
	}
}

func run(apkPath, outPath, channel, keyRaw, liveReleaseDir string) error {
	apkPath = strings.TrimSpace(apkPath)
	outPath = strings.TrimSpace(outPath)
	channel = strings.TrimSpace(channel)
	if apkPath == "" || outPath == "" {
		return fmt.Errorf("-apk and -out are required")
	}
	if channel == "" {
		channel = "stable"
	}
	key, err := loadKey(keyRaw)
	if err != nil {
		return err
	}
	absAPK, err := filepath.Abs(apkPath)
	if err != nil {
		return err
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if err := refuseExistingOut(absOut, absAPK, liveReleaseDir); err != nil {
		return err
	}
	parent := filepath.Dir(absOut)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".tv-release-staging-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, 0o700); err != nil {
		return err
	}
	snapshot := filepath.Join(staging, apkName)
	copied, err := snapshotRegularFile(absAPK, snapshot, maxAPKBytes)
	if err != nil {
		return err
	}
	meta, err := inspectAPK(snapshot, copied)
	if err != nil {
		return err
	}
	if meta.PackageName != tvPackage {
		return fmt.Errorf("APK package is %s, not %s", meta.PackageName, tvPackage)
	}
	payload, err := json.Marshal(manifest{
		PackageName:   meta.PackageName,
		Channel:       channel,
		VersionCode:   meta.VersionCode,
		VersionName:   meta.VersionName,
		MinSdk:        meta.MinSdk,
		SupportedABIs: meta.SupportedABIs,
		SignerSHA256:  meta.SignerSHA256,
		SHA256:        meta.SHA256,
		ByteSize:      meta.ByteSize,
	})
	if err != nil {
		return err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	if len(payloadB64) == 0 || len(payloadB64) > maxPayloadB64 {
		return fmt.Errorf("signed payload exceeds the 16KiB envelope cap")
	}
	sig := ed25519.Sign(key, payload)
	doc := sidecar{
		PackageName:           meta.PackageName,
		VersionName:           meta.VersionName,
		SignerSHA256:          meta.SignerSHA256,
		MinSdk:                meta.MinSdk,
		Channel:               channel,
		ManifestPayloadBase64: payloadB64,
		ManifestSignature:     base64.RawURLEncoding.EncodeToString(sig),
		SigningKeyID:          signingKeyID,
	}
	if err := os.WriteFile(filepath.Join(staging, versionName), []byte(strconv.FormatInt(meta.VersionCode, 10)+"\n"), 0o600); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, sidecarName), append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(staging, absOut); err != nil {
		return fmt.Errorf("publish staging directory: %w", err)
	}
	fmt.Printf("wrote %s versionCode=%d sha256=%s\n", absOut, meta.VersionCode, meta.SHA256)
	fmt.Fprintln(os.Stderr, "staging only; atomically replace the TV_RELEASE_DIR symlink. Do not publish live from this command.")
	return nil
}

func refuseExistingOut(absOut, absAPK, liveReleaseDir string) error {
	if _, err := os.Lstat(absOut); err == nil {
		return fmt.Errorf("-out already exists; refuse to clobber %s", absOut)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if samePath(absOut, absAPK) || samePath(filepath.Join(absOut, apkName), absAPK) {
		return fmt.Errorf("-out must not be the source APK")
	}
	live := strings.TrimSpace(liveReleaseDir)
	if live == "" {
		return nil
	}
	absLive, err := filepath.Abs(live)
	if err != nil {
		return err
	}
	if samePath(absOut, absLive) || strings.HasPrefix(absOut+string(os.PathSeparator), absLive+string(os.PathSeparator)) {
		return fmt.Errorf("-out must not be the live TV_RELEASE_DIR")
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.EvalSymlinks(a)
	if errA != nil {
		aa = a
	}
	bb, errB := filepath.EvalSymlinks(b)
	if errB != nil {
		bb = b
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func snapshotRegularFile(src, dest string, maxBytes int64) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("apk missing")
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("apk is not a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxBytes {
		return 0, fmt.Errorf("apk size out of bounds")
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(out, io.LimitReader(in, maxBytes+1))
	if err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return 0, err
	}
	if written != info.Size() || written > maxBytes {
		_ = os.Remove(dest)
		return 0, fmt.Errorf("apk size changed during copy")
	}
	again, err := os.Stat(src)
	if err != nil {
		_ = os.Remove(dest)
		return 0, err
	}
	if !again.Mode().IsRegular() || again.Size() != written {
		_ = os.Remove(dest)
		return 0, fmt.Errorf("apk changed during copy")
	}
	return written, nil
}

func inspectAPK(path string, expectedSize int64) (apkMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return apkMeta{}, fmt.Errorf("apk missing")
	}
	if !info.Mode().IsRegular() {
		return apkMeta{}, fmt.Errorf("apk is not a regular file")
	}
	if info.Size() != expectedSize || info.Size() <= 0 || info.Size() > maxAPKBytes {
		return apkMeta{}, fmt.Errorf("apk size out of bounds")
	}
	sum, err := fileSHA256(path, expectedSize)
	if err != nil {
		return apkMeta{}, err
	}
	dump, err := exec.Command(toolPath("TV_AAPT2", "aapt2"), "dump", "badging", path).CombinedOutput()
	if err != nil {
		return apkMeta{}, fmt.Errorf("aapt2 rejected apk: %s", sanitize(dump))
	}
	meta, err := parseBadging(string(dump))
	if err != nil {
		return apkMeta{}, err
	}
	verify, err := exec.Command(toolPath("TV_APKSIGNER", "apksigner"), "verify", "--print-certs", path).CombinedOutput()
	if err != nil {
		return apkMeta{}, fmt.Errorf("apksigner rejected apk: %s", sanitize(verify))
	}
	cert, err := parseSignerSHA256(string(verify))
	if err != nil {
		return apkMeta{}, err
	}
	again, err := os.Stat(path)
	if err != nil {
		return apkMeta{}, err
	}
	if again.Size() != expectedSize {
		return apkMeta{}, fmt.Errorf("apk size changed after inspect")
	}
	meta.SignerSHA256 = cert
	meta.SHA256 = sum
	meta.ByteSize = expectedSize
	return meta, nil
}

func parseBadging(out string) (apkMeta, error) {
	var meta apkMeta
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "package:"):
			if badgingField(line, "versionCodeMajor") != "" {
				return apkMeta{}, fmt.Errorf("unsupported versionCodeMajor")
			}
			meta.PackageName = badgingField(line, "name")
			if v := badgingField(line, "versionCode"); v != "" {
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil || n <= 0 {
					return apkMeta{}, fmt.Errorf("invalid versionCode")
				}
				meta.VersionCode = n
			}
			meta.VersionName = badgingField(line, "versionName")
		case strings.HasPrefix(line, "sdkVersion:"), strings.HasPrefix(line, "minSdkVersion:"):
			field := strings.TrimPrefix(strings.TrimPrefix(line, "minSdkVersion:"), "sdkVersion:")
			n, err := strconv.Atoi(strings.Trim(field, "'"))
			if err != nil || n <= 0 {
				return apkMeta{}, fmt.Errorf("invalid minSdk")
			}
			meta.MinSdk = n
		case strings.HasPrefix(line, "native-code:"):
			for _, part := range strings.Fields(strings.TrimPrefix(line, "native-code:")) {
				abi := strings.Trim(part, "'")
				if abi == "" {
					continue
				}
				if !nativeCodeRE.MatchString(abi) {
					return apkMeta{}, fmt.Errorf("invalid native ABI %q", abi)
				}
				meta.SupportedABIs = append(meta.SupportedABIs, abi)
			}
		}
	}
	if meta.PackageName == "" || meta.VersionCode <= 0 || meta.MinSdk <= 0 {
		return apkMeta{}, fmt.Errorf("apk metadata incomplete")
	}
	if !packageNameRE.MatchString(meta.PackageName) {
		return apkMeta{}, fmt.Errorf("invalid package name")
	}
	if meta.SupportedABIs == nil {
		meta.SupportedABIs = []string{}
	}
	return meta, nil
}

func badgingField(line, key string) string {
	re := regexp.MustCompile(key + `='([^']*)'`)
	m := re.FindStringSubmatch(line)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}

func parseSignerSHA256(out string) (string, error) {
	var digest string
	signers := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := signerLineRE.FindStringSubmatch(line); len(m) == 2 {
			signers++
			hexDigest := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(m[1], ":", ""), " ", ""))
			if !hexSHA256RE.MatchString(hexDigest) {
				return "", fmt.Errorf("apk signer digest missing")
			}
			digest = hexDigest
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "signer #") && strings.Contains(lower, "certificate sha-256 digest") && !strings.Contains(lower, "signer #1") {
			return "", fmt.Errorf("unsupported multi-signer APK")
		}
		if strings.HasPrefix(lower, "number of signers:") {
			n := strings.TrimSpace(strings.TrimPrefix(lower, "number of signers:"))
			if n != "" && n != "1" {
				return "", fmt.Errorf("unsupported multi-signer APK")
			}
		}
	}
	if signers != 1 || digest == "" {
		return "", fmt.Errorf("apk signer digest missing")
	}
	return digest, nil
}

func fileSHA256(path string, expectedSize int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, expectedSize+1))
	if err != nil {
		return "", err
	}
	if n != expectedSize {
		return "", fmt.Errorf("apk size changed during digest")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadKey(raw string) (ed25519.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("TV_RELEASE_SIGNING_KEY must be a 64-byte ed25519 private key (hex or base64url)")
	}
	var b []byte
	if decoded, err := hex.DecodeString(raw); err == nil {
		b = decoded
	} else if decoded, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		b = decoded
	} else {
		return nil, fmt.Errorf("TV_RELEASE_SIGNING_KEY must be a 64-byte ed25519 private key (hex or base64url)")
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("TV_RELEASE_SIGNING_KEY must be a 64-byte ed25519 private key (hex or base64url)")
	}
	seed := b[:ed25519.SeedSize]
	derived := ed25519.NewKeyFromSeed(seed)
	if !derived.Equal(ed25519.PrivateKey(b)) {
		return nil, fmt.Errorf("TV_RELEASE_SIGNING_KEY seed does not match public key")
	}
	return derived, nil
}

func toolPath(env, name string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func sanitize(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
