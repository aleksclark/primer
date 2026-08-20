package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The types in this file are the Tasks wire boundary. Huma derives the
// request, response, and error schemas from these types and the typed handler
// signatures below. Persistence and the browser-facing implementation remain
// behind that boundary.
type Health struct {
	Status        string    `json:"status"`
	ModelProvider string    `json:"modelProvider"`
	StartedAt     time.Time `json:"startedAt" format:"date-time"`
}

type Session struct {
	SubjectRef string `json:"subjectRef"`
	TenantID   string `json:"tenantId"`
}

type StatusResponse struct {
	Status string `json:"status"`
}

type StudentPage struct {
	Items      []Student `json:"items" nullable:"false"`
	TotalCount int       `json:"totalCount"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

type CreateStudent struct {
	DisplayName string `json:"displayName"`
}

type UpdateStudent struct {
	DisplayName string `json:"displayName"`
}

type PairCode struct {
	Code string `json:"code"`
}

type StudentPair struct {
	StudentID string `json:"studentId"`
}

type DevicePair struct {
	Token     string `json:"token"`
	StudentID string `json:"studentId"`
}

type Pairing struct {
	PairingID string `json:"pairingId"`
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt" format:"date-time"`
	QRPayload string `json:"qrPayload"`
}

type ChecklistItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type Checklist struct {
	Items []ChecklistItem `json:"items" nullable:"false"`
}

type AgentConversation struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenantId"`
	PolicyVersion string    `json:"policyVersion"`
	CreatedAt     time.Time `json:"createdAt" format:"date-time"`
}

type AgentConversationOutput struct {
	ResponseHeaders
	Body AgentConversation
}

type AuthLoginInput struct {
	ReturnTo  string `query:"return_to"`
	Principal string `query:"principal"`
}

type AuthCallbackInput struct {
	Error string `query:"error"`
	State string `query:"state"`
	Code  string `query:"code"`
}

type ListStudentsInput struct {
	Q      string `query:"q"`
	Limit  int    `query:"limit"`
	Offset int    `query:"offset"`
}

type StudentIDInput struct {
	ID string `path:"id"`
}

type CreateStudentInput struct {
	Body CreateStudent `required:"true"`
}

type UpdateStudentInput struct {
	ID   string        `path:"id"`
	Body UpdateStudent `required:"true"`
}

type PairCodeInput struct {
	Body PairCode `required:"true"`
}

// ResponseHeaders is embedded by every output that can carry browser
// headers. Set-Cookie is a slice because one request may set more than one
// cookie (the OAuth callback does this).
type ResponseHeaders struct {
	SetCookie []string `header:"Set-Cookie"`
	Location  string   `header:"Location"`
}

type HealthOutput struct {
	ResponseHeaders
	Body Health
}
type SessionOutput struct {
	ResponseHeaders
	Body Session
}
type StatusOutput struct {
	ResponseHeaders
	Body StatusResponse
}
type StudentPageOutput struct {
	ResponseHeaders
	Body StudentPage
}
type StudentOutput struct {
	ResponseHeaders
	Body Student
}
type CreatedStudentOutput struct {
	ResponseHeaders
	Body Student
}
type PairingOutput struct {
	ResponseHeaders
	Body Pairing
}
type StudentPairOutput struct {
	ResponseHeaders
	Body StudentPair
}
type DevicePairOutput struct {
	ResponseHeaders
	Body DevicePair
}
type ChecklistOutput struct {
	ResponseHeaders
	Body Checklist
}
type RedirectOutput struct {
	ResponseHeaders
}
type NoContentOutput struct {
	ResponseHeaders
}

// ArtifactReservationInput and ArtifactFinalizeInput are the strict Phase 5 JSON
// boundaries. The binary upload/download routes are deliberately kept outside
// these Huma operations (see registerArtifactRoutes).
type ArtifactOccurrenceInput struct {
	Occurrence string                   `path:"occurrence" format:"uuid"`
	Body       ArtifactReservationInput `required:"true"`
}
type ArtifactFinalizeBoundaryInput struct {
	Occurrence string                `path:"occurrence" format:"uuid"`
	Body       ArtifactFinalizeInput `required:"true"`
}
type ArtifactOccurrencePath struct {
	Occurrence string `path:"occurrence" format:"uuid"`
}
type ArtifactStateOutput struct {
	ResponseHeaders
	Body ArtifactStateResponse
}

