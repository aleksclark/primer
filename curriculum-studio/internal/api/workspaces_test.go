package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
	"github.com/aleksclark/primer/curriculum-studio/internal/authn"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/factory"
	"github.com/aleksclark/primer/curriculum-studio/internal/testutil/jwttest"
)

// workspaceSuite is a helper that wires pool → server → httptest for workspace
// API tests. Each test builds its own suite so data is isolated by UUID seeds.
type workspaceSuite struct {
	t    *testing.T
	pool interface {
		QueryRow(interface{}, string, ...interface{}) interface{}
	}
	handler http.Handler
	srv     *httptest.Server
	key     *jwttest.Keypair
	now     time.Time
}

func newWorkspaceSuite(t *testing.T) (http.Handler, *httptest.Server, *jwttest.Keypair, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	key := jwttest.GenerateKey(t)
	jwks := serveJWKS(t, key)
	pool := testutil.DB(t)
	validator, err := authn.NewValidator(authn.Options{
		Issuer:   jwttest.Issuer,
		Audience: jwttest.Audience,
		JWKSURL:  jwks.URL,
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	_, handler := api.New(pool, api.Options{
		Validator: validator,
		Now:       func() time.Time { return now },
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return handler, srv, key, now
}

func mintHuman(t *testing.T, key *jwttest.Keypair, now time.Time, subUUID uuid.UUID) string {
	t.Helper()
	return jwttest.Mint(t, key, jwttest.ValidHumanClaims(now, subUUID.String()))
}

func doJSON(t *testing.T, handler http.Handler, method, path string, body interface{}, tok string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ─── P3-S1: first workspace creation (zero-membership path) ──────────────────

// TestP3S1_FirstWorkspaceCreation verifies that an authenticated subject who
// has NO existing workspace memberships can POST /workspaces and get a 201
// with ws_* id, a created workspace row, an owner membership row, and a
// tenant row — all in one transaction. This is the core edge case: the
// authMiddleware must NOT gate on zero memberships for this endpoint.
func TestP3S1_FirstWorkspaceCreation(t *testing.T) {
	t.Parallel()
	_, srv, key, now := newWorkspaceSuite(t)
	pool := testutil.DB(t)
	sub := uuid.New()
	tok := mintHuman(t, key, now, sub)

	// Subject has no memberships at all.
	reqBody := map[string]string{"name": "Oak Family Homeschool", "kind": "family"}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/studio/v1/workspaces", bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(respBody))

	var got api.WorkspaceView
	require.NoError(t, json.Unmarshal(respBody, &got))

	// P3-S5: name is the primary field, id is secondary.
	assert.Equal(t, "Oak Family Homeschool", got.Name)
	assert.True(t, strings.HasPrefix(got.ID, "ws_"), "id must be prefixed ws_: %s", got.ID)
	assert.NotEmpty(t, got.Slug)
	assert.Equal(t, "family", got.Kind)
	assert.Equal(t, "active", got.Status)
	assert.NotEmpty(t, got.TenantID)

	// Verify DB rows: workspace, owner membership, tenant, audit event.
	var wsUUIDStr string
	// Extract UUID from ws_ prefix for DB lookup.
	wsHex := got.ID[len("ws_"):]
	if len(wsHex) == 32 {
		wsUUIDStr = wsHex[:8] + "-" + wsHex[8:12] + "-" + wsHex[12:16] + "-" + wsHex[16:20] + "-" + wsHex[20:]
	} else {
		wsUUIDStr = wsHex
	}
	wsUUID, err := uuid.Parse(wsUUIDStr)
	require.NoError(t, err)

	// Workspace row exists.
	var wsName string
	err = pool.QueryRow(t.Context(), `SELECT name FROM curriculum_studio.workspaces WHERE id = $1`, wsUUID).Scan(&wsName)
	require.NoError(t, err)
	assert.Equal(t, "Oak Family Homeschool", wsName)

	// Owner membership created for subject.
	subjectRef := domain.HumanSubjectRef(sub)
	var memRole, memStatus string
	err = pool.QueryRow(t.Context(), `
		SELECT role, status FROM curriculum_studio.workspace_memberships
		WHERE workspace_id = $1 AND subject_ref = $2`, wsUUID, subjectRef).Scan(&memRole, &memStatus)
	require.NoError(t, err)
	assert.Equal(t, domain.MembershipRoleOwner, memRole)
	assert.Equal(t, domain.MembershipStatusActive, memStatus)

	// Tenant row exists.
	tenantID, err := uuid.Parse(got.TenantID)
	require.NoError(t, err)
	var tenantStatus string
	err = pool.QueryRow(t.Context(), `SELECT status FROM curriculum_studio.tenants WHERE id = $1`, tenantID).Scan(&tenantStatus)
	require.NoError(t, err)
	assert.Equal(t, domain.TenantStatusActive, tenantStatus)

	// Audit event created.
	var auditAction string
	err = pool.QueryRow(t.Context(), `
		SELECT action FROM curriculum_studio.audit_events
		WHERE workspace_id = $1 AND action = 'workspace.create'`, wsUUID).Scan(&auditAction)
	require.NoError(t, err)
	assert.Equal(t, "workspace.create", auditAction)

	// Idempotency: middleware accepted request with zero initial memberships.
	// A second call creates a SECOND workspace (distinct) — no state carryover.
	reqBody2 := map[string]string{"name": "Second Workspace"}
	b2, _ := json.Marshal(reqBody2)
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/studio/v1/workspaces", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+tok)
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)
}

// TestP3S1_FirstWorkspaceDeniedWithoutJWT verifies the exempt path still
// requires a valid Bearer token (authMiddleware validates JWT before the
// membership-count gate).
func TestP3S1_FirstWorkspaceDeniedWithoutJWT(t *testing.T) {
	t.Parallel()
	_, srv, _, _ := newWorkspaceSuite(t)

	b, _ := json.Marshal(map[string]string{"name": "No Auth Workspace"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/studio/v1/workspaces", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── P3-E1: Workspace CRUD + list (server-side q/sort/dir/limit/offset) ──────

// TestP3E1_WorkspaceList_PaginationAndFilter creates 3 workspaces for one subject
// and 1 for another, then exercises q/limit/sort/dir/offset — all filtering must
// happen in SQL, not in-memory.
func TestP3E1_WorkspaceList_PaginationAndFilter(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	_, srv, key, now := newWorkspaceSuite(t)
	sub := uuid.New()
	subjectRef := domain.HumanSubjectRef(sub)

	// Create 3 workspaces for our subject.
	ws1 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Alpha Workspace" })
	ws2 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Beta Workspace" })
	ws3 := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Zeta Workspace" })
	factory.SeedMembership(t, pool, ws1.ID, subjectRef, domain.MembershipRoleOwner)
	factory.SeedMembership(t, pool, ws2.ID, subjectRef, domain.MembershipRoleAuthor)
	factory.SeedMembership(t, pool, ws3.ID, subjectRef, domain.MembershipRoleViewer)

	// Foreign subject with a separate workspace — must never appear in our list.
	foreignSub := uuid.New()
	foreignWS := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Foreign Workspace" })
	factory.SeedMembership(t, pool, foreignWS.ID, domain.HumanSubjectRef(foreignSub), domain.MembershipRoleOwner)

	tok := mintHuman(t, key, now, sub)

	doGet := func(t *testing.T, query string) (int, map[string]interface{}) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/workspaces"+query, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		_ = json.Unmarshal(b, &result)
		return resp.StatusCode, result
	}

	// Full list: should get exactly 3 (not 4).
	status, body := doGet(t, "")
	require.Equal(t, http.StatusOK, status)
	items, _ := body["items"].([]interface{})
	require.Len(t, items, 3, "foreign workspace must be excluded")
	assert.Equal(t, float64(3), body["total"])
	// All foreign workspace names must be absent.
	for _, item := range items {
		m := item.(map[string]interface{})
		assert.NotEqual(t, "Foreign Workspace", m["name"])
	}

	// P3-S2: q filter applied in SQL — only workspaces matching "Beta" returned.
	status, body = doGet(t, "?q=Beta")
	require.Equal(t, http.StatusOK, status)
	items, _ = body["items"].([]interface{})
	require.Len(t, items, 1, "q=Beta must match exactly one workspace")
	assert.Equal(t, "Beta Workspace", items[0].(map[string]interface{})["name"])
	// Total reflects filter result.
	assert.Equal(t, float64(1), body["total"])

	// limit=1 returns first page only.
	status, body = doGet(t, "?limit=1&sort=name&dir=asc")
	require.Equal(t, http.StatusOK, status)
	items, _ = body["items"].([]interface{})
	require.Len(t, items, 1)
	assert.Equal(t, float64(1), body["limit"])
	// Sorted ascending by name: Alpha comes first.
	assert.Equal(t, "Alpha Workspace", items[0].(map[string]interface{})["name"])

	// offset=1, limit=1, sort=name asc → Beta.
	status, body = doGet(t, "?limit=1&offset=1&sort=name&dir=asc")
	require.Equal(t, http.StatusOK, status)
	items, _ = body["items"].([]interface{})
	require.Len(t, items, 1)
	assert.Equal(t, "Beta Workspace", items[0].(map[string]interface{})["name"])

	// dir=desc, limit=1 → Zeta comes first.
	status, body = doGet(t, "?limit=1&sort=name&dir=desc")
	require.Equal(t, http.StatusOK, status)
	items, _ = body["items"].([]interface{})
	require.Len(t, items, 1)
	assert.Equal(t, "Zeta Workspace", items[0].(map[string]interface{})["name"])

	// Verify IDs are ws_ prefixed in list response.
	status, body = doGet(t, "")
	require.Equal(t, http.StatusOK, status)
	items, _ = body["items"].([]interface{})
	for _, item := range items {
		m := item.(map[string]interface{})
		id, _ := m["id"].(string)
		assert.True(t, strings.HasPrefix(id, "ws_"), "list item id must be prefixed ws_: %s", id)
		assert.NotEmpty(t, m["name"])
	}

	// Verify zero-membership subject gets empty list (not forbidden).
	newSub := uuid.New()
	newTok := mintHuman(t, key, now, newSub)
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/workspaces", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+newTok)
	emptyResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer emptyResp.Body.Close()
	emptyBody, _ := io.ReadAll(emptyResp.Body)
	require.Equal(t, http.StatusOK, emptyResp.StatusCode, string(emptyBody))
	var emptyPage map[string]interface{}
	require.NoError(t, json.Unmarshal(emptyBody, &emptyPage))
	emptyItems, _ := emptyPage["items"].([]interface{})
	assert.Empty(t, emptyItems, "zero-membership subject must get empty list, not foreign workspaces")

	_ = ws1
	_ = ws2
	_ = ws3
}

// TestP3E1_GetAndUpdateWorkspace covers getWorkspace and updateWorkspace.
func TestP3E1_GetAndUpdateWorkspace(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)

	sub := uuid.New()
	ws := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Oak Family Homeschool" })
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(sub), domain.MembershipRoleOwner)

	tok := mintHuman(t, key, now, sub)

	// GET with raw UUID (backward-compat).
	rr := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String(), nil, tok)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got api.WorkspaceView
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "Oak Family Homeschool", got.Name)
	assert.True(t, strings.HasPrefix(got.ID, "ws_"))

	// GET with ws_ prefixed ID.
	wsIDPrefixed := got.ID
	rr2 := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+wsIDPrefixed, nil, tok)
	assert.Equal(t, http.StatusOK, rr2.Code)

	// Cross-workspace GET returns 404.
	foreignWS := factory.Workspace(t, pool)
	rr3 := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+foreignWS.ID.String(), nil, tok)
	assert.Equal(t, http.StatusNotFound, rr3.Code)

	// PATCH update workspace name.
	rr4 := doJSON(t, handler, http.MethodPatch, "/studio/v1/workspaces/"+ws.ID.String(),
		map[string]string{"name": "Renamed Workspace", "status": "active"}, tok)
	assert.Equal(t, http.StatusOK, rr4.Code)
	var updated api.WorkspaceView
	require.NoError(t, json.Unmarshal(rr4.Body.Bytes(), &updated))
	assert.Equal(t, "Renamed Workspace", updated.Name)

	// Author cannot update workspace metadata (CanManageMembers = owner/admin only).
	authorSub := uuid.New()
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(authorSub), domain.MembershipRoleAuthor)
	authorTok := mintHuman(t, key, now, authorSub)
	rr5 := doJSON(t, handler, http.MethodPatch, "/studio/v1/workspaces/"+ws.ID.String(),
		map[string]string{"name": "Should Fail"}, authorTok)
	assert.Equal(t, http.StatusForbidden, rr5.Code)

	// Audit event for update exists.
	var auditAction string
	err := pool.QueryRow(t.Context(), `
		SELECT action FROM curriculum_studio.audit_events
		WHERE workspace_id = $1 AND action = 'workspace.update'
		ORDER BY created_at DESC LIMIT 1`, ws.ID).Scan(&auditAction)
	require.NoError(t, err)
	assert.Equal(t, "workspace.update", auditAction)
}

