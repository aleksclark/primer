package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/mastery"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// deviceTokenSecurityScheme names the student device bearer scheme.
const deviceTokenSecurityScheme = "studentDeviceToken"

// deviceTokenHeader is an alternate header for the device token.
const deviceTokenHeader = "X-Device-Token"

type studentDeviceContextKey struct{}

// StudentDeviceFromContext returns the authenticated student device.
func StudentDeviceFromContext(ctx context.Context) (*domain.StudentDevice, bool) {
	d, ok := ctx.Value(studentDeviceContextKey{}).(*domain.StudentDevice)
	return d, ok
}

// StudentDeviceGuard authenticates student API routes via device token.
func StudentDeviceGuard(api huma.API, q repo.Querier) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token := ctx.Header(deviceTokenHeader)
		if token == "" {
			token = BearerToken(ctx.Header("Authorization"))
		}
		if token == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing device token")
			return
		}
		dev, err := repo.StudentDeviceByToken(ctx.Context(), q, token)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "unknown or revoked device token")
				return
			}
			_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "authenticate device")
			return
		}
		_ = repo.TouchStudentDevice(ctx.Context(), q, dev.ID, time.Now().UTC())
		next(huma.WithValue(ctx, studentDeviceContextKey{}, dev))
	}
}

func studentDevice(ctx context.Context) (*domain.StudentDevice, error) {
	d, ok := StudentDeviceFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("device not authenticated")
	}
	return d, nil
}

func studentOp(h huma.API, q repo.Querier, op huma.Operation) huma.Operation {
	op.Security = []map[string][]string{{deviceTokenSecurityScheme: {}}}
	op.Middlewares = huma.Middlewares{StudentDeviceGuard(h, q)}
	op.Errors = append(op.Errors, http.StatusUnauthorized)
	return op
}

func registerStudentAPI(h huma.API, q repo.Querier) {
	// Pair (unauthenticated)
	huma.Register(h, huma.Operation{
		OperationID:   "pair-student-device",
		Method:        http.MethodPost,
		Path:          "/student-devices/pair",
		Summary:       "Pair a student device",
		Description:   "Exchange a parent-issued pairing code for a device token. The token is returned once.",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusForbidden},
	}, func(ctx context.Context, in *pairStudentDeviceInput) (*pairStudentDeviceOutput, error) {
		name := in.Body.DeviceName
		if name == "" {
			name = "workstation"
		}
		token, dev, err := repo.ClaimStudentPairingCode(ctx, q, in.Body.Code, name, time.Now().UTC())
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, huma.Error403Forbidden("pairing code is invalid, expired, or already used")
			}
			return nil, MapError(err)
		}
		student, err := repo.Students.Get(ctx, q, dev.StudentID)
		if err != nil {
			return nil, MapError(err)
		}
		return &pairStudentDeviceOutput{Body: PairStudentDeviceResponse{
			DeviceID: dev.ID,
			Token:    token,
			Student:  *student,
			Device:   *dev,
		}}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID: "get-student-profile",
		Method:      http.MethodGet,
		Path:        "/student/profile",
		Summary:     "Student profile for the paired device",
		Tags:        []string{"Student"},
	}), func(ctx context.Context, _ *struct{}) (*studentProfileOutput, error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		student, err := repo.Students.Get(ctx, q, dev.StudentID)
		if err != nil {
			return nil, MapError(err)
		}
		return &studentProfileOutput{Body: StudentProfile{
			Student:    *student,
			DeviceID:   dev.ID,
			DeviceName: dev.Name,
		}}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID: "get-student-work",
		Method:      http.MethodGet,
		Path:        "/student/work",
		Summary:     "Work queue for the paired student",
		Tags:        []string{"Student"},
	}), func(ctx context.Context, in *studentWorkInput) (*studentWorkOutput, error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		items, cursor, err := repo.ListStudentWork(ctx, q, dev.StudentID, in.After, in.Limit)
		if err != nil {
			return nil, MapError(err)
		}
		return &studentWorkOutput{Body: StudentWorkResponse{
			Items:  items,
			Cursor: cursor,
		}}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "start-student-session",
		Method:        http.MethodPost,
		Path:          "/student/sessions",
		Summary:       "Start or resume a learning session",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *startSessionInput) (*itemOutput[domain.LearningSession], error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		sess, err := repo.StartOrResumeSession(ctx, q, dev, in.Body.ClientSessionID, in.Body.AssignmentID, time.Now().UTC())
		if err != nil {
			return nil, MapError(err)
		}
		status := http.StatusCreated
		// Resumed sessions still return 200 when already existed — keep 201 for simplicity
		// unless we detect same client id was already there; clients treat either as OK.
		_ = status
		return &itemOutput[domain.LearningSession]{Body: *sess}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID: "post-session-events",
		Method:      http.MethodPost,
		Path:        "/student/sessions/{id}/events",
		Summary:     "Ingest idempotent session event batch",
		Tags:        []string{"Student"},
	}), func(ctx context.Context, in *postEventsInput) (*postEventsOutput, error) {
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
		acked, err := repo.IngestSessionEvents(ctx, q, sess.ID, in.Body.Events, time.Now().UTC())
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict(err.Error())
			}
			return nil, MapError(err)
		}
		return &postEventsOutput{Body: EventsAck{AcknowledgedSequence: acked}}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "post-session-artifact",
		Method:        http.MethodPost,
		Path:          "/student/sessions/{id}/artifacts",
		Summary:       "Register session artifact metadata",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *postArtifactInput) (*itemOutput[domain.LearningSessionArtifact], error) {
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
		meta := in.Body
		if meta.SchemaVersion == "" {
			meta.SchemaVersion = contracts.ArtifactSchemaVersion
		}
		art, err := repo.UpsertSessionArtifact(ctx, q, sess.ID, meta)
		if err != nil {
			return nil, MapError(err)
		}
		return &itemOutput[domain.LearningSessionArtifact]{Body: *art}, nil
	})

	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID: "complete-student-session",
		Method:      http.MethodPost,
		Path:        "/student/sessions/{id}/complete",
		Summary:     "Complete a session (idempotent)",
		Tags:        []string{"Student"},
	}), func(ctx context.Context, in *completeSessionInput) (*completeSessionOutput, error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		result, err := mastery.ApplyCompletion(ctx, q, dev, in.ID, in.Body, time.Now().UTC())
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict(err.Error())
			}
			return nil, MapError(err)
		}
		return &completeSessionOutput{Body: result}, nil
	})
}

