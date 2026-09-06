package devicemanagement

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	defaultMaxArtifactBytes = 200 << 20
	releaseTrustMissing     = "release trust root is not configured"
)

var hexSHA256RE = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type ReleaseManifest struct {
	PackageName   string   `json:"packageName"`
	Channel       string   `json:"channel"`
	VersionCode   int64    `json:"versionCode" minimum:"1"`
	VersionName   string   `json:"versionName"`
	MinSdk        int      `json:"minSdk" minimum:"1"`
	SupportedABIs []string `json:"supportedAbis" nullable:"false"`
	SignerSHA256  string   `json:"signerSha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	SHA256        string   `json:"sha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	ByteSize      int64    `json:"byteSize" minimum:"1"`
}

type Release struct {
	ID                    string          `json:"id" format:"uuid"`
	PackageName           string          `json:"packageName"`
	Channel               string          `json:"channel"`
	VersionCode           int64           `json:"versionCode" minimum:"1"`
	VersionName           string          `json:"versionName"`
	MinSdk                int             `json:"minSdk" minimum:"1"`
	SupportedABIs         []string        `json:"supportedAbis" nullable:"false"`
	SignerSHA256          string          `json:"signerSha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	SHA256                string          `json:"sha256" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	ByteSize              int64           `json:"byteSize" minimum:"1"`
	Status                string          `json:"status" enum:"published,paused"`
	Manifest              ReleaseManifest `json:"manifest"`
	ManifestPayloadBase64 string          `json:"manifestPayloadBase64"`
	ManifestSignature     string          `json:"manifestSignature"`
	SigningKeyID          string          `json:"signingKeyId"`
	PublishedAt           time.Time       `json:"publishedAt" format:"date-time"`
}

type ReleasePage struct {
	Items []Release `json:"items" nullable:"false"`
}

type ReleaseTargetInput struct {
	ReleaseID         string `json:"releaseId" format:"uuid"`
	BaseTargetVersion int64  `json:"baseTargetVersion" minimum:"0"`
	Requeue           bool   `json:"requeue,omitempty"`
}

type ReleaseReceiptInput struct {
	ReportID             string `json:"reportId" format:"uuid"`
	TargetID             string `json:"targetId" format:"uuid"`
	Status               string `json:"status" enum:"queued,downloading,verifying,installing,confirmed,blocked,failed"`
	InstalledVersionCode *int64 `json:"installedVersionCode,omitempty"`
	InstalledVersionName string `json:"installedVersionName,omitempty"`
	Error                string `json:"error,omitempty" maxLength:"200"`
	TargetVersion        int64  `json:"targetVersion" minimum:"1"`
}

type ReleaseReceipt struct {
	ID                   string    `json:"id"`
	ReportID             string    `json:"reportId"`
	TargetID             string    `json:"targetId"`
	Status               string    `json:"status"`
	InstalledVersionCode *int64    `json:"installedVersionCode,omitempty"`
	ReceivedAt           time.Time `json:"receivedAt" format:"date-time"`
}

type APKMetadata struct {
	PackageName   string
	VersionCode   int64
	VersionName   string
	MinSdk        int
	SupportedABIs []string
	SignerSHA256  string
	SHA256        string
	ByteSize      int64
}

func (s *Service) maxBytes() int64 {
	if s.MaxArtifactBytes > 0 {
		return s.MaxArtifactBytes
	}
	return defaultMaxArtifactBytes
}

func (s *Service) trustKey() (ed25519.PublicKey, error) {
	if len(s.ReleaseSigningKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, releaseTrustMissing)
	}
	return s.ReleaseSigningKey.Public().(ed25519.PublicKey), nil
}

