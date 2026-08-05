package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/overseer"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

func registerParentCourse(h huma.API, q repo.Querier) {
	// Publish course document (JSON body) as an immutable curriculum revision.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "publish-course-document",
		Method:        http.MethodPost,
		Path:          "/courses/publish",
		Summary:       "Publish course document as curriculum revision",
		Description:   "Persists an immutable curriculum revision with ordered activity membership. Referenced activities must already be published.",
		Tags:          []string{"Courses"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *publishCourseInput) (*publishCourseOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		res, err := repo.PublishCourseDocument(ctx, q, &in.Body.Document, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &publishCourseOutput{Body: PublishCourseResponse{
			Curriculum: *res.Curriculum,
			Revision:   *res.Revision,
			Activities: res.Activities,
		}}, nil
	})

	// Publish course from a server-local path (admin convenience for basic_linux).
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "publish-course-path",
		Method:        http.MethodPost,
		Path:          "/courses/publish-path",
		Summary:       "Publish course.json from filesystem path",
		Description:   "Loads and validates a course.json, then publishes a curriculum revision. Activities must already be published.",
		Tags:          []string{"Courses"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *publishCoursePathInput) (*publishCourseOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		path := in.Body.Path
		if path == "" {
			return nil, huma.Error400BadRequest("path is required")
		}
		if !filepath.IsAbs(path) {
			// Allow repo-relative paths from process cwd.
			if wd, err := os.Getwd(); err == nil {
				path = filepath.Join(wd, path)
			}
		}
		doc, err := contracts.LoadCourseDocument(path)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		res, err := repo.PublishCourseDocument(ctx, q, doc, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		return &publishCourseOutput{Body: PublishCourseResponse{
			Curriculum: *res.Curriculum,
			Revision:   *res.Revision,
			Activities: res.Activities,
		}}, nil
	})

	// List curriculum revisions.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-curriculum-revisions",
		Method:      http.MethodGet,
		Path:        "/curriculum-revisions",
		Summary:     "List curriculum revisions",
		Tags:        []string{"Courses"},
	}), func(ctx context.Context, in *ListInput) (*listOutput[domain.CurriculumRevision], error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		filters, err := ParseFilters(in.Filter)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		page, err := repo.CurriculumRevisions.List(ctx, q, repo.ListParams{
			Limit: in.Limit, Offset: in.Offset, Search: in.Q, Sort: in.Sort, Dir: repo.SortDir(in.Dir), Filters: filters,
		})
		if err != nil {
			return nil, MapError(err)
		}
		return &listOutput[domain.CurriculumRevision]{Body: PageBody[domain.CurriculumRevision]{
			Items: page.Items, TotalCount: page.TotalCount, Limit: page.Limit, Offset: page.Offset,
		}}, nil
	})

	// Enroll student in a course revision (extends enrollments).
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID:   "enroll-student-course",
		Method:        http.MethodPost,
		Path:          "/students/{id}/enrollments",
		Summary:       "Enroll student in a curriculum revision",
		Description:   "Creates or resumes an enrollment pointed at an explicit curriculum revision.",
		Tags:          []string{"Enrollments"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *enrollStudentInput) (*itemOutput[domain.Enrollment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		curriculumID := in.Body.CurriculumID
		revisionID := in.Body.CurriculumRevisionID
		switch {
		case revisionID != "":
			// Derive curriculum from revision when needed.
			if curriculumID == "" {
				rev, err := repo.CurriculumRevisions.Get(ctx, q, revisionID)
				if err != nil {
					return nil, MapError(err)
				}
				curriculumID = rev.CurriculumID
			}
		case in.Body.CurriculumSlug != "":
			curr, err := repo.GetCurriculumBySlug(ctx, q, in.Body.CurriculumSlug)
			if err != nil {
				return nil, MapError(err)
			}
			curriculumID = curr.ID
			rev, err := repo.LatestCurriculumRevision(ctx, q, curr.ID)
			if err != nil {
				return nil, MapError(err)
			}
			revisionID = rev.ID
		case curriculumID != "":
			rev, err := repo.LatestCurriculumRevision(ctx, q, curriculumID)
			if err != nil {
				return nil, MapError(err)
			}
			revisionID = rev.ID
		default:
			return nil, huma.Error400BadRequest("curriculumId, curriculumRevisionId, or curriculumSlug is required")
		}
		if curriculumID == "" || revisionID == "" {
			return nil, huma.Error400BadRequest("could not resolve curriculum revision")
		}
		priority := 0
		if in.Body.Priority != nil {
			priority = *in.Body.Priority
		}
		edID := ed.ID
		en, err := repo.EnrollStudentInCurriculumRevision(ctx, q, in.ID, curriculumID, revisionID, &edID, priority)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Enrollment]{Body: *en}, nil
	})

	// Pause enrollment.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "pause-enrollment",
		Method:      http.MethodPost,
		Path:        "/enrollments/{id}/pause",
		Summary:     "Pause an enrollment",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *enrollmentActionInput) (*itemOutput[domain.Enrollment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		edID := ed.ID
		en, err := repo.SetEnrollmentStatus(ctx, q, in.ID, "paused", &edID, in.Body.Reason)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Enrollment]{Body: *en}, nil
	})

	// Resume enrollment.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "resume-enrollment",
		Method:      http.MethodPost,
		Path:        "/enrollments/{id}/resume",
		Summary:     "Resume a paused enrollment",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *enrollmentActionInput) (*itemOutput[domain.Enrollment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		edID := ed.ID
		en, err := repo.SetEnrollmentStatus(ctx, q, in.ID, "active", &edID, in.Body.Reason)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Enrollment]{Body: *en}, nil
	})

	// Pin next activity.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "pin-enrollment-activity",
		Method:      http.MethodPost,
		Path:        "/enrollments/{id}/pin",
		Summary:     "Pin next activity on enrollment",
		Description: "Sets or clears (empty slug) the parent pin used by AssignNext.",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *pinEnrollmentInput) (*itemOutput[domain.Enrollment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		edID := ed.ID
		en, err := repo.PinEnrollmentActivity(ctx, q, in.ID, in.Body.Slug, in.Body.Reason, &edID)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Enrollment]{Body: *en}, nil
	})

	// Override prerequisite for a slug.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "override-enrollment-prereq",
		Method:      http.MethodPost,
		Path:        "/enrollments/{id}/override",
		Summary:     "Override prerequisite for an activity",
		Description: "Auditable parent override allowing an activity despite unmet prerequisites/gates.",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *overrideEnrollmentInput) (*itemOutput[domain.Enrollment], error) {
		ed, err := parentEducator(ctx)
		if err != nil {
			return nil, err
		}
		edID := ed.ID
		en, err := repo.OverrideEnrollmentPrereq(ctx, q, in.ID, in.Body.Slug, in.Body.Reason, &edID)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.Enrollment]{Body: *en}, nil
	})

	// Eligibility preview / course map.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "enrollment-eligibility",
		Method:      http.MethodGet,
		Path:        "/enrollments/{id}/eligibility",
		Summary:     "Preview enrollment eligibility and course map",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *getInput) (*eligibilityOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &eligibilityOutput{Body: *prev}, nil
	})

	// Enrollment progress (alias of eligibility with lighter name for parents).
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "enrollment-progress",
		Method:      http.MethodGet,
		Path:        "/enrollments/{id}/progress",
		Summary:     "Course map progress statuses for an enrollment",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *getInput) (*eligibilityOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		prev, err := overseer.EvaluateEnrollmentEligibility(ctx, q, in.ID)
		if err != nil {
			return nil, MapError(err)
		}
		return &eligibilityOutput{Body: *prev}, nil
	})

	// List audit events.
	huma.Register(h, parentOp(h, q, huma.Operation{
		OperationID: "list-enrollment-audit",
		Method:      http.MethodGet,
		Path:        "/enrollments/{id}/audit",
		Summary:     "List enrollment audit events",
		Tags:        []string{"Enrollments"},
	}), func(ctx context.Context, in *getInput) (*enrollmentAuditOutput, error) {
		if _, err := parentEducator(ctx); err != nil {
			return nil, err
		}
		items, err := repo.ListEnrollmentAudit(ctx, q, in.ID, 100)
		if err != nil {
			return nil, MapError(err)
		}
		return &enrollmentAuditOutput{Body: EnrollmentAuditList{Items: items}}, nil
	})
}

