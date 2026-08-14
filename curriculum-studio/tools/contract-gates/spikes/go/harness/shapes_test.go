//go:build spike

// Package spiketest implements C3 hard-type qualification against generated stubs.
// Build/run only via spikes/run_spikes.sh (gen path is build-only).
package spiketest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "spike.local/gen"
)

const bufSize = 1024 * 1024

func fixturesDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	// .../spikes/go/harness/x.go -> .../spikes/fixtures
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "fixtures"))
}

func evidenceDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "evidence"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeEvidence(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join(evidenceDir(t), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// compactJSON re-encodes JSON with encoding/json defaults so evidence is
// byte-stable across protojson Indent/spacing drift between toolchains.
func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("compactJSON: %v (raw=%s)", err, raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- gRPC integration harness (in-process) ---------------------------------

type matState struct {
	mu     sync.Mutex
	byKey  map[string]*v1.Materialization
	byID   map[string]*v1.Materialization
	bundle map[string]*v1.MaterializationBundle
	items  map[string][]*v1.MaterializedItem
	seq    int
}

func newMatState() *matState {
	return &matState{
		byKey:  map[string]*v1.Materialization{},
		byID:   map[string]*v1.Materialization{},
		bundle: map[string]*v1.MaterializationBundle{},
		items:  map[string][]*v1.MaterializedItem{},
	}
}

type integrationServer struct {
	v1.UnimplementedCurriculumIntegrationServiceServer
	state *matState
}

func (s *integrationServer) Materialize(ctx context.Context, req *v1.MaterializeRequest) (*v1.Materialization, error) {
	if req.GetIdempotencyKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key required")
	}
	if req.GetContext() == nil || req.GetContext().GetPlanRevisionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "context.plan_revision_id required")
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if existing, ok := s.state.byKey[req.IdempotencyKey]; ok {
		return proto.Clone(existing).(*v1.Materialization), nil
	}
	s.state.seq++
	id := fmt.Sprintf("mat_spike_%d", s.state.seq)
	fp := fmt.Sprintf("sha256:spike-%d", s.state.seq)
	m := &v1.Materialization{
		Id:                 id,
		PlanRevisionId:     req.Context.PlanRevisionId,
		ContextFingerprint: fp,
		Status:             v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED,
		Context:            req.Context,
		CreatedAt:          timestamppb.Now(),
	}
	s.state.byKey[req.IdempotencyKey] = m
	s.state.byID[id] = m
	return proto.Clone(m).(*v1.Materialization), nil
}

func (s *integrationServer) GetMaterialization(ctx context.Context, req *v1.GetMaterializationRequest) (*v1.Materialization, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	m, ok := s.state.byID[req.GetMaterializationId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "materialization not found")
	}
	return proto.Clone(m).(*v1.Materialization), nil
}

func (s *integrationServer) GetMaterializationBundle(ctx context.Context, req *v1.GetMaterializationBundleRequest) (*v1.MaterializationBundle, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	m, ok := s.state.byID[req.GetMaterializationId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "materialization not found")
	}
	if m.Status != v1.MaterializationStatus_MATERIALIZATION_STATUS_READY {
		st := status.New(codes.FailedPrecondition, "bundle not ready")
		st, _ = st.WithDetails(&v1.ErrorDetail{
			Code:     v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION,
			Location: "materialization.status",
			Message:  "bundle available only when status is ready",
		})
		return nil, st.Err()
	}
	b, ok := s.state.bundle[m.Id]
	if !ok {
		return nil, status.Error(codes.Internal, "missing bundle")
	}
	return proto.Clone(b).(*v1.MaterializationBundle), nil
}

