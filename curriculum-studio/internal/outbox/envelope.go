package outbox

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// DomainEvent is the C9 JSON webhook envelope posted to subscribers.
type DomainEvent struct {
	ID                string          `json:"id"`
	Type              string          `json:"type"`
	WorkspaceID       string          `json:"workspaceId"`
	CurriculumID      string          `json:"curriculumId,omitempty"`
	RevisionID        string          `json:"revisionId,omitempty"`
	MaterializationID string          `json:"materializationId,omitempty"`
	ItemID            string          `json:"itemId,omitempty"`
	OccurredAt        time.Time       `json:"occurredAt"`
	Data              json.RawMessage `json:"data,omitempty"`
}

func envelopeFor(event *domain.OutboxEvent) DomainEvent {
	out := DomainEvent{
		ID:         event.ID.String(),
		Type:       event.EventType,
		OccurredAt: event.CreatedAt.UTC(),
		Data:       event.Payload,
	}
	if event.WorkspaceID != nil && *event.WorkspaceID != uuid.Nil {
		out.WorkspaceID = event.WorkspaceID.String()
	}
	switch event.AggregateKind {
	case "curriculum":
		out.CurriculumID = event.AggregateID.String()
	case "plan_revision":
		out.RevisionID = event.AggregateID.String()
	case "materialization_run":
		out.MaterializationID = event.AggregateID.String()
	case "materialized_item":
		out.ItemID = event.AggregateID.String()
	}
	if event.Payload != nil {
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) == nil {
			if out.CurriculumID == "" {
				out.CurriculumID = stringField(payload, "curriculum_id", "curriculumId")
			}
			if out.RevisionID == "" {
				out.RevisionID = stringField(payload, "revision_id", "revisionId")
			}
			if out.MaterializationID == "" {
				out.MaterializationID = stringField(payload, "materialization_id", "materializationId", "run_id")
			}
			if out.ItemID == "" {
				out.ItemID = stringField(payload, "item_id", "itemId")
			}
		}
	}
	return out
}

func stringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// IdempotencyKey is the stable (endpoint,event) delivery identity.
func IdempotencyKey(endpointID, eventID uuid.UUID) string {
	return "wh:" + endpointID.String() + ":" + eventID.String()
}
