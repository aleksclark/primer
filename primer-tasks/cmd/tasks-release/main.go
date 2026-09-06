package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/db"
	"primer-tasks/internal/devicemanagement"
)

func main() {
	apk := flag.String("apk", "", "path to a signed APK")
	channel := flag.String("channel", "stable", "release channel")
	pauseID := flag.String("pause", "", "published release UUID to pause")
	publisher := flag.String("publisher", os.Getenv("TASKS_RELEASE_PUBLISHER"), "publisher identity recorded in audit")
	flag.Parse()
	if *apk == "" && *pauseID == "" {
		fail(" -apk or -pause is required")
	}
	dsn := os.Getenv("TASKS_DATABASE_URL")
	if dsn == "" {
		fail("TASKS_DATABASE_URL is required")
	}
	if db.SafeDatabaseName(dsn) != nil {
		fail("unsafe Tasks database")
	}
	key := loadKey(os.Getenv("TASKS_RELEASE_SIGNING_KEY"))
	if len(key) != ed25519.PrivateKeySize {
		fail("TASKS_RELEASE_SIGNING_KEY must be a 64-byte ed25519 private key (hex or base64url)")
	}
	dir := os.Getenv("TASKS_RELEASE_ARTIFACT_DIR")
	if dir == "" {
		fail("TASKS_RELEASE_ARTIFACT_DIR is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fail("database configuration invalid")
	}
	defer pool.Close()
	svc := &devicemanagement.Service{DB: pool, ArtifactDir: dir, ReleaseSigningKey: key}
	var rel devicemanagement.Release
	if *pauseID != "" {
		rel, err = svc.PauseRelease(ctx, strings.TrimSpace(*publisher), *pauseID)
	} else {
		rel, err = svc.PublishAPK(ctx, strings.TrimSpace(*publisher), *apk, *channel)
	}
	if err != nil {
		fail(err.Error())
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rel)
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

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(2) }
