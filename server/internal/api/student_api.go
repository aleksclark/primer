package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/mastery"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/tutor"
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

func registerStudentAPI(h huma.API, q repo.Querier, opts Options) {
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
		caps := in.Body.Capabilities
		sess, err := repo.StartOrResumeSession(ctx, q, dev, in.Body.ClientSessionID, in.Body.AssignmentID, time.Now().UTC(), caps...)
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

	// Server-owned tutor: policy-constrained coaching from session + activity revision.
	// Client message is untrusted user content only; activity body is never taken from client.
	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "post-session-tutor-message",
		Method:        http.MethodPost,
		Path:          "/student/sessions/{id}/tutor/messages",
		Summary:       "Send a tutor chat message",
		Description:   "Returns brief discovery-oriented coaching. Falls back to activity hints when the provider fails. Does not mutate mastery.",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusOK,
	}), func(ctx context.Context, in *tutorMessageInput) (*tutorMessageOutput, error) {
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

		reply, meta, err := coachSession(ctx, q, opts, sess, in.Body.Message)
		if err != nil {
			return nil, MapError(err)
		}
		return &tutorMessageOutput{Body: TutorMessageResponse{
			Reply:       reply,
			Provider:    meta.Provider,
			Fallback:    meta.Fallback,
			RateLimited: meta.RateLimited,
		}}, nil
	})

	// Conceptual response submission (student-authored text only; never tutor output).
	huma.Register(h, studentOp(h, q, huma.Operation{
		OperationID:   "submit-student-response",
		Method:        http.MethodPost,
		Path:          "/student/sessions/{id}/responses",
		Summary:       "Submit a constructed response",
		Description:   "Idempotent by submissionId. Creates conceptual_response evidence. Tutor clients must not call this with generated text.",
		Tags:          []string{"Student"},
		DefaultStatus: http.StatusCreated,
	}), func(ctx context.Context, in *submitResponseInput) (*submitResponseOutput, error) {
		dev, err := studentDevice(ctx)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		req := in.Body
		if req.SchemaVersion == "" {
			req.SchemaVersion = contracts.ResponseSchemaVersion
		}
		if req.ClientTime.IsZero() {
			req.ClientTime = now
		}
		resp, created, err := repo.SubmitStudentResponse(ctx, q, dev, in.ID, req, now)
		if err != nil {
			if errors.Is(err, repo.ErrConflict) {
				return nil, huma.Error409Conflict(err.Error())
			}
			return nil, MapError(err)
		}
		var evidenceIDs []string
		if created {
			evidenceIDs, err = mastery.ApplyConceptualResponse(ctx, q, resp, now)
			if err != nil {
				return nil, MapError(err)
			}
		}
		status := http.StatusCreated
		if !created {
			status = http.StatusOK
		}
		_ = status
		return &submitResponseOutput{Body: contracts.ResponseSubmissionResult{
			SchemaVersion:  contracts.ResponseSchemaVersion,
			SubmissionID:   resp.SubmissionID,
			ResponseID:     resp.ID,
			Status:         resp.Status,
			EvidenceIDs:    evidenceIDs,
			ReviewRequired: resp.ParentReviewRequired,
			Message:        "accepted",
		}}, nil
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
		ClientSessionID string   `json:"clientSessionId" minLength:"1"`
		AssignmentID    string   `json:"assignmentId" format:"uuid"`
		Capabilities    []string `json:"capabilities,omitempty" doc:"Runner capability flags such as structured_command_evidence."`
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

type tutorMessageInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Message string `json:"message"`
	}
}

type submitResponseInput struct {
	ID   string                       `path:"id" format:"uuid"`
	Body contracts.ResponseSubmission
}

type submitResponseOutput struct {
	Body contracts.ResponseSubmissionResult
}

// TutorMessageResponse is a short coaching reply.
type TutorMessageResponse struct {
	Reply       string `json:"reply"`
	Provider    string `json:"provider,omitempty"`
	Fallback    bool   `json:"fallback,omitempty"`
	RateLimited bool   `json:"rateLimited,omitempty"`
}

type tutorMessageOutput struct {
	Body TutorMessageResponse
}

type tutorReplyMeta struct {
	Provider    string
	Fallback    bool
	RateLimited bool
	Disabled    bool
}

