package api

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"net/http"
)

type TaskListInput2 struct {
	Q      string `query:"q"`
	Limit  int    `query:"limit"`
	Offset int    `query:"offset"`
	Sort   string `query:"sort"`
	Dir    string `query:"dir"`
	Status string `query:"status"`
}
type TaskInputEnvelope2 struct {
	Body TaskInput2 `required:"true"`
}
type ScheduleInputEnvelope2 struct {
	Body ScheduleInput2 `required:"true"`
}
type ScheduleUpdateEnvelope2 struct {
	ID   string         `path:"id"`
	Body ScheduleInput2 `required:"true"`
}
type DecisionInputEnvelope2 struct {
	ID   string         `path:"id"`
	Body DecisionInput2 `required:"true"`
}
type OccurrenceIDInput2 struct {
	ID string `path:"id"`
}
type TaskIDInput2 struct {
	ID string `path:"id"`
}
type TaskPageOutput2 struct {
	ResponseHeaders
	Body TaskPage2
}
type TaskOutput2 struct {
	ResponseHeaders
	Body TaskRevision
}
type ScheduleOutput2 struct {
	ResponseHeaders
	Body Schedule2
}
type SchedulePageOutput2 struct {
	ResponseHeaders
	Body map[string]any
}
type OccurrencePageOutput2 struct {
	ResponseHeaders
	Body OccurrencePage2
}
type OccurrenceOutput2 struct {
	ResponseHeaders
	Body Occurrence2
}
type GenericJSONOutput2 struct {
	ResponseHeaders
	Body map[string]any
}

