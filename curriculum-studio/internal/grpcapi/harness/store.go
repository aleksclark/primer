// Package harness provides an in-memory harness store for CurriculumIntegrationService
// contract tests. It is test-only infrastructure; no production business logic lives here.
package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/aleksclark/primer/curriculum-studio/contracts/gen/go/curriculumstudio/v1"
	"github.com/aleksclark/primer/curriculum-studio/internal/boundary"
)

// Store is a goroutine-safe in-memory fixture store for the gRPC harness.
type Store struct {
	mu sync.Mutex

	// materializations keyed by id.
	materializations map[string]*v1.Materialization
	// bundles keyed by materialization id.
	bundles map[string]*v1.MaterializationBundle
	// idempotency records: key → idempotencyRecord.
	idempotencyRecords map[string]*idempotencyRecord
	// publishedRevisions keyed by id.
	publishedRevisions map[string]*v1.PlanRevision
	// graphs keyed by revision id.
	graphs map[string]*v1.PlanGraph
	// validationReports keyed by revision id.
	validationReports map[string]*v1.ValidationReport
	// standards keyed by code.
	standards map[string]*v1.Standard
	// resources keyed by id.
	resources map[string]*v1.Resource
	// events ordered by insertion.
	events []*v1.DomainEvent
	// acks keyed by "eventID/consumerID".
	acks map[string]bool
	// items keyed by id.
	items map[string]*v1.MaterializedItem
	// webhook deliveries recorded by the harness dispatcher.
	deliveries []WebhookDelivery
}

type idempotencyRecord struct {
	// The complete MaterializeRequest context used to detect conflicts.
	context *v1.MaterializationContext
	// The resulting materialization id.
	materializationID string
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		materializations:   make(map[string]*v1.Materialization),
		bundles:            make(map[string]*v1.MaterializationBundle),
		idempotencyRecords: make(map[string]*idempotencyRecord),
		publishedRevisions: make(map[string]*v1.PlanRevision),
		graphs:             make(map[string]*v1.PlanGraph),
		validationReports:  make(map[string]*v1.ValidationReport),
		standards:          make(map[string]*v1.Standard),
		resources:          make(map[string]*v1.Resource),
		events:             nil,
		acks:               make(map[string]bool),
		items:              make(map[string]*v1.MaterializedItem),
	}
}

// --- Materialization ---

// CreateMaterialization stores a new materialization (status REQUESTED) and
// records the idempotency key. Returns ErrConflict if the complete context
// differs from the first request using the key.
type ErrConflict struct{ Key string }

func (e *ErrConflict) Error() string { return fmt.Sprintf("idempotency conflict for key %q", e.Key) }

// CreateMaterializationResult is the output of CreateMaterialization.
type CreateMaterializationResult struct {
	Materialization *v1.Materialization
	AlreadyExisted  bool
}

// CreateMaterialization stores or returns an existing materialization under key.
// The context is cloned at the store boundary so callers cannot mutate the
// idempotency record or materialization after the request has been accepted.
func (s *Store) CreateMaterialization(key string, context *v1.MaterializationContext) (*CreateMaterializationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec, ok := s.idempotencyRecords[key]; ok {
		if !proto.Equal(rec.context, context) {
			return nil, &ErrConflict{Key: key}
		}
		mat := s.materializations[rec.materializationID]
		return &CreateMaterializationResult{Materialization: clone(mat), AlreadyExisted: true}, nil
	}

	storedContext := clone(context)
	id := fmt.Sprintf("mat_%d", len(s.materializations)+1)
	mat := &v1.Materialization{
		Id:             id,
		PlanRevisionId: storedContext.GetPlanRevisionId(),
		Status:         v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED,
		Context:        storedContext,
		CreatedAt:      timestamppb.New(time.Now().UTC()),
	}
	s.materializations[id] = mat
	s.idempotencyRecords[key] = &idempotencyRecord{
		context:           clone(storedContext),
		materializationID: id,
	}
	return &CreateMaterializationResult{Materialization: clone(mat)}, nil
}

// GetMaterialization returns a defensive copy of a materialization by id or nil.
func (s *Store) GetMaterialization(id string) *v1.Materialization {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.materializations[id])
}