func (s *Service) PublishAPK(ctx context.Context, publisher, apkPath, channel string) (Release, error) {
	if strings.TrimSpace(publisher) == "" {
		return Release{}, ErrUnauthorized
	}
	pub, err := s.trustKey()
	if err != nil {
		return Release{}, err
	}
	if strings.TrimSpace(s.ArtifactDir) == "" {
		return Release{}, fmt.Errorf("%w: artifact store is not configured", ErrUnavailable)
	}
	if channel == "" {
		channel = "stable"
	}
	snapshot, err := snapshotAPK(apkPath, s.maxBytes())
	if err != nil {
		return Release{}, err
	}
	defer os.Remove(snapshot)
	meta, err := InspectAPK(snapshot, s.maxBytes())
	if err != nil {
		return Release{}, err
	}
	manifest := ReleaseManifest{
		PackageName:   meta.PackageName,
		Channel:       channel,
		VersionCode:   meta.VersionCode,
		VersionName:   meta.VersionName,
		MinSdk:        meta.MinSdk,
		SupportedABIs: meta.SupportedABIs,
		SignerSHA256:  meta.SignerSHA256,
		SHA256:        meta.SHA256,
		ByteSize:      meta.ByteSize,
	}
	payload, err := canonicalManifest(manifest)
	if err != nil {
		return Release{}, err
	}
	sig := ed25519.Sign(s.ReleaseSigningKey, payload)
	keyID := hex.EncodeToString(pub)
	destDir := filepath.Join(s.ArtifactDir, meta.SHA256)
	if err = os.MkdirAll(destDir, 0700); err != nil {
		return Release{}, err
	}
	dest := filepath.Join(destDir, "app.apk")
	if err = publishImmutableAPK(snapshot, dest, meta.SHA256, meta.ByteSize); err != nil {
		return Release{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback(ctx)
	var existing Release
	err = scanReleaseRow(tx.QueryRow(ctx, `SELECT id,package_name,channel,version_code,version_name,min_sdk,supported_abis,signer_sha256,sha256,byte_size,status,manifest,manifest_payload,manifest_signature,signing_key_id,published_at FROM management_releases WHERE package_name=$1 AND channel=$2 AND version_code=$3`, manifest.PackageName, manifest.Channel, manifest.VersionCode), &existing)
	if err == nil {
		if existing.SHA256 != manifest.SHA256 || existing.ByteSize != manifest.ByteSize || existing.SignerSHA256 != manifest.SignerSHA256 || existing.ManifestPayloadBase64 != base64.RawURLEncoding.EncodeToString(payload) {
			return Release{}, fmt.Errorf("%w: package/channel/version already published with different bytes", ErrConflict)
		}
		if err = tx.Commit(ctx); err != nil {
			return Release{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Release{}, err
	}
	id := uuid.New()
	var out Release
	err = scanReleaseRow(tx.QueryRow(ctx, `INSERT INTO management_releases(id,package_name,channel,version_code,version_name,min_sdk,supported_abis,signer_sha256,sha256,byte_size,artifact_path,manifest,manifest_payload,manifest_signature,signing_key_id,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
 RETURNING id,package_name,channel,version_code,version_name,min_sdk,supported_abis,signer_sha256,sha256,byte_size,status,manifest,manifest_payload,manifest_signature,signing_key_id,published_at`,
		id, manifest.PackageName, manifest.Channel, manifest.VersionCode, manifest.VersionName, manifest.MinSdk, manifest.SupportedABIs, manifest.SignerSHA256, manifest.SHA256, manifest.ByteSize, dest, payload, payload, base64.RawURLEncoding.EncodeToString(sig), keyID, publisher),
		&out)
	if err != nil {
		return Release{}, err
	}
	metaJSON, err := marshalJSON(map[string]any{"releaseId": out.ID, "sha256": out.SHA256})
	if err != nil {
		return Release{}, err
	}
	if err = insertAudit(ctx, tx, uuid.Nil, nil, "release_publisher", publisher, "management.release_published", metaJSON); err != nil {
		return Release{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Release{}, err
	}
	return out, nil
}

func snapshotAPK(src string, maxBytes int64) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("%w: apk missing", ErrInvalid)
	}
	if int64(len(data)) <= 0 || int64(len(data)) > maxBytes {
		return "", fmt.Errorf("%w: apk size out of bounds", ErrInvalid)
	}
	f, err := os.CreateTemp("", "primer-apk-*.apk")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func publishImmutableAPK(src, dest, digest string, size int64) error {
	if info, err := os.Stat(dest); err == nil {
		existing, err := os.ReadFile(dest)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(existing)
		if hex.EncodeToString(sum[:]) != digest || int64(len(existing)) != size || info.Size() != size {
			return fmt.Errorf("%w: immutable artifact path already holds different bytes", ErrConflict)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".app-*.apk.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err = os.Link(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		if errors.Is(err, os.ErrExist) {
			return publishImmutableAPK(src, dest, digest, size)
		}
		return err
	}
	_ = os.Remove(tmpName)
	return nil
}

func canonicalManifest(m ReleaseManifest) ([]byte, error) {
	type wire struct {
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
	return json.Marshal(wire(m))
}

func InspectAPK(path string, maxBytes int64) (APKMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return APKMetadata{}, fmt.Errorf("%w: apk missing", ErrInvalid)
	}
	if info.Size() <= 0 || info.Size() > maxBytes {
		return APKMetadata{}, fmt.Errorf("%w: apk size out of bounds", ErrInvalid)
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return APKMetadata{}, err
	}
	aapt := toolPath("TASKS_AAPT2", "aapt2")
	signer := toolPath("TASKS_APKSIGNER", "apksigner")
	dump, err := exec.Command(aapt, "dump", "badging", path).CombinedOutput()
	if err != nil {
		return APKMetadata{}, fmt.Errorf("%w: aapt2 rejected apk: %s", ErrInvalid, sanitizeTool(dump))
	}
	meta, err := parseBadging(string(dump))
	if err != nil {
		return APKMetadata{}, err
	}
	verify, err := exec.Command(signer, "verify", "--print-certs", path).CombinedOutput()
	if err != nil {
		return APKMetadata{}, fmt.Errorf("%w: apksigner rejected apk: %s", ErrInvalid, sanitizeTool(verify))
	}
	cert, err := parseSignerSHA256(string(verify))
	if err != nil {
		return APKMetadata{}, err
	}
	meta.SignerSHA256 = cert
	meta.SHA256 = sum
	meta.ByteSize = info.Size()
	return meta, nil
}

func parseBadging(out string) (APKMetadata, error) {
	var meta APKMetadata
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "package:"):
			meta.PackageName = badgingField(line, "name")
			if v := badgingField(line, "versionCode"); v != "" {
				n, err := strconv.ParseInt(v, 10, 64)
				if err != nil || n <= 0 {
					return APKMetadata{}, fmt.Errorf("%w: invalid versionCode", ErrInvalid)
				}
				meta.VersionCode = n
			}
			meta.VersionName = badgingField(line, "versionName")
		case strings.HasPrefix(line, "sdkVersion:"), strings.HasPrefix(line, "minSdkVersion:"):
			field := strings.TrimPrefix(strings.TrimPrefix(line, "minSdkVersion:"), "sdkVersion:")
			n, err := strconv.Atoi(strings.Trim(field, "'"))
			if err != nil || n <= 0 {
				return APKMetadata{}, fmt.Errorf("%w: invalid minSdk", ErrInvalid)
			}
			meta.MinSdk = n
		case strings.HasPrefix(line, "native-code:"):
			for _, part := range strings.Fields(strings.TrimPrefix(line, "native-code:")) {
				meta.SupportedABIs = append(meta.SupportedABIs, strings.Trim(part, "'"))
			}
		}
	}
	if meta.PackageName == "" || meta.VersionCode <= 0 || meta.MinSdk <= 0 {
		return APKMetadata{}, fmt.Errorf("%w: apk metadata incomplete", ErrInvalid)
	}
	if !packageNameRE.MatchString(meta.PackageName) {
		return APKMetadata{}, fmt.Errorf("%w: invalid package name", ErrInvalid)
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
		if !strings.Contains(strings.ToLower(line), "sha-256") && !strings.Contains(strings.ToLower(line), "sha256") {
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
	return "", fmt.Errorf("%w: apk signer digest missing", ErrInvalid)
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func sanitizeTool(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}

func toolPath(env, name string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"), "/opt/android-sdk"} {
		if root == "" {
			continue
		}
		for _, ver := range []string{"35.0.0", "34.0.0"} {
			candidate := filepath.Join(root, "build-tools", ver, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return name
}

func (s *Service) ListPublishedReleases(ctx context.Context) ([]Release, error) {
	rows, err := s.pool().Query(ctx, `SELECT id,package_name,channel,version_code,version_name,min_sdk,supported_abis,signer_sha256,sha256,byte_size,status,manifest,manifest_payload,manifest_signature,signing_key_id,published_at FROM management_releases WHERE status='published' ORDER BY package_name,version_code DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Release{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) GetPublishedRelease(ctx context.Context, id string) (Release, error) {
	return s.getRelease(ctx, id, true)
}

func (s *Service) GetRelease(ctx context.Context, id string) (Release, error) {
	return s.getRelease(ctx, id, false)
}

func (s *Service) getRelease(ctx context.Context, id string, publishedOnly bool) (Release, error) {
	q := `SELECT id,package_name,channel,version_code,version_name,min_sdk,supported_abis,signer_sha256,sha256,byte_size,status,manifest,manifest_payload,manifest_signature,signing_key_id,published_at FROM management_releases WHERE id=$1`
	if publishedOnly {
		q += ` AND status='published'`
	}
	var r Release
	err := scanReleaseRow(s.pool().QueryRow(ctx, q, id), &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	return r, err
}

func scanRelease(row interface {
	Scan(dest ...any) error
}) (Release, error) {
	var r Release
	err := scanReleaseRow(row, &r)
	return r, err
}

func scanReleaseRow(row interface {
	Scan(dest ...any) error
}, r *Release) error {
	var raw, payload []byte
	var abis []string
	err := row.Scan(&r.ID, &r.PackageName, &r.Channel, &r.VersionCode, &r.VersionName, &r.MinSdk, &abis, &r.SignerSHA256, &r.SHA256, &r.ByteSize, &r.Status, &raw, &payload, &r.ManifestSignature, &r.SigningKeyID, &r.PublishedAt)
	if err != nil {
		return err
	}
	r.SupportedABIs = abis
	if len(payload) == 0 {
		return fmt.Errorf("%w: signed manifest payload missing", ErrUnavailable)
	}
	r.ManifestPayloadBase64 = base64.RawURLEncoding.EncodeToString(payload)
	if err = json.Unmarshal(payload, &r.Manifest); err != nil {
		return fmt.Errorf("%w: signed manifest payload is not valid JSON", ErrUnavailable)
	}
	return nil
}

func (s *Service) SetReleaseTarget(ctx context.Context, sc Scope, deviceID string, in ReleaseTargetInput) (ReleaseTarget, error) {
	deviceUUID, err := parseUUID(deviceID, "deviceId")
	if err != nil {
		return ReleaseTarget{}, err
	}
	tenantUUID, err := uuid.Parse(sc.TenantID)
	if err != nil {
		return ReleaseTarget{}, err
	}
	rel, err := s.GetPublishedRelease(ctx, in.ReleaseID)
	if err != nil {
		return ReleaseTarget{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return ReleaseTarget{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantUUID, deviceUUID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return ReleaseTarget{}, ErrNotFound
	} else if err != nil {
		return ReleaseTarget{}, err
	}
	if state != string(DeviceActive) {
		return ReleaseTarget{}, ErrForbidden
	}
	policy, err := s.LatestPolicy(ctx, sc.TenantID, deviceID)
	if err != nil {
		return ReleaseTarget{}, err
	}
	if err = releaseAllowedByPolicy(policy, rel); err != nil {
		return ReleaseTarget{}, err
	}
	var existingID, existingRelease string
	var existingVersion int64
	var existingStatus string
	err = tx.QueryRow(ctx, `SELECT id,release_id,target_version,status FROM management_release_targets WHERE tenant_id=$1 AND device_id=$2 AND package_name=$3 AND channel=$4 FOR UPDATE`, tenantUUID, deviceUUID, rel.PackageName, rel.Channel).Scan(&existingID, &existingRelease, &existingVersion, &existingStatus)
	if err == nil {
		if existingRelease == rel.ID && !in.Requeue {
			if in.BaseTargetVersion != 0 && in.BaseTargetVersion != existingVersion {
				return ReleaseTarget{}, fmt.Errorf("%w: stale target version", ErrConflict)
			}
			if err = tx.Commit(ctx); err != nil {
				return ReleaseTarget{}, err
			}
			return s.targetFromRelease(existingID, existingVersion, existingStatus, rel), nil
		}
		if in.BaseTargetVersion != existingVersion {
			return ReleaseTarget{}, fmt.Errorf("%w: stale target version", ErrConflict)
		}
		next := existingVersion + 1
		if _, err = tx.Exec(ctx, `UPDATE management_release_targets SET release_id=$1,status='queued',target_version=$2,desired_by=$3,desired_at=now(),last_error='' WHERE tenant_id=$4 AND id=$5 AND target_version=$6`, rel.ID, next, sc.ActorRef, tenantUUID, existingID, existingVersion); err != nil {
			return ReleaseTarget{}, err
		}
		meta, err := marshalJSON(map[string]any{"releaseId": rel.ID, "targetVersion": next})
		if err != nil {
			return ReleaseTarget{}, err
		}
		if err = insertAudit(ctx, tx, tenantUUID, &deviceUUID, "parent", sc.ActorRef, "management.release_targeted", meta); err != nil {
			return ReleaseTarget{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return ReleaseTarget{}, err
		}
		return s.targetFromRelease(existingID, next, "queued", rel), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReleaseTarget{}, err
	}
	if in.BaseTargetVersion != 0 {
		return ReleaseTarget{}, fmt.Errorf("%w: stale target version", ErrConflict)
	}
	id := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO management_release_targets(id,tenant_id,device_id,release_id,package_name,channel,status,target_version,desired_by) VALUES($1,$2,$3,$4,$5,$6,'queued',1,$7)`, id, tenantUUID, deviceUUID, rel.ID, rel.PackageName, rel.Channel, sc.ActorRef); err != nil {
		return ReleaseTarget{}, err
	}
	meta, err := marshalJSON(map[string]any{"releaseId": rel.ID, "targetVersion": 1})
	if err != nil {
		return ReleaseTarget{}, err
	}
	if err = insertAudit(ctx, tx, tenantUUID, &deviceUUID, "parent", sc.ActorRef, "management.release_targeted", meta); err != nil {
		return ReleaseTarget{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReleaseTarget{}, err
	}
	return s.targetFromRelease(id.String(), 1, "queued", rel), nil
}

func releaseAllowedByPolicy(policy *PolicyRevision, rel Release) error {
	if policy == nil {
		return fmt.Errorf("%w: device has no approved-app policy", ErrInvalid)
	}
	for _, app := range append(append([]ApprovedApp{}, policy.Policy.ApprovedApps...), policy.Policy.RequiredPackages...) {
		if app.PackageName == rel.PackageName && app.SignerSHA256 == rel.SignerSHA256 {
			return nil
		}
	}
	return fmt.Errorf("%w: release package/signer is not approved", ErrInvalid)
}

func (s *Service) targetFromRelease(id string, version int64, status string, rel Release) ReleaseTarget {
	return ReleaseTarget{ID: id, ReleaseID: rel.ID, PackageName: rel.PackageName, Channel: rel.Channel, Status: status, VersionCode: rel.VersionCode, VersionName: rel.VersionName, SHA256: rel.SHA256, ByteSize: rel.ByteSize, TargetVersion: version, MinSdk: rel.MinSdk, SignerSHA256: rel.SignerSHA256, ManifestPayloadBase64: rel.ManifestPayloadBase64, ManifestSignature: rel.ManifestSignature, SigningKeyID: rel.SigningKeyID}
}

func (s *Service) DeviceReleaseTargets(ctx context.Context, tenantID, deviceID string) ([]ReleaseTarget, error) {
	rows, err := s.pool().Query(ctx, `SELECT t.id,t.release_id,t.package_name,t.channel,t.status,r.version_code,r.version_name,r.sha256,r.byte_size,t.target_version,r.min_sdk,r.signer_sha256,r.manifest_payload,r.manifest_signature,r.signing_key_id
 FROM management_release_targets t JOIN management_releases r ON r.id=t.release_id
 WHERE t.tenant_id=$1 AND t.device_id=$2 AND r.status='published' ORDER BY t.desired_at DESC LIMIT 32`, tenantID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseTarget{}
	for rows.Next() {
		var t ReleaseTarget
		var payload []byte
		if err = rows.Scan(&t.ID, &t.ReleaseID, &t.PackageName, &t.Channel, &t.Status, &t.VersionCode, &t.VersionName, &t.SHA256, &t.ByteSize, &t.TargetVersion, &t.MinSdk, &t.SignerSHA256, &payload, &t.ManifestSignature, &t.SigningKeyID); err != nil {
			return nil, err
		}
		t.ManifestPayloadBase64 = base64.RawURLEncoding.EncodeToString(payload)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) ReportRelease(ctx context.Context, sc DeviceScope, in ReleaseReceiptInput) (ReleaseReceipt, error) {
	reportID, err := parseUUID(in.ReportID, "reportId")
	if err != nil {
		return ReleaseReceipt{}, err
	}
	targetID, err := parseUUID(in.TargetID, "targetId")
	if err != nil {
		return ReleaseReceipt{}, err
	}
	if !validReleaseStatus(in.Status) {
		return ReleaseReceipt{}, ErrInvalid
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return ReleaseReceipt{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM management_devices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, sc.TenantID, sc.DeviceID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return ReleaseReceipt{}, ErrUnauthorized
	} else if err != nil {
		return ReleaseReceipt{}, err
	}
	if state != string(DeviceActive) {
		return ReleaseReceipt{}, ErrUnauthorized
	}
	var existing ReleaseReceipt
	var existingRaw []byte
	err = tx.QueryRow(ctx, `SELECT id,report_id,target_id,status,installed_version_code,raw,received_at FROM management_release_receipts WHERE tenant_id=$1 AND device_id=$2 AND report_id=$3`, sc.TenantID, sc.DeviceID, reportID).Scan(&existing.ID, &existing.ReportID, &existing.TargetID, &existing.Status, &existing.InstalledVersionCode, &existingRaw, &existing.ReceivedAt)
	if err == nil {
		var stored ReleaseReceiptInput
		if err = json.Unmarshal(existingRaw, &stored); err != nil {
			return ReleaseReceipt{}, err
		}
		want, err := marshalJSON(in)
		if err != nil {
			return ReleaseReceipt{}, err
		}
		got, err := marshalJSON(stored)
		if err != nil {
			return ReleaseReceipt{}, err
		}
		if string(want) != string(got) {
			return ReleaseReceipt{}, fmt.Errorf("%w: receipt payload changed", ErrConflict)
		}
		if err = tx.Commit(ctx); err != nil {
			return ReleaseReceipt{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReleaseReceipt{}, err
	}
	var currentVersion, expectedVersion int64
	var currentStatus, relStatus string
	if err = tx.QueryRow(ctx, `SELECT t.target_version,t.status,r.version_code,r.status FROM management_release_targets t JOIN management_releases r ON r.id=t.release_id WHERE t.tenant_id=$1 AND t.device_id=$2 AND t.id=$3 FOR UPDATE OF t`, sc.TenantID, sc.DeviceID, targetID).Scan(&currentVersion, &currentStatus, &expectedVersion, &relStatus); errors.Is(err, pgx.ErrNoRows) {
		return ReleaseReceipt{}, ErrNotFound
	} else if err != nil {
		return ReleaseReceipt{}, err
	}
	if relStatus != "published" {
		return ReleaseReceipt{}, fmt.Errorf("%w: paused release is not auto-deliverable", ErrConflict)
	}
	if in.TargetVersion != currentVersion {
		return ReleaseReceipt{}, fmt.Errorf("%w: stale release target", ErrConflict)
	}
	if currentStatus == "confirmed" && in.Status != "confirmed" {
		return ReleaseReceipt{}, fmt.Errorf("%w: cannot regress terminal release status", ErrConflict)
	}
	if in.Status == "confirmed" {
		if in.InstalledVersionCode == nil || *in.InstalledVersionCode != expectedVersion {
			return ReleaseReceipt{}, fmt.Errorf("%w: confirmed receipt requires exact installed versionCode", ErrInvalid)
		}
	}
	id := uuid.New()
	raw, err := marshalJSON(in)
	if err != nil {
		return ReleaseReceipt{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO management_release_receipts(id,tenant_id,device_id,target_id,report_id,status,installed_version_code,installed_version_name,error,raw) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, sc.TenantID, sc.DeviceID, targetID, reportID, in.Status, in.InstalledVersionCode, in.InstalledVersionName, in.Error, raw); err != nil {
		return ReleaseReceipt{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE management_release_targets SET status=$1,last_receipt_at=now(),last_error=$2 WHERE tenant_id=$3 AND id=$4 AND target_version=$5`, in.Status, in.Error, sc.TenantID, targetID, currentVersion); err != nil {
		return ReleaseReceipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReleaseReceipt{}, err
	}
	return ReleaseReceipt{ID: id.String(), ReportID: in.ReportID, TargetID: targetID.String(), Status: in.Status, InstalledVersionCode: in.InstalledVersionCode, ReceivedAt: s.now()}, nil
}

func validReleaseStatus(v string) bool {
	switch v {
	case "queued", "downloading", "verifying", "installing", "confirmed", "blocked", "failed":
		return true
	default:
		return false
	}
}

func (s *Service) PauseRelease(ctx context.Context, publisher, releaseID string) (Release, error) {
	if strings.TrimSpace(publisher) == "" {
		return Release{}, ErrUnauthorized
	}
	id, err := parseUUID(releaseID, "releaseId")
	if err != nil {
		return Release{}, err
	}
	tx, err := s.pool().Begin(ctx)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE management_releases SET status='paused' WHERE id=$1 AND status='published'`, id)
	if err != nil {
		return Release{}, err
	}
	if tag.RowsAffected() == 0 {
		return Release{}, ErrNotFound
	}
	meta, err := marshalJSON(map[string]any{"releaseId": id.String()})
	if err != nil {
		return Release{}, err
	}
	if err = insertAudit(ctx, tx, uuid.Nil, nil, "release_publisher", publisher, "management.release_paused", meta); err != nil {
		return Release{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Release{}, err
	}
	return s.GetRelease(ctx, releaseID)
}

func (s *Service) ParentArtifactBytes(ctx context.Context, sc Scope, releaseID string) ([]byte, Release, error) {
	rel, err := s.GetPublishedRelease(ctx, releaseID)
	if err != nil {
		return nil, Release{}, err
	}
	data, err := readPublishedAPK(s.ArtifactDir, rel)
	if err != nil {
		return nil, Release{}, err
	}
	return data, rel, nil
}

func (s *Service) DeviceRelease(ctx context.Context, sc DeviceScope, releaseID string) (Release, error) {
	rel, err := s.GetPublishedRelease(ctx, releaseID)
	if err != nil {
		return Release{}, err
	}
	if !s.deviceTargeted(ctx, sc, releaseID) {
		return Release{}, ErrForbidden
	}
	return rel, nil
}

func (s *Service) ArtifactBytes(ctx context.Context, sc DeviceScope, releaseID string) ([]byte, Release, error) {
	rel, err := s.GetPublishedRelease(ctx, releaseID)
	if err != nil {
		return nil, Release{}, err
	}
	if !s.deviceTargeted(ctx, sc, releaseID) {
		return nil, Release{}, ErrForbidden
	}
	data, err := readPublishedAPK(s.ArtifactDir, rel)
	if err != nil {
		return nil, Release{}, err
	}
	return data, rel, nil
}

func (s *Service) deviceTargeted(ctx context.Context, sc DeviceScope, releaseID string) bool {
	targets, err := s.DeviceReleaseTargets(ctx, sc.TenantID, sc.DeviceID)
	if err != nil {
		return false
	}
	for _, t := range targets {
		if t.ReleaseID == releaseID {
			return true
		}
	}
	return false
}

func readPublishedAPK(dir string, rel Release) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, rel.SHA256, "app.apk"))
	if err != nil {
		return nil, ErrNotFound
	}
	if len(data) == 0 || int64(len(data)) != rel.ByteSize {
		return nil, fmt.Errorf("%w: artifact size mismatch", ErrUnavailable)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != rel.SHA256 {
		return nil, fmt.Errorf("%w: artifact digest mismatch", ErrUnavailable)
	}
	return data, nil
}
