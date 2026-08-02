package api

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
)

// instructionLogsTag groups the instruction log operations in the spec.
const instructionLogsTag = "Instruction Logs"

// serviceSecurityScheme names the API-key scheme documented on the ingest.
const serviceSecurityScheme = "serviceToken"

// serviceTokenHeader carries the shared token another Primer service presents
// when pushing data into the LMS.
const serviceTokenHeader = "X-Service-Token"

// DayFormat is the calendar-day form the ingest accepts. A viewing happened on
// a household day, not at a UTC instant: the producer has already decided
// which day that was in its own timezone, and sending a plain date keeps the
// LMS from second-guessing it.
const DayFormat = "2006-01-02"

// InstructionLogCreate records instructional time by hand.
type InstructionLogCreate struct {
	Source         *string    `json:"source,omitempty" db:"source" enum:"tv,manual" required:"false"`
	StudentID      *string    `json:"studentId,omitempty" db:"student_id" format:"uuid" required:"false"`
	MediaTitle     string     `json:"mediaTitle" db:"media_title" minLength:"1"`
	Class          string     `json:"class" db:"class" enum:"educational,mixed"`
	SubjectTags    *[]string  `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes  *[]string  `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	WatchedSeconds int        `json:"watchedSeconds" db:"watched_seconds" minimum:"0"`
	OccurredOn     *time.Time `json:"occurredOn,omitempty" db:"occurred_on" required:"false"`
	Notes          *string    `json:"notes,omitempty" db:"notes" required:"false"`
}

// InstructionLogUpdate corrects a logged entry.
type InstructionLogUpdate struct {
	StudentID      *string    `json:"studentId,omitempty" db:"student_id" format:"uuid" required:"false"`
	MediaTitle     *string    `json:"mediaTitle,omitempty" db:"media_title" required:"false"`
	Class          *string    `json:"class,omitempty" db:"class" enum:"educational,mixed" required:"false"`
	SubjectTags    *[]string  `json:"subjectTags,omitempty" db:"subject_tags" required:"false"`
	StandardCodes  *[]string  `json:"standardCodes,omitempty" db:"standard_codes" required:"false"`
	WatchedSeconds *int       `json:"watchedSeconds,omitempty" db:"watched_seconds" minimum:"0" required:"false"`
	OccurredOn     *time.Time `json:"occurredOn,omitempty" db:"occurred_on" required:"false"`
	Notes          *string    `json:"notes,omitempty" db:"notes" required:"false"`
}

// InstructionLogIngest is one finished viewing as another service reports it.
type InstructionLogIngest struct {
	Source         string   `json:"source,omitempty" enum:"tv,manual" default:"tv" required:"false" doc:"Producing service."`
	SourceRef      string   `json:"sourceRef" minLength:"1" doc:"The producer's stable identifier for this event — the TV server sends its playback session ID. Re-posting the same reference returns the log already recorded instead of counting the time twice."`
	StudentID      *string  `json:"studentId,omitempty" format:"uuid" required:"false"`
	MediaTitle     string   `json:"mediaTitle" minLength:"1"`
	Class          string   `json:"class" enum:"educational,mixed" doc:"Only educational and mixed viewing is instructional time; entertainment is refused."`
	SubjectTags    []string `json:"subjectTags,omitempty" required:"false"`
	StandardCodes  []string `json:"standardCodes,omitempty" required:"false"`
	WatchedSeconds int      `json:"watchedSeconds" minimum:"1"`
	OccurredOn     string   `json:"occurredOn" pattern:"^\\d{4}-\\d{2}-\\d{2}$" example:"2031-04-15" doc:"Calendar day the viewing happened, YYYY-MM-DD, in the producer's own timezone."`
	Notes          string   `json:"notes,omitempty" required:"false"`
}

// InstructionLogIngestResult reports what an ingest did.
type InstructionLogIngestResult struct {
	Log     domain.InstructionLog `json:"log"`
	Created bool                  `json:"created" doc:"False when this source reference had already been ingested, in which case the existing log is returned unchanged."`
}

