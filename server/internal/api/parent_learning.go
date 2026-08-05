package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/overseer"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/tutor"
)

func parentOp(h huma.API, q repo.Querier, op huma.Operation) huma.Operation {
	op.Security = []map[string][]string{{parentSessionSecurityScheme: {}}}
	op.Middlewares = huma.Middlewares{ParentSessionGuard(h, q)}
	op.Errors = append(op.Errors, http.StatusUnauthorized, http.StatusForbidden)
	return op
}

func registerParentLearning(h huma.API, q repo.Querier, opts Options) {
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

	// Device list (parent diagnostics)
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-student-devices",
		Method:      http.MethodGet,
		Path:        "/student-devices",
		Summary:     "List student devices",
		Description: "Household device diagnostics: name, lastSeenAt, revokedAt. Single-family: all devices.",
		Tags:        []string{"Devices"},
	}), func(ctx context.Context, in *listStudentDevicesInput) (*listOutput[domain.StudentDevice], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		items, err := repo.ListStudentDevices(ctx, q, in.StudentID)
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.StudentDevice]{Body: PageBody[domain.StudentDevice]{
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

	// Parent-visible tutor diagnostics (provider choice, enablement, recent failures).
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "get-student-tutor-status",
		Method:      http.MethodGet,
		Path:        "/students/{id}/tutor-status",
		Summary:     "Tutor diagnostics for a student",
		Description: "Reports whether tutoring is enabled, the configured provider, and recent failure count. Does not expose system prompts.",
		Tags:        []string{"Tutor"},
	}), func(ctx context.Context, in *getInput) (*tutorStatusOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		student, err := repo.Students.Get(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		enabled := opts.TutorEnabled
		if studentTutorDisabled(student.Notes) {
			enabled = false
		}
		provider := opts.TutorProviderName
		if provider == "" {
			provider = "fake"
		}
		failures := 0
		if ps, ok := opts.Tutor.(*tutor.PolicyService); ok {
			failures = ps.RecentFailureCount(student.ID)
			if !ps.Enabled() {
				enabled = false
			}
			if provider == "" {
				provider = ps.ProviderName()
			}
		}
		return &tutorStatusOutput{Body: TutorStatusResponse{
			Enabled:             enabled,
			Provider:            provider,
			RecentFailureCount:  failures,
			StudentNotesDisable: studentTutorDisabled(student.Notes),
		}}, nil
	})

	// Overseer: assign next activity
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "assign-next-activity",
		Method:        http.MethodPost,
		Path:          "/students/{id}/assign-next",
		Summary:       "Assign next activity for a student",
		Description:   "Overseer helper: prefers reinforcement-due mastery, else next unassigned published curriculum activity. Optional slug forces a specific activity.",
		Tags:          []string{"Assignments"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *assignNextInput) (*assignNextOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		by := ed.ID
		opts := overseer.Options{
			Slug:       in.Body.Slug,
			AssignedBy: &by,
			Priority:   0,
			Reason:     "",
		}
		if in.Body.Priority != nil {
			opts.Priority = *in.Body.Priority
		}
		if in.Body.Reason != nil {
			opts.Reason = *in.Body.Reason
		}
		if in.Body.PreferReinforcement != nil {
			opts.PreferReinforcement = in.Body.PreferReinforcement
		}
		res, err := overseer.AssignNext(ctx, q, in.ID, opts)
		if err != nil {
			return nil, MapError(err)
		}
		status := http.StatusCreated
		if !res.Created {
			status = http.StatusOK
		}
		_ = status // huma DefaultStatus is Created; clients accept 200/201
		return &assignNextOutput{Body: AssignNextResponse{
			Assignment: *res.Assignment,
			Reason:     res.Reason,
			Created:    res.Created,
		}}, nil
	})

	// Rotate device credential: revoke current token and issue a fresh pairing code.
	// Parent must re-pair the workstation; plaintext device tokens are never re-issued.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "rotate-student-device-token",
		Method:        http.MethodPost,
		Path:          "/student-devices/{id}/rotate-token",
		Summary:       "Rotate student device credential",
		Description:   "Revokes the device token and returns a new one-time pairing code for the same student. Re-pair is required; the old token stops working immediately.",
		Tags:          []string{"Devices"},
		DefaultStatus: http.StatusOK,
	}), func(ctx context.Context, in *getInput) (*rotateDeviceOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		dev, err := repo.StudentDevices.Get(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		now := time.Now().UTC()
		if dev.RevokedAt == nil {
			if _, err := repo.RevokeStudentDevice(ctx, q, in.ID, now); err != nil {
				return nil, MapError(err)
			}
			// Reload revoked view for response.
			if d2, err := repo.StudentDevices.Get(ctx, q, in.ID); err == nil {
				dev = d2
			}
		}
		createdBy := ed.ID
		code, row, err := repo.CreatePairingCode(ctx, q, dev.StudentID, &createdBy, now)
		if err != nil {
			return nil, MapError(err)
		}
		return &rotateDeviceOutput{Body: RotateDeviceResponse{
			Device:    *dev,
			Code:      code,
			ExpiresAt: row.ExpiresAt,
			PairingID: row.ID,
		}}, nil
	})

	// Cancel assignment
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "cancel-assignment",
		Method:      http.MethodPost,
		Path:        "/assignments/{id}/cancel",
		Summary:     "Cancel an assignment",
		Description: "Sets assignment state to cancelled. Completed work cannot be cancelled.",
		Tags:        []string{"Assignments"},
	}), func(ctx context.Context, in *getInput) (*itemOutput[domain.StudentAssignment], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		asg, err := repo.CancelAssignment(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.StudentAssignment]{Body: *asg}, nil
	})

	// Retry assignment: new row for same revision
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "retry-assignment",
		Method:        http.MethodPost,
		Path:          "/assignments/{id}/retry",
		Summary:       "Retry an assignment",
		Description:   "Creates a new available assignment for the same activity revision. Does not modify the original row; cancel first when replacing open work.",
		Tags:          []string{"Assignments"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *retryAssignmentInput) (*itemOutput[domain.StudentAssignment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		by := ed.ID
		reason := ""
		if in.Body.Reason != nil {
			reason = *in.Body.Reason
		}
		asg, err := repo.RetryAssignment(ctx, q, in.ID, &by, reason)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.StudentAssignment]{Body: *asg}, nil
	})

	// List all assignments (parent diagnostics / SPA)
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-assignments",
		Method:      http.MethodGet,
		Path:        "/assignments",
		Summary:     "List student assignments",
		Tags:        []string{"Assignments"},
	}), func(ctx context.Context, in *ListInput) (*listOutput[domain.StudentAssignment], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		filters, err := ParseFilters(in.Filter)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		page, err := repo.StudentAssignments.List(ctx, q, repo.ListParams{
			Limit: in.Limit, Offset: in.Offset, Search: in.Q, Sort: in.Sort, Dir: repo.SortDir(in.Dir), Filters: filters,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.StudentAssignment]{Body: PageBody[domain.StudentAssignment]{
			Items: page.Items, TotalCount: page.TotalCount, Limit: page.Limit, Offset: page.Offset,
		}}, nil
	})

	// List learning sessions
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-learning-sessions",
		Method:      http.MethodGet,
		Path:        "/learning-sessions",
		Summary:     "List learning sessions",
		Tags:        []string{"Sessions"},
	}), func(ctx context.Context, in *ListInput) (*listOutput[domain.LearningSession], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		filters, err := ParseFilters(in.Filter)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		page, err := repo.LearningSessions.List(ctx, q, repo.ListParams{
			Limit: in.Limit, Offset: in.Offset, Search: in.Q, Sort: in.Sort, Dir: repo.SortDir(in.Dir), Filters: filters,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.LearningSession]{Body: PageBody[domain.LearningSession]{
			Items: page.Items, TotalCount: page.TotalCount, Limit: page.Limit, Offset: page.Offset,
		}}, nil
	})

	// Learning overview for one student
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "get-student-learning-overview",
		Method:      http.MethodGet,
		Path:        "/students/{id}/learning-overview",
		Summary:     "Student learning overview",
		Description: "Aggregate: devices, open assignments, recent sessions, mastery summary, tutor-off flag.",
		Tags:        []string{"Students"},
	}), func(ctx context.Context, in *getInput) (*learningOverviewOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		ov, err := repo.GetStudentLearningOverview(ctx, q, in.ID, 20)
		if err != nil {
			return nil, MapError(err)
		}
		// Attach live tutor status (process-local failure counts).
		tutorStatus := TutorStatusResponse{
			Enabled:             opts.TutorEnabled && !ov.TutorNotesDisable,
			Provider:            opts.TutorProviderName,
			StudentNotesDisable: ov.TutorNotesDisable,
		}
		if tutorStatus.Provider == "" {
			tutorStatus.Provider = "fake"
		}
		if ps, ok := opts.Tutor.(*tutor.PolicyService); ok {
			tutorStatus.RecentFailureCount = ps.RecentFailureCount(in.ID)
			if !ps.Enabled() {
				tutorStatus.Enabled = false
			}
		}
		return &learningOverviewOutput{Body: LearningOverviewResponse{
			Student:           ov.Student,
			Devices:           ov.Devices,
			OpenAssignments:   ov.OpenAssignments,
			RecentSessions:    ov.RecentSessions,
			MasterySummary:    ov.MasterySummary,
			EvidenceStatuses:  ov.EvidenceStatuses,
			Tutor:             tutorStatus,
			TutorNotesDisable: ov.TutorNotesDisable,
		}}, nil
	})

	// Toggle per-student tutor via notes marker
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "set-student-tutor",
		Method:      http.MethodPost,
		Path:        "/students/{id}/tutor",
		Summary:     "Enable or disable tutoring for a student",
		Description: "Toggles the tutor:off marker in student.notes. Does not change deployment-wide tutor config.",
		Tags:        []string{"Tutor"},
	}), func(ctx context.Context, in *setTutorInput) (*itemOutput[domain.Student], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		st, err := repo.SetStudentTutorEnabled(ctx, q, in.ID, in.Body.Enabled)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Student]{Body: *st}, nil
	})

	// Parent correction: supersede mastery evidence
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "supersede-mastery-evidence",
		Method:        http.MethodPost,
		Path:          "/mastery-evidence/{id}/supersede",
		Summary:       "Supersede mastery evidence",
		Description:   "Marks existing evidence with an audited superseded note and inserts replacement evidence. Does not rewrite device event history.",
		Tags:          []string{"Mastery"},
		DefaultStatus: http.StatusOK,
	}), func(ctx context.Context, in *supersedeEvidenceInput) (*supersedeEvidenceOutput, error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		note := ""
		if in.Body.Note != nil {
			note = *in.Body.Note
		}
		orig, rep, err := repo.SupersedeMasteryEvidence(ctx, q, in.ID, note, ed.ID, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &supersedeEvidenceOutput{Body: SupersedeEvidenceResponse{
			Original:    *orig,
			Replacement: *rep,
		}}, nil
	})

	// Household student-client metrics
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "get-student-metrics",
		Method:      http.MethodGet,
		Path:        "/ops/student-metrics",
		Summary:     "Student client metrics",
		Description: "Simple JSON counters for devices, open assignments, active sessions, and completions in the last 24h.",
		Tags:        []string{"Ops"},
	}), func(ctx context.Context, _ *struct{}) (*studentMetricsOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		m, err := repo.GetStudentClientMetrics(ctx, q, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		// Best-effort: sum process-local tutor failures across known students.
		if ps, ok := opts.Tutor.(*tutor.PolicyService); ok {
			page, err := repo.Students.List(ctx, q, repo.ListParams{Limit: 200})
			if err == nil {
				total := 0
				for _, st := range page.Items {
					total += ps.RecentFailureCount(st.ID)
				}
				m.TutorFailuresLast24h = total
			}
		}
		return &studentMetricsOutput{Body: StudentMetricsResponse{
			DevicesActive:        m.DevicesActive,
			DevicesRevoked:       m.DevicesRevoked,
			AssignmentsOpen:      m.AssignmentsOpen,
			SessionsActive:       m.SessionsActive,
			CompletionsLast24h:   m.CompletionsLast24h,
			TutorFailuresLast24h: m.TutorFailuresLast24h,
		}}, nil
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

type listStudentDevicesInput struct {
	StudentID string `query:"studentId,omitempty" doc:"Optional student UUID filter; empty lists all household devices."`
}

type assignNextInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Slug                string  `json:"slug,omitempty" doc:"Force assign latest published revision of this activity slug."`
		PreferReinforcement *bool   `json:"preferReinforcement,omitempty" doc:"Default true when slug is empty."`
		Priority            *int    `json:"priority,omitempty"`
		Reason              *string `json:"reason,omitempty"`
	}
}

