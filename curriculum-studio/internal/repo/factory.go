package repo

import (
	"context"
	"fmt"
)

// Factory constructs domain repositories bound to a Querier.
type Factory struct {
	Q Querier
}

// NewFactory binds repositories to q. q must be non-nil.
func NewFactory(q Querier) *Factory {
	if q == nil {
		panic("repo.NewFactory: nil Querier")
	}
	return &Factory{Q: q}
}

// Ping verifies the underlying Querier can execute a trivial query.
func (f *Factory) Ping(ctx context.Context) error {
	if f == nil || f.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	var one int
	if err := f.Q.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return MapError(err)
	}
	if one != 1 {
		return fmt.Errorf("unexpected ping result %d", one)
	}
	return nil
}

// Curricula returns the workspace-scoped curriculum repository.
func (f *Factory) Curricula() *CurriculumRepo { return NewCurriculumRepo(f.Q) }

// PlanRevisions returns the revision lifecycle repository.
func (f *Factory) PlanRevisions() *PlanRevisionRepo { return NewPlanRevisionRepo(f.Q) }

// PlanGraph returns the revision-scoped graph write/read facade.
func (f *Factory) PlanGraph() *PlanGraphRepo { return NewPlanGraphRepo(f.Q) }

// ValidationReports returns the durable validation output repository.
func (f *Factory) ValidationReports() *ValidationReportRepo { return NewValidationReportRepo(f.Q) }

// LearnerProfiles returns the workspace-scoped profile repository.
func (f *Factory) LearnerProfiles() *LearnerProfileRepo { return NewLearnerProfileRepo(f.Q) }

// MaterializationRuns returns the durable run repository.
func (f *Factory) MaterializationRuns() *MaterializationRunRepo {
	return NewMaterializationRunRepo(f.Q)
}

// Workflow returns the durable stage/attempt checkpoint repository.
func (f *Factory) Workflow() *WorkflowRepo { return NewWorkflowRepo(f.Q) }

// MaterializedItems returns the generated-item lifecycle repository.
func (f *Factory) MaterializedItems() *MaterializedItemRepo { return NewMaterializedItemRepo(f.Q) }

// AssessmentSupports returns assessment support-link persistence.
func (f *Factory) AssessmentSupports() *AssessmentSupportRepo { return NewAssessmentSupportRepo(f.Q) }

// Exports returns export-job metadata persistence.
func (f *Factory) Exports() *ExportRepo { return NewExportRepo(f.Q) }

// Outbox returns the durable event repository.
func (f *Factory) Outbox() *OutboxRepo { return NewOutboxRepo(f.Q) }

// WebhookEndpoints returns webhook subscription persistence.
func (f *Factory) WebhookEndpoints() *WebhookEndpointRepo { return NewWebhookEndpointRepo(f.Q) }

// WebhookDeliveries returns leased delivery persistence.
func (f *Factory) WebhookDeliveries() *WebhookDeliveryRepo { return NewWebhookDeliveryRepo(f.Q) }

// IdempotencyKeys returns inbound idempotency persistence.
func (f *Factory) IdempotencyKeys() *IdempotencyRepo { return NewIdempotencyRepo(f.Q) }

// Health is an optional thin health repository exposed via the factory.
func (f *Factory) Health() *HealthRepo {
	return &HealthRepo{Q: f.Q}
}

// Tenants returns the tenant repository.
func (f *Factory) Tenants() *TenantRepo {
	return NewTenantRepo(f.Q)
}

// Workspaces returns the workspace repository.
func (f *Factory) Workspaces() *WorkspaceRepo {
	return NewWorkspaceRepo(f.Q)
}

// Memberships returns the workspace membership repository.
func (f *Factory) Memberships() *MembershipRepo {
	return NewMembershipRepo(f.Q)
}

// IntegrationIdentities returns the integration identity repository.
func (f *Factory) IntegrationIdentities() *IntegrationIdentityRepo {
	return NewIntegrationIdentityRepo(f.Q)
}

// Audits returns the audit-event repository.
func (f *Factory) Audits() *AuditRepo {
	return NewAuditRepo(f.Q)
}

// Frameworks returns the standards-framework repository.
func (f *Factory) Frameworks() *FrameworkRepo {
	return NewFrameworkRepo(f.Q)
}

// CatalogStandards returns the hierarchical catalog-standard repository.
func (f *Factory) CatalogStandards() *CatalogStandardRepo {
	return NewCatalogStandardRepo(f.Q)
}

// CatalogPrereqs returns the catalog prerequisite-edge repository.
func (f *Factory) CatalogPrereqs() *CatalogPrereqRepo {
	return NewCatalogPrereqRepo(f.Q)
}

// Crosswalks returns the standard-crosswalk repository.
func (f *Factory) Crosswalks() *CrosswalkRepo {
	return NewCrosswalkRepo(f.Q)
}

// Resources returns the resource-metadata repository.
func (f *Factory) Resources() *ResourceRepo {
	return NewResourceRepo(f.Q)
}

// HealthRepo exposes readiness probes used by later service wiring.
type HealthRepo struct {
	Q Querier
}

// Ready returns nil when the database answers SELECT 1.
func (h *HealthRepo) Ready(ctx context.Context) error {
	if h == nil || h.Q == nil {
		return fmt.Errorf("%w", ErrClosed)
	}
	var one int
	if err := h.Q.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return MapError(err)
	}
	return nil
}
