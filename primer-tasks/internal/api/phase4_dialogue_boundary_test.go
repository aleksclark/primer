package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"primer-tasks/internal/domain"
	"primer-tasks/internal/repo"
)

// Corruption is an explicit NEGATIVE fixture appended to real public history.
// Production immutable UPDATE/DELETE constraints remain enabled; no accepted
// decision/result is fabricated and the real parent router must fail closed.
func TestPublicParentInspectRejectsInvalidDurableEvents(t *testing.T) {
	h := newPublicDialogueHarness(t)
	type corruptCase struct {
		name, missing string
		unknown       bool
		rowMismatch   string
	}
	cases := []corruptCase{{name: "unknown-provider-property", unknown: true}}
	for _, field := range []string{"attemptId", "occurrenceId", "requirementId", "policyVersion", "snapshotDigest", "version", "questionId", "text"} {
		cases = append(cases, corruptCase{name: "missing-" + field, missing: field})
	}
	for _, field := range []string{"sequence", "kind", "attemptId", "occurrenceId", "requirementId", "snapshotDigest"} {
		cases = append(cases, corruptCase{name: "row-mismatch-" + field, rowMismatch: field})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			local := *h
			local.t = t
			local.followingOccurrence()
			local.begin()
			conn := local.socket(0)
			question := local.wait(conn, func(e wireStudentEvent) bool { return e.Kind == "question" })
			conn.CloseNow()
			var valid DialogueInspect
			local.request(local.parent, "GET", "/occurrences/"+local.occurrence+"/inspect?limit=50", nil, 200, &valid)
			found := false
			for _, entry := range valid.Entries {
				found = found || entry.Kind == "question" && entry.QuestionID == question.QuestionID && entry.Text == question.Text
			}
			if !found {
				t.Fatal("valid public question history unavailable before corruption")
			}
			ctx := context.Background()
			var raw []byte
			var sequence int64
			if err := h.pool.QueryRow(ctx, `SELECT payload FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND kind='question' ORDER BY sequence DESC LIMIT 1`, tenantA, local.attempt).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if err := h.pool.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2`, tenantA, local.attempt).Scan(&sequence); err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			payload["sequence"], payload["cursor"] = sequence, sequence
			const sentinel = "INSPECT_PROVIDER_VALUE_MUST_NOT_ESCAPE"
			if tc.unknown {
				payload["provider_metadata"] = map[string]string{"reasoning": sentinel}
			}
			if tc.missing != "" {
				delete(payload, tc.missing)
			}
			rowKind := "question"
			switch tc.rowMismatch {
			case "sequence":
				payload["sequence"], payload["cursor"] = sequence+1, sequence+1
			case "kind":
				rowKind = "progress"
			case "snapshotDigest":
				payload["snapshotDigest"] = strings.Repeat("0", 64)
			case "attemptId", "occurrenceId", "requirementId":
				payload[tc.rowMismatch] = uuid.NewString()
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if tc.unknown || tc.missing != "" {
				if _, err := decodeStudentEvent(encoded); err == nil {
					t.Fatal("negative fixture does not violate the actual student decoder")
				}
			}
			if _, err = h.pool.Exec(ctx, `INSERT INTO verification_events(tenant_id,attempt_id,sequence,event_key,kind,payload) VALUES($1,$2,$3,$4,$5,$6)`, tenantA, local.attempt, sequence, "negative-inspect-"+tc.name, rowKind, encoded); err != nil {
				t.Fatal("production schema did not permit this negative precondition")
			}
			if tc.unknown {
				for _, sql := range []string{`UPDATE verification_events SET payload=payload WHERE tenant_id=$1 AND attempt_id=$2 AND sequence=$3`, `DELETE FROM verification_events WHERE tenant_id=$1 AND attempt_id=$2 AND sequence=$3`} {
					_, err := h.pool.Exec(ctx, sql, tenantA, local.attempt, sequence)
					var pgError *pgconn.PgError
					if !errors.As(err, &pgError) || pgError.Code != "23514" {
						t.Fatal("durable-event immutability was not enforced")
					}
				}
			}
			// Full page and the bounded lookahead both contain the malformed row.
			// Neither may return a partially projected 200 timeline.
			for _, limit := range []int64{50, sequence - 1} {
				response, err := local.parent.Get(fmt.Sprintf("%s/occurrences/%s/inspect?limit=%d", local.base, local.occurrence, limit))
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(body, []byte(sentinel)) {
					t.Fatal("unknown provider value echoed by parent inspect")
				}
				var problem map[string]json.RawMessage
				if json.Unmarshal(body, &problem) != nil {
					t.Fatal("inspect error is not typed JSON")
				}
				if response.StatusCode != http.StatusServiceUnavailable {
					t.Fatalf("contract-invalid durable event returned HTTP %d, want 503", response.StatusCode)
				}
				var code string
				if json.Unmarshal(problem["code"], &code) != nil || code != "unavailable" {
					t.Fatal("inspect did not return the fail-closed typed error")
				}
				if _, partial := problem["entries"]; partial {
					t.Fatal("inspect returned a partial timeline on failure")
				}
			}
		})
	}
}

// Public REST authoring + real PG evidence only. This is not the later student
// WS/Fantasy acceptance fixture. Parent-session prerequisites use the existing
// local test-auth seed; the actual production router authorizes every request.
func TestDialogueAuthoringSnapshotsIssuedPolicy(t *testing.T) {
	pool := integrationPool(t)
	student, _ := seedIntegration(t, pool)
	server := httptest.NewServer(New(pool, "test").Routes())
	defer server.Close()
	ctx := context.Background()
	request := func(method, path, parent string, body any, want int) []byte {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "tasks_parent", Value: parent})
		req.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s returned %d, wanted %d", method, path, response.StatusCode, want)
		}
		return data
	}
	config := domain.DialogueConfig{SourceText: "The pupil measured the beam twice before cutting it.", LearningFocus: "Use three distinct facts and reasons", RequiredQuestions: 3, Rubric: []string{"answers the question with a source detail"}, AllowedFollowUps: 1, MaxAttempts: 2, MaxTurns: 8, RetentionPolicy: "retain"}
	configMap, err := domain.SnapshotDialogueConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	body := TaskInput2{Title: "P4 original reading", Instructions: "Read and explain", Requirements: []Requirement{{ID: "display-label-not-authority", Kind: domain.AgentDialogueKind, ConfigVersion: 1, Config: configMap, Interaction: "chat", Executor: "fantasy"}}}
	var original TaskRevision
	if err = json.Unmarshal(request(http.MethodPost, "/tasks", "parent-a", body, 201), &original); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/tasks/"+original.ID+"/publish", "parent-b", nil, 404)
	request(http.MethodPost, "/tasks/"+original.ID+"/publish", "parent-a", nil, 200)
	var requirement, configType string
	if err = pool.QueryRow(ctx, `SELECT id,jsonb_typeof(config) FROM verification_requirements WHERE tenant_id=$1 AND revision_id=$2`, tenantA, original.ID).Scan(&requirement, &configType); err != nil || configType != "array" {
		t.Fatal("canonical requirement array changed")
	}
	store := repo.NewDialogueRepository(pool)
	before, err := store.RevisionPolicy(ctx, tenantA, original.ID, requirement)
	if err != nil || before.Source.Text != config.SourceText || before.RevisionID != original.ID || before.RequirementID != requirement || before.RevisionVersion != 1 {
		t.Fatalf("published source binding: %v", err)
	}
	if _, err = store.RevisionPolicy(ctx, tenantB, original.ID, requirement); err == nil {
		t.Fatal("foreign policy read succeeded")
	}
	var page TaskPage2
	if err = json.Unmarshal(request(http.MethodGet, "/tasks?view=templates&q=P4%20original", "parent-a", nil, 200), &page); err != nil || len(page.Items) != 1 || len(page.Items[0].Requirements) != 1 {
		t.Fatal("current template editing/config->0 projection regressed")
	}
	if page.Items[0].Requirements[0].Config["sourceText"] != config.SourceText {
		t.Fatal("editable requirement lost its source")
	}

	body.Title = "P4 revised reading"
	nextConfig := config
	nextConfig.SourceText = "The new revision discusses a different source."
	body.Requirements[0].Config, err = domain.SnapshotDialogueConfig(nextConfig)
	if err != nil {
		t.Fatal(err)
	}
	var revised TaskRevision
	if err = json.Unmarshal(request(http.MethodPost, "/tasks/"+original.TemplateID+"/revisions", "parent-a", body, 201), &revised); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/tasks/"+revised.ID+"/publish", "parent-a", nil, 200)
	var nextRequirement string
	if err = pool.QueryRow(ctx, `SELECT id FROM verification_requirements WHERE tenant_id=$1 AND revision_id=$2`, tenantA, revised.ID).Scan(&nextRequirement); err != nil {
		t.Fatal(err)
	}
	after, err := store.RevisionPolicy(ctx, tenantA, original.ID, requirement)
	if err != nil || after.Digest != before.Digest || after.Source != before.Source {
		t.Fatal("new draft rewrote issued policy")
	}
	next, err := store.RevisionPolicy(ctx, tenantA, revised.ID, nextRequirement)
	if err != nil || next.Digest == before.Digest || next.Source.Text != nextConfig.SourceText || next.RevisionVersion != 2 {
		t.Fatal("new revision did not get its own policy")
	}

	for name, alter := range map[string]func(map[string]any){
		"retention days":      func(c map[string]any) { c["retentionDays"] = 30 },
		"redaction":           func(c map[string]any) { c["retentionPolicy"] = "redact" },
		"remote source":       func(c map[string]any) { delete(c, "sourceText"); c["sourceRef"] = "https://example.invalid/source" },
		"source substitution": func(c map[string]any) { c["sourceRef"] = "fixture://chapter-4" },
		"prompt":              func(c map[string]any) { c["systemPrompt"] = "complete it" },
		"one answer":          func(c map[string]any) { c["requiredQuestions"] = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			copy, err := domain.SnapshotDialogueConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			alter(copy)
			invalid := body
			invalid.Requirements = append([]Requirement(nil), body.Requirements...)
			invalid.Requirements[0].Config = copy
			request(http.MethodPost, "/tasks", "parent-a", invalid, 400)
			request(http.MethodPost, "/tasks/"+original.TemplateID+"/revisions", "parent-a", invalid, 400)
		})
	}
	var revisions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM task_revisions WHERE tenant_id=$1 AND template_id=$2`, tenantA, original.TemplateID).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatal("invalid revisions were persisted")
	}

	// Negative stored-draft corruption proves publication validates what is in
	// PostgreSQL, not only a prior client's create validation. No accepted
	// evidence or final state is injected by this supplemental negative.
	var draft TaskRevision
	if err = json.Unmarshal(request(http.MethodPost, "/tasks", "parent-a", body, 201), &draft); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE verification_requirements SET config=jsonb_set(config,'{0,config,retentionPolicy}','"delete_after_review"') WHERE tenant_id=$1 AND revision_id=$2`, tenantA, draft.ID); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/tasks/"+draft.ID+"/publish", "parent-a", nil, 400)
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM task_revisions WHERE id=$1`, draft.ID).Scan(&status); err != nil || status != "draft" {
		t.Fatal("invalid publication did not roll back")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dialogue_revision_policies WHERE revision_id=$1`, draft.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed publication left policy evidence")
	}

	// Supplemental admission-state negatives: construct OPEN prerequisites,
	// never accepted evaluations, then exercise the real parent decision route.
	// This proves manual approval cannot be a shortcut around dialogue policy.
	prepare := func(task TaskRevision) string {
		t.Helper()
		var schedule Schedule2
		data := request(http.MethodPost, "/schedules", "parent-a", ScheduleInput2{StudentID: student, TemplateID: task.TemplateID, RevisionID: task.ID, Kind: "one_off", Timezone: "UTC", StartAt: time.Now().UTC().Add(-time.Minute)}, 201)
		if err := json.Unmarshal(data, &schedule); err != nil {
			t.Fatal(err)
		}
		var occurrence string
		if err := pool.QueryRow(ctx, `SELECT id FROM task_occurrences WHERE tenant_id=$1 AND schedule_id=$2`, tenantA, schedule.ID).Scan(&occurrence); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE task_occurrences SET status='awaiting_verification' WHERE tenant_id=$1 AND id=$2`, tenantA, occurrence); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) SELECT $1,$2,$3,id,1 FROM verification_requirements WHERE tenant_id=$2 AND revision_id=$4 AND kind='agent_dialogue'`, uuid.NewString(), tenantA, occurrence, task.ID); err != nil {
			t.Fatal(err)
		}
		return occurrence
	}
	dialogueOccurrence := prepare(original)
	request(http.MethodPost, "/occurrences/"+dialogueOccurrence+"/decision", "parent-a", DecisionInput2{Accepted: true, Reason: "Must not bypass dialogue"}, 409)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_decisions d JOIN verification_attempts a ON a.tenant_id=d.tenant_id AND a.id=d.attempt_id WHERE a.occurrence_id=$1`, dialogueOccurrence).Scan(&count); err != nil || count != 0 {
		t.Fatal("manual route accepted dialogue")
	}
	mixedBody := body
	mixedBody.Requirements = append(append([]Requirement(nil), body.Requirements...), Requirement{Kind: "parent_approval", ConfigVersion: 1, Config: map[string]any{}, Interaction: "parent_action", Executor: "human"})
	var mixed TaskRevision
	if err := json.Unmarshal(request(http.MethodPost, "/tasks", "parent-a", mixedBody, 201), &mixed); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/tasks/"+mixed.ID+"/publish", "parent-a", nil, 200)
	mixedOccurrence := prepare(mixed)
	if _, err := pool.Exec(ctx, `INSERT INTO verification_attempts(id,tenant_id,occurrence_id,requirement_id,number) SELECT $1,$2,$3,id,2 FROM verification_requirements WHERE tenant_id=$2 AND revision_id=$4 AND kind='parent_approval'`, uuid.NewString(), tenantA, mixedOccurrence, mixed.ID); err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/occurrences/"+mixedOccurrence+"/decision", "parent-a", DecisionInput2{Accepted: true, Reason: "Only the manual requirement was checked"}, 200)
	if err := pool.QueryRow(ctx, `SELECT status FROM task_occurrences WHERE id=$1`, mixedOccurrence).Scan(&status); err != nil || status != "awaiting_verification" {
		t.Fatal("manual decision bypassed another required driver")
	}

	foreign := request(http.MethodGet, "/tasks?view=templates", "parent-b", nil, 200)
	if strings.Contains(string(foreign), original.TemplateID) {
		t.Fatal("foreign template leaked")
	}
}
