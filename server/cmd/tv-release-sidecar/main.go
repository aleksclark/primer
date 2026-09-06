// Command tv-release-sidecar writes an operator-authored signed
// ReleaseManifest envelope beside a Primer TV APK. It does not talk to
// Tasks, open a database, or use a Tasks token. Do not publish anything live
// from this command; it only produces files for a later atomic directory swap.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	apkName      = "primer-tv.apk"
	versionName  = "version"
	sidecarName  = "release-manifest.json"
	tvPackage    = "com.aleksclark.primer.tv"
	signingKeyID = "ed25519-v1"
	maxAPKBytes  = 256 << 20
	maxPayloadB64 = 16_384
)

var (
	hexSHA256RE   = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	packageNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
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

func main() {
	apk := flag.String("apk", "", "path to the signed Primer TV APK")
	out := flag.String("out", "", "staging directory to write primer-tv.apk, version, and release-manifest.json")
	channel := flag.String("channel", "stable", "release channel recorded in the signed payload")
	flag.Parse()
	if strings.TrimSpace(*apk) == "" || strings.TrimSpace(*out) == "" {
		fail("-apk and -out are required")
	}
	key := loadKey(os.Getenv("TV_RELEASE_SIGNING_KEY"))
	if len(key) != ed25519.PrivateKeySize {
		fail("TV_RELEASE_SIGNING_KEY must be a 64-byte ed25519 private key (hex or base64url)")
	}

	meta, err := inspectAPK(*apk)
	if err != nil {
		fail(err.Error())
	}
	if meta.PackageName != tvPackage {
		fail("APK package is " + meta.PackageName + ", not " + tvPackage)
	}
	payload, err := json.Marshal(manifest{
		PackageName:   meta.PackageName,
		Channel:       *channel,
		VersionCode:   meta.VersionCode,
		VersionName:   meta.VersionName,
		MinSdk:        meta.MinSdk,
		SupportedABIs: meta.SupportedABIs,
		SignerSHA256:  meta.SignerSHA256,
		SHA256:        meta.SHA256,
		ByteSize:      meta.ByteSize,
	})
	if err != nil {
		fail(err.Error())
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	if len(payloadB64) == 0 || len(payloadB64) > maxPayloadB64 {
		fail("signed payload exceeds the 16KiB envelope cap")
	}
	sig := ed25519.Sign(key, payload)
	doc := sidecar{
		PackageName:           meta.PackageName,
		VersionName:           meta.VersionName,
		SignerSHA256:          meta.SignerSHA256,
		MinSdk:                meta.MinSdk,
		Channel:               *channel,
		ManifestPayloadBase64: payloadB64,
		ManifestSignature:     base64.RawURLEncoding.EncodeToString(sig),
		SigningKeyID:          signingKeyID,
	}

	if err := os.MkdirAll(*out, 0o700); err != nil {
		fail(err.Error())
	}
	if err := copyFile(*apk, filepath.Join(*out, apkName)); err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(filepath.Join(*out, versionName), []byte(strconv.FormatInt(meta.VersionCode, 10)+"\n"), 0o600); err != nil {
		fail(err.Error())
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(filepath.Join(*out, sidecarName), append(encoded, '\n'), 0o600); err != nil {
		fail(err.Error())
	}
	fmt.Printf("wrote %s versionCode=%d sha256=%s\n", *out, meta.VersionCode, meta.SHA256)
	fmt.Fprintln(os.Stderr, "staging only; swap the directory onto TV_RELEASE_DIR atomically. Do not publish live from this command.")
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

func inspectAPK(path string) (apkMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return apkMeta{}, fmt.Errorf("apk missing")
	}
	if info.Size() <= 0 || info.Size() > maxAPKBytes {
		return apkMeta{}, fmt.Errorf("apk size out of bounds")
	}
	sum, err := fileSHA256(path)
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
	meta.SignerSHA256 = cert
	meta.SHA256 = sum
	meta.ByteSize = info.Size()
	return meta, nil
}

func parseBadging(out string) (apkMeta, error) {
	var meta apkMeta
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "package:"):
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
				meta.SupportedABIs = append(meta.SupportedABIs, strings.Trim(part, "'"))
			}
		}
	}
	if meta.PackageName == "" || meta.VersionCode <= 0 || meta.MinSdk <= 0 {
		return apkMeta{}, fmt.Errorf("apk metadata incomplete")
	}
	if !packageNameRE.MatchString(meta.PackageName) {
		return apkMeta{}, fmt.Errorf("invalid package name")
	}
	if len(meta.SupportedABIs) == 0 {
		meta.SupportedABIs = []string{"armeabi-v7a", "arm64-v8a"}
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
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "sha-256") && !strings.Contains(lower, "sha256") {
			continue
		}
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			continue
		}
		hexDigest := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(line[idx+1:]), ":", ""))
		hexDigest = strings.ReplaceAll(hexDigest, " ", "")
		if hexSHA256RE.MatchString(hexDigest) {
			return hexDigest, nil
		}
	}
	return "", fmt.Errorf("apk signer digest missing")
}

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

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func loadKey(raw string) ed25519.PrivateKey {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if b, err := hex.DecodeString(raw); err == nil && len(b) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(b)
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(b) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(b)
	}
	return nil
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
