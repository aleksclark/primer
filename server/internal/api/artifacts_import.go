package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// --- Artifact upload / promote / continuity ---

type reserveArtifactInput struct {
	ID   string `path:"id" doc:"Session ID"`
	Body contracts.ArtifactMeta
}

type uploadArtifactInput struct {
	ID   string `path:"id" doc:"Session ID"`
	Body struct {
		ArtifactID string `json:"artifactId" doc:"Client artifact UUID from reservation"`
		// ContentBase64 is the file bytes (MVP; multipart can replace later).
		ContentBase64 string `json:"contentBase64"`
		// SHA256 must match reserved meta.
		SHA256 string `json:"sha256,omitempty"`
	}
}

type promoteArtifactInput struct {
	Body struct {
		ArtifactID  string `json:"artifactId" doc:"Client artifact UUID"`
		Title       string `json:"title,omitempty"`
		Destination string `json:"destination,omitempty" enum:"portfolio,fixture_bundle"`
	}
}

type promoteArtifactOutput struct {
	Body struct {
		Item   domain.PortfolioItem          `json:"item"`
		Bundle *domain.ApprovedFixtureBundle `json:"bundle,omitempty"`
	}
}

type listPortfolioInput struct {
	StudentID string `path:"id" doc:"Student ID"`
}

type bindContinuityInput struct {
	ID   string `path:"id" doc:"Assignment ID"`
	Body struct {
		StudentID      string  `json:"studentId"`
		ContinuityMode string  `json:"continuityMode" enum:"fresh,optional_previous,required_project,portfolio_review"`
		BundleID       *string `json:"bundleId,omitempty"`
		Notes          string  `json:"notes,omitempty"`
	}
}

type sessionContinuityOutput struct {
	Body struct {
		Mode   string                        `json:"mode"`
		Bundle *domain.ApprovedFixtureBundle `json:"bundle,omitempty"`
	}
}

type curriculumImportPlanInput struct {
	Body curriculum.ImportBundle
}

type curriculumImportApplyInput struct {
	Body struct {
		Bundle       curriculum.ImportBundle `json:"bundle"`
		BundleDigest string                  `json:"bundleDigest"`
		SourceLabel  string                  `json:"sourceLabel,omitempty"`
	}
}

type curriculumImportPlanOutput struct {
	Body curriculum.ImportPlan
}

type curriculumImportApplyOutput struct {
	Body struct {
		Manifest curriculum.ImportResultManifest `json:"manifest"`
		Run      domain.CurriculumImportRun      `json:"run"`
	}
}