// artifactPathRequest is shared by the JSON state and retry operations.
type artifactPathRequest struct {
	Occurrence string `path:"occurrence" format:"uuid"`
}
type ArtifactReservationBoundaryOutput struct {
	ResponseHeaders
	Body ArtifactReservationOutput
}
type ArtifactOutputBoundary struct {
	ResponseHeaders
	Body ArtifactOutput
}

// Problem is the existing browser error envelope. It is also Huma's error
// type, so validation failures and handler failures use the same wire shape.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
	status  int
}

func (e *Problem) Error() string             { return e.Detail }
func (e *Problem) GetStatus() int            { return e.status }
func (e *Problem) ContentType(string) string { return "application/json" }

func newProblem(status int, message string, _ ...error) huma.StatusError {
	// The old browser API reports malformed and semantically invalid JSON as
	// 400. Huma normally uses 422 for schema validation, so keep that public
	// behavior while still deriving the validation schema from the input type.
	if status == http.StatusUnprocessableEntity {
		status = http.StatusBadRequest
	}
	return &Problem{Code: "invalid_request", Message: message, Detail: message, status: status}
}

func init() {
	// Huma exposes its error factory specifically for applications that need a
	// stable error envelope. Tasks is a standalone binary, so installing this
	// once keeps offline emission and runtime serialization identical.
	huma.NewError = newProblem
}

type humaContextKey struct{}

func register[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	huma.Register(api, op, handler)
	// Huma adds 422 for every operation with a body. The legacy public API
	// reports malformed/empty JSON as 400; Problem handles runtime errors and
	// this removes the unreachable generic status from the generated contract.
	item := api.OpenAPI().Paths[op.Path]
	var registered *huma.Operation
	switch op.Method {
	case http.MethodGet:
		registered = item.Get
	case http.MethodPost:
		registered = item.Post
	case http.MethodPatch:
		registered = item.Patch
	case http.MethodDelete:
		registered = item.Delete
	}
	if registered != nil {
		delete(registered.Responses, "422")
	}
}

func humaRequest(ctx context.Context) (*http.Request, error) {
	hctx, ok := ctx.Value(humaContextKey{}).(huma.Context)
	if !ok {
		return nil, huma.Error500InternalServerError("Huma request context missing")
	}
	req, _ := humachi.Unwrap(hctx)
	return req, nil
}

