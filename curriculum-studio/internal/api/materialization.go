package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/fingerprint"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
)

const (
	materializationIDPrefix = "mat_"
	itemIDPrefix            = "mit_"
	profileIDPrefix         = "lpr_"
)

type createLearnerProfileInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        struct {
		Kind      string         `json:"kind" enum:"learner,class"`
		Label     string         `json:"label"`
		GradeBand string         `json:"gradeBand,omitempty"`
		Profile   map[string]any `json:"profile,omitempty"`
	}
}
type listLearnerProfilesInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type learnerProfileView struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	Kind        string          `json:"kind"`
	Label       string          `json:"label"`
	GradeBand   string          `json:"gradeBand,omitempty"`
	Profile     json.RawMessage `json:"profile"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
}
type learnerProfileOutput struct{ Body learnerProfileView }
type learnerProfilePageOutput struct {
	Body struct {
		Items      []learnerProfileView `json:"items"`
		TotalCount int                  `json:"totalCount"`
		Limit      int                  `json:"limit"`
		Offset     int                  `json:"offset"`
	}
}

func encodeMaterializationID(id uuid.UUID) string { return materializationIDPrefix + compactUUID(id) }
func encodeItemID(id uuid.UUID) string            { return itemIDPrefix + compactUUID(id) }
func encodeProfileID(id uuid.UUID) string         { return profileIDPrefix + compactUUID(id) }

func decodeMaterializationID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, materializationIDPrefix, "materialization")
}
func decodeItemID(raw string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, itemIDPrefix, "materialized item")
}

func (s *Server) runForCaller(ctx context.Context, raw string) (*domain.MaterializationRun, uuid.UUID, MembershipView, error) {
	id, err := decodeMaterializationID(raw)
	if err != nil {
		return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
	}
	for _, membership := range MembershipsFromContext(ctx) {
		v, e := repo.NewMaterializationRunRepo(s.querier).Get(ctx, membership.WorkspaceID, id)
		if e == nil {
			return v, membership.WorkspaceID, membership, nil
		}
	}
	return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
}

func (s *Server) itemForCaller(ctx context.Context, raw string) (*domain.MaterializedItem, uuid.UUID, MembershipView, error) {
	id, err := decodeItemID(raw)
	if err != nil {
		return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
	}
	for _, membership := range MembershipsFromContext(ctx) {
		v, e := repo.NewMaterializedItemRepo(s.querier).Get(ctx, membership.WorkspaceID, id)
		if e == nil {
			return v, membership.WorkspaceID, membership, nil
		}
	}
	return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
}

func materializationError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, repo.ErrLocked):
		return huma.Error409Conflict("resource locked")
	case errors.Is(err, repo.ErrConflict):
		return huma.Error409Conflict("conflict")
	case errors.Is(err, repo.ErrInvalidTransition), errors.Is(err, repo.ErrImmutable):
		return huma.Error409Conflict("invalid materialization state")
	case errors.Is(err, repo.ErrCheckViolation), errors.Is(err, repo.ErrForeignKey):
		return huma.Error400BadRequest("invalid materialization")
	default:
		return huma.Error503ServiceUnavailable("materialization operation failed")
	}
}

func materializationView(v *domain.MaterializationRun) Materialization {
	return Materialization{
		ID:                 encodeMaterializationID(v.ID),
		PlanRevisionID:     revisionID(v.PlanRevisionID),
		ContextFingerprint: v.InputFingerprint,
		Status:             MaterializationStatus(v.Status),
		CreatedAt:          v.CreatedAt,
		CompletedAt:        v.CompletedAt,
	}
}

func itemBodyString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
}

func itemView(v *domain.MaterializedItem, editor string) MaterializedItem {
	lock := ItemLockState("editable")
	if v.Locked {
		lock = "locked"
	}
	out := MaterializedItem{
		ID:                encodeItemID(v.ID),
		MaterializationID: encodeMaterializationID(v.RunID),
		Kind:              MaterializedItemKind(v.Kind),
		Title:             v.Title,
		Body:              itemBodyString(v.Body),
		Status:            MaterializedItemStatus(v.Status),
		LockState:         lock,
		EditedBy:          editor,
	}
	if v.OutcomeID != nil {
		out.PlanNodeID = v.OutcomeID.String()
	}
	if v.SupersedesItemID != nil {
		out.SupersedesItemID = encodeItemID(*v.SupersedesItemID)
	}
	return out
}

func latestEdit(ctx context.Context, q repo.Querier, workspaceID, itemID uuid.UUID) string {
	edits, err := repo.NewMaterializedItemRepo(q).ListEdits(ctx, workspaceID, itemID)
	if err != nil || len(edits) == 0 {
		return ""
	}
	return edits[len(edits)-1].EditorSubjectRef
}

func profileView(v *domain.LearnerProfile) learnerProfileView {
	return learnerProfileView{
		ID:          encodeProfileID(v.ID),
		WorkspaceID: encodeWSID(v.WorkspaceID),
		Kind:        v.Kind,
		Label:       v.Label,
		GradeBand:   v.GradeBand,
		Profile:     v.Profile,
		CreatedAt:   v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   v.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func canonicalSnapshot(revisionID uuid.UUID, profileID *uuid.UUID, req AuthoringMaterializeRequest) (json.RawMessage, string, error) {
	payload := map[string]any{
		"planRevisionId": revisionID.String(),
		"window": map[string]any{
			"start":            req.Window.Start,
			"end":              req.Window.End,
			"availableMinutes": req.Window.AvailableMinutes,
		},
	}
	if profileID != nil {
		payload["learnerProfileId"] = profileID.String()
	}
	if req.Learner != nil {
		payload["learner"] = map[string]any{
			"label":          req.Learner.Label,
			"grade":          req.Learner.Grade,
			"accommodations": req.Learner.Accommodations,
		}
	}
	if req.Policy != nil {
		payload["policy"] = map[string]any{
			"voice":             req.Policy.Voice,
			"includeAnswerKeys": req.Policy.IncludeAnswerKeys,
			"includePrintables": req.Policy.IncludePrintables,
			"maxItems":          req.Policy.MaxItems,
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	canonical, err := fingerprint.CanonicalJSON(raw)
	if err != nil {
		return nil, "", err
	}
	fp, err := fingerprint.Hash(canonical)
	if err != nil {
		return nil, "", err
	}
	return canonical, fp, nil
}

func requestHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Server) registerMaterializationRoutes(api huma.API) {
	s.registerLearnerProfileRoutes(api)

	huma.Register(api, huma.Operation{OperationID: "list-materializations", Method: http.MethodGet, Path: "/studio/v1/revisions/{revisionId}/materializations", Tags: []string{"Materializations"}, Summary: "List materializations"}, func(ctx context.Context, in *materializationListInput) (*materializationPageResponse, error) {
		rev, ws, _, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, materializationError(err)
		}
		rows, err := repo.NewMaterializationRunRepo(s.querier).ListByRevision(ctx, ws, rev.ID)
		if err != nil {
			return nil, materializationError(err)
		}
		limit, offset := pageBounds(in.Limit, in.Offset)
		page := MaterializationPage{Items: []Materialization{}, TotalCount: len(rows), Limit: limit, Offset: offset}
		for i, row := range rows {
			if i < offset || len(page.Items) >= limit {
				continue
			}
			page.Items = append(page.Items, materializationView(&row))
		}
		return &materializationPageResponse{Body: page}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "create-materialization", Method: http.MethodPost, Path: "/studio/v1/revisions/{revisionId}/materializations", Tags: []string{"Materializations"}, Summary: "Start an authoring materialization", DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createMaterializationInput) (*materializationResponse, error) {
		rev, ws, membership, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, materializationError(err)
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		if rev.Status != "published" {
			return nil, huma.Error409Conflict("revision is not published")
		}
		principal, _ := AuthFromContext(ctx)
		snapshot, fp, err := canonicalSnapshot(rev.ID, nil, in.Body)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid materialization snapshot")
		}
		var created *domain.MaterializationRun
		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			inserted := true
			if key := strings.TrimSpace(in.Headers.IdempotencyKey); key != "" {
				rec, e := repo.NewIdempotencyRepo(q).Begin(ctx, ws, "materialization.create", key, requestHash(snapshot))
				if e != nil {
					return e
				}
				if rec.ResponseRef != "" {
					id, e := uuid.Parse(rec.ResponseRef)
					if e != nil {
						return e
					}
					created, e = repo.NewMaterializationRunRepo(q).Get(ctx, ws, id)
					return e
				}
			}
			before, e := repo.NewMaterializationRunRepo(q).ListByRevision(ctx, ws, rev.ID)
			if e != nil {
				return e
			}
			existing := map[uuid.UUID]struct{}{}
			for _, row := range before {
				existing[row.ID] = struct{}{}
			}
			created, err = repo.NewMaterializationRunRepo(q).CreateRunIdempotent(ctx, &domain.MaterializationRun{
				WorkspaceID:           ws,
				PlanRevisionID:        rev.ID,
				WindowStart:           in.Body.Window.Start,
				WindowEnd:             in.Body.Window.End,
				Status:                domain.MaterializationStatusRequested,
				InputSnapshot:         snapshot,
				InputFingerprint:      fp,
				RequestedBySubjectRef: principal.SubjectRef,
			})
			if err != nil {
				return err
			}
			if _, seen := existing[created.ID]; seen {
				inserted = false
			}
			if inserted {
				if _, e := repo.NewOutboxRepo(q).Enqueue(ctx, &domain.OutboxEvent{
					WorkspaceID:   &ws,
					EventType:     domain.EventMaterializationRequested,
					AggregateKind: "materialization_run",
					AggregateID:   created.ID,
					Payload:       json.RawMessage(`{"version":1}`),
				}); e != nil {
					return e
				}
				if s.matStub && created.Status == domain.MaterializationStatusRequested {
					if _, e := repo.NewMaterializationRunRepo(q).Start(ctx, ws, created.ID); e != nil {
						return e
					}
					if _, e := repo.NewMaterializationRunRepo(q).Ready(ctx, ws, created.ID); e != nil {
						return e
					}
					ready, e := repo.NewMaterializationRunRepo(q).Get(ctx, ws, created.ID)
					if e != nil {
						return e
					}
					created = ready
				}
			}
			if key := strings.TrimSpace(in.Headers.IdempotencyKey); key != "" {
				if _, e := repo.NewIdempotencyRepo(q).SetResponse(ctx, ws, "materialization.create", key, created.ID.String()); e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return nil, materializationError(err)
		}
		return &materializationResponse{Body: materializationView(created)}, nil
	})

	// Retrying deliberately transitions only the run back to running. The worker
	// claims the persisted failed stage; succeeded checkpoints and items remain.
	huma.Register(api, huma.Operation{OperationID: "retry-materialization", Method: http.MethodPost, Path: "/studio/v1/materializations/{materializationId}/retry", Tags: []string{"Materializations"}, Summary: "Retry failed materialization stage"}, func(ctx context.Context, in *materializationPath) (*materializationResponse, error) {
		run, ws, membership, err := s.runForCaller(ctx, in.MaterializationID)
		if err != nil {
			return nil, materializationError(err)
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		if run.Status != domain.MaterializationStatusFailed {
			return nil, huma.Error409Conflict("materialization is not failed")
		}
		retried, err := repo.NewMaterializationRunRepo(s.querier).Start(ctx, ws, run.ID)
		if err != nil {
			return nil, materializationError(err)
		}
		return &materializationResponse{Body: materializationView(retried)}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "get-materialization", Method: http.MethodGet, Path: "/studio/v1/materializations/{materializationId}", Tags: []string{"Materializations"}, Summary: "Get materialization status"}, func(ctx context.Context, in *materializationPath) (*materializationResponse, error) {
		run, _, _, err := s.runForCaller(ctx, in.MaterializationID)
		if err != nil {
			return nil, materializationError(err)
		}
		return &materializationResponse{Body: materializationView(run)}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "get-materialization-bundle", Method: http.MethodGet, Path: "/studio/v1/materializations/{materializationId}/bundle", Tags: []string{"Materializations"}, Summary: "Get an authoring bundle summary"}, func(ctx context.Context, in *materializationPath) (*bundleResponse, error) {
		run, ws, _, err := s.runForCaller(ctx, in.MaterializationID)
		if err != nil {
			return nil, materializationError(err)
		}
		items, err := repo.NewMaterializedItemRepo(s.querier).ListByRun(ctx, ws, run.ID)
		if err != nil {
			return nil, materializationError(err)
		}
		return &bundleResponse{Body: AuthoringBundle{
			ID:                 encodeMaterializationID(run.ID),
			PlanRevisionID:     revisionID(run.PlanRevisionID),
			ContextFingerprint: run.InputFingerprint,
			ItemCount:          len(items),
		}}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "list-materialized-items", Method: http.MethodGet, Path: "/studio/v1/materializations/{materializationId}/items", Tags: []string{"Items"}, Summary: "List materialized items"}, func(ctx context.Context, in *itemListInput) (*itemPageResponse, error) {
		run, ws, _, err := s.runForCaller(ctx, in.MaterializationID)
		if err != nil {
			return nil, materializationError(err)
		}
		kind, status := "", ""
		if q := strings.TrimSpace(in.Q); strings.HasPrefix(q, "kind:") {
			kind = strings.TrimPrefix(q, "kind:")
		}
		rows, err := repo.NewMaterializedItemRepo(s.querier).ListByRunFiltered(ctx, ws, run.ID, kind, status)
		if err != nil {
			return nil, materializationError(err)
		}
		limit, offset := pageBounds(in.Limit, in.Offset)
		page := MaterializedItemPage{Items: []MaterializedItem{}, TotalCount: len(rows), Limit: limit, Offset: offset}
		for i, row := range rows {
			if i < offset || len(page.Items) >= limit {
				continue
			}
			page.Items = append(page.Items, itemView(&row, latestEdit(ctx, s.querier, ws, row.ID)))
		}
		return &itemPageResponse{Body: page}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "get-materialized-item", Method: http.MethodGet, Path: "/studio/v1/materialized-items/{itemId}", Tags: []string{"Items"}, Summary: "Get a materialized item"}, func(ctx context.Context, in *itemPath) (*itemResponse, error) {
		item, ws, _, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, materializationError(err)
		}
		return &itemResponse{Body: itemView(item, latestEdit(ctx, s.querier, ws, item.ID))}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "update-materialized-item", Method: http.MethodPatch, Path: "/studio/v1/materialized-items/{itemId}", Tags: []string{"Items"}, Summary: "Update a materialized item"}, func(ctx context.Context, in *updateItemInput) (*itemResponse, error) {
		item, ws, membership, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, materializationError(err)
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		title := item.Title
		if in.Body.Title != nil {
			title = *in.Body.Title
		}
		body := item.Body
		if in.Body.Body != nil {
			encoded, e := json.Marshal(*in.Body.Body)
			if e != nil {
				return nil, huma.Error400BadRequest("invalid item body")
			}
			body = encoded
		}
		principal, _ := AuthFromContext(ctx)
		updated, err := repo.NewMaterializedItemRepo(s.querier).UpdateContent(ctx, ws, item.ID, title, body, principal.SubjectRef)
		if err != nil {
			return nil, materializationError(err)
		}
		return &itemResponse{Body: itemView(updated, principal.SubjectRef)}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "lock-materialized-item", Method: http.MethodPost, Path: "/studio/v1/materialized-items/{itemId}/lock", Tags: []string{"Items"}, Summary: "Lock a materialized item"}, func(ctx context.Context, in *itemActionInput) (*itemResponse, error) {
		item, ws, membership, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, materializationError(err)
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		principal, _ := AuthFromContext(ctx)
		locked, err := repo.NewMaterializedItemRepo(s.querier).Lock(ctx, ws, item.ID, principal.SubjectRef)
		if err != nil {
			return nil, materializationError(err)
		}
		wsID := ws
		_, _ = repo.NewAuditRepo(s.querier).Insert(ctx, &repo.AuditEvent{
			WorkspaceID:     &wsID,
			ActorSubjectRef: principal.SubjectRef,
			Action:          "materialized_item.lock",
			EntityKind:      "materialized_item",
			EntityID:        &item.ID,
			After:           json.RawMessage(`{"locked":true}`),
		})
		return &itemResponse{Body: itemView(locked, principal.SubjectRef)}, nil
	})

	huma.Register(api, huma.Operation{OperationID: "unlock-materialized-item", Method: http.MethodPost, Path: "/studio/v1/materialized-items/{itemId}/unlock", Tags: []string{"Items"}, Summary: "Unlock a materialized item"}, func(ctx context.Context, in *itemActionInput) (*itemResponse, error) {
		item, ws, membership, err := s.itemForCaller(ctx, in.ItemID)
		if err != nil {
			return nil, materializationError(err)
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		principal, _ := AuthFromContext(ctx)
		unlocked, err := repo.NewMaterializedItemRepo(s.querier).Unlock(ctx, ws, item.ID)
		if err != nil {
			return nil, materializationError(err)
		}
		wsID := ws
		_, _ = repo.NewAuditRepo(s.querier).Insert(ctx, &repo.AuditEvent{
			WorkspaceID:     &wsID,
			ActorSubjectRef: principal.SubjectRef,
			Action:          "materialized_item.unlock",
			EntityKind:      "materialized_item",
			EntityID:        &item.ID,
			After:           json.RawMessage(`{"locked":false}`),
		})
		return &itemResponse{Body: itemView(unlocked, principal.SubjectRef)}, nil
	})
}

func (s *Server) registerLearnerProfileRoutes(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listLearnerProfiles", Method: http.MethodGet, Path: "/studio/v1/workspaces/{workspaceId}/learner-profiles", Tags: []string{"Profiles"}, Summary: "List learner and class profiles"}, func(ctx context.Context, in *listLearnerProfilesInput) (*learnerProfilePageOutput, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		rows, err := repo.NewLearnerProfileRepo(s.querier).ListByWorkspace(ctx, ws)
		if err != nil {
			return nil, materializationError(err)
		}
		limit, offset := pageBounds(in.Limit, in.Offset)
		out := &learnerProfilePageOutput{}
		out.Body.Items = []learnerProfileView{}
		out.Body.TotalCount = len(rows)
		out.Body.Limit = limit
		out.Body.Offset = offset
		for i, row := range rows {
			if i < offset || len(out.Body.Items) >= limit {
				continue
			}
			out.Body.Items = append(out.Body.Items, profileView(&row))
		}
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "createLearnerProfile", Method: http.MethodPost, Path: "/studio/v1/workspaces/{workspaceId}/learner-profiles", Tags: []string{"Profiles"}, Summary: "Create a learner or class profile", DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createLearnerProfileInput) (*learnerProfileOutput, error) {
		ws, membership, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if err := requireAuthor(ctx, membership); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Body.Label) == "" {
			return nil, huma.Error400BadRequest("label is required")
		}
		kind := in.Body.Kind
		if kind == "" {
			kind = domain.ProfileKindLearner
		}
		if kind != domain.ProfileKindLearner && kind != domain.ProfileKindClass {
			return nil, huma.Error400BadRequest("kind must be learner or class")
		}
		raw := json.RawMessage(`{}`)
		if in.Body.Profile != nil {
			encoded, e := json.Marshal(in.Body.Profile)
			if e != nil {
				return nil, huma.Error400BadRequest("profile must be a JSON object")
			}
			raw = encoded
		}
		created, err := repo.NewLearnerProfileRepo(s.querier).Create(ctx, &domain.LearnerProfile{
			WorkspaceID: ws,
			Kind:        kind,
			Label:       strings.TrimSpace(in.Body.Label),
			GradeBand:   in.Body.GradeBand,
			Profile:     raw,
		})
		if err != nil {
			return nil, materializationError(err)
		}
		return &learnerProfileOutput{Body: profileView(created)}, nil
	})
}

func pageBounds(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