func (s *integrationServer) ListMaterializedItems(ctx context.Context, req *v1.ListMaterializedItemsRequest) (*v1.ListMaterializedItemsResponse, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	items := s.state.items[req.GetMaterializationId()]
	if items == nil {
		items = []*v1.MaterializedItem{}
	}
	pageSize := int(req.GetPage().GetPageSize())
	if pageSize <= 0 {
		pageSize = 25
	}
	offset := 0
	if tok := req.GetPage().GetPageToken(); tok != "" {
		fmt.Sscanf(tok, "%d", &offset)
	}
	end := offset + pageSize
	if end > len(items) {
		end = len(items)
	}
	var page []*v1.MaterializedItem
	if offset < len(items) {
		page = items[offset:end]
	}
	next := ""
	if end < len(items) {
		next = fmt.Sprintf("%d", end)
	}
	out := make([]*v1.MaterializedItem, len(page))
	for i, it := range page {
		out[i] = proto.Clone(it).(*v1.MaterializedItem)
	}
	return &v1.ListMaterializedItemsResponse{
		Items: out,
		Page: &v1.PageResponse{
			NextPageToken: next,
			TotalCount:    int64(len(items)),
		},
	}, nil
}

func (s *integrationServer) advance(id string, st v1.MaterializationStatus, withBundle bool, tutor map[string]any) error {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	m, ok := s.state.byID[id]
	if !ok {
		return fmt.Errorf("missing mat %s", id)
	}
	m.Status = st
	if st == v1.MaterializationStatus_MATERIALIZATION_STATUS_READY || st == v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED {
		m.CompletedAt = timestamppb.Now()
	}
	if st == v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED {
		m.ErrorMessage = "spike failure"
	}
	if withBundle && st == v1.MaterializationStatus_MATERIALIZATION_STATUS_READY {
		tc, err := structpb.NewStruct(tutor)
		if err != nil {
			return err
		}
		s.state.bundle[id] = &v1.MaterializationBundle{
			Id:                 id,
			PlanRevisionId:     m.PlanRevisionId,
			ContextFingerprint: m.ContextFingerprint,
			Sessions: []*v1.SessionSpec{{
				Id:           "sess_1",
				TutorContext: tc,
			}},
		}
		s.state.items[id] = []*v1.MaterializedItem{
			{Id: "mit_1", MaterializationId: id, Kind: v1.MaterializedItemKind_MATERIALIZED_ITEM_KIND_LESSON, Title: "L1", Status: v1.MaterializedItemStatus_MATERIALIZED_ITEM_STATUS_READY, LockState: v1.ItemLockState_ITEM_LOCK_STATE_EDITABLE},
			{Id: "mit_2", MaterializationId: id, Kind: v1.MaterializedItemKind_MATERIALIZED_ITEM_KIND_ASSESSMENT, Title: "A1", Status: v1.MaterializedItemStatus_MATERIALIZED_ITEM_STATUS_READY, LockState: v1.ItemLockState_ITEM_LOCK_STATE_EDITABLE},
			{Id: "mit_3", MaterializationId: id, Kind: v1.MaterializedItemKind_MATERIALIZED_ITEM_KIND_SESSION_SPEC, Title: "S1", Status: v1.MaterializedItemStatus_MATERIALIZED_ITEM_STATUS_READY, LockState: v1.ItemLockState_ITEM_LOCK_STATE_EDITABLE},
			{Id: "mit_4", MaterializationId: id, Kind: v1.MaterializedItemKind_MATERIALIZED_ITEM_KIND_RUBRIC, Title: "R1", Status: v1.MaterializedItemStatus_MATERIALIZED_ITEM_STATUS_DRAFT, LockState: v1.ItemLockState_ITEM_LOCK_STATE_EDITABLE},
			{Id: "mit_5", MaterializationId: id, Kind: v1.MaterializedItemKind_MATERIALIZED_ITEM_KIND_ANSWER_KEY, Title: "K1", Status: v1.MaterializedItemStatus_MATERIALIZED_ITEM_STATUS_DRAFT, LockState: v1.ItemLockState_ITEM_LOCK_STATE_LOCKED},
		}
	}
	return nil
}

func startGRPC(t *testing.T) (v1.CurriculumIntegrationServiceClient, *integrationServer, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	impl := &integrationServer{state: newMatState()}
	v1.RegisterCurriculumIntegrationServiceServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := v1.NewCurriculumIntegrationServiceClient(conn)
	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return client, impl, cleanup
}

// --- Shape tests -----------------------------------------------------------