// AssignNextResponse is the overseer assign-next result.
type AssignNextResponse struct {
	Assignment domain.StudentAssignment `json:"assignment"`
	Reason     string                   `json:"reason"`
	Created    bool                     `json:"created"`
}

type assignNextOutput struct {
	Body AssignNextResponse
}

// TutorStatusResponse is parent-visible tutor diagnostics.
type TutorStatusResponse struct {
	Enabled             bool   `json:"enabled"`
	Provider            string `json:"provider"`
	RecentFailureCount  int    `json:"recentFailureCount"`
	StudentNotesDisable bool   `json:"studentNotesDisable,omitempty" doc:"True when student.notes contains tutor:off."`
}

type tutorStatusOutput struct {
	Body TutorStatusResponse
}

// RotateDeviceResponse is returned after revoke + new pairing code.
type RotateDeviceResponse struct {
	Device    domain.StudentDevice `json:"device"`
	Code      string               `json:"code" doc:"One-time pairing code for re-pairing the workstation."`
	ExpiresAt time.Time            `json:"expiresAt"`
	PairingID string               `json:"pairingId" format:"uuid"`
}

type rotateDeviceOutput struct {
	Body RotateDeviceResponse
}

type retryAssignmentInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Reason *string `json:"reason,omitempty"`
	}
}

