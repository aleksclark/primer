package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// artifactSelect is the full column list for learning_session_artifacts.
const artifactSelect = `
id, session_id, artifact_id, filename, media_type, byte_size, sha256, storage_path,
status, student_id, activity_revision_id, bytes_stored, retention_until, created_at, updated_at`

// GetSessionArtifactByClientID returns an artifact by client-supplied artifact_id.
func GetSessionArtifactByClientID(ctx context.Context, q Querier, artifactID string) (*domain.LearningSessionArtifact, error) {
	sqlStr := `SELECT ` + artifactSelect + ` FROM learning_session_artifacts WHERE artifact_id = $1`
	rows, err := q.Query(ctx, sqlStr, artifactID)
	if err != nil {
		return nil, fmt.Errorf("query artifact: %w", err)
	}
	art, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionArtifact])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan artifact: %w", err)
	}
	return &art, nil
}

// ListSessionArtifacts returns all artifacts for a session.
func ListSessionArtifacts(ctx context.Context, q Querier, sessionID string) ([]domain.LearningSessionArtifact, error) {
	sqlStr := `SELECT ` + artifactSelect + ` FROM learning_session_artifacts WHERE session_id = $1 ORDER BY created_at ASC`
	rows, err := q.Query(ctx, sqlStr, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.LearningSessionArtifact])
	if err != nil {
		return nil, fmt.Errorf("scan artifacts: %w", err)
	}
	return items, nil
}

// SessionArtifactUsage returns file count and total bytes for quota checks.
func SessionArtifactUsage(ctx context.Context, q Querier, sessionID string) (files int, totalBytes int64, err error) {
	err = q.QueryRow(ctx, `
SELECT COUNT(*)::int, COALESCE(SUM(byte_size), 0)::bigint
FROM learning_session_artifacts
WHERE session_id = $1 AND status IN ('reserved', 'uploaded', 'metadata_only')`, sessionID).
		Scan(&files, &totalBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("artifact usage: %w", err)
	}
	return files, totalBytes, nil
}

// StudentArtifactBytes returns total uploaded bytes for a student (soft global quota).
func StudentArtifactBytes(ctx context.Context, q Querier, studentID string) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `
SELECT COALESCE(SUM(byte_size), 0)::bigint
FROM learning_session_artifacts
WHERE student_id = $1 AND bytes_stored = true`, studentID).Scan(&n)
	return n, err
}

// ReserveSessionArtifact creates or returns a reserved/uploaded artifact row.
// Idempotent on artifact_id: same meta reuses; conflicting meta returns ErrConflict.
func ReserveSessionArtifact(ctx context.Context, q Querier, sess *domain.LearningSession, meta contracts.ArtifactMeta, status string) (*domain.LearningSessionArtifact, error) {
	if status == "" {
		status = domain.ArtifactStatusReserved
	}
	if existing, err := GetSessionArtifactByClientID(ctx, q, meta.ArtifactID); err == nil {
		if existing.SessionID != sess.ID {
			return nil, ErrConflict
		}
		if existing.SHA256 != meta.SHA256 || existing.ByteSize != meta.ByteSize || existing.Filename != meta.Filename {
			return nil, ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Insert-only on conflict. Never mutate an existing row: a concurrent
	// reservation for the same artifact_id must re-read and validate identity
	// rather than overwriting filename/session fields (TOCTOU race).
	const sqlStr = `
INSERT INTO learning_session_artifacts
    (session_id, artifact_id, filename, media_type, byte_size, sha256, status,
     student_id, activity_revision_id, bytes_stored, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, now())
ON CONFLICT (artifact_id) DO NOTHING
RETURNING ` + artifactSelect
	rows, err := q.Query(ctx, sqlStr,
		sess.ID, meta.ArtifactID, meta.Filename, meta.MediaType, meta.ByteSize, meta.SHA256, status,
		sess.StudentID, sess.ActivityRevisionID,
	)
	if err != nil {
		return nil, fmt.Errorf("reserve artifact: %w", err)
	}
	art, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionArtifact])
	if err == nil {
		return &art, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("scan reserved artifact: %w", err)
	}

	// Lost the insert race (or lost the pre-check race). Re-read and enforce
	// same-session + same-meta idempotency without writing.
	existing, err := GetSessionArtifactByClientID(ctx, q, meta.ArtifactID)
	if err != nil {
		return nil, err
	}
	if existing.SessionID != sess.ID {
		return nil, ErrConflict
	}
	if existing.SHA256 != meta.SHA256 || existing.ByteSize != meta.ByteSize || existing.Filename != meta.Filename {
		return nil, ErrConflict
	}
	return existing, nil
}

