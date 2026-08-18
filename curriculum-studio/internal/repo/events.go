package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// OutboxRepo is the durable event source. Enqueue accepts the ambient UoW.
type OutboxRepo struct{ Q Querier }

func NewOutboxRepo(q Querier) *OutboxRepo { return &OutboxRepo{Q: q} }
func (r *OutboxRepo) Enqueue(ctx context.Context, event *domain.OutboxEvent) (*domain.OutboxEvent, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if event == nil || strings.TrimSpace(event.EventType) == "" || strings.TrimSpace(event.AggregateKind) == "" || event.AggregateID == uuid.Nil {
		return nil, fmt.Errorf("event type, aggregate kind, and aggregate id are required")
	}
	payload, e := materializationObject(event.Payload, "event payload")
	if e != nil {
		return nil, e
	}
	out, e := scanOutboxEvent(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.outbox_events(workspace_id,event_type,aggregate_kind,aggregate_id,payload) VALUES($1,$2,$3,$4,$5) RETURNING id,workspace_id,event_type,aggregate_kind,aggregate_id,payload,created_at,published_at`, event.WorkspaceID, event.EventType, event.AggregateKind, event.AggregateID, payload))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *OutboxRepo) Get(ctx context.Context, id uuid.UUID) (*domain.OutboxEvent, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanOutboxEvent(r.Q.QueryRow(ctx, `SELECT id,workspace_id,event_type,aggregate_kind,aggregate_id,payload,created_at,published_at FROM curriculum_studio.outbox_events WHERE id=$1`, id))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *OutboxRepo) MarkPublished(ctx context.Context, id uuid.UUID) (*domain.OutboxEvent, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanOutboxEvent(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.outbox_events SET published_at=now() WHERE id=$1 AND published_at IS NULL RETURNING id,workspace_id,event_type,aggregate_kind,aggregate_id,payload,created_at,published_at`, id))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

// WebhookEndpointRepo persists subscriptions; secret_ref is only a secret-store pointer.
type WebhookEndpointRepo struct{ Q Querier }

func NewWebhookEndpointRepo(q Querier) *WebhookEndpointRepo { return &WebhookEndpointRepo{Q: q} }
func (r *WebhookEndpointRepo) Create(ctx context.Context, in *domain.WebhookEndpoint) (*domain.WebhookEndpoint, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || in.WorkspaceID == uuid.Nil || strings.TrimSpace(in.URL) == "" {
		return nil, fmt.Errorf("workspace and URL are required")
	}
	status := in.Status
	if status == "" {
		status = "active"
	}
	types := in.EventTypes
	if types == nil {
		types = []string{}
	}
	out, e := scanWebhookEndpoint(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.webhook_endpoints(workspace_id,url,secret_ref,event_types,status) VALUES($1,$2,$3,$4,$5) RETURNING id,workspace_id,url,secret_ref,event_types,status,created_at,updated_at`, in.WorkspaceID, in.URL, in.SecretRef, types, status))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *WebhookEndpointRepo) List(ctx context.Context, workspaceID uuid.UUID) ([]domain.WebhookEndpoint, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	rows, e := r.Q.Query(ctx, `SELECT id,workspace_id,url,secret_ref,event_types,status,created_at,updated_at FROM curriculum_studio.webhook_endpoints WHERE workspace_id=$1 ORDER BY created_at,id`, workspaceID)
	if e != nil {
		return nil, MapError(e)
	}
	defer rows.Close()
	var out []domain.WebhookEndpoint
	for rows.Next() {
		v, e := scanWebhookEndpoint(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if e := rows.Err(); e != nil {
		return nil, MapError(e)
	}
	if out == nil {
		out = []domain.WebhookEndpoint{}
	}
	return out, nil
}

// WebhookDeliveryRepo provides DB lease/at-least-once delivery persistence.
type WebhookDeliveryRepo struct{ Q Querier }

func NewWebhookDeliveryRepo(q Querier) *WebhookDeliveryRepo { return &WebhookDeliveryRepo{Q: q} }
func (r *WebhookDeliveryRepo) Schedule(ctx context.Context, endpointID, eventID uuid.UUID, idempotencyKey string) (*domain.WebhookDelivery, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if endpointID == uuid.Nil || eventID == uuid.Nil || strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("endpoint, event, and idempotency key are required")
	}
	out, e := scanWebhookDelivery(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.webhook_deliveries(endpoint_id,event_id,idempotency_key) VALUES($1,$2,$3) ON CONFLICT (endpoint_id,event_id) DO UPDATE SET endpoint_id=EXCLUDED.endpoint_id RETURNING id,endpoint_id,event_id,idempotency_key,status,attempt_count,last_error,delivered_at,lease_owner,lease_expires_at,attempt_started_at,created_at`, endpointID, eventID, idempotencyKey))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}
