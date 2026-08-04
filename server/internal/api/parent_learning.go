package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func parentOp(h huma.API, q repo.Querier, op huma.Operation) huma.Operation {
	op.Security = []map[string][]string{{parentSessionSecurityScheme: {}}}
	op.Middlewares = huma.Middlewares{ParentSessionGuard(h, q)}
	op.Errors = append(op.Errors, http.StatusUnauthorized, http.StatusForbidden)
	return op
}

func registerParentLearning(h huma.API, q repo.Querier) {
	// Pairing codes
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "create-pairing-code",
		Method:        http.MethodPost,
		Path:          "/pairing-codes",
		Summary:       "Create student device pairing code",
		Description:   "Issues a short-lived one-time pairing code for a student. Single-family: any parent/admin may manage all students.",
		Tags:          []string{"Devices"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *createPairingCodeInput) (*createPairingCodeOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := repo.Students.Get(ctx, q, in.Body.StudentID); err != nil {
			return nil, MapError(err)
		}
		createdBy := ed.ID
		code, row, err := repo.CreatePairingCode(ctx, q, in.Body.StudentID, &createdBy, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &createPairingCodeOutput{Body: PairingCodeResponse{
			Code:      code,
			ExpiresAt: row.ExpiresAt,
			StudentID: row.StudentID,
			ID:        row.ID,
		}}, nil
	})

	// Activities: create draft
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "create-learning-activity",
		Method:        http.MethodPost,
		Path:          "/learning-activities",
		Summary:       "Create draft learning activity",
		Tags:          []string{"Learning Activities"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *createActivityInput) (*itemOutput[domain.LearningActivity], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		act, err := repo.CreateDraftActivity(ctx, q, in.Body.Slug, in.Body.Title, in.Body.Summary, in.Body.Kind, in.Body.SubjectID)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.LearningActivity]{Body: *act}, nil
	})

	// List activities
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-learning-activities",
		Method:      http.MethodGet,
		Path:        "/learning-activities",
		Summary:     "List learning activities",
		Tags:        []string{"Learning Activities"},
	}), func(ctx context.Context, in *ListInput) (*listOutput[domain.LearningActivity], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		filters, err := ParseFilters(in.Filter)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		page, err := repo.LearningActivities.List(ctx, q, repo.ListParams{
			Limit: in.Limit, Offset: in.Offset, Search: in.Q, Sort: in.Sort, Dir: repo.SortDir(in.Dir), Filters: filters,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.LearningActivity]{Body: PageBody[domain.LearningActivity]{
			Items: page.Items, TotalCount: page.TotalCount, Limit: page.Limit, Offset: page.Offset,
		}}, nil
	})

	// Publish revision (content immutable once written)
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "publish-activity-revision",
		Method:        http.MethodPost,
		Path:          "/learning-activities/{id}/revisions",
		Summary:       "Publish immutable activity revision",
		Tags:          []string{"Learning Activities"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *publishRevisionInput) (*itemOutput[domain.LearningActivityRevision], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		codeToID := map[string]string{}
		// Resolve standard codes from DB.
		for _, ref := range in.Body.Standards {
			if _, ok := codeToID[ref.Code]; ok {
				continue
			}
			page, err := repo.Standards.List(ctx, q, repo.ListParams{
				Limit: 1, Filters: map[string]any{"code": ref.Code},
			})
			if err != nil || page.TotalCount == 0 {
				return nil, huma.Error422UnprocessableEntity("unknown standard code: " + ref.Code)
			}
			codeToID[ref.Code] = page.Items[0].ID
		}
		schema := in.Body.SchemaVersion
		if schema == "" {
			schema = contracts.SchemaVersion
		}
		rev, err := repo.PublishDraftRevision(ctx, q, in.ID, in.Body.Content, schema, in.Body.Standards, codeToID, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.LearningActivityRevision]{Body: *rev}, nil
	})

	// List revisions for activity
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-activity-revisions",
		Method:      http.MethodGet,
		Path:        "/learning-activities/{id}/revisions",
		Summary:     "List activity revisions",
		Tags:        []string{"Learning Activities"},
	}), func(ctx context.Context, in *getInput) (*listOutput[domain.LearningActivityRevision], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		page, err := repo.LearningActivityRevisions.List(ctx, q, repo.ListParams{
			Limit: 100, Filters: map[string]any{"activity_id": in.ID}, Sort: "revision", Dir: repo.SortDesc,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.LearningActivityRevision]{Body: PageBody[domain.LearningActivityRevision]{
			Items: page.Items, TotalCount: page.TotalCount, Limit: page.Limit, Offset: page.Offset,
		}}, nil
	})

	// Assignments
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "create-assignment",
		Method:        http.MethodPost,
		Path:          "/assignments",
		Summary:       "Assign activity revision to student",
		Tags:          []string{"Assignments"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *createAssignmentInput) (*itemOutput[domain.StudentAssignment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := repo.Students.Get(ctx, q, in.Body.StudentID); err != nil {
			return nil, MapError(err)
		}
		if _, err := repo.GetRevision(ctx, q, in.Body.ActivityRevisionID); err != nil {
			return nil, MapError(err)
		}
		priority := 0
		if in.Body.Priority != nil {
			priority = *in.Body.Priority
		}
		reason := ""
		if in.Body.Reason != nil {
			reason = *in.Body.Reason
		}
		by := ed.ID
		asg, err := repo.CreateAssignment(ctx, q, in.Body.StudentID, in.Body.ActivityRevisionID, &by, priority, reason)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.StudentAssignment]{Body: *asg}, nil
	})

	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-student-assignments",
		Method:      http.MethodGet,
		Path:        "/students/{id}/assignments",
		Summary:     "List assignments for a student",
		Tags:        []string{"Assignments"},
	}), func(ctx context.Context, in *getInput) (*listOutput[domain.StudentAssignment], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		items, err := repo.ListAssignmentsForStudent(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.StudentAssignment]{Body: PageBody[domain.StudentAssignment]{
			Items: items, TotalCount: len(items), Limit: len(items), Offset: 0,
		}}, nil
	})

	// Device revoke
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "revoke-student-device",
		Method:      http.MethodPost,
		Path:        "/student-devices/{id}/revoke",
		Summary:     "Revoke a student device",
		Tags:        []string{"Devices"},
	}), func(ctx context.Context, in *getInput) (*itemOutput[domain.StudentDevice], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		dev, err := repo.RevokeStudentDevice(ctx, q, in.ID, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.StudentDevice]{Body: *dev}, nil
	})
}