func (s *Server) humaStudent(ctx context.Context) (uuid.UUID, error) {
	req, err := humaRequest(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	student, err := s.studentFromCookie(req)
	if err != nil {
		return uuid.Nil, newProblem(http.StatusUnauthorized, "student session required")
	}
	return student, nil
}

func (s *Server) humaParent(ctx context.Context) (scope, error) {
	req, err := humaRequest(ctx)
	if err != nil {
		return scope{}, err
	}
	parent, err := s.parentScope(req)
	if err != nil {
		return scope{}, newProblem(http.StatusUnauthorized, "parent session required")
	}
	return parent, nil
}

// humaAPI is the sole production registration function. Routes and offline
// OpenAPI emission both call it; there is no parallel route or schema builder.
func (s *Server) humaAPI() huma.API {
	r := chi.NewRouter()
	config := huma.DefaultConfig("Primer Tasks", "1.0.0")
	config.DocsPath = ""
	config.SchemasPath = ""
	config.OpenAPIPath = "/openapi"
	api := humachi.New(r, config)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, humaContextKey{}, ctx))
	})

	register(api, huma.Operation{OperationID: "health", Method: http.MethodGet, Path: "/health"}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		body, headers, err := legacyJSON[Health](ctx, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			jsonOK(w, Health{Status: "ok", ModelProvider: envOr("TASKS_MODEL_PROVIDER", "disabled"), StartedAt: s.StartedAt})
		}), nil)
		return &HealthOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "auth-login", Method: http.MethodGet, Path: "/auth/login", DefaultStatus: http.StatusFound, Errors: []int{500, 503}}, func(ctx context.Context, _ *AuthLoginInput) (*RedirectOutput, error) {
		headers, err := legacyEmpty(ctx, http.HandlerFunc(s.login), nil)
		return &RedirectOutput{ResponseHeaders: headers}, err
	})
	register(api, huma.Operation{OperationID: "auth-callback", Method: http.MethodGet, Path: "/auth/callback", DefaultStatus: http.StatusFound, Errors: []int{400, 401, 403, 500}}, func(ctx context.Context, _ *AuthCallbackInput) (*RedirectOutput, error) {
		headers, err := legacyEmpty(ctx, http.HandlerFunc(s.callback), nil)
		return &RedirectOutput{ResponseHeaders: headers}, err
	})
	register(api, huma.Operation{OperationID: "auth-session", Method: http.MethodGet, Path: "/auth/session", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*SessionOutput, error) {
		body, headers, err := legacyJSON[Session](ctx, http.HandlerFunc(s.parentSession), nil)
		return &SessionOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "auth-logout", Method: http.MethodPost, Path: "/auth/logout", Errors: []int{500}}, func(ctx context.Context, _ *struct{}) (*StatusOutput, error) {
		body, headers, err := legacyJSON[StatusResponse](ctx, http.HandlerFunc(s.logout), nil)
		return &StatusOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "students-list", Method: http.MethodGet, Path: "/students", Errors: []int{401, 500}}, func(ctx context.Context, _ *ListStudentsInput) (*StudentPageOutput, error) {
		body, headers, err := legacyJSON[StudentPage](ctx, s.requireParent(s.listStudents), nil)
		return &StudentPageOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "students-create", Method: http.MethodPost, Path: "/students", DefaultStatus: http.StatusCreated, Errors: []int{400, 401, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *CreateStudentInput) (*CreatedStudentOutput, error) {
		body, headers, err := legacyJSON[Student](ctx, s.requireParent(s.createStudent), in.Body)
		return &CreatedStudentOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "students-get", Method: http.MethodGet, Path: "/students/{id}", Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *StudentIDInput) (*StudentOutput, error) {
		body, headers, err := legacyJSON[Student](ctx, s.requireParent(s.getStudent), nil)
		return &StudentOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "students-update", Method: http.MethodPatch, Path: "/students/{id}", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *UpdateStudentInput) (*StudentOutput, error) {
		body, headers, err := legacyJSON[Student](ctx, s.requireParent(s.updateStudent), in.Body)
		return &StudentOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "students-archive", Method: http.MethodDelete, Path: "/students/{id}", DefaultStatus: http.StatusNoContent, Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *StudentIDInput) (*NoContentOutput, error) {
		headers, err := legacyEmpty(ctx, s.requireParent(s.archiveStudent), nil)
		return &NoContentOutput{ResponseHeaders: headers}, err
	})
	register(api, huma.Operation{OperationID: "students-pairing", Method: http.MethodPost, Path: "/students/{id}/pairing", Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *StudentIDInput) (*PairingOutput, error) {
		body, headers, err := legacyJSON[Pairing](ctx, s.requireParent(s.issuePairing), nil)
		return &PairingOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "student-pair", Method: http.MethodPost, Path: "/student/pair", Errors: []int{400, 410}, SkipValidateBody: true}, func(ctx context.Context, in *PairCodeInput) (*StudentPairOutput, error) {
		body, headers, err := legacyJSON[StudentPair](ctx, http.HandlerFunc(s.pairBrowser), in.Body)
		return &StudentPairOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "student-profile", Method: http.MethodGet, Path: "/student/profile", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*StudentOutput, error) {
		body, headers, err := legacyJSON[Student](ctx, s.requireStudent(s.studentProfile), nil)
		return &StudentOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "student-checklist", Method: http.MethodGet, Path: "/student/checklist", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*ChecklistOutput, error) {
		body, headers, err := legacyJSON[Checklist](ctx, s.requireStudent(s.checklist), nil)
		return &ChecklistOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "device-pair", Method: http.MethodPost, Path: "/device/pair", Errors: []int{400, 410}, SkipValidateBody: true}, func(ctx context.Context, in *PairCodeInput) (*DevicePairOutput, error) {
		body, headers, err := legacyJSON[DevicePair](ctx, http.HandlerFunc(s.devicePair), in.Body)
		return &DevicePairOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "device-profile", Method: http.MethodGet, Path: "/device/profile", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*StudentOutput, error) {
		body, headers, err := legacyJSON[Student](ctx, s.requireDevice(s.deviceProfile), nil)
		return &StudentOutput{ResponseHeaders: headers, Body: body}, err
	})
	register(api, huma.Operation{OperationID: "device-checklist", Method: http.MethodGet, Path: "/device/checklist", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*ChecklistOutput, error) {
		body, headers, err := legacyJSON[Checklist](ctx, s.requireDevice(s.checklist), nil)
		return &ChecklistOutput{ResponseHeaders: headers, Body: body}, err
	})

	register(api, huma.Operation{OperationID: "agent-conversation-create", Method: http.MethodPost, Path: "/agent/conversations", DefaultStatus: http.StatusCreated, Errors: []int{401, 500}}, func(ctx context.Context, _ *struct{}) (*AgentConversationOutput, error) {
		body, headers, err := legacyJSON[AgentConversation](ctx, s.requireParent(s.createAgentConversation), nil)
		return &AgentConversationOutput{ResponseHeaders: headers, Body: body}, err
	})

	s.registerPhase2(api)
	s.registerDialogueRoutes(api)
	// WebSocket transport is an owned protocol adapter, not an OpenAPI
	// operation. It shares this production router so offline REST emission and
	// runtime routes cannot drift.
	// Artifact JSON routes are registered through the same Huma boundary as
	// the rest of the REST API. Binary PUTs remain the single explicitly
	// allowlisted streaming façade and are registered on the shared router.
	register(api, huma.Operation{OperationID: "student-artifact-reserve", Method: http.MethodPost, Path: "/student/occurrences/{occurrence}/artifacts/reserve", DefaultStatus: http.StatusCreated, Errors: []int{400, 401, 404, 409, 500}}, func(ctx context.Context, in *ArtifactOccurrenceInput) (*ArtifactReservationBoundaryOutput, error) {
		student, err := s.humaStudent(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.reserveArtifactData(ctx, student, in.Occurrence, in.Body)
		return &ArtifactReservationBoundaryOutput{Body: out}, err
	})
	register(api, huma.Operation{OperationID: "student-artifact-state", Method: http.MethodGet, Path: "/student/occurrences/{occurrence}/artifacts", Errors: []int{401, 404, 500}}, func(ctx context.Context, in *artifactPathRequest) (*ArtifactStateOutput, error) {
		student, err := s.humaStudent(ctx)
		if err != nil {
			return nil, err
		}
		tenant, err := s.artifactTenant(ctx, student)
		if err != nil {
			return nil, newProblem(http.StatusUnauthorized, "student session required")
		}
		out, err := s.artifactState(ctx, tenant, in.Occurrence, &student)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newProblem(http.StatusNotFound, "artifact requirement not found")
		}
		if err != nil {
			return nil, newProblem(http.StatusInternalServerError, "unable to load artifact state")
		}
		return &ArtifactStateOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "student-artifact-retry", Method: http.MethodPost, Path: "/student/occurrences/{occurrence}/artifacts/retry", Errors: []int{401, 404, 409, 500}}, func(ctx context.Context, in *artifactPathRequest) (*ArtifactStateOutput, error) {
		student, err := s.humaStudent(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.retryArtifactState(ctx, student, in.Occurrence)
		if err != nil {
			return nil, err
		}
		return &ArtifactStateOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "parent-artifact-inspect", Method: http.MethodGet, Path: "/occurrences/{occurrence}/artifacts/inspect", Errors: []int{401, 404, 500}}, func(ctx context.Context, in *artifactPathRequest) (*ArtifactStateOutput, error) {
		parent, err := s.humaParent(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.artifactState(ctx, uuid.MustParse(parent.Tenant), in.Occurrence, nil)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, newProblem(http.StatusNotFound, "artifact requirement not found")
		}
		if err != nil {
			return nil, newProblem(http.StatusInternalServerError, "unable to load artifact state")
		}
		return &ArtifactStateOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "student-artifact-finalize", Method: http.MethodPost, Path: "/student/occurrences/{occurrence}/artifacts/finalize", Errors: []int{400, 401, 404, 409, 500}}, func(ctx context.Context, in *ArtifactFinalizeBoundaryInput) (*ArtifactOutputBoundary, error) {
		student, err := s.humaStudent(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.finalizeArtifactData(ctx, student, in.Occurrence, in.Body)
		return &ArtifactOutputBoundary{Body: out}, err
	})
	r.Handle("/ws", http.HandlerFunc(s.agentWS))
	r.Handle("/student/ws", http.HandlerFunc(s.studentWS))
	s.registerArtifactRoutes(r)
	return api
}

type capturedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *capturedResponse) Header() http.Header { return r.header }
func (r *capturedResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}
func (r *capturedResponse) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

func legacyResponse(ctx context.Context, handler http.Handler, body any) (*capturedResponse, error) {
	hctx, ok := ctx.Value(humaContextKey{}).(huma.Context)
	if !ok {
		return nil, huma.Error500InternalServerError("Huma request context missing")
	}
	req, _ := humachi.Unwrap(hctx)
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, huma.Error500InternalServerError("unable to encode request body")
		}
		req = req.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(payload))
		req.ContentLength = int64(len(payload))
	}
	response := &capturedResponse{header: make(http.Header)}
	handler.ServeHTTP(response, req)
	return response, nil
}