// ─── P3-E2: Membership lifecycle ─────────────────────────────────────────────

// TestP3E2_MembershipLifecycle exercises add/list/update-role/revoke on
// memberships, verifying access change after revoke.
func TestP3E2_MembershipLifecycle(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)

	ownerSub := uuid.New()
	secondSub := uuid.New()
	secondRef := domain.HumanSubjectRef(secondSub)

	ws := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Lifecycle WS" })
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(ownerSub), domain.MembershipRoleOwner)

	ownerTok := mintHuman(t, key, now, ownerSub)

	// P3-S3: owner adds author membership for second identity.
	rr := doJSON(t, handler, http.MethodPost,
		"/studio/v1/workspaces/"+ws.ID.String()+"/memberships",
		map[string]string{
			"subjectRef":  secondRef,
			"role":        "author",
			"displayName": "Second User",
		}, ownerTok)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var addedMem api.WorkspaceMembershipView
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &addedMem))
	assert.True(t, strings.HasPrefix(addedMem.ID, "mem_"), "membership id must be mem_ prefixed: %s", addedMem.ID)
	assert.Equal(t, secondRef, addedMem.SubjectRef)
	assert.Equal(t, "author", addedMem.Role)
	assert.Equal(t, "active", addedMem.Status)

	// Verify DB status is active.
	var dbStatus string
	memUUID := extractUUIDFromPrefixed(t, addedMem.ID, "mem_")
	err := pool.QueryRow(t.Context(), `
		SELECT status FROM curriculum_studio.workspace_memberships WHERE id = $1`, memUUID).Scan(&dbStatus)
	require.NoError(t, err)
	assert.Equal(t, domain.MembershipStatusActive, dbStatus)

	// P3-S3: second subject can now list the workspace.
	secondTok := mintHuman(t, key, now, secondSub)
	rr2 := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String(), nil, secondTok)
	require.Equal(t, http.StatusOK, rr2.Code, "added member should see workspace")

	// List memberships: owner sees both rows.
	rrList := doJSON(t, handler, http.MethodGet,
		"/studio/v1/workspaces/"+ws.ID.String()+"/memberships", nil, ownerTok)
	require.Equal(t, http.StatusOK, rrList.Code)
	var listBody struct {
		Items []api.WorkspaceMembershipView `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rrList.Body.Bytes(), &listBody))
	assert.Len(t, listBody.Items, 2)

	// Update role: owner promotes second to admin.
	rrUpdate := doJSON(t, handler, http.MethodPatch,
		fmt.Sprintf("/studio/v1/workspaces/%s/memberships/%s", ws.ID.String(), addedMem.ID),
		map[string]string{"role": "admin"}, ownerTok)
	require.Equal(t, http.StatusOK, rrUpdate.Code, rrUpdate.Body.String())
	var updatedMem api.WorkspaceMembershipView
	require.NoError(t, json.Unmarshal(rrUpdate.Body.Bytes(), &updatedMem))
	assert.Equal(t, "admin", updatedMem.Role)

	// P3-S4: revoke second subject's membership.
	rrRevoke := doJSON(t, handler, http.MethodDelete,
		fmt.Sprintf("/studio/v1/workspaces/%s/memberships/%s", ws.ID.String(), addedMem.ID),
		nil, ownerTok)
	require.Equal(t, http.StatusNoContent, rrRevoke.Code, rrRevoke.Body.String())

	// DB status is now revoked.
	err = pool.QueryRow(t.Context(), `
		SELECT status FROM curriculum_studio.workspace_memberships WHERE id = $1`, memUUID).Scan(&dbStatus)
	require.NoError(t, err)
	assert.Equal(t, domain.MembershipStatusRevoked, dbStatus)

	// P3-S4: revoked subject can no longer list or get workspace — middleware
	// loads their memberships (membership is revoked, ListActiveBySubject excludes it)
	// → zero active memberships → handler returns 403 (from middleware) or 404 (from
	// handler check); either way the resource is inaccessible.
	rr3 := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String(), nil, secondTok)
	assert.True(t, rr3.Code == http.StatusForbidden || rr3.Code == http.StatusNotFound,
		"revoked subject must not access workspace (got %d)", rr3.Code)

	// Viewer cannot add memberships.
	viewerSub := uuid.New()
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(viewerSub), domain.MembershipRoleViewer)
	viewerTok := mintHuman(t, key, now, viewerSub)
	rrViewerAdd := doJSON(t, handler, http.MethodPost,
		"/studio/v1/workspaces/"+ws.ID.String()+"/memberships",
		map[string]string{"subjectRef": domain.HumanSubjectRef(uuid.New()), "role": "viewer"},
		viewerTok)
	assert.Equal(t, http.StatusForbidden, rrViewerAdd.Code)

	// Audit events exist for membership operations.
	var auditCount int
	err = pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM curriculum_studio.audit_events
		WHERE workspace_id = $1 AND action IN ('membership.create','membership.update','membership.revoke')`,
		ws.ID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 3, auditCount, "expected 3 membership audit events")
}