// MarkArtifactUploaded sets storage path and uploaded status after bytes land.
func MarkArtifactUploaded(ctx context.Context, q Querier, artifactID, storagePath string, retainUntil *time.Time) (*domain.LearningSessionArtifact, error) {
	const sqlStr = `
UPDATE learning_session_artifacts
SET storage_path = $2,
    status = $3,
    bytes_stored = true,
    retention_until = $4,
    updated_at = now()
WHERE artifact_id = $1
RETURNING ` + artifactSelect
	rows, err := q.Query(ctx, sqlStr, artifactID, storagePath, domain.ArtifactStatusUploaded, retainUntil)
	if err != nil {
		return nil, fmt.Errorf("mark uploaded: %w", err)
	}
	art, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.LearningSessionArtifact])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan uploaded artifact: %w", err)
	}
	return &art, nil
}

// UpsertSessionArtifact stores artifact metadata idempotently by artifact_id.
// Kept for backward compatibility; prefers metadata_only when no bytes.
func UpsertSessionArtifact(ctx context.Context, q Querier, sessionID string, meta contracts.ArtifactMeta) (*domain.LearningSessionArtifact, error) {
	sess, err := GetSession(ctx, q, sessionID)
	if err != nil {
		return nil, err
	}
	if existing, err := GetSessionArtifactByClientID(ctx, q, meta.ArtifactID); err == nil {
		if existing.SessionID != sessionID {
			return nil, ErrConflict
		}
		// Already have richer status — leave it.
		if existing.BytesStored || existing.Status == domain.ArtifactStatusUploaded || existing.Status == domain.ArtifactStatusReserved {
			return existing, nil
		}
		if existing.SHA256 != meta.SHA256 || existing.ByteSize != meta.ByteSize {
			return nil, ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return ReserveSessionArtifact(ctx, q, sess, meta, domain.ArtifactStatusMetadataOnly)
}

// ArtifactPolicyFromRevision extracts ArtifactPolicy from revision content JSON.
func ArtifactPolicyFromRevision(rev *domain.LearningActivityRevision) (*contracts.ArtifactPolicy, error) {
	if rev == nil {
		return nil, nil
	}
	raw, err := json.Marshal(rev.Content)
	if err != nil {
		return nil, err
	}
	var content contracts.ActivityContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, err
	}
	return content.Artifacts, nil
}

// PromoteArtifactToPortfolio creates a portfolio item from an uploaded artifact.
// destination is portfolio or fixture_bundle.
func PromoteArtifactToPortfolio(ctx context.Context, q Querier, artifactClientID string, educatorID, title, destination string, now time.Time) (*domain.PortfolioItem, *domain.ApprovedFixtureBundle, error) {
	if destination == "" {
		destination = domain.PortfolioDestinationPortfolio
	}
	if destination != domain.PortfolioDestinationPortfolio && destination != domain.PortfolioDestinationFixtureBundle {
		return nil, nil, ErrBadRequest{Msg: "destination must be portfolio or fixture_bundle"}
	}
	art, err := GetSessionArtifactByClientID(ctx, q, artifactClientID)
	if err != nil {
		return nil, nil, err
	}
	if !art.BytesStored || art.Status != domain.ArtifactStatusUploaded {
		return nil, nil, ErrBadRequest{Msg: "artifact bytes must be uploaded before promotion"}
	}
	if art.StudentID == nil || art.ActivityRevisionID == nil {
		return nil, nil, ErrBadRequest{Msg: "artifact missing student or revision provenance"}
	}
	if title == "" {
		title = art.Filename
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	var item *domain.PortfolioItem
	var bundle *domain.ApprovedFixtureBundle
	err = WithTx(ctx, q, func(tx Querier) error {
		// Idempotent: same student+artifact+destination.
		if existing, e := portfolioBySource(ctx, tx, *art.StudentID, art.ArtifactID, destination); e == nil {
			item = existing
			if destination == domain.PortfolioDestinationFixtureBundle {
				b, e2 := bundleBySourceArtifact(ctx, tx, *art.StudentID, art.ArtifactID)
				if e2 == nil {
					bundle = b
				} else if !errors.Is(e2, ErrNotFound) {
					return e2
				}
			}
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}

		prov := map[string]any{
			"sourceArtifactId":   art.ArtifactID,
			"sessionId":          art.SessionID,
			"activityRevisionId": *art.ActivityRevisionID,
			"sha256":             art.SHA256,
			"promotedAt":         now.Format(time.RFC3339Nano),
			"destination":        destination,
		}
		var ed any
		if educatorID != "" {
			ed = educatorID
		}
		const ins = `
INSERT INTO portfolio_items
    (student_id, source_artifact_id, session_id, activity_revision_id, title, media_type,
     byte_size, sha256, storage_path, promoted_by, status, destination, provenance, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'active',$11,$12,$13,$13)
RETURNING id, student_id, source_artifact_id, session_id, activity_revision_id, title, media_type,
          byte_size, sha256, storage_path, promoted_by, status, destination, provenance, created_at, updated_at`
		rows, err := tx.Query(ctx, ins,
			*art.StudentID, art.ArtifactID, art.SessionID, *art.ActivityRevisionID, title, art.MediaType,
			art.ByteSize, art.SHA256, art.StoragePath, ed, destination, prov, now,
		)
		if err != nil {
			return fmt.Errorf("insert portfolio item: %w", err)
		}
		pi, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PortfolioItem])
		if err != nil {
			return fmt.Errorf("scan portfolio item: %w", err)
		}
		item = &pi

		if destination == domain.PortfolioDestinationFixtureBundle {
			b, err := createApprovedBundle(ctx, tx, item, educatorID, now)
			if err != nil {
				return err
			}
			bundle = b
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return item, bundle, nil
}

func portfolioBySource(ctx context.Context, q Querier, studentID, sourceArtifactID, destination string) (*domain.PortfolioItem, error) {
	const sqlStr = `
SELECT id, student_id, source_artifact_id, session_id, activity_revision_id, title, media_type,
       byte_size, sha256, storage_path, promoted_by, status, destination, provenance, created_at, updated_at
FROM portfolio_items
WHERE student_id = $1 AND source_artifact_id = $2 AND destination = $3`
	rows, err := q.Query(ctx, sqlStr, studentID, sourceArtifactID, destination)
	if err != nil {
		return nil, err
	}
	item, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.PortfolioItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func bundleBySourceArtifact(ctx context.Context, q Querier, studentID, sourceArtifactID string) (*domain.ApprovedFixtureBundle, error) {
	const sqlStr = `
SELECT id, student_id, source_portfolio_item_id, source_artifact_id, digest, label, entries,
       storage_root, approved_by, status, provenance, created_at
FROM approved_fixture_bundles
WHERE student_id = $1 AND source_artifact_id = $2 AND status = 'approved'
ORDER BY created_at DESC LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, studentID, sourceArtifactID)
	if err != nil {
		return nil, err
	}
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ApprovedFixtureBundle])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

func createApprovedBundle(ctx context.Context, q Querier, item *domain.PortfolioItem, educatorID string, now time.Time) (*domain.ApprovedFixtureBundle, error) {
	// Single-file bundle entry using original filename.
	entries := []map[string]any{{
		"path":      item.Title,
		"type":      "file",
		"sha256":    item.SHA256,
		"mediaType": item.MediaType,
		"byteSize":  item.ByteSize,
	}}
	// Prefer safe relative filename.
	if item.Title == "" {
		entries[0]["path"] = "artifact.bin"
	}
	prov := map[string]any{
		"portfolioItemId":    item.ID,
		"sourceArtifactId":   item.SourceArtifactID,
		"activityRevisionId": item.ActivityRevisionID,
		"approvedAt":         now.Format(time.RFC3339Nano),
	}
	var ed any
	if educatorID != "" {
		ed = educatorID
	}
	itemID := item.ID
	const sqlStr = `
INSERT INTO approved_fixture_bundles
    (student_id, source_portfolio_item_id, source_artifact_id, digest, label, entries,
     storage_root, approved_by, status, provenance, created_at)
VALUES ($1,$2,$3,$4,$5,$6,'',$7,'approved',$8,$9)
ON CONFLICT (student_id, digest) DO UPDATE SET label = EXCLUDED.label
RETURNING id, student_id, source_portfolio_item_id, source_artifact_id, digest, label, entries,
          storage_root, approved_by, status, provenance, created_at`
	rows, err := q.Query(ctx, sqlStr,
		item.StudentID, itemID, item.SourceArtifactID, item.SHA256, item.Title, entries, ed, prov, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert bundle: %w", err)
	}
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ApprovedFixtureBundle])
	if err != nil {
		return nil, fmt.Errorf("scan bundle: %w", err)
	}
	return &b, nil
}

// SetBundleStorageRoot records where bundle bytes were materialized in the store.
func SetBundleStorageRoot(ctx context.Context, q Querier, bundleID, storageRoot string) error {
	_, err := q.Exec(ctx, `UPDATE approved_fixture_bundles SET storage_root = $2 WHERE id = $1`, bundleID, storageRoot)
	return err
}

// GetApprovedBundle returns a bundle by id.
func GetApprovedBundle(ctx context.Context, q Querier, id string) (*domain.ApprovedFixtureBundle, error) {
	const sqlStr = `
SELECT id, student_id, source_portfolio_item_id, source_artifact_id, digest, label, entries,
       storage_root, approved_by, status, provenance, created_at
FROM approved_fixture_bundles WHERE id = $1`
	rows, err := q.Query(ctx, sqlStr, id)
	if err != nil {
		return nil, err
	}
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.ApprovedFixtureBundle])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

// ListPortfolioItems returns active portfolio items for a student.
func ListPortfolioItems(ctx context.Context, q Querier, studentID string) ([]domain.PortfolioItem, error) {
	const sqlStr = `
SELECT id, student_id, source_artifact_id, session_id, activity_revision_id, title, media_type,
       byte_size, sha256, storage_path, promoted_by, status, destination, provenance, created_at, updated_at
FROM portfolio_items WHERE student_id = $1 AND status = 'active' ORDER BY created_at DESC`
	rows, err := q.Query(ctx, sqlStr, studentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[domain.PortfolioItem])
}

// BindAssignmentContinuity records how an assignment should materialize.
func BindAssignmentContinuity(ctx context.Context, q Querier, assignmentID, studentID, mode string, bundleID *string, educatorID, notes string, now time.Time) (*domain.AssignmentContinuityBinding, error) {
	switch mode {
	case contracts.ContinuityFresh, contracts.ContinuityOptionalPrevious, contracts.ContinuityRequiredProject, contracts.ContinuityPortfolioReview:
	default:
		return nil, ErrBadRequest{Msg: "invalid continuity mode"}
	}
	if mode == contracts.ContinuityRequiredProject && (bundleID == nil || *bundleID == "") {
		return nil, ErrBadRequest{Msg: "required_project continuity needs an approved bundleId"}
	}
	if mode == contracts.ContinuityFresh {
		bundleID = nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	asg, err := StudentAssignments.Get(ctx, q, assignmentID)
	if err != nil {
		return nil, err
	}
	if asg.StudentID != studentID {
		return nil, ErrNotFound
	}
	var ed any
	if educatorID != "" {
		ed = educatorID
	}
	var bid any
	if bundleID != nil && *bundleID != "" {
		// Ensure bundle belongs to student and is approved.
		b, err := GetApprovedBundle(ctx, q, *bundleID)
		if err != nil {
			return nil, err
		}
		if b.StudentID != studentID {
			return nil, ErrBadRequest{Msg: "bundle does not belong to student"}
		}
		if b.Status != "approved" {
			return nil, ErrBadRequest{Msg: "bundle is not approved"}
		}
		bid = *bundleID
	}
	const sqlStr = `
INSERT INTO assignment_continuity_bindings
    (assignment_id, student_id, continuity_mode, bundle_id, decided_by, notes, decided_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (assignment_id) DO UPDATE SET
    continuity_mode = EXCLUDED.continuity_mode,
    bundle_id = EXCLUDED.bundle_id,
    decided_by = EXCLUDED.decided_by,
    notes = EXCLUDED.notes,
    decided_at = EXCLUDED.decided_at
RETURNING id, assignment_id, student_id, continuity_mode, bundle_id, decided_by, notes, decided_at`
	rows, err := q.Query(ctx, sqlStr, assignmentID, studentID, mode, bid, ed, notes, now)
	if err != nil {
		return nil, fmt.Errorf("bind continuity: %w", err)
	}
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.AssignmentContinuityBinding])
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// GetAssignmentContinuity returns the continuity binding for an assignment if any.
func GetAssignmentContinuity(ctx context.Context, q Querier, assignmentID string) (*domain.AssignmentContinuityBinding, error) {
	const sqlStr = `
SELECT id, assignment_id, student_id, continuity_mode, bundle_id, decided_by, notes, decided_at
FROM assignment_continuity_bindings WHERE assignment_id = $1`
	rows, err := q.Query(ctx, sqlStr, assignmentID)
	if err != nil {
		return nil, err
	}
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.AssignmentContinuityBinding])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

// ResolveContinuityForSession decides materialization for a session's assignment.
// Default is fresh when no binding exists.
type ContinuityResolution struct {
	Mode   string
	Bundle *domain.ApprovedFixtureBundle // nil when fresh
}

// ResolveContinuityForAssignment loads binding + bundle for workspace setup.
func ResolveContinuityForAssignment(ctx context.Context, q Querier, assignmentID string) (*ContinuityResolution, error) {
	bind, err := GetAssignmentContinuity(ctx, q, assignmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &ContinuityResolution{Mode: contracts.ContinuityFresh}, nil
		}
		return nil, err
	}
	res := &ContinuityResolution{Mode: bind.ContinuityMode}
	if bind.BundleID != nil && *bind.BundleID != "" {
		b, err := GetApprovedBundle(ctx, q, *bind.BundleID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				if bind.ContinuityMode == contracts.ContinuityRequiredProject {
					return nil, ErrBadRequest{Msg: "required approved bundle is missing"}
				}
				res.Mode = contracts.ContinuityFresh
				return res, nil
			}
			return nil, err
		}
		if b.Status != "approved" {
			if bind.ContinuityMode == contracts.ContinuityRequiredProject {
				return nil, ErrBadRequest{Msg: "required approved bundle was withdrawn"}
			}
			res.Mode = contracts.ContinuityFresh
			return res, nil
		}
		res.Bundle = b
	} else if bind.ContinuityMode == contracts.ContinuityRequiredProject {
		return nil, ErrBadRequest{Msg: "required_project continuity has no bundle"}
	}
	return res, nil
}

// InsertCurriculumImportRun records a plan or apply attempt.
func InsertCurriculumImportRun(ctx context.Context, q Querier, run *domain.CurriculumImportRun) (*domain.CurriculumImportRun, error) {
	const sqlStr = `
INSERT INTO curriculum_import_runs
    (bundle_digest, actor_id, source_label, mode, status, plan, result_manifest, error_message, created_at, applied_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, bundle_digest, actor_id, source_label, mode, status, plan, result_manifest, error_message, created_at, applied_at`
	var actor any
	if run.ActorID != nil {
		actor = *run.ActorID
	}
	if run.Plan == nil {
		run.Plan = map[string]any{}
	}
	if run.ResultManifest == nil {
		run.ResultManifest = map[string]any{}
	}
	rows, err := q.Query(ctx, sqlStr,
		run.BundleDigest, actor, run.SourceLabel, run.Mode, run.Status, run.Plan, run.ResultManifest,
		run.ErrorMessage, run.CreatedAt, run.AppliedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert import run: %w", err)
	}
	out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.CurriculumImportRun])
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// LatestAppliedImportByDigest returns the most recent successful apply for a digest.
func LatestAppliedImportByDigest(ctx context.Context, q Querier, digest string) (*domain.CurriculumImportRun, error) {
	const sqlStr = `
SELECT id, bundle_digest, actor_id, source_label, mode, status, plan, result_manifest, error_message, created_at, applied_at
FROM curriculum_import_runs
WHERE bundle_digest = $1 AND mode = 'apply' AND status = 'applied'
ORDER BY applied_at DESC NULLS LAST, created_at DESC
LIMIT 1`
	rows, err := q.Query(ctx, sqlStr, digest)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[domain.CurriculumImportRun])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}