func legacyEmpty(ctx context.Context, handler http.Handler, body any) (ResponseHeaders, error) {
	response, err := legacyResponse(ctx, handler, body)
	if err != nil {
		return ResponseHeaders{}, err
	}
	if response.status >= 400 {
		return ResponseHeaders{}, capturedError(response)
	}
	return responseHeaders(response), nil
}

func legacyJSON[T any](ctx context.Context, handler http.Handler, body any) (T, ResponseHeaders, error) {
	response, err := legacyResponse(ctx, handler, body)
	if err != nil {
		return *new(T), ResponseHeaders{}, err
	}
	if response.status >= 400 {
		return *new(T), ResponseHeaders{}, capturedError(response)
	}
	var out T
	if response.body.Len() > 0 {
		if err := json.Unmarshal(response.body.Bytes(), &out); err != nil {
			return *new(T), ResponseHeaders{}, huma.Error500InternalServerError("invalid handler response")
		}
	}
	return out, responseHeaders(response), nil
}

func responseHeaders(response *capturedResponse) ResponseHeaders {
	return ResponseHeaders{SetCookie: response.header.Values("Set-Cookie"), Location: response.header.Get("Location")}
}

func capturedError(response *capturedResponse) error {
	var problem Problem
	if err := json.Unmarshal(response.body.Bytes(), &problem); err != nil || problem.Detail == "" {
		return &Problem{Code: "internal", Message: http.StatusText(response.status), Detail: http.StatusText(response.status), status: response.status}
	}
	problem.status = response.status
	return &problem
}

func OpenAPI() string {
	data, err := New(nil, "openapi").humaAPI().OpenAPI().YAML()
	if err != nil {
		panic(err)
	}
	return string(data)
}

func OpenAPIJSON() string {
	data, err := json.Marshal(New(nil, "openapi").humaAPI().OpenAPI())
	if err != nil {
		panic(err)
	}
	return string(data)
}