type createPairingCodeInput struct {
	Body struct {
		StudentID string `json:"studentId" format:"uuid"`
	}
}

// PairingCodeResponse returns the plaintext code once.
type PairingCodeResponse struct {
	ID        string    `json:"id" format:"uuid"`
	Code      string    `json:"code" doc:"One-time pairing code; shown only once."`
	StudentID string    `json:"studentId" format:"uuid"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type createPairingCodeOutput struct {
	Body PairingCodeResponse
}

type createActivityInput struct {
	Body struct {
		Slug      string  `json:"slug" minLength:"1"`
		Title     string  `json:"title" minLength:"1"`
		Summary   string  `json:"summary,omitempty"`
		Kind      string  `json:"kind" enum:"terminal,typing"`
		SubjectID *string `json:"subjectId,omitempty" format:"uuid"`
	}
}

type publishRevisionInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		SchemaVersion string                     `json:"schemaVersion,omitempty"`
		Content       contracts.ActivityContent  `json:"content"`
		Standards     []contracts.StandardRef    `json:"standards,omitempty"`
	}
}

type createAssignmentInput struct {
	Body struct {
		StudentID          string  `json:"studentId" format:"uuid"`
		ActivityRevisionID string  `json:"activityRevisionId" format:"uuid"`
		Priority           *int    `json:"priority,omitempty"`
		Reason             *string `json:"reason,omitempty"`
	}
}
