package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	EventCurriculumCreated          = "curriculum.created"
	EventPlanRevisionPublished      = "plan_revision.published"
	EventMaterializationRequested   = "materialization.requested"
	EventMaterializationReady       = "materialization.ready"
	EventMaterializationFailed      = "materialization.failed"
	EventMaterializedItemSuperseded = "materialized_item.superseded"
	EventPlanChangeProposed         = "plan_change.proposed"
)

type OutboxEvent struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	WorkspaceID   *uuid.UUID      `json:"workspaceId,omitempty" db:"workspace_id"`
	EventType     string          `json:"eventType" db:"event_type"`
	AggregateKind string          `json:"aggregateKind" db:"aggregate_kind"`
	AggregateID   uuid.UUID       `json:"aggregateId" db:"aggregate_id"`
	Payload       json.RawMessage `json:"payload" db:"payload"`
	CreatedAt     time.Time       `json:"createdAt" db:"created_at"`
	PublishedAt   *time.Time      `json:"publishedAt,omitempty" db:"published_at"`
}

type WebhookEndpoint struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId" db:"workspace_id"`
	URL         string    `json:"url" db:"url"`
	SecretRef   string    `json:"secretRef" db:"secret_ref"`
	EventTypes  []string  `json:"eventTypes" db:"event_types"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

type WebhookDelivery struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	EndpointID       uuid.UUID  `json:"endpointId" db:"endpoint_id"`
	EventID          uuid.UUID  `json:"eventId" db:"event_id"`
	IdempotencyKey   string     `json:"idempotencyKey" db:"idempotency_key"`
	Status           string     `json:"status" db:"status"`
	AttemptCount     int        `json:"attemptCount" db:"attempt_count"`
	LastError        string     `json:"lastError" db:"last_error"`
	DeliveredAt      *time.Time `json:"deliveredAt,omitempty" db:"delivered_at"`
	LeaseOwner       string     `json:"leaseOwner,omitempty" db:"lease_owner"`
	LeaseExpiresAt   *time.Time `json:"leaseExpiresAt,omitempty" db:"lease_expires_at"`
	AttemptStartedAt *time.Time `json:"attemptStartedAt,omitempty" db:"attempt_started_at"`
	CreatedAt        time.Time  `json:"createdAt" db:"created_at"`
}

type IdempotencyKey struct {
	ID          uuid.UUID `json:"id" db:"id"`
	WorkspaceID uuid.UUID `json:"workspaceId" db:"workspace_id"`
	Scope       string    `json:"scope" db:"scope"`
	Key         string    `json:"key" db:"key"`
	RequestHash string    `json:"requestHash" db:"request_hash"`
	ResponseRef string    `json:"responseRef" db:"response_ref"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}