type pairStudentDeviceInput struct {
	Body struct {
		Code       string `json:"code" minLength:"4"`
		DeviceName string `json:"deviceName,omitempty"`
	}
}

// PairStudentDeviceResponse is returned once at pairing time.
type PairStudentDeviceResponse struct {
	DeviceID string               `json:"deviceId" format:"uuid"`
	Token    string               `json:"token"`
	Student  domain.Student       `json:"student"`
	Device   domain.StudentDevice `json:"device"`
}

type pairStudentDeviceOutput struct {
	Body PairStudentDeviceResponse
}

// StudentProfile is the paired student's identity view.
type StudentProfile struct {
	Student    domain.Student `json:"student"`
	DeviceID   string         `json:"deviceId" format:"uuid"`
	DeviceName string         `json:"deviceName"`
}

type studentProfileOutput struct {
	Body StudentProfile
}

type studentWorkInput struct {
	After string `query:"after" doc:"Cursor from a previous work response."`
	Limit int    `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

// StudentWorkResponse is the work queue sync payload.
type StudentWorkResponse struct {
	Items  []repo.StudentWorkItem `json:"items"`
	Cursor string                 `json:"cursor,omitempty"`
}

type studentWorkOutput struct {
	Body StudentWorkResponse
}

type startSessionInput struct {
	Body struct {
		ClientSessionID string `json:"clientSessionId" minLength:"1"`
		AssignmentID    string `json:"assignmentId" format:"uuid"`
	}
}

type postEventsInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Events []contracts.SessionEvent `json:"events"`
	}
}

// EventsAck reports the highest contiguous acknowledged sequence.
type EventsAck struct {
	AcknowledgedSequence int64 `json:"acknowledgedSequence"`
}

type postEventsOutput struct {
	Body EventsAck
}

type postArtifactInput struct {
	ID   string `path:"id" format:"uuid"`
	Body contracts.ArtifactMeta
}

type completeSessionInput struct {
	ID   string `path:"id" format:"uuid"`
	Body contracts.CompletionRequest
}

type completeSessionOutput struct {
	Body contracts.CompletionResult
}