// ─── P3-E3: Names-first payloads + ws_ id prefix ─────────────────────────────

// TestP3E3_NameFieldsAndIDPrefix verifies that workspace JSON responses always
// carry name as a top-level primary field and that all ids are ws_-prefixed
// (not raw UUIDs). This covers P3-S5 and the anti-cheat "Opaque IDs" gate.
func TestP3E3_NameFieldsAndIDPrefix(t *testing.T) {
	t.Parallel()
	pool := testutil.DB(t)
	handler, _, key, now := newWorkspaceSuite(t)

	sub := uuid.New()
	ws := factory.Workspace(t, pool, func(w *domain.Workspace) { w.Name = "Oak Family Homeschool" })
	factory.SeedMembership(t, pool, ws.ID, domain.HumanSubjectRef(sub), domain.MembershipRoleOwner)
	tok := mintHuman(t, key, now, sub)

	// GET single workspace — name must be present and id prefixed.
	rr := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces/"+ws.ID.String(), nil, tok)
	require.Equal(t, http.StatusOK, rr.Code)

	var single map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &single))

	// Primary field: name.
	nameVal, ok := single["name"]
	require.True(t, ok, "response must have 'name' field")
	assert.Equal(t, "Oak Family Homeschool", nameVal)

	// ID is secondary and prefixed.
	idVal, ok := single["id"]
	require.True(t, ok, "response must have 'id' field")
	idStr, _ := idVal.(string)
	assert.True(t, strings.HasPrefix(idStr, "ws_"), "id must be ws_ prefixed, got %q", idStr)
	// id must not be a raw RFC-4122 UUID (no prefix).
	assert.NotEqual(t, ws.ID.String(), idStr, "id in response must not be raw UUID")
	// id must be a valid decodable ws_ id (prefix + 32 hex chars).
	hexPart := idStr[3:]
	assert.Len(t, hexPart, 32, "ws_ prefix must be followed by 32 hex chars")

	// LIST — same invariants for every item.
	rrList := doJSON(t, handler, http.MethodGet, "/studio/v1/workspaces", nil, tok)
	require.Equal(t, http.StatusOK, rrList.Code)
	var listBody map[string]interface{}
	require.NoError(t, json.Unmarshal(rrList.Body.Bytes(), &listBody))
	items, _ := listBody["items"].([]interface{})
	require.NotEmpty(t, items)
	for _, item := range items {
		m := item.(map[string]interface{})
		itemID, _ := m["id"].(string)
		itemName, _ := m["name"].(string)
		assert.True(t, strings.HasPrefix(itemID, "ws_"), "list item id must be ws_ prefixed: %s", itemID)
		assert.NotEmpty(t, itemName, "list item must have non-empty name")
	}

	// CREATE — response also has ws_-prefixed id and name as primary.
	createRR := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces",
		map[string]string{"name": "Created WS", "kind": "teacher"}, tok)
	require.Equal(t, http.StatusCreated, createRR.Code)
	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(createRR.Body.Bytes(), &created))
	assert.Equal(t, "Created WS", created["name"])
	createdID, _ := created["id"].(string)
	assert.True(t, strings.HasPrefix(createdID, "ws_"), "create response id must be ws_ prefixed: %s", createdID)
}