type publishCourseInput struct {
	Body struct {
		Document contracts.CourseDocument `json:"document"`
	}
}

type publishCoursePathInput struct {
	Body struct {
		Path string `json:"path" minLength:"1"`
	}
}

// PublishCourseResponse is returned after publishing a course revision.
type PublishCourseResponse struct {
	Curriculum domain.Curriculum         `json:"curriculum"`
	Revision   domain.CurriculumRevision `json:"revision"`
	Activities int                       `json:"activities"`
}

type publishCourseOutput struct {
	Body PublishCourseResponse
}

type enrollStudentInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		CurriculumID         string  `json:"curriculumId,omitempty" format:"uuid"`
		CurriculumRevisionID string  `json:"curriculumRevisionId,omitempty" format:"uuid"`
		CurriculumSlug       string  `json:"curriculumSlug,omitempty"`
		Priority             *int    `json:"priority,omitempty"`
	}
}

type enrollmentActionInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Reason string `json:"reason,omitempty"`
	}
}

type pinEnrollmentInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Slug   string `json:"slug"`
		Reason string `json:"reason,omitempty"`
	}
}

type overrideEnrollmentInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Slug   string `json:"slug" minLength:"1"`
		Reason string `json:"reason" minLength:"1"`
	}
}

type eligibilityOutput struct {
	Body overseer.EligibilityPreview
}

// EnrollmentAuditList is GET /enrollments/{id}/audit.
type EnrollmentAuditList struct {
	Items []domain.EnrollmentAuditEvent `json:"items"`
}

type enrollmentAuditOutput struct {
	Body EnrollmentAuditList
}