// AdvanceStatus moves a materialization to the given status. It is used by
// tests to simulate the async LRO progression without a real materializer.
func (s *Store) AdvanceStatus(id string, status v1.MaterializationStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	mat, ok := s.materializations[id]
	if !ok {
		return false
	}
	mat.Status = status
	if status == v1.MaterializationStatus_MATERIALIZATION_STATUS_READY ||
		status == v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED ||
		status == v1.MaterializationStatus_MATERIALIZATION_STATUS_CANCELLED {
		mat.CompletedAt = timestamppb.New(time.Now().UTC())
	}
	return true
}

// SeedBundle stores a bundle for a ready materialization.
func (s *Store) SeedBundle(matID string, bundle *v1.MaterializationBundle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bundles[matID] = clone(bundle)
}

// GetBundle returns a defensive copy of the bundle for a materialization or nil.
func (s *Store) GetBundle(matID string) *v1.MaterializationBundle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.bundles[matID])
}

// --- Published revisions ---

// SeedPublishedRevision stores a published revision fixture.
func (s *Store) SeedPublishedRevision(rev *v1.PlanRevision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishedRevisions[rev.GetId()] = clone(rev)
}

// GetPublishedRevision returns a published revision or nil.
func (s *Store) GetPublishedRevision(id string) *v1.PlanRevision {
	s.mu.Lock()
	defer s.mu.Unlock()
	rev := s.publishedRevisions[id]
	if rev == nil {
		return nil
	}
	if rev.GetState() != v1.RevisionState_REVISION_STATE_PUBLISHED {
		return nil
	}
	return clone(rev)
}

// SeedPlanGraph stores a plan graph for a published revision.
func (s *Store) SeedPlanGraph(revisionID string, graph *v1.PlanGraph) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.graphs[revisionID] = clone(graph)
}

// GetPlanGraph returns a plan graph for a revision or nil.
func (s *Store) GetPlanGraph(revisionID string) *v1.PlanGraph {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.graphs[revisionID])
}

// SeedValidationReport stores a validation report for a published revision.
func (s *Store) SeedValidationReport(revisionID string, report *v1.ValidationReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validationReports[revisionID] = clone(report)
}

// GetValidationReport returns a validation report or builds an empty one.
func (s *Store) GetValidationReport(revisionID string) *v1.ValidationReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.validationReports[revisionID]; ok {
		return clone(r)
	}
	return &v1.ValidationReport{RevisionId: revisionID}
}

// --- Standards and resources ---

// SeedStandard stores a standard fixture by code.
func (s *Store) SeedStandard(std *v1.Standard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.standards[std.GetCode()] = clone(std)
}

// GetStandard returns a standard by code or nil.
func (s *Store) GetStandard(code string) *v1.Standard {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.standards[code])
}

// SeedResource stores a resource fixture.
func (s *Store) SeedResource(r *v1.Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[r.GetId()] = clone(r)
}

// GetResource returns a resource by id or nil.
func (s *Store) GetResource(id string) *v1.Resource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.resources[id])
}

// --- Events ---

// SeedEvent appends a domain event to the outbox.
func (s *Store) SeedEvent(ev *v1.DomainEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, clone(ev))
}

// PullEvents returns events that have not been acknowledged by consumerID.
// If types is non-empty, only events of those types are returned.
func (s *Store) PullEvents(consumerID string, types []v1.EventType, pageSize int32) []*v1.DomainEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pageSize <= 0 {
		pageSize = 25
	}
	typeSet := make(map[v1.EventType]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	var out []*v1.DomainEvent
	for _, ev := range s.events {
		if len(typeSet) > 0 && !typeSet[ev.GetType()] {
			continue
		}
		ackKey := ackKey(ev.GetId(), consumerID)
		if s.acks[ackKey] {
			continue
		}
		out = append(out, clone(ev))
		if int32(len(out)) >= pageSize {
			break
		}
	}
	return out
}

// AcknowledgeEvent marks an event as durable-acked for consumerID.
func (s *Store) AcknowledgeEvent(eventID, consumerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acks[ackKey(eventID, consumerID)] = true
}

// IsAcknowledged reports whether an event has been acked by consumerID.
func (s *Store) IsAcknowledged(eventID, consumerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acks[ackKey(eventID, consumerID)]
}

// WebhookDelivery is the durable attempt record owned by the harness. The
// production dispatcher and lease policy are platform/database concerns.
type WebhookDelivery struct {
	EndpointID  string
	EventID     string
	HTTPStatus  int
	Attempt     int
	AttemptedAt time.Time
}