func (s *Server) registerPhase2(api huma.API) {
	register(api, huma.Operation{OperationID: "tasks-list", Method: http.MethodGet, Path: "/tasks", Errors: []int{401, 500}}, func(ctx context.Context, _ *TaskListInput2) (*TaskPageOutput2, error) {
		b, h, e := legacyJSON[TaskPage2](ctx, s.requireParent(s.listTasks2), nil)
		return &TaskPageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "tasks-create", Method: http.MethodPost, Path: "/tasks", DefaultStatus: 201, Errors: []int{400, 401, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *TaskInputEnvelope2) (*TaskOutput2, error) {
		b, h, e := legacyJSON[TaskRevision](ctx, s.requireParent(s.createTask2), in.Body)
		return &TaskOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "tasks-revise", Method: http.MethodPost, Path: "/tasks/{id}/revisions", DefaultStatus: 201, Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, in *TaskInputEnvelope2) (*TaskOutput2, error) {
		b, h, e := legacyJSON[TaskRevision](ctx, s.requireParent(s.reviseTask2), in.Body)
		return &TaskOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "tasks-publish", Method: http.MethodPost, Path: "/tasks/{id}/publish", Errors: []int{400, 401, 404, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *TaskIDInput2) (*TaskOutput2, error) {
		b, h, e := legacyJSON[TaskRevision](ctx, s.requireParent(s.publishTask2), nil)
		return &TaskOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "tasks-retire", Method: http.MethodPost, Path: "/tasks/{id}/retire", DefaultStatus: 204, Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *TaskIDInput2) (*NoContentOutput, error) {
		h, e := legacyEmpty(ctx, s.requireParent(s.retireTask2), nil)
		return &NoContentOutput{h}, e
	})
	register(api, huma.Operation{OperationID: "schedules-list", Method: http.MethodGet, Path: "/schedules", Errors: []int{401, 500}}, func(ctx context.Context, _ *TaskListInput2) (*SchedulePageOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.listSchedules2), nil)
		return &SchedulePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "schedules-create", Method: http.MethodPost, Path: "/schedules", DefaultStatus: 201, Errors: []int{400, 401, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *ScheduleInputEnvelope2) (*ScheduleOutput2, error) {
		b, h, e := legacyJSON[Schedule2](ctx, s.requireParent(s.createSchedule2), in.Body)
		return &ScheduleOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "schedules-update", Method: http.MethodPatch, Path: "/schedules/{id}", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, in *ScheduleUpdateEnvelope2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.updateSchedule2), in.Body)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "schedules-retire", Method: http.MethodDelete, Path: "/schedules/{id}", DefaultStatus: 204, Errors: []int{401, 404, 500}, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*NoContentOutput, error) {
		h, e := legacyEmpty(ctx, s.requireParent(s.retireSchedule2), nil)
		return &NoContentOutput{h}, e
	})
	register(api, huma.Operation{OperationID: "occurrences-list", Method: http.MethodGet, Path: "/occurrences", Errors: []int{401, 500}}, func(ctx context.Context, _ *TaskListInput2) (*OccurrencePageOutput2, error) {
		b, h, e := legacyJSON[OccurrencePage2](ctx, s.requireParent(s.listOccurrences2), nil)
		return &OccurrencePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-get", Method: http.MethodGet, Path: "/occurrences/{id}", Errors: []int{401, 404, 500}}, func(ctx context.Context, _ *OccurrenceIDInput2) (*OccurrenceOutput2, error) {
		b, h, e := legacyJSON[Occurrence2](ctx, s.requireParent(s.parentGetOccurrence2), nil)
		return &OccurrenceOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-approve", Method: http.MethodPost, Path: "/occurrences/{id}/decision", Errors: []int{400, 401, 409, 500}, SkipValidateBody: true}, func(ctx context.Context, in *DecisionInputEnvelope2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.decideOccurrence2), in.Body)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-retry", Method: http.MethodPost, Path: "/occurrences/{id}/retry", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(s.retryOccurrence2), nil)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-skip", Method: http.MethodPost, Path: "/occurrences/{id}/skip", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(func(w http.ResponseWriter, r *http.Request, sc scope) { s.setOccurrenceStatus2(w, r, sc, "excused") }), nil)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "occurrence-cancel", Method: http.MethodPost, Path: "/occurrences/{id}/cancel", Errors: []int{400, 401, 404, 409, 500}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireParent(func(w http.ResponseWriter, r *http.Request, sc scope) { s.setOccurrenceStatus2(w, r, sc, "canceled") }), nil)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-today", Method: http.MethodGet, Path: "/student/today", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*OccurrencePageOutput2, error) {
		b, h, e := legacyJSON[OccurrencePage2](ctx, s.requireStudent(s.studentListWrapper), nil)
		return &OccurrencePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-upcoming", Method: http.MethodGet, Path: "/student/upcoming", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*OccurrencePageOutput2, error) {
		b, h, e := legacyJSON[OccurrencePage2](ctx, s.requireStudent(s.studentListWrapper), nil)
		return &OccurrencePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-occurrence", Method: http.MethodGet, Path: "/student/occurrences/{id}", Errors: []int{401, 404}}, func(ctx context.Context, _ *OccurrenceIDInput2) (*OccurrenceOutput2, error) {
		b, h, e := legacyJSON[Occurrence2](ctx, s.requireStudent(s.studentDetail2), nil)
		return &OccurrenceOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "student-occurrence-start", Method: http.MethodPost, Path: "/student/occurrences/{id}/start", Errors: []int{400, 401, 404, 409}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireStudent(s.studentStart2), nil)
		return &GenericJSONOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "device-today", Method: http.MethodGet, Path: "/device/today", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*OccurrencePageOutput2, error) {
		b, h, e := legacyJSON[OccurrencePage2](ctx, s.requireDevice(s.deviceListWrapper), nil)
		return &OccurrencePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "device-upcoming", Method: http.MethodGet, Path: "/device/upcoming", Errors: []int{401}}, func(ctx context.Context, _ *struct{}) (*OccurrencePageOutput2, error) {
		b, h, e := legacyJSON[OccurrencePage2](ctx, s.requireDevice(s.deviceListWrapper), nil)
		return &OccurrencePageOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "device-occurrence", Method: http.MethodGet, Path: "/device/occurrences/{id}", Errors: []int{401, 404}}, func(ctx context.Context, _ *OccurrenceIDInput2) (*OccurrenceOutput2, error) {
		b, h, e := legacyJSON[Occurrence2](ctx, s.requireDevice(s.deviceDetailWrapper), nil)
		return &OccurrenceOutput2{h, b}, e
	})
	register(api, huma.Operation{OperationID: "device-occurrence-start", Method: http.MethodPost, Path: "/device/occurrences/{id}/start", Errors: []int{400, 401, 404, 409}, SkipValidateBody: true, SkipValidateParams: true}, func(ctx context.Context, _ *OccurrenceIDInput2) (*GenericJSONOutput2, error) {
		b, h, e := legacyJSON[map[string]any](ctx, s.requireDevice(s.deviceStartWrapper), nil)
		return &GenericJSONOutput2{h, b}, e
	})
}
