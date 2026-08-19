package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"primer-tasks/internal/agent/protocol"
)

func TestPostgresRepositoryDurableReplayAndIdempotency(t *testing.T) {
	dsn := os.Getenv("TASKS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TASKS_TEST_DATABASE_URL for the PostgreSQL runtime contract test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresRepository(db)
	tenant := NewID()
	convID := NewID()
	runID := NewID()
	msgID := NewID()
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM agent_run_events WHERE tenant_id=$1; DELETE FROM agent_confirmation_previews WHERE tenant_id=$1; DELETE FROM agent_jobs WHERE tenant_id=$1; DELETE FROM agent_runs WHERE tenant_id=$1; DELETE FROM agent_messages WHERE tenant_id=$1; DELETE FROM agent_conversations WHERE tenant_id=$1`, tenant)
	})
	if err := store.CreateConversation(ctx, Conversation{ID: convID, TenantID: tenant, ActorID: NewID(), Status: ConversationActive, PolicyVersion: "p3", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	m := Message{ID: msgID, TenantID: tenant, ConversationID: convID, ClientMessageID: "client-1", Role: RoleUser, Content: "hello", Sequence: 1, CreatedAt: now}
	got, inserted, err := store.AppendUserMessage(ctx, m)
	if err != nil || !inserted || got.ID != msgID {
		t.Fatalf("append=%#v inserted=%v err=%v", got, inserted, err)
	}
	got, inserted, err = store.AppendUserMessage(ctx, m)
	if err != nil || inserted || got.ID != msgID {
		t.Fatalf("idempotency=%#v inserted=%v err=%v", got, inserted, err)
	}
	run := Run{ID: runID, TenantID: tenant, ConversationID: convID, UserMessageID: msgID, Status: RunQueued, MaxSteps: 3, MaxTokens: 100, Deadline: now.Add(time.Hour), Provenance: Provenance{Provider: "scripted", Model: "qualification", PolicyVersion: "p3", PromptDigest: "digest"}, CreatedAt: now}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	e := protocol.TextStart(runID, 1)
	if err := store.AppendEvent(ctx, RunEvent{RunID: runID, TenantID: tenant, Sequence: 1, EventType: string(e.Kind), Payload: JSON(e), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReplayEvents(ctx, tenant, runID, 0, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("replay=%#v err=%v", events, err)
	}
}