func registerArtifactsAndImport(h huma.API, q repo.Querier, opts Options) {
	// Reserve artifact (metadata + quotas) before bytes.
	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "reserve-session-artifact",
		Method:        http.MethodPost,
		Path:          "/student/sessions/{id}/artifacts/reserve",
		Summary:       "Reserve session artifact upload slot",
		Description:   "Validates activity artifact policy and quotas, then records a reserved artifact row. Idempotent on artifactId.",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *reserveArtifactInput) (*itemOutput[domain.LearningSessionArtifact], error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		sess, err := repo.GetSession(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		if sess.StudentID != dev.StudentID {
			return nil, huma.Error404NotFound("session not found")
		}
		art, err := reserveArtifact(ctx, q, sess, in.Body)
		if err != nil {
			return nil, mapArtifactErr(err)
		}
		return &itemOutput[domain.LearningSessionArtifact]{Body: *art}, nil
	})

	// Upload bytes for a reserved artifact.
	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "upload-session-artifact",
		Method:        http.MethodPost,
		Path:          "/student/sessions/{id}/artifacts/upload",
		Summary:       "Upload reserved artifact bytes",
		Description:   "Verifies digest and size against the reservation and activity policy, then stores bytes under the configured artifact root.",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *uploadArtifactInput) (*itemOutput[domain.LearningSessionArtifact], error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		sess, err := repo.GetSession(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		if sess.StudentID != dev.StudentID {
			return nil, huma.Error404NotFound("session not found")
		}
		store := opts.ArtifactStore
		if store == nil {
			return nil, huma.Error503ServiceUnavailable("artifact storage is not configured")
		}
		art, err := uploadArtifactBytes(ctx, q, store, sess, in.Body.ArtifactID, in.Body.ContentBase64, in.Body.SHA256)
		if err != nil {
			return nil, mapArtifactErr(err)
		}
		return &itemOutput[domain.LearningSessionArtifact]{Body: *art}, nil
	})

	// Continuity resolution for a session (student client materialization).
	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID: "get-session-continuity",
		Method:      http.MethodGet,
		Path:        "/student/sessions/{id}/continuity",
		Summary:     "Resolve workspace continuity for a session",
		Description: "Returns fresh (default) or a parent-approved fixture bundle binding for the session assignment.",
		Tags:        []string{"Student"},
	}), func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*sessionContinuityOutput, error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		sess, err := repo.GetSession(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		if sess.StudentID != dev.StudentID {
			return nil, huma.Error404NotFound("session not found")
		}
		res, err := repo.ResolveContinuityForAssignment(ctx, q, sess.AssignmentID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &sessionContinuityOutput{}
		out.Body.Mode = res.Mode
		out.Body.Bundle = res.Bundle
		return out, nil
	})

	// Parent: promote artifact to portfolio / fixture bundle.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "promote-artifact",
		Method:        http.MethodPost,
		Path:          "/portfolio/promote",
		Summary:       "Promote uploaded artifact to portfolio or fixture bundle",
		Description:   "Parent-only. Records provenance and optionally creates an approved fixture bundle for later continuity.",
		Tags:          []string{"Portfolio"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *promoteArtifactInput) (*promoteArtifactOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		store := opts.ArtifactStore
		item, bundle, err := repo.PromoteArtifactToPortfolio(ctx, q, in.Body.ArtifactID, ed.ID, in.Body.Title, in.Body.Destination, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		// Materialize fixture bundle bytes into store when destination is fixture_bundle.
		if bundle != nil && store != nil {
			if err := materializePromotedBundle(store, item, bundle); err == nil {
				rel, _ := store.BundleRoot(item.StudentID, bundle.Digest)
				_ = repo.SetBundleStorageRoot(ctx, q, bundle.ID, rel)
				bundle.StorageRoot = rel
			}
		}
		out := &promoteArtifactOutput{}
		out.Body.Item = *item
		out.Body.Bundle = bundle
		return out, nil
	})

	// Parent: list portfolio.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-student-portfolio",
		Method:      http.MethodGet,
		Path:        "/students/{id}/portfolio",
		Summary:     "List student portfolio items",
		Tags:        []string{"Portfolio"},
	}), func(ctx context.Context, in *listPortfolioInput) (*listOutput[domain.PortfolioItem], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		items, err := repo.ListPortfolioItems(ctx, q, in.StudentID)
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.PortfolioItem]{Body: PageBody[domain.PortfolioItem]{
			Items: items, TotalCount: len(items), Limit: len(items), Offset: 0,
		}}, nil
	})

	// Parent: bind continuity on an assignment.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "bind-assignment-continuity",
		Method:        http.MethodPost,
		Path:          "/assignments/{id}/continuity",
		Summary:       "Bind assignment continuity policy",
		Description:   "Default is fresh. optional_previous/required_project may reference a parent-approved fixture bundle.",
		Tags:          []string{"Portfolio"},
		DefaultStatus: http.StatusOK,
	}), func(ctx context.Context, in *bindContinuityInput) (*itemOutput[domain.AssignmentContinuityBinding], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		mode := in.Body.ContinuityMode
		if mode == "" {
			mode = contracts.ContinuityFresh
		}
		b, err := repo.BindAssignmentContinuity(ctx, q, in.ID, in.Body.StudentID, mode, in.Body.BundleID, ed.ID, in.Body.Notes, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.AssignmentContinuityBinding]{Body: *b}, nil
	})

	// Curriculum import plan (read-only).
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "curriculum-import-plan",
		Method:      http.MethodPost,
		Path:        "/curriculum/import/plan",
		Summary:     "Plan a guarded curriculum import",
		Description: "Validates the bundle and returns a read-only diff. Does not write standards, activities, courses, enrollments, or assignments.",
		Tags:        []string{"Curriculum Import"},
	}), func(ctx context.Context, in *curriculumImportPlanInput) (*curriculumImportPlanOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		plan, err := curriculum.PlanImport(ctx, q, &in.Body, curriculum.ImportOptions{
			ActorID: ed.ID,
			Now:     time.Now().UTC(),
		})
		if err != nil {
			return nil, MapError(err)
		}
		// Durable plan audit (even when invalid).
		_, _ = repo.InsertCurriculumImportRun(ctx, q, &domain.CurriculumImportRun{
			BundleDigest: plan.BundleDigest,
			ActorID:      &ed.ID,
			SourceLabel:  plan.SourceLabel,
			Mode:         "plan",
			Status:       "planned",
			Plan:         planToMap(plan),
			CreatedAt:    time.Now().UTC(),
		})
		return &curriculumImportPlanOutput{Body: *plan}, nil
	})

	// Curriculum import apply-by-digest.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "curriculum-import-apply",
		Method:        http.MethodPost,
		Path:          "/curriculum/import/apply",
		Summary:       "Apply a curriculum import by planned bundle digest",
		Description:   "Applies standards, activities, and course revisions in one DB transaction. Rejects digest drift. Never enrolls or assigns students.",
		Tags:          []string{"Curriculum Import"},
		DefaultStatus: http.StatusOK,
	}), func(ctx context.Context, in *curriculumImportApplyInput) (*curriculumImportApplyOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		manifest, run, err := curriculum.ApplyImport(ctx, q, &in.Body.Bundle, in.Body.BundleDigest, curriculum.ImportOptions{
			ActorID:     ed.ID,
			SourceLabel: in.Body.SourceLabel,
			Now:         time.Now().UTC(),
		})
		if err != nil {
			return nil, MapError(err)
		}
		out := &curriculumImportApplyOutput{}
		out.Body.Manifest = *manifest
		if run != nil {
			out.Body.Run = *run
		}
		return out, nil
	})
}