// registerInstructionLogs wires the instruction log resource and the ingest
// endpoint other Primer services push to.
//
// The two write paths are deliberately separate. Generic create is the
// parent's hand entry through the admin SPA and shares the unauthenticated
// surface of every other LMS resource. Ingest is machine-to-machine: it is
// idempotent on a caller-supplied reference and it is the only endpoint
// carrying a credential, so a service token can be required in deployment
// without locking the parent out of the admin UI.
func registerInstructionLogs(h huma.API, q repo.Querier, opts Options) {
	RegisterCRUD[domain.InstructionLog, InstructionLogCreate, InstructionLogUpdate](
		h, q, repo.InstructionLogs, "instruction-log", "instruction-logs", "/instruction-logs")

	huma.Register(h, huma.Operation{
		OperationID:   "ingest-instruction-log",
		Method:        http.MethodPost,
		Path:          "/instruction-logs/ingest",
		Summary:       "Ingest instructional time from a service",
		Description:   "Records a finished viewing as instructional time. Idempotent on (source, sourceRef): a retry answers 200 with the existing log and created=false, so a producer whose own bookkeeping failed can safely try again. Entertainment viewing is not instructional time and is refused.",
		Tags:          []string{instructionLogsTag},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{serviceSecurityScheme: {}}},
		Middlewares:   huma.Middlewares{SharedSecretGuard(h, opts.ServiceToken, serviceTokenHeader, "service credentials required")},
		Errors:        []int{http.StatusUnauthorized, http.StatusUnprocessableEntity},
		// The replay answer is declared by hand: Huma documents only an
		// operation's default status, and a client needs to know that 200 is a
		// success carrying the log it already created.
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Already ingested. The existing log is returned unchanged and no time was added.",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: h.OpenAPI().Components.Schemas.Schema(
							reflect.TypeFor[InstructionLogIngestResult](), true, "InstructionLogIngestResult"),
					},
				},
			},
		},
	}, func(ctx context.Context, in *ingestInstructionLogInput) (*ingestInstructionLogOutput, error) {
		return ingestInstructionLog(ctx, q, in)
	})
}

// ingestInstructionLogInput wraps the ingest body.
type ingestInstructionLogInput struct {
	Body InstructionLogIngest
}

// ingestInstructionLogOutput carries the stored log. The status distinguishes
// a fresh record (201) from a replay of one already held (200), so a producer
// can tell whether its retry actually added hours.
type ingestInstructionLogOutput struct {
	Status int
	Body   InstructionLogIngestResult
}

func ingestInstructionLog(ctx context.Context, q repo.Querier, in *ingestInstructionLogInput) (*ingestInstructionLogOutput, error) {
	occurredOn, err := time.Parse(DayFormat, in.Body.OccurredOn)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("occurredOn must be a calendar day in YYYY-MM-DD form")
	}

	source := in.Body.Source
	if source == "" {
		source = domain.InstructionSourceTV
	}

	values := map[string]any{
		"source":          source,
		"source_ref":      in.Body.SourceRef,
		"media_title":     in.Body.MediaTitle,
		"class":           in.Body.Class,
		"subject_tags":    orEmpty(in.Body.SubjectTags),
		"standard_codes":  orEmpty(in.Body.StandardCodes),
		"watched_seconds": in.Body.WatchedSeconds,
		"occurred_on":     occurredOn,
		"notes":           in.Body.Notes,
	}
	if in.Body.StudentID != nil {
		values["student_id"] = *in.Body.StudentID
	}

	log, created, err := repo.IngestInstructionLog(ctx, q, values)
	if err != nil {
		return nil, MapError(err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &ingestInstructionLogOutput{
		Status: status,
		Body:   InstructionLogIngestResult{Log: *log, Created: created},
	}, nil
}

// orEmpty normalizes a nil slice to an empty one, since the columns are
// NOT NULL arrays.
func orEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
