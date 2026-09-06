package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aleksclark/primer/curriculum-studio/internal/artifacts"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/validation"
	"github.com/google/uuid"
)

// Service writes artifact and provenance bytes before transitioning requested
// to ready. It is synchronous; an interrupted request can leave requested rows
// or orphan objects, never a ready row preceding a successful Put.
type Service struct {
	Q     repo.Querier
	Store artifacts.Store
}
type Request struct {
	WorkspaceID, RevisionID uuid.UUID
	RunID                   *uuid.UUID
	Format, CreatedBy       string
}

func (s Service) Create(ctx context.Context, in Request) (*domain.Export, error) {
	ext, contentType, err := FormatInfo(in.Format)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateSubjectRef(in.CreatedBy, ""); err != nil {
		return nil, err
	}
	content := Content{}
	// Read workspace-scoped source data and reject unrelated run provenance.
	// Published graphs are immutable; draft exports reflect the current reads.
	err = repo.WithTx(ctx, s.Q, func(q repo.Querier) error {
		var err error
		content.Graph, err = repo.NewPlanGraphRepo(q).Load(ctx, in.WorkspaceID, in.RevisionID)
		if err != nil {
			return err
		}
		if in.RunID != nil {
			run, err := repo.NewMaterializationRunRepo(q).Get(ctx, in.WorkspaceID, *in.RunID)
			if err != nil {
				return err
			}
			if run.PlanRevisionID != in.RevisionID {
				return repo.ErrRunRevisionMismatch
			}
			if run.Status != domain.MaterializationStatusReady {
				return repo.ErrInvalidTransition
			}
			content.Items, err = repo.NewMaterializedItemRepo(q).ListByRun(ctx, in.WorkspaceID, *in.RunID)
			if err != nil {
				return err
			}
		}
		if in.Format == domain.ExportFormatCSV {
			result := validation.Run(content.Graph)
			content.Findings = result.Findings
			content.Report, err = repo.NewValidationReportRepo(q).CreateReportWithFindings(ctx, in.WorkspaceID, &domain.ValidationReport{PlanRevisionID: in.RevisionID, Status: result.Status, Summary: json.RawMessage(`{"source":"export"}`)}, result.Findings)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	exports := repo.NewExportRepo(s.Q)
	job, err := exports.Create(ctx, &domain.Export{WorkspaceID: in.WorkspaceID, PlanRevisionID: &in.RevisionID, RunID: in.RunID, Format: in.Format, RequestedBySubjectRef: in.CreatedBy})
	if err != nil {
		return nil, err
	}
	key := "exports/" + in.WorkspaceID.String() + "/" + job.ID.String() + "." + ext
	content.Manifest = Manifest{ExportID: job.ID, PlanRevisionID: in.RevisionID, MaterializationRunID: in.RunID, CreatedBy: in.CreatedBy, CreatedAt: job.CreatedAt, Format: in.Format, ContentType: contentType}
	fail := func(cause error) (*domain.Export, error) {
		// Persist failure even if the client disconnects. Cleanup is best-effort but
		// its errors remain visible to the caller/logs, not silently discarded.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if s.Store != nil {
			cause = errors.Join(cause, s.Store.Delete(cleanupCtx, key), s.Store.Delete(cleanupCtx, key+".manifest.json"))
		}
		failed, ferr := exports.Fail(cleanupCtx, in.WorkspaceID, job.ID)
		if ferr != nil {
			return job, errors.Join(cause, ferr)
		}
		return failed, cause
	}
	data, err := Render(in.Format, content)
	if err != nil {
		return fail(err)
	}
	sum := sha256.Sum256(data)
	content.Manifest.Checksum = hex.EncodeToString(sum[:])
	manifest, err := json.MarshalIndent(content.Manifest, "", "  ")
	if err != nil {
		return fail(err)
	}
	if s.Store == nil {
		return fail(fmt.Errorf("artifact store is not configured"))
	}
	if err = s.Store.Put(ctx, key, data, contentType); err != nil {
		return fail(fmt.Errorf("store artifact: %w", err))
	}
	if err = s.Store.Put(ctx, key+".manifest.json", manifest, "application/json"); err != nil {
		return fail(fmt.Errorf("store provenance: %w", err))
	}
	ready, err := exports.Complete(ctx, in.WorkspaceID, job.ID, "obj:"+key, content.Manifest.Checksum)
	if err != nil {
		// Completion may have committed despite a lost response. Do not delete the
		// object on an ambiguous DB error: it might already be referenced by ready.
		return job, fmt.Errorf("complete export metadata: %w", err)
	}
	return ready, nil
}