// LearningOverviewResponse is the parent dashboard aggregate for one student.
type LearningOverviewResponse struct {
	Student           domain.Student                 `json:"student"`
	Devices           []domain.StudentDevice         `json:"devices"`
	OpenAssignments   []domain.StudentAssignment     `json:"openAssignments"`
	RecentSessions    []domain.LearningSession       `json:"recentSessions"`
	MasterySummary    []domain.MasteryRecord         `json:"masterySummary"`
	EvidenceStatuses  []repo.StandardEvidenceStatus  `json:"evidenceStatuses"`
	Tutor             TutorStatusResponse            `json:"tutor"`
	TutorNotesDisable bool                           `json:"tutorNotesDisable"`
}

type learningOverviewOutput struct {
	Body LearningOverviewResponse
}

type setTutorInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

type supersedeEvidenceInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Note *string `json:"note,omitempty" doc:"Parent correction note stored on replacement evidence."`
	}
}

// SupersedeEvidenceResponse returns the marked original and the new evidence row.
type SupersedeEvidenceResponse struct {
	Original    domain.MasteryEvidence `json:"original"`
	Replacement domain.MasteryEvidence `json:"replacement"`
}

type supersedeEvidenceOutput struct {
	Body SupersedeEvidenceResponse
}

// StudentMetricsResponse is GET /ops/student-metrics.
type StudentMetricsResponse struct {
	DevicesActive        int `json:"devicesActive"`
	DevicesRevoked       int `json:"devicesRevoked"`
	AssignmentsOpen      int `json:"assignmentsOpen"`
	SessionsActive       int `json:"sessionsActive"`
	CompletionsLast24h   int `json:"completionsLast24h"`
	TutorFailuresLast24h int `json:"tutorFailuresLast24h"`
}

type studentMetricsOutput struct {
	Body StudentMetricsResponse
}