func TestSpikeNullableOptionalPresence(t *testing.T) {
	// Proto3: unset timestamp end has no presence in JSON omit; set end round-trips.
	w := &v1.MaterializationWindow{
		Start:            timestamppb.New(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)),
		AvailableMinutes: 45,
	}
	if w.ProtoReflect().Has(w.ProtoReflect().Descriptor().Fields().ByName("end")) {
		t.Fatal("end should be absent")
	}
	b, err := protojson.MarshalOptions{EmitUnpopulated: false}.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["end"]; ok {
		t.Fatalf("JSON should omit absent end, got %s", b)
	}

	end := timestamppb.New(time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	w.End = end
	if !w.ProtoReflect().Has(w.ProtoReflect().Descriptor().Fields().ByName("end")) {
		t.Fatal("end should be present")
	}
	b2, err := protojson.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	w2 := &v1.MaterializationWindow{}
	if err := protojson.Unmarshal(b2, w2); err != nil {
		t.Fatal(err)
	}
	if !w2.GetEnd().AsTime().Equal(end.AsTime()) {
		t.Fatalf("end round-trip mismatch: %v vs %v", w2.GetEnd(), end)
	}

	// REST authoring window: omit end vs include end in JSON object.
	type authoringWindow struct {
		Start            *string `json:"start,omitempty"`
		End              *string `json:"end,omitempty"`
		AvailableMinutes int32   `json:"availableMinutes"`
	}
	absent, _ := json.Marshal(authoringWindow{AvailableMinutes: 45})
	var am map[string]any
	_ = json.Unmarshal(absent, &am)
	if _, ok := am["end"]; ok {
		t.Fatal("REST JSON should omit end when nil")
	}
	endStr := "2026-08-08T00:00:00Z"
	present, _ := json.Marshal(authoringWindow{End: &endStr, AvailableMinutes: 45})
	_ = json.Unmarshal(present, &am)
	if am["end"] != endStr {
		t.Fatalf("REST end present mismatch: %v", am["end"])
	}

	// Intentional failing fixture teeth: claiming null and absent are identical without policy is wrong.
	// Document: OpenAPI date-time fields are optional (absent) not nullable null in current baseline.
	writeEvidence(t, "nullable_optional.txt", fmt.Sprintf(
		"proto_end_absent_json=%s\nproto_end_present_json=%s\nrest_absent=%s\nrest_present=%s\npolicy=OpenAPI AuthoringWindow.end is optional string date-time (absent), not typed null\n",
		compactJSON(t, b), compactJSON(t, b2), string(absent), string(present),
	))
}