func reserveArtifact(ctx context.Context, q repo.Querier, sess *domain.LearningSession, meta contracts.ArtifactMeta) (*domain.LearningSessionArtifact, error) {
	if meta.ArtifactID == "" {
		return nil, repo.ErrBadRequest{Msg: "artifactId is required"}
	}
	name, err := artifacts.SafeFilename(meta.Filename)
	if err != nil {
		return nil, repo.ErrBadRequest{Msg: err.Error()}
	}
	meta.Filename = name
	if meta.SchemaVersion == "" {
		meta.SchemaVersion = contracts.ArtifactSchemaVersion
	}
	rev, err := repo.GetRevision(ctx, q, sess.ActivityRevisionID)
	if err != nil {
		return nil, err
	}
	pol, err := repo.ArtifactPolicyFromRevision(rev)
	if err != nil {
		return nil, err
	}
	// Existing reservation/upload short-circuits quotas.
	if existing, err := repo.GetSessionArtifactByClientID(ctx, q, meta.ArtifactID); err == nil {
		if existing.SessionID != sess.ID {
			return nil, repo.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}
	files, total, err := repo.SessionArtifactUsage(ctx, q, sess.ID)
	if err != nil {
		return nil, err
	}
	if err := artifacts.PolicyCheck(pol, meta.MediaType, meta.ByteSize, files, total); err != nil {
		return nil, repo.ErrBadRequest{Msg: err.Error()}
	}
	if studentTotal, err := repo.StudentArtifactBytes(ctx, q, sess.StudentID); err == nil {
		if studentTotal+meta.ByteSize > artifacts.DefaultMaxStudentBytes {
			return nil, repo.ErrBadRequest{Msg: "student artifact storage quota exceeded"}
		}
	}
	return repo.ReserveSessionArtifact(ctx, q, sess, meta, domain.ArtifactStatusReserved)
}

func uploadArtifactBytes(ctx context.Context, q repo.Querier, store *artifacts.Store, sess *domain.LearningSession, artifactID, contentB64, sha string) (*domain.LearningSessionArtifact, error) {
	if artifactID == "" {
		return nil, repo.ErrBadRequest{Msg: "artifactId is required"}
	}
	art, err := repo.GetSessionArtifactByClientID(ctx, q, artifactID)
	if err != nil {
		return nil, err
	}
	if art.SessionID != sess.ID {
		return nil, repo.ErrNotFound
	}
	if art.BytesStored && art.Status == domain.ArtifactStatusUploaded {
		// Idempotent retry.
		return art, nil
	}
	raw, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		// Also accept raw URL encoding.
		raw, err = base64.URLEncoding.DecodeString(contentB64)
		if err != nil {
			return nil, repo.ErrBadRequest{Msg: "contentBase64 is invalid"}
		}
	}
	if int64(len(raw)) != art.ByteSize {
		return nil, repo.ErrBadRequest{Msg: "uploaded size does not match reservation"}
	}
	digest := art.SHA256
	if sha != "" && !strings.EqualFold(sha, digest) {
		return nil, repo.ErrBadRequest{Msg: "sha256 does not match reservation"}
	}
	// Re-check policy at upload time.
	rev, err := repo.GetRevision(ctx, q, sess.ActivityRevisionID)
	if err != nil {
		return nil, err
	}
	pol, err := repo.ArtifactPolicyFromRevision(rev)
	if err != nil {
		return nil, err
	}
	files, total, err := repo.SessionArtifactUsage(ctx, q, sess.ID)
	if err != nil {
		return nil, err
	}
	// Exclude this reservation from counts for the check.
	if files > 0 {
		files--
	}
	if total >= art.ByteSize {
		total -= art.ByteSize
	}
	if err := artifacts.PolicyCheck(pol, art.MediaType, art.ByteSize, files, total); err != nil {
		return nil, repo.ErrBadRequest{Msg: err.Error()}
	}

	_, _, err = store.PutObject(digest, bytes.NewReader(raw), art.ByteSize)
	if err != nil {
		return nil, repo.ErrBadRequest{Msg: err.Error()}
	}
	rel, err := store.LinkSessionArtifact(sess.ID, art.ArtifactID, digest)
	if err != nil {
		return nil, err
	}
	var retain *time.Time
	if pol != nil && pol.RetainDays > 0 {
		t := time.Now().UTC().Add(time.Duration(pol.RetainDays) * 24 * time.Hour)
		retain = &t
	}
	return repo.MarkArtifactUploaded(ctx, q, art.ArtifactID, rel, retain)
}

func materializePromotedBundle(store *artifacts.Store, item *domain.PortfolioItem, bundle *domain.ApprovedFixtureBundle) error {
	root, err := store.BundleRoot(item.StudentID, bundle.Digest)
	if err != nil {
		return err
	}
	f, err := store.OpenObject(item.SHA256)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	name := item.Title
	if name == "" {
		name = "artifact.bin"
	}
	// Use only the base name for path safety.
	safe, err := artifacts.SafeFilename(name)
	if err != nil {
		safe = "artifact.bin"
	}
	return store.WriteBundleEntry(root, safe, data, 0o644)
}

func mapArtifactErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repo.ErrConflict) {
		return huma.Error409Conflict(err.Error())
	}
	return MapError(err)
}

func planToMap(p *curriculum.ImportPlan) map[string]any {
	if p == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}