// DeliverEvent posts an envelope to a test subscriber and records the attempt.
// It exercises the C9 header contract without introducing a production bus.
func (s *Store) DeliverEvent(ctx context.Context, endpointID, endpointURL string, event *v1.DomainEvent, secret []byte) (WebhookDelivery, error) {
	payload, err := protojson.Marshal(event)
	if err != nil {
		return WebhookDelivery{}, err
	}
	headers, err := boundary.EventDeliveryHeaders(event, payload, secret)
	if err != nil {
		return WebhookDelivery{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, io.NopCloser(bytes.NewReader(payload)))
	if err != nil {
		return WebhookDelivery{}, err
	}
	req.Header = headers
	resp, err := http.DefaultClient.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		_ = resp.Body.Close()
	}
	delivery := WebhookDelivery{EndpointID: endpointID, EventID: event.GetId(), HTTPStatus: status, Attempt: 1, AttemptedAt: time.Now().UTC()}
	s.mu.Lock()
	s.deliveries = append(s.deliveries, delivery)
	s.mu.Unlock()
	if err != nil {
		return delivery, err
	}
	return delivery, nil
}

// Deliveries returns a snapshot of recorded delivery attempts.
func (s *Store) Deliveries() []WebhookDelivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]WebhookDelivery(nil), s.deliveries...)
}

func ackKey(eventID, consumerID string) string {
	return eventID + "/" + consumerID
}

// clone keeps protobuf values crossing the store boundary immutable. Without
// this, a handler or test could mutate a pointer after the store lock is
// released and race with another RPC.
func clone[T proto.Message](msg T) T {
	value := reflect.ValueOf(msg)
	if !value.IsValid() || value.IsNil() {
		return msg
	}
	return proto.Clone(msg).(T)
}

// --- Fixture helpers ---

// FixtureBundle builds a minimal ready bundle with Struct tutor_context for tests.
func FixtureBundle(matID, revID string) *v1.MaterializationBundle {
	tc, _ := structpb.NewStruct(map[string]any{
		"version":    "1.0",
		"subject":    "math",
		"grade":      6,
		"objectives": []any{"TN.MATH.6.RP.A.3"},
	})
	return &v1.MaterializationBundle{
		Id:             matID,
		PlanRevisionId: revID,
		Sessions: []*v1.SessionSpec{
			{
				Id:                 "sess_1",
				TargetOutcomes:     []string{"TN.MATH.6.RP.A.3"},
				Activities:         []string{"ratio-warm-up", "proportional-table"},
				TutorContext:       tc,
				CompletionCriteria: []string{"solve 3 ratio problems correctly"},
			},
		},
	}
}

// FixtureRevision builds a published revision fixture.
func FixtureRevision(id, curriculumID string) *v1.PlanRevision {
	return &v1.PlanRevision{
		Id:             id,
		CurriculumId:   curriculumID,
		State:          v1.RevisionState_REVISION_STATE_PUBLISHED,
		RevisionNumber: 1,
		Brief: &v1.CurriculumBrief{
			LearnerLevel: "6",
			Subjects:     []string{"math"},
			TimeHorizon:  "semester",
		},
		PublishedAt: timestamppb.New(time.Now().UTC().Add(-24 * time.Hour)),
		CreatedAt:   timestamppb.New(time.Now().UTC().Add(-48 * time.Hour)),
		UpdatedAt:   timestamppb.New(time.Now().UTC().Add(-24 * time.Hour)),
	}
}

// FixtureGraph builds a minimal plan graph for a revision.
func FixtureGraph(revisionID string) *v1.PlanGraph {
	return &v1.PlanGraph{
		RevisionId: revisionID,
		Nodes: []*v1.PlanNode{
			{Id: "node_1", Kind: v1.NodeKind_NODE_KIND_UNIT, Title: "Ratios & Proportional Relationships"},
		},
	}
}

// FixtureEvent builds a domain event fixture.
func FixtureEvent(id string, evType v1.EventType, workspaceID, revisionID string) *v1.DomainEvent {
	return &v1.DomainEvent{
		Id:          id,
		Type:        evType,
		WorkspaceId: workspaceID,
		RevisionId:  revisionID,
		OccurredAt:  timestamppb.New(time.Now().UTC()),
	}
}