func TestSpikeTimestampDuration(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixturesDir(t), "timestamp_duration.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		ValidRFC3339     string `json:"valid_rfc3339"`
		ValidWithOffset  string `json:"valid_with_offset"`
		NormalizedUTC    string `json:"normalized_utc"`
		Invalid          string `json:"invalid"`
		AvailableMinutes int32  `json:"available_minutes"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, fx.ValidWithOffset)
	if err != nil {
		t.Fatal(err)
	}
	norm := ts.UTC().Format(time.RFC3339)
	if norm != fx.NormalizedUTC {
		t.Fatalf("UTC normalize: got %s want %s", norm, fx.NormalizedUTC)
	}
	msg := &v1.MaterializationWindow{
		Start:            timestamppb.New(ts.UTC()),
		AvailableMinutes: fx.AvailableMinutes,
	}
	bin, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	msg2 := &v1.MaterializationWindow{}
	if err := proto.Unmarshal(bin, msg2); err != nil {
		t.Fatal(err)
	}
	if msg2.GetAvailableMinutes() != fx.AvailableMinutes {
		t.Fatalf("minutes: %d", msg2.GetAvailableMinutes())
	}
	if !msg2.GetStart().AsTime().Equal(ts.UTC()) {
		t.Fatal("timestamp binary round-trip failed")
	}
	// Invalid REST timestamp should fail typed parse (not silent zero).
	_, err = time.Parse(time.RFC3339, fx.Invalid)
	if err == nil {
		t.Fatal("invalid timestamp should error")
	}
	writeEvidence(t, "timestamp_duration.txt", fmt.Sprintf("normalized=%s minutes=%d invalid_err=%v\n", norm, fx.AvailableMinutes, err))
}

func TestSpikeTypedErrorsGRPCAndREST(t *testing.T) {
	// gRPC: ErrorDetail in status details
	st := status.New(codes.FailedPrecondition, "bundle not ready")
	st, err := st.WithDetails(&v1.ErrorDetail{
		Code:     v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION,
		Location: "materialization.status",
		Message:  "not ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := status.Convert(st.Err())
	var code v1.ErrorCode
	var loc string
	for _, d := range got.Details() {
		if ed, ok := d.(*v1.ErrorDetail); ok {
			code = ed.Code
			loc = ed.Location
		}
	}
	if code != v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION || loc != "materialization.status" {
		t.Fatalf("grpc detail missing: code=%v loc=%s", code, loc)
	}

	// REST problem+json shape
	type problem struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
		Errors []struct {
			Code     string `json:"code"`
			Location string `json:"location"`
			Message  string `json:"message"`
		} `json:"errors"`
	}
	p := problem{
		Type:   "about:blank",
		Title:  "Failed Precondition",
		Status: 409,
		Detail: "bundle not ready",
		Errors: []struct {
			Code     string `json:"code"`
			Location string `json:"location"`
			Message  string `json:"message"`
		}{{Code: "failed_precondition", Location: "materialization.status", Message: "not ready"}},
	}
	body, _ := json.Marshal(p)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(409)
		_, _ = w.Write(body)
	})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/studio/v1/materializations/x/bundle", nil))
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Fatalf("content-type %s", ct)
	}
	var parsed problem
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Errors) != 1 || parsed.Errors[0].Code != "failed_precondition" {
		t.Fatalf("rest errors: %+v", parsed.Errors)
	}

	// Unknown code policy: fail closed to internal for wire consumers
	unknown := "not_a_real_code"
	mapped := mapUnknown(unknown)
	if mapped != "internal" {
		t.Fatalf("unknown policy want internal got %s", mapped)
	}
	writeEvidence(t, "typed_errors.txt", fmt.Sprintf("grpc_code=%v rest=%s unknown=%s\n", code, rr.Body.String(), mapped))
}

func mapUnknown(code string) string {
	known := map[string]struct{}{
		"unauthenticated": {}, "permission_denied": {}, "not_found": {}, "already_exists": {},
		"failed_precondition": {}, "aborted": {}, "invalid_argument": {}, "resource_locked": {},
		"revision_immutable": {}, "validation_failed": {}, "materialization_failed": {},
		"idempotency_key_conflict": {}, "quota_exceeded": {}, "internal": {},
	}
	if _, ok := known[code]; ok {
		return code
	}
	return "internal"
}

func TestSpikePaginationRESTAndGRPC(t *testing.T) {
	client, impl, cleanup := startGRPC(t)
	defer cleanup()
	ctx := context.Background()
	m, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: "page-1",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := impl.advance(m.Id, v1.MaterializationStatus_MATERIALIZATION_STATUS_READY, true, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	var all []string
	token := ""
	for {
		resp, err := client.ListMaterializedItems(ctx, &v1.ListMaterializedItemsRequest{
			MaterializationId: m.Id,
			Page:              &v1.PageRequest{PageSize: 2, PageToken: token},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range resp.Items {
			all = append(all, it.Id)
		}
		token = resp.GetPage().GetNextPageToken()
		if token == "" {
			break
		}
	}
	if len(all) != 5 {
		t.Fatalf("grpc walk want 5 got %d %v", len(all), all)
	}

	// REST limit/offset PageMeta — must match OpenAPI PageMeta wire (totalCount).
	// Hand-spelled json:"total" false-greens P3-S4/E3-04 against the authoritative contract.
	rawFx, err := os.ReadFile(filepath.Join(fixturesDir(t), "pagination.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		REST struct {
			PageSize   int      `json:"page_size"`
			TotalCount int      `json:"totalCount"`
			Items      []string `json:"items"`
		} `json:"rest"`
	}
	if err := json.Unmarshal(rawFx, &fx); err != nil {
		t.Fatal(err)
	}
	if fx.REST.TotalCount != 5 || len(fx.REST.Items) != 5 {
		t.Fatalf("pagination fixture: totalCount=%d items=%d", fx.REST.TotalCount, len(fx.REST.Items))
	}

	// Authoritative OpenAPI PageMeta wire spelling (contracts/openapi/... PageMeta.totalCount).
	type pageMeta struct {
		Limit      int `json:"limit"`
		Offset     int `json:"offset"`
		TotalCount int `json:"totalCount"`
	}
	items := fx.REST.Items
	var restAll []string
	for offset := 0; offset < len(items); {
		limit := fx.REST.PageSize
		if limit <= 0 {
			limit = 2
		}
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		chunk := items[offset:end]
		restAll = append(restAll, chunk...)
		meta := pageMeta{Limit: limit, Offset: offset, TotalCount: len(items)}
		b, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(b, &wire); err != nil {
			t.Fatal(err)
		}
		if _, ok := wire["total"]; ok {
			t.Fatalf("OpenAPI PageMeta must not emit %q; got %s", "total", b)
		}
		if _, ok := wire["totalCount"]; !ok {
			t.Fatalf("OpenAPI PageMeta must emit totalCount; got %s", b)
		}
		// Round-trip through the same authoritative field spelling (fixture + OpenAPI).
		var m2 pageMeta
		if err := json.Unmarshal(b, &m2); err != nil {
			t.Fatal(err)
		}
		if m2.TotalCount != fx.REST.TotalCount {
			t.Fatalf("page meta totalCount roundtrip: got %d want %d (wire=%s)", m2.TotalCount, fx.REST.TotalCount, b)
		}
		// Also accept authoritative fixture JSON blob as PageMeta wire seed.
		seed, err := json.Marshal(map[string]any{
			"limit":      limit,
			"offset":     offset,
			"totalCount": fx.REST.TotalCount,
		})
		if err != nil {
			t.Fatal(err)
		}
		var fromFixture pageMeta
		if err := json.Unmarshal(seed, &fromFixture); err != nil {
			t.Fatal(err)
		}
		if fromFixture.TotalCount != fx.REST.TotalCount {
			t.Fatalf("fixture PageMeta totalCount: got %d want %d", fromFixture.TotalCount, fx.REST.TotalCount)
		}
		offset = end
	}
	if !reflect.DeepEqual(restAll, items) {
		t.Fatalf("rest walk %v", restAll)
	}
	writeEvidence(t, "pagination.txt", fmt.Sprintf("grpc_ids=%v rest=%v totalCount=%d\n", all, restAll, fx.REST.TotalCount))
}

func TestSpikeLongRunningMaterialization(t *testing.T) {
	client, impl, cleanup := startGRPC(t)
	defer cleanup()
	ctx := context.Background()
	key := "idem-spike-1"
	m1, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: key,
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_spike_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m1.Status != v1.MaterializationStatus_MATERIALIZATION_STATUS_REQUESTED {
		t.Fatalf("status %v", m1.Status)
	}
	// create response is not a bundle
	_, err = client.GetMaterializationBundle(ctx, &v1.GetMaterializationBundleRequest{MaterializationId: m1.Id})
	if err == nil {
		t.Fatal("expected failed_precondition before ready")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition got %v", err)
	}
	var appCode v1.ErrorCode
	for _, d := range st.Details() {
		if ed, ok := d.(*v1.ErrorDetail); ok {
			appCode = ed.Code
		}
	}
	if appCode != v1.ErrorCode_ERROR_CODE_FAILED_PRECONDITION {
		t.Fatalf("app code %v", appCode)
	}

	_ = impl.advance(m1.Id, v1.MaterializationStatus_MATERIALIZATION_STATUS_RUNNING, false, nil)
	cur, err := client.GetMaterialization(ctx, &v1.GetMaterializationRequest{MaterializationId: m1.Id})
	if err != nil || cur.Status != v1.MaterializationStatus_MATERIALIZATION_STATUS_RUNNING {
		t.Fatalf("running: %v %v", cur, err)
	}
	tutor := map[string]any{"schema_version": "tutor_context.v1", "nested": map[string]any{"x": 1.0}}
	_ = impl.advance(m1.Id, v1.MaterializationStatus_MATERIALIZATION_STATUS_READY, true, tutor)
	bundle, err := client.GetMaterializationBundle(ctx, &v1.GetMaterializationBundleRequest{MaterializationId: m1.Id})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Id != m1.Id || len(bundle.Sessions) != 1 {
		t.Fatalf("bundle %+v", bundle)
	}

	// idempotent same key
	m2, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: key,
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_spike_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m2.Id != m1.Id {
		t.Fatalf("idempotency broken: %s vs %s", m1.Id, m2.Id)
	}

	// failure path
	m3, err := client.Materialize(ctx, &v1.MaterializeRequest{
		IdempotencyKey: "idem-fail",
		Context:        &v1.MaterializationContext{PlanRevisionId: "prev_x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = impl.advance(m3.Id, v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED, false, nil)
	failed, _ := client.GetMaterialization(ctx, &v1.GetMaterializationRequest{MaterializationId: m3.Id})
	if failed.Status != v1.MaterializationStatus_MATERIALIZATION_STATUS_FAILED {
		t.Fatal("failure path")
	}
	writeEvidence(t, "lro.txt", fmt.Sprintf("id=%s idempotent=%v fail=%v\n", m1.Id, m2.Id == m1.Id, failed.Status))
}

func TestSpikeStructTutorContextAndEventData(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixturesDir(t), "struct_roundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		TutorContext map[string]any `json:"tutor_context"`
		EventData    map[string]any `json:"event_data"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	// Normalize numbers to float64 like JSON
	tc, err := structpb.NewStruct(fx.TutorContext)
	if err != nil {
		t.Fatal(err)
	}
	sess := &v1.SessionSpec{Id: "s1", TutorContext: tc}
	bin, err := proto.Marshal(sess)
	if err != nil {
		t.Fatal(err)
	}
	sess2 := &v1.SessionSpec{}
	if err := proto.Unmarshal(bin, sess2); err != nil {
		t.Fatal(err)
	}
	got := sess2.GetTutorContext().AsMap()
	if !deepEqualJSON(fx.TutorContext, got) {
		t.Fatalf("tutor_context mismatch\nwant %#v\ngot  %#v", fx.TutorContext, got)
	}

	ed, err := structpb.NewStruct(fx.EventData)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed semantic timestamp (matches timestamp_duration fixture) so
	// evidence/struct.txt is byte-stable across ordinary spike runs.
	fixedOccurredAt := time.Date(2026, 8, 13, 15, 4, 5, 0, time.UTC)
	ev := &v1.DomainEvent{
		Id:          "evt_1",
		Type:        v1.EventType_EVENT_TYPE_MATERIALIZED_ITEM_SUPERSEDED,
		WorkspaceId: "ws_1",
		OccurredAt:  timestamppb.New(fixedOccurredAt),
		Data:        ed,
	}
	j, err := protojson.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	ev2 := &v1.DomainEvent{}
	if err := protojson.Unmarshal(j, ev2); err != nil {
		t.Fatal(err)
	}
	if !deepEqualJSON(fx.EventData, ev2.GetData().AsMap()) {
		t.Fatalf("event data mismatch: %s", j)
	}

	// REST DomainEvent.data is free-form object
	rest := map[string]any{
		"id":          "evt_1",
		"type":        "materialized_item.superseded",
		"workspaceId": "ws_1",
		"occurredAt":  fixedOccurredAt.Format(time.RFC3339),
		"data":        fx.EventData,
	}
	rb, _ := json.Marshal(rest)
	var rest2 map[string]any
	_ = json.Unmarshal(rb, &rest2)
	if !deepEqualJSON(fx.EventData, rest2["data"].(map[string]any)) {
		t.Fatal("rest data")
	}
	writeEvidence(t, "struct.txt", fmt.Sprintf("protojson=%s\nrest=%s\n", compactJSON(t, j), string(rb)))
}

func deepEqualJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	var ax, bx any
	_ = json.Unmarshal(aj, &ax)
	_ = json.Unmarshal(bj, &bx)
	return reflect.DeepEqual(ax, bx)
}

func TestSpikePlantedAssertionTeeth(t *testing.T) {
	// Prove assertions fail when Struct keys are dropped.
	src := map[string]any{"keep": "yes", "nested": map[string]any{"k": 1.0}}
	st, err := structpb.NewStruct(src)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt: drop nested
	bad := st.AsMap()
	delete(bad, "nested")
	if deepEqualJSON(src, bad) {
		t.Fatal("teeth broken: dropped key still equal")
	}
}

// Ensure unused imports for tools that staticcheck might flag in some builds.
var _ = errors.New
var _ = io.EOF
var _ = sort.Strings
