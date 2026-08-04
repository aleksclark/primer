package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/tv/domain"
)

// ContentManifestBySlug finds a manifest entry by its stable curriculum slug.
func ContentManifestBySlug(ctx context.Context, q repo.Querier, slug string) (*domain.ContentManifestEntry, error) {
	sqlStr := fmt.Sprintf(`SELECT %s FROM content_manifest_entries WHERE slug = $1`,
		strings.Join(ContentManifestEntries.Config().Columns, ", "))
	rows, err := q.Query(ctx, sqlStr, slug)
	if err != nil {
		return nil, fmt.Errorf("content manifest by slug: %w", err)
	}
	entry, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ContentManifestEntry])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, err
	}
	return &entry, nil
}

// ManifestFailPolicy is the attempt/day threshold used when recording an
// acquisition attempt against a still-missing entry.
type ManifestFailPolicy struct {
	MaxAttempts int
	MaxDays     int
	Now         time.Time
}

// RecordManifestAttempt increments attempt counters for a missing entry and
// flips it to failed when either threshold is crossed. Present/failed/manual
// entries are left unchanged (idempotent no-op).
func RecordManifestAttempt(ctx context.Context, q repo.Querier, slug string, lastError string, policy ManifestFailPolicy) (*domain.ContentManifestEntry, error) {
	entry, err := ContentManifestBySlug(ctx, q, slug)
	if err != nil {
		return nil, err
	}
	switch entry.Status {
	case domain.ManifestStatusPresent, domain.ManifestStatusFailed, domain.ManifestStatusManual:
		return entry, nil
	}

	now := policy.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	attemptCount := entry.AttemptCount + 1
	first := entry.FirstAttemptAt
	if first == nil {
		first = &now
	}
	values := map[string]any{
		"attempt_count":    attemptCount,
		"first_attempt_at": first,
		"last_attempt_at":  now,
		"last_error":       lastError,
	}
	if shouldFailManifest(attemptCount, *first, now, policy) {
		values["status"] = domain.ManifestStatusFailed
		values["failed_at"] = now
	}
	return ContentManifestEntries.Update(ctx, q, entry.ID, values)
}

// MarkManifestPresent records that the title is available in Jellyfin.
// Re-marking an already-present entry is a no-op (keeps the original present_at).
func MarkManifestPresent(ctx context.Context, q repo.Querier, slug string, at time.Time) (*domain.ContentManifestEntry, error) {
	entry, err := ContentManifestBySlug(ctx, q, slug)
	if err != nil {
		return nil, err
	}
	if entry.Status == domain.ManifestStatusPresent && entry.PresentAt != nil {
		return entry, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	return ContentManifestEntries.Update(ctx, q, entry.ID, map[string]any{
		"status":     domain.ManifestStatusPresent,
		"present_at": at,
		"last_error": "",
		"failed_at":  nil,
	})
}

// UpsertManifestDesired writes the desired-state fields for one slug. Status
// and attempt counters are preserved on update; new rows start missing (or
// manual for DVD-rip kinds).
func UpsertManifestDesired(ctx context.Context, q repo.Querier, values map[string]any) (*domain.ContentManifestEntry, error) {
	slug, _ := values["slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	existing, err := ContentManifestBySlug(ctx, q, slug)
	if err == nil {
		// Never let a bulk desired-state sync clobber acquisition tracking.
		delete(values, "status")
		delete(values, "attempt_count")
		delete(values, "first_attempt_at")
		delete(values, "last_attempt_at")
		delete(values, "present_at")
		delete(values, "failed_at")
		delete(values, "last_error")
		return ContentManifestEntries.Update(ctx, q, existing.ID, values)
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}
	if _, ok := values["status"]; !ok {
		if kind, _ := values["kind"].(string); kind == domain.ManifestKindManual {
			values["status"] = domain.ManifestStatusManual
		} else {
			values["status"] = domain.ManifestStatusMissing
		}
	}
	return ContentManifestEntries.Create(ctx, q, values)
}

func shouldFailManifest(attemptCount int, firstAttempt, now time.Time, policy ManifestFailPolicy) bool {
	if policy.MaxAttempts > 0 && attemptCount >= policy.MaxAttempts {
		return true
	}
	if policy.MaxDays > 0 && !firstAttempt.IsZero() {
		deadline := firstAttempt.Add(time.Duration(policy.MaxDays) * 24 * time.Hour)
		if !now.Before(deadline) {
			return true
		}
	}
	return false
}