// ─── edge: invalid kind rejected ─────────────────────────────────────────────

func TestCreateWorkspace_InvalidKind(t *testing.T) {
	t.Parallel()
	handler, _, key, now := newWorkspaceSuite(t)
	sub := uuid.New()
	tok := mintHuman(t, key, now, sub)

	rr := doJSON(t, handler, http.MethodPost, "/studio/v1/workspaces",
		map[string]string{"name": "WS", "kind": "supermagic"}, tok)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

// ─── edge: /auth/me still 403 for zero-membership subject ────────────────────

// TestAuthMeStillForbiddenForZeroMembershipSubject ensures the membership-
// exempt carve-out for /workspaces does NOT bleed into /auth/me. The
// isMembershipExemptPath function covers only the exact collection path.
func TestAuthMeStillForbiddenForZeroMembershipSubject(t *testing.T) {
	t.Parallel()
	_, srv, key, now := newWorkspaceSuite(t)
	sub := uuid.New()
	tok := mintHuman(t, key, now, sub)
	// subject has zero memberships
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/studio/v1/auth/me", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"/auth/me must still 403 for zero-membership subject; isMembershipExemptPath must not match /auth/me")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// extractUUIDFromPrefixed strips a known prefix (e.g. "ws_" or "mem_") and
// rehydrates a compact 32-hex UUID string to RFC-4122 form for DB lookups.
func extractUUIDFromPrefixed(t *testing.T, s, prefix string) uuid.UUID {
	t.Helper()
	s = strings.TrimPrefix(s, prefix)
	if len(s) == 32 {
		s = s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
	}
	id, err := uuid.Parse(s)
	require.NoError(t, err, "could not parse id from %q", s)
	return id
}

// Ensure api.WorkspaceView and api.WorkspaceMembershipView are exported so tests
// can reference them directly; this compile-time check also catches missing fields.
var _ = api.WorkspaceView{}
var _ = api.WorkspaceMembershipView{}

// Satisfy repo import.
var _ = repo.NewFactory