// coachSession builds a server-side tutor.Request and records a tutor_message event.
func coachSession(ctx context.Context, q repo.Querier, opts Options, sess *domain.LearningSession, studentMessage string) (string, tutorReplyMeta, error) {
	now := time.Now().UTC()
	content, slug, err := loadSessionActivity(ctx, q, sess)
	if err != nil {
		// Still return a safe fallback so the activity remains completable.
		fb := tutor.DefaultFallback
		_ = recordTutorEvent(ctx, q, sess.ID, studentMessage, fb, tutorReplyMeta{Provider: opts.TutorProviderName, Fallback: true}, now)
		return fb, tutorReplyMeta{Provider: opts.TutorProviderName, Fallback: true}, nil
	}

	student, err := repo.Students.Get(ctx, q, sess.StudentID)
	if err != nil {
		return "", tutorReplyMeta{}, err
	}

	prior, _ := repo.ListTutorReplyHints(ctx, q, sess.ID, 20)
	svc := opts.Tutor
	if svc == nil {
		svc = tutor.NewFake()
	}

	// Deployment or per-student off switch → activity-local hint only.
	enabled := opts.TutorEnabled
	if ps, ok := svc.(*tutor.PolicyService); ok {
		enabled = ps.Enabled()
	}
	if !enabled || studentTutorDisabled(student.Notes) {
		fb := tutor.FallbackHint(content, prior, 0)
		meta := tutorReplyMeta{Provider: opts.TutorProviderName, Fallback: true, Disabled: true}
		_ = recordTutorEvent(ctx, q, sess.ID, studentMessage, fb, meta, now)
		return fb, meta, nil
	}

	req := tutor.Request{
		SessionID:      sess.ID,
		StudentID:      sess.StudentID,
		ActivitySlug:   slug,
		Activity:       content,
		StudentMessage: studentMessage,
		PriorHints:     prior,
		// CurrentTask / Observations intentionally omitted unless server derives them later.
		// Never accept system prompt, standards, or mastery from the client body.
	}

	resp, err := svc.Coach(ctx, req)
	if err != nil {
		fb := tutor.FallbackHint(content, prior, 0)
		meta := tutorReplyMeta{Provider: opts.TutorProviderName, Fallback: true}
		_ = recordTutorEvent(ctx, q, sess.ID, studentMessage, fb, meta, now)
		return fb, meta, nil
	}
	meta := tutorReplyMeta{
		Provider:    resp.Provider,
		Fallback:    resp.Fallback,
		RateLimited: resp.RateLimited,
		Disabled:    resp.Disabled,
	}
	if meta.Provider == "" {
		meta.Provider = opts.TutorProviderName
	}
	_ = recordTutorEvent(ctx, q, sess.ID, studentMessage, resp.Reply, meta, now)
	return resp.Reply, meta, nil
}

func loadSessionActivity(ctx context.Context, q repo.Querier, sess *domain.LearningSession) (contracts.ActivityContent, string, error) {
	revID := sess.ActivityRevisionID
	if revID == "" {
		asg, err := repo.StudentAssignments.Get(ctx, q, sess.AssignmentID)
		if err != nil {
			return contracts.ActivityContent{}, "", err
		}
		revID = asg.ActivityRevisionID
	}
	rev, err := repo.GetRevision(ctx, q, revID)
	if err != nil {
		return contracts.ActivityContent{}, "", err
	}
	content, err := decodeRevisionContent(rev.Content)
	if err != nil {
		return contracts.ActivityContent{}, "", err
	}
	slug := ""
	if act, err := repo.LearningActivities.Get(ctx, q, rev.ActivityID); err == nil {
		slug = act.Slug
	}
	return content, slug, nil
}

func recordTutorEvent(ctx context.Context, q repo.Querier, sessionID, studentMessage, reply string, meta tutorReplyMeta, now time.Time) error {
	// Retention-friendly payload: short reply + flags; no provider credentials or raw PTY.
	payload := map[string]any{
		"reply":       reply,
		"provider":    meta.Provider,
		"fallback":    meta.Fallback,
		"rateLimited": meta.RateLimited,
		"disabled":    meta.Disabled,
		"source":      "server",
	}
	// Bound stored student text; never treat it as system policy.
	msg := tutor.SanitizeStudentMessage(studentMessage)
	if msg != "" {
		payload["studentMessage"] = msg
	}
	return repo.InsertServerSessionEvent(ctx, q, sessionID, contracts.EventTutorMessage, payload, now)
}

// studentTutorDisabled is the simple per-student off switch via notes marker.
func studentTutorDisabled(notes string) bool {
	return strings.Contains(strings.ToLower(notes), "tutor:off")
}

func decodeRevisionContent(m map[string]any) (contracts.ActivityContent, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return contracts.ActivityContent{}, err
	}
	var c contracts.ActivityContent
	if err := json.Unmarshal(raw, &c); err != nil {
		return contracts.ActivityContent{}, err
	}
	return c, nil
}