func (r *WebhookDeliveryRepo) Claim(ctx context.Context, owner string, leaseTTL time.Duration) (*domain.WebhookDelivery, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if strings.TrimSpace(owner) == "" || leaseTTL <= 0 {
		return nil, fmt.Errorf("owner and positive lease are required")
	}
	var out *domain.WebhookDelivery
	err := WithTx(ctx, r.Q, func(q Querier) error {
		const sel = `SELECT id FROM curriculum_studio.webhook_deliveries WHERE status IN ('pending','failed') AND (lease_expires_at IS NULL OR lease_expires_at<=now()) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`
		var id uuid.UUID
		if e := q.QueryRow(ctx, sel).Scan(&id); e != nil {
			return e
		}
		var e error
		out, e = scanWebhookDelivery(q.QueryRow(ctx, `UPDATE curriculum_studio.webhook_deliveries SET lease_owner=$2,lease_expires_at=now()+($3 * interval '1 second'),attempt_started_at=now(),attempt_count=attempt_count+1 WHERE id=$1 RETURNING id,endpoint_id,event_id,idempotency_key,status,attempt_count,last_error,delivered_at,lease_owner,lease_expires_at,attempt_started_at,created_at`, id, owner, leaseTTL.Seconds()))
		return e
	})
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}
func (r *WebhookDeliveryRepo) MarkDelivered(ctx context.Context, id uuid.UUID, owner string) (*domain.WebhookDelivery, error) {
	return r.finishDelivery(ctx, id, owner, "delivered", "")
}
func (r *WebhookDeliveryRepo) MarkFailed(ctx context.Context, id uuid.UUID, owner, message string) (*domain.WebhookDelivery, error) {
	return r.finishDelivery(ctx, id, owner, "failed", message)
}
func (r *WebhookDeliveryRepo) finishDelivery(ctx context.Context, id uuid.UUID, owner, status, message string) (*domain.WebhookDelivery, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanWebhookDelivery(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.webhook_deliveries SET status=$3,last_error=$4,delivered_at=CASE WHEN $3='delivered' THEN now() ELSE delivered_at END,lease_owner='',lease_expires_at=NULL,attempt_started_at=NULL WHERE id=$1 AND lease_owner=$2 AND lease_expires_at>now() RETURNING id,endpoint_id,event_id,idempotency_key,status,attempt_count,last_error,delivered_at,lease_owner,lease_expires_at,attempt_started_at,created_at`, id, owner, status, message))
	if e != nil {
		if e == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w", ErrLeaseLost)
		}
		return nil, MapError(e)
	}
	return out, nil
}

// IdempotencyRepo stores inbound request deduplication state.
type IdempotencyRepo struct{ Q Querier }

func NewIdempotencyRepo(q Querier) *IdempotencyRepo { return &IdempotencyRepo{Q: q} }
func (r *IdempotencyRepo) Begin(ctx context.Context, workspaceID uuid.UUID, scope, key, requestHash string) (*domain.IdempotencyKey, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || strings.TrimSpace(scope) == "" || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("workspace, scope, and key are required")
	}
	const q = `INSERT INTO curriculum_studio.idempotency_keys(workspace_id,scope,key,request_hash) VALUES($1,$2,$3,$4) ON CONFLICT (workspace_id,scope,key) DO NOTHING RETURNING id,workspace_id,scope,key,request_hash,response_ref,created_at`
	out, e := scanIdempotency(r.Q.QueryRow(ctx, q, workspaceID, scope, key, requestHash))
	if e == nil {
		return out, nil
	}
	if e != pgx.ErrNoRows {
		return nil, MapError(e)
	}
	out, e = scanIdempotency(r.Q.QueryRow(ctx, `SELECT id,workspace_id,scope,key,request_hash,response_ref,created_at FROM curriculum_studio.idempotency_keys WHERE workspace_id=$1 AND scope=$2 AND key=$3`, workspaceID, scope, key))
	if e != nil {
		return nil, MapError(e)
	}
	if out.RequestHash != requestHash {
		return nil, fmt.Errorf("%w: idempotency request hash mismatch", ErrConflict)
	}
	return out, nil
}
func (r *IdempotencyRepo) SetResponse(ctx context.Context, workspaceID uuid.UUID, scope, key, responseRef string) (*domain.IdempotencyKey, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	out, e := scanIdempotency(r.Q.QueryRow(ctx, `UPDATE curriculum_studio.idempotency_keys SET response_ref=$4 WHERE workspace_id=$1 AND scope=$2 AND key=$3 RETURNING id,workspace_id,scope,key,request_hash,response_ref,created_at`, workspaceID, scope, key, responseRef))
	if e != nil {
		return nil, MapError(e)
	}
	return out, nil
}

func scanOutboxEvent(s interface{ Scan(...any) error }) (*domain.OutboxEvent, error) {
	v := new(domain.OutboxEvent)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.EventType, &v.AggregateKind, &v.AggregateID, &v.Payload, &v.CreatedAt, &v.PublishedAt)
	return v, e
}
func scanWebhookEndpoint(s interface{ Scan(...any) error }) (*domain.WebhookEndpoint, error) {
	v := new(domain.WebhookEndpoint)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.URL, &v.SecretRef, &v.EventTypes, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanWebhookDelivery(s interface{ Scan(...any) error }) (*domain.WebhookDelivery, error) {
	v := new(domain.WebhookDelivery)
	e := s.Scan(&v.ID, &v.EndpointID, &v.EventID, &v.IdempotencyKey, &v.Status, &v.AttemptCount, &v.LastError, &v.DeliveredAt, &v.LeaseOwner, &v.LeaseExpiresAt, &v.AttemptStartedAt, &v.CreatedAt)
	return v, e
}
func scanIdempotency(s interface{ Scan(...any) error }) (*domain.IdempotencyKey, error) {
	v := new(domain.IdempotencyKey)
	e := s.Scan(&v.ID, &v.WorkspaceID, &v.Scope, &v.Key, &v.RequestHash, &v.ResponseRef, &v.CreatedAt)
	return v, e
}
