package api

// This file contains the authoring HTTP boundary. The handlers intentionally
// return empty fixture values for now: C6 owns the wire shapes and shared
// registration path, while persistence and product behavior land in later
// platform/database waves. The DTOs are explicit so Huma, rather than a hand
// copied schema, emits the authoring OpenAPI contract.

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Closed authoring wire enums. Their values are parity-checked against proto
// and the Studio database; they are not a second enum catalog.
type ErrorCode string
type CurriculumTemplate string
type CurriculumStatus string
type RevisionState string
type NodeKind string
type EdgeKind string
type ValidationSeverity string
type ValidationRule string
type StandardSource string
type ResourceKind string
type MaterializationStatus string
type MaterializedItemKind string
type ItemLockState string
type MaterializedItemStatus string
type ExportFormat string
type EventTypeName string

func enumSchema(values ...string) *huma.Schema {
	enum := make([]any, len(values))
	for i, value := range values {
		enum[i] = value
	}
	return &huma.Schema{Type: "string", Enum: enum}
}

func registerEnumComponents(api huma.API) {
	components := api.OpenAPI().Components.Schemas.Map()
	components["ErrorCode"] = enumSchema("unauthenticated", "permission_denied", "not_found", "already_exists", "failed_precondition", "aborted", "invalid_argument", "resource_locked", "revision_immutable", "validation_failed", "materialization_failed", "idempotency_key_conflict", "quota_exceeded", "internal")
	components["CurriculumTemplate"] = enumSchema("homeschool_year", "single_subject", "classroom_semester", "project_based_unit", "standards_remediation", "custom")
	components["CurriculumStatus"] = enumSchema("active", "archived")
	components["RevisionState"] = enumSchema("draft", "published", "superseded")
	components["NodeKind"] = enumSchema("objective", "outcome", "learning_arc", "unit", "project", "lesson_slot", "evidence_requirement", "assessment_plan", "scheduling_policy")
	components["EdgeKind"] = enumSchema("parent_child", "prerequisite", "addresses_standard", "uses_resource", "produces_evidence", "sequence")
	components["ValidationSeverity"] = enumSchema("info", "warning", "error")
	components["ValidationRule"] = enumSchema("standard_exists", "prerequisite_acyclic", "outcome_has_evidence", "item_traces_to_node", "revision_immutable", "locked_content_preserved", "materialization_snapshot_complete", "assessment_has_key_or_rubric", "coverage_gap", "workload_overload", "unassessed_outcome")
	components["StandardSource"] = enumSchema("tennessee", "common_core", "custom")
	components["ResourceKind"] = enumSchema("book", "document", "video", "tool", "project_supply", "url")
	components["MaterializationStatus"] = enumSchema("requested", "running", "ready", "failed", "cancelled")
	components["MaterializedItemKind"] = enumSchema("lesson", "teacher_guide", "student_instructions", "practice", "assignment", "discussion_guide", "worksheet", "assessment", "rubric", "project_task", "answer_key", "media_prompt", "printable_packet", "session_spec")
	components["ItemLockState"] = enumSchema("editable", "locked")
	components["MaterializedItemStatus"] = enumSchema("draft", "ready", "published", "superseded")
	components["ExportFormat"] = enumSchema("markdown", "pdf", "docx", "csv_coverage", "json_bundle", "ical")
	components["EventTypeName"] = enumSchema("curriculum.created", "plan_revision.published", "materialization.requested", "materialization.ready", "materialization.failed", "materialized_item.superseded", "plan_change.proposed")
	// Huma's standard problem envelope is already referenced by generated
	// error responses. Add the shared machine code at that boundary so the
	// ErrorCode component is emitted and remains the single error enum source.
	if problem := components["ErrorModel"]; problem != nil {
		problem.Properties["code"] = enumRef("ErrorCode")
		problem.Required = append(problem.Required, "code")
	}
}

func enumRef(name string) *huma.Schema                       { return &huma.Schema{Ref: "#/components/schemas/" + name} }
func (ErrorCode) Schema(huma.Registry) *huma.Schema          { return enumRef("ErrorCode") }
func (CurriculumTemplate) Schema(huma.Registry) *huma.Schema { return enumRef("CurriculumTemplate") }
func (CurriculumStatus) Schema(huma.Registry) *huma.Schema   { return enumRef("CurriculumStatus") }
func (RevisionState) Schema(huma.Registry) *huma.Schema      { return enumRef("RevisionState") }
func (NodeKind) Schema(huma.Registry) *huma.Schema           { return enumRef("NodeKind") }
func (EdgeKind) Schema(huma.Registry) *huma.Schema           { return enumRef("EdgeKind") }
func (ValidationSeverity) Schema(huma.Registry) *huma.Schema { return enumRef("ValidationSeverity") }
func (ValidationRule) Schema(huma.Registry) *huma.Schema     { return enumRef("ValidationRule") }
func (StandardSource) Schema(huma.Registry) *huma.Schema     { return enumRef("StandardSource") }
func (ResourceKind) Schema(huma.Registry) *huma.Schema       { return enumRef("ResourceKind") }
func (MaterializationStatus) Schema(huma.Registry) *huma.Schema {
	return enumRef("MaterializationStatus")
}
func (MaterializedItemKind) Schema(huma.Registry) *huma.Schema {
	return enumRef("MaterializedItemKind")
}
func (ItemLockState) Schema(huma.Registry) *huma.Schema { return enumRef("ItemLockState") }
func (MaterializedItemStatus) Schema(huma.Registry) *huma.Schema {
	return enumRef("MaterializedItemStatus")
}
func (ExportFormat) Schema(huma.Registry) *huma.Schema  { return enumRef("ExportFormat") }
func (EventTypeName) Schema(huma.Registry) *huma.Schema { return enumRef("EventTypeName") }

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name" minLength:"1"`
	Slug      string    `json:"slug,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WorkspaceCreate struct {
	Name string `json:"name" minLength:"1"`
	Slug string `json:"slug,omitempty"`
}

type WorkspaceUpdate struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`
}

type CurriculumBrief struct {
	LearnerLevel            string   `json:"learnerLevel,omitempty"`
	Subjects                []string `json:"subjects,omitempty"`
	TimeHorizon             string   `json:"timeHorizon,omitempty"`
	AcademicGoals           []string `json:"academicGoals,omitempty"`
	StandardsJurisdiction   string   `json:"standardsJurisdiction,omitempty"`
	AvailableMinutesPerWeek int      `json:"availableMinutesPerWeek,omitempty"`
	ResourceIDs             []string `json:"resourceIds,omitempty"`
	DesiredProjects         []string `json:"desiredProjects,omitempty"`
	TestingRequirements     []string `json:"testingRequirements,omitempty"`
	Philosophy              string   `json:"philosophy,omitempty"`
}

type Curriculum struct {
	ID          string             `json:"id"`
	WorkspaceID string             `json:"workspaceId"`
	Name        string             `json:"name" minLength:"1"`
	Description string             `json:"description,omitempty"`
	Template    CurriculumTemplate `json:"template,omitempty"`
	Status      CurriculumStatus   `json:"status"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type CurriculumCreate struct {
	Name        string             `json:"name" minLength:"1"`
	Description string             `json:"description,omitempty"`
	Template    CurriculumTemplate `json:"template,omitempty"`
	Brief       *CurriculumBrief   `json:"brief,omitempty"`
}

type CurriculumUpdate struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Status      *CurriculumStatus `json:"status,omitempty"`
}

type PlanRevision struct {
	ID           string           `json:"id"`
	CurriculumID string           `json:"curriculumId"`
	RevisionNum  int              `json:"revisionNumber,omitempty"`
	State        RevisionState    `json:"state"`
	Brief        *CurriculumBrief `json:"brief,omitempty"`
	ETag         string           `json:"etag"`
	PublishedBy  string           `json:"publishedBy,omitempty"`
	PublishedAt  *time.Time       `json:"publishedAt,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

type RevisionCreate struct {
	ForkFromRevisionID string           `json:"forkFromRevisionId,omitempty"`
	Brief              *CurriculumBrief `json:"brief,omitempty"`
}

type RevisionUpdate struct {
	Brief *CurriculumBrief `json:"brief,omitempty"`
}

type PlanNode struct {
	ID               string            `json:"id"`
	RevisionID       string            `json:"revisionId"`
	Kind             NodeKind          `json:"kind"`
	Title            string            `json:"title"`
	Body             string            `json:"body,omitempty"`
	StandardCodes    []string          `json:"standardCodes,omitempty"`
	EstimatedMinutes int               `json:"estimatedMinutes,omitempty"`
	Position         int               `json:"position"`
	Attributes       map[string]string `json:"attributes,omitempty"`
}

type PlanNodeWrite struct {
	ID               string            `json:"id,omitempty"`
	Kind             NodeKind          `json:"kind"`
	Title            string            `json:"title"`
	Body             string            `json:"body,omitempty"`
	StandardCodes    []string          `json:"standardCodes,omitempty"`
	EstimatedMinutes int               `json:"estimatedMinutes,omitempty"`
	Position         int               `json:"position,omitempty"`
	Attributes       map[string]string `json:"attributes,omitempty"`
}

type PlanEdge struct {
	ID         string   `json:"id"`
	RevisionID string   `json:"revisionId"`
	Kind       EdgeKind `json:"kind"`
	FromNodeID string   `json:"fromNodeId"`
	ToNodeID   string   `json:"toNodeId"`
	Note       string   `json:"note,omitempty"`
}

type PlanEdgeWrite struct {
	Kind       EdgeKind `json:"kind"`
	FromNodeID string   `json:"fromNodeId"`
	ToNodeID   string   `json:"toNodeId"`
	Note       string   `json:"note,omitempty"`
}

type PlanGraph struct {
	RevisionID string     `json:"revisionId"`
	Nodes      []PlanNode `json:"nodes"`
	Edges      []PlanEdge `json:"edges"`
	ETag       string     `json:"etag"`
}

type PlanGraphWrite struct {
	Nodes []PlanNodeWrite `json:"nodes"`
	Edges []PlanEdgeWrite `json:"edges"`
}

type ValidationFinding struct {
	Rule          ValidationRule     `json:"rule"`
	Severity      ValidationSeverity `json:"severity"`
	Message       string             `json:"message"`
	NodeIDs       []string           `json:"nodeIds,omitempty"`
	StandardCodes []string           `json:"standardCodes,omitempty"`
}

type ValidationReport struct {
	ID                string              `json:"id"`
	RevisionID        string              `json:"revisionId"`
	MaterializationID string              `json:"materializationId,omitempty"`
	Findings          []ValidationFinding `json:"findings"`
	Passed            bool                `json:"passed"`
	CreatedAt         time.Time           `json:"createdAt"`
}

type StandardsCatalog struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspaceId,omitempty"`
	Source       StandardSource `json:"source"`
	Jurisdiction string         `json:"jurisdiction,omitempty"`
	Title        string         `json:"title"`
	CreatedAt    time.Time      `json:"createdAt"`
}
type StandardsCatalogImport struct {
	Source       StandardSource  `json:"source"`
	Jurisdiction string          `json:"jurisdiction,omitempty"`
	Title        string          `json:"title"`
	Standards    []StandardWrite `json:"standards,omitempty"`
}
type Standard struct {
	ID                string         `json:"id"`
	CatalogID         string         `json:"catalogId"`
	Code              string         `json:"code"`
	Source            StandardSource `json:"source"`
	Subject           string         `json:"subject,omitempty"`
	Grade             string         `json:"grade,omitempty"`
	Domain            string         `json:"domain,omitempty"`
	Cluster           string         `json:"cluster,omitempty"`
	Description       string         `json:"description"`
	TCAPWeight        string         `json:"tcapWeight,omitempty"`
	MasteryCriteria   []string       `json:"masteryCriteria,omitempty"`
	PrerequisiteCodes []string       `json:"prerequisiteCodes,omitempty"`
}
type StandardWrite struct {
	Code              string         `json:"code"`
	Source            StandardSource `json:"source"`
	Subject           string         `json:"subject,omitempty"`
	Grade             string         `json:"grade,omitempty"`
	Domain            string         `json:"domain,omitempty"`
	Cluster           string         `json:"cluster,omitempty"`
	Description       string         `json:"description"`
	TCAPWeight        string         `json:"tcapWeight,omitempty"`
	MasteryCriteria   []string       `json:"masteryCriteria,omitempty"`
	PrerequisiteCodes []string       `json:"prerequisiteCodes,omitempty"`
}
type StandardCrosswalk struct {
	ID       string `json:"id"`
	FromCode string `json:"fromCode"`
	ToCode   string `json:"toCode"`
	Note     string `json:"note,omitempty"`
}

type Resource struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspaceId"`
	Kind        ResourceKind `json:"kind"`
	Title       string       `json:"title"`
	Creators    []string     `json:"creators,omitempty"`
	Locator     string       `json:"locator,omitempty"`
	Notes       string       `json:"notes,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}
type ResourceWrite struct {
	Kind     ResourceKind `json:"kind"`
	Title    string       `json:"title"`
	Creators []string     `json:"creators,omitempty"`
	Locator  string       `json:"locator,omitempty"`
	Notes    string       `json:"notes,omitempty"`
}

type GenericLearnerProfile struct {
	Label          string   `json:"label,omitempty"`
	Grade          string   `json:"grade,omitempty"`
	Accommodations []string `json:"accommodations,omitempty"`
}
type AuthoringWindow struct {
	Start            *time.Time `json:"start,omitempty"`
	End              *time.Time `json:"end,omitempty"`
	AvailableMinutes int        `json:"availableMinutes" minimum:"1"`
}
type AuthoringGenerationPolicy struct {
	Voice             string `json:"voice,omitempty"`
	IncludeAnswerKeys bool   `json:"includeAnswerKeys,omitempty"`
	IncludePrintables bool   `json:"includePrintables,omitempty"`
	MaxItems          int    `json:"maxItems,omitempty"`
}
type AuthoringMaterializeRequest struct {
	Learner    *GenericLearnerProfile     `json:"learner,omitempty"`
	Window     AuthoringWindow            `json:"window"`
	Policy     *AuthoringGenerationPolicy `json:"policy,omitempty"`
	Attributes map[string]string          `json:"attributes,omitempty"`
}
type Materialization struct {
	ID                 string                `json:"id"`
	PlanRevisionID     string                `json:"planRevisionId"`
	ContextFingerprint string                `json:"contextFingerprint"`
	Status             MaterializationStatus `json:"status"`
	ErrorMessage       string                `json:"errorMessage,omitempty"`
	CreatedAt          time.Time             `json:"createdAt"`
	CompletedAt        *time.Time            `json:"completedAt,omitempty"`
}
type AuthoringBundle struct {
	ID                 string `json:"id"`
	PlanRevisionID     string `json:"planRevisionId"`
	ContextFingerprint string `json:"contextFingerprint"`
	ItemCount          int    `json:"itemCount"`
	SessionCount       int    `json:"sessionCount,omitempty"`
}
type MaterializedItem struct {
	ID                string                 `json:"id"`
	MaterializationID string                 `json:"materializationId"`
	PlanNodeID        string                 `json:"planNodeId,omitempty"`
	Kind              MaterializedItemKind   `json:"kind"`
	Title             string                 `json:"title"`
	Body              string                 `json:"body,omitempty"`
	Status            MaterializedItemStatus `json:"status"`
	LockState         ItemLockState          `json:"lockState"`
	EditedBy          string                 `json:"editedBy,omitempty"`
	EditedAt          *time.Time             `json:"editedAt,omitempty"`
	SupersedesItemID  string                 `json:"supersedesItemId,omitempty"`
}
type MaterializedItemUpdate struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
}
type ExportCreate struct {
	Format            ExportFormat `json:"format"`
	MaterializationID string       `json:"materializationId,omitempty"`
}
type ExportJob struct {
	ID                string                `json:"id"`
	RevisionID        string                `json:"revisionId"`
	MaterializationID string                `json:"materializationId,omitempty"`
	Format            ExportFormat          `json:"format"`
	Status            MaterializationStatus `json:"status"`
	ArtifactURI       string                `json:"artifactUri,omitempty"`
	DownloadURL       string                `json:"downloadUrl,omitempty"`
	ManifestURL       string                `json:"manifestUrl,omitempty"`
	CreatedBy         string                `json:"createdBy"`
	Checksum          string                `json:"checksum,omitempty"`
	ErrorMessage      string                `json:"errorMessage,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
	CompletedAt       *time.Time            `json:"completedAt,omitempty"`
}
type DomainEvent struct {
	ID                string         `json:"id"`
	Type              EventTypeName  `json:"type"`
	WorkspaceID       string         `json:"workspaceId"`
	CurriculumID      string         `json:"curriculumId,omitempty"`
	RevisionID        string         `json:"revisionId,omitempty"`
	MaterializationID string         `json:"materializationId,omitempty"`
	ItemID            string         `json:"itemId,omitempty"`
	OccurredAt        time.Time      `json:"occurredAt"`
	Data              map[string]any `json:"data,omitempty"`
}
type WebhookEndpoint struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	URL         string          `json:"url" format:"uri"`
	EventTypes  []EventTypeName `json:"eventTypes,omitempty"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"createdAt"`
}
type WebhookEndpointWrite struct {
	URL        string          `json:"url" format:"uri"`
	EventTypes []EventTypeName `json:"eventTypes,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
}
type WebhookDelivery struct {
	ID          string    `json:"id"`
	EndpointID  string    `json:"endpointId"`
	EventID     string    `json:"eventId"`
	HTTPStatus  int       `json:"httpStatus,omitempty"`
	Attempt     int       `json:"attempt"`
	AttemptedAt time.Time `json:"attemptedAt"`
}

// Page DTOs are transport envelopes; no domain model is hidden in them.

type CurriculumPage struct {
	Items      []Curriculum `json:"items"`
	TotalCount int          `json:"totalCount"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
}
type PlanRevisionPage struct {
	Items      []PlanRevision `json:"items"`
	TotalCount int            `json:"totalCount"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
}
type StandardsCatalogPage struct {
	Items      []StandardsCatalog `json:"items"`
	TotalCount int                `json:"totalCount"`
	Limit      int                `json:"limit"`
	Offset     int                `json:"offset"`
}
type StandardPage struct {
	Items      []Standard `json:"items"`
	TotalCount int        `json:"totalCount"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}
type StandardCrosswalkPage struct {
	Items      []StandardCrosswalk `json:"items"`
	TotalCount int                 `json:"totalCount"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
}
type ResourcePage struct {
	Items      []Resource `json:"items"`
	TotalCount int        `json:"totalCount"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}
type MaterializationPage struct {
	Items      []Materialization `json:"items"`
	TotalCount int               `json:"totalCount"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}
type MaterializedItemPage struct {
	Items      []MaterializedItem `json:"items"`
	TotalCount int                `json:"totalCount"`
	Limit      int                `json:"limit"`
	Offset     int                `json:"offset"`
}
type DomainEventPage struct {
	Items      []DomainEvent `json:"items"`
	TotalCount int           `json:"totalCount"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}
type WebhookEndpointPage struct {
	Items      []WebhookEndpoint `json:"items"`
	TotalCount int               `json:"totalCount"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}
type WebhookDeliveryPage struct {
	Items      []WebhookDelivery `json:"items"`
	TotalCount int               `json:"totalCount"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}

type authoringListQuery struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"200" default:"25"`
	Offset int    `query:"offset" minimum:"0" default:"0"`
	Q      string `query:"q"`
	Sort   string `query:"sort"`
	Dir    string `query:"dir" enum:"asc,desc" default:"asc"`
}
type curriculumPath struct {
	CurriculumID string `path:"curriculumId"`
}
type revisionPath struct {
	RevisionID string `path:"revisionId"`
}
type catalogPath struct {
	CatalogID string `path:"catalogId"`
}
type standardPath struct {
	StandardID string `path:"standardId"`
}
type resourcePath struct {
	ResourceID string `path:"resourceId"`
}
type materializationPath struct {
	MaterializationID string `path:"materializationId"`
}
type itemPath struct {
	ItemID string `path:"itemId"`
}
type exportPath struct {
	ExportID string `path:"exportId"`
}
type webhookPath struct {
	WebhookID string `path:"webhookId"`
}
type nodePath struct {
	RevisionID string `path:"revisionId"`
	NodeID     string `path:"nodeId"`
}
type edgePath struct {
	RevisionID string `path:"revisionId"`
	EdgeID     string `path:"edgeId"`
}

type workspaceListInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type curriculumListInput struct {
	WorkspaceID string `path:"workspaceId"`
	Limit       int    `query:"limit" minimum:"1" maximum:"200" default:"25"`
	Offset      int    `query:"offset" minimum:"0" default:"0"`
	Q           string `query:"q"`
	Sort        string `query:"sort"`
	Dir         string `query:"dir" enum:"asc,desc" default:"asc"`
}
type revisionListInput struct {
	CurriculumID string `path:"curriculumId"`
	Limit        int    `query:"limit" minimum:"1" maximum:"200" default:"25"`
	Offset       int    `query:"offset" minimum:"0" default:"0"`
	Q            string `query:"q"`
	Sort         string `query:"sort"`
	Dir          string `query:"dir" enum:"asc,desc" default:"asc"`
}
type catalogListInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type standardListInput struct {
	authoringListQuery
	CatalogID string `path:"catalogId"`
}
type resourceListInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type materializationListInput struct {
	authoringListQuery
	RevisionID string `path:"revisionId"`
}
type itemListInput struct {
	authoringListQuery
	MaterializationID string `path:"materializationId"`
}
type eventListInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type webhookListInput struct {
	authoringListQuery
	WorkspaceID string `path:"workspaceId"`
}
type deliveryListInput struct {
	authoringListQuery
	WebhookID string `path:"webhookId"`
}

type createCurriculumInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        CurriculumCreate
}
type updateCurriculumInput struct {
	CurriculumID string `path:"curriculumId"`
	Body         CurriculumUpdate
}
type createRevisionInput struct {
	CurriculumID string `path:"curriculumId"`
	Body         RevisionCreate
}
type updateRevisionInput struct {
	RevisionID string `path:"revisionId"`
	Body       RevisionUpdate
}
type graphInput struct {
	RevisionID string `path:"revisionId"`
	Body       PlanGraphWrite
}
type nodeWriteInput struct {
	RevisionID string `path:"revisionId"`
	Body       PlanNodeWrite
}
type nodeUpdateInput struct {
	RevisionID string `path:"revisionId"`
	NodeID     string `path:"nodeId"`
	Body       PlanNodeWrite
}
type edgeWriteInput struct {
	RevisionID string `path:"revisionId"`
	Body       PlanEdgeWrite
}
type catalogImportInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        StandardsCatalogImport
}
type standardWriteInput struct {
	CatalogID string `path:"catalogId"`
	Body      StandardWrite
}
type createResourceInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        ResourceWrite
}
type updateResourceInput struct {
	ResourceID string `path:"resourceId"`
	Body       ResourceWrite
}
type createMaterializationInput struct {
	RevisionID string `path:"revisionId"`
	Headers    struct {
		IdempotencyKey string `header:"Idempotency-Key"`
	}
	Body AuthoringMaterializeRequest
}
type createExportInput struct {
	RevisionID string `path:"revisionId"`
	Headers    struct {
		IdempotencyKey string `header:"Idempotency-Key"`
	}
	Body ExportCreate
}
type updateItemInput struct {
	ItemID string `path:"itemId"`
	Body   MaterializedItemUpdate
}
type createWebhookInput struct {
	WorkspaceID string `path:"workspaceId"`
	Body        WebhookEndpointWrite
}
type updateWebhookInput struct {
	WebhookID string `path:"webhookId"`
	Body      WebhookEndpointWrite
}
type revisionActionInput struct {
	RevisionID string `path:"revisionId"`
}
type itemActionInput struct {
	ItemID string `path:"itemId"`
}

type curriculumResponse struct{ Body Curriculum }
type revisionResponse struct{ Body PlanRevision }
type nodeResponse struct{ Body PlanNode }
type edgeResponse struct{ Body PlanEdge }
type graphResponse struct{ Body PlanGraph }
type validationResponse struct{ Body ValidationReport }
type catalogResponse struct{ Body StandardsCatalog }
type standardResponse struct{ Body Standard }
type resourceResponse struct{ Body Resource }
type materializationResponse struct{ Body Materialization }
type bundleResponse struct{ Body AuthoringBundle }
type itemResponse struct{ Body MaterializedItem }
type exportResponse struct{ Body ExportJob }
type eventPageResponse struct{ Body DomainEventPage }
type webhookResponse struct{ Body WebhookEndpoint }
type deliveryPageResponse struct{ Body WebhookDeliveryPage }
type webhookPageResponse struct{ Body WebhookEndpointPage }
type curriculumPageResponse struct{ Body CurriculumPage }
type revisionPageResponse struct{ Body PlanRevisionPage }
type catalogPageResponse struct{ Body StandardsCatalogPage }
type standardPageResponse struct{ Body StandardPage }
type crosswalkPageResponse struct{ Body StandardCrosswalkPage }
type resourcePageResponse struct{ Body ResourcePage }
type materializationPageResponse struct{ Body MaterializationPage }
type itemPageResponse struct{ Body MaterializedItemPage }

func authoringOperation(id, method, path, tag, summary string) huma.Operation {
	return huma.Operation{OperationID: id, Method: method, Path: path, Tags: []string{tag}, Summary: summary, Security: []map[string][]string{{"bearerAuth": {}}}}
}

func emptyPage[T any]() T { var page T; return page }

func registerAuthoringRoutes(api huma.API) {
	// Remaining authoring operations are contract placeholders until their
	// domain waves land. Materialization routes are owned by S11 handlers.

	huma.Register(api, authoringOperation("list-webhooks", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/webhooks", "Webhooks", "List webhook endpoints"), func(context.Context, *webhookListInput) (*webhookPageResponse, error) {
		return &webhookPageResponse{}, nil
	})
	huma.Register(api, authoringOperation("create-webhook", http.MethodPost, "/studio/v1/workspaces/{workspaceId}/webhooks", "Webhooks", "Create a webhook endpoint"), func(context.Context, *createWebhookInput) (*webhookResponse, error) { return &webhookResponse{}, nil })
	huma.Register(api, authoringOperation("get-webhook", http.MethodGet, "/studio/v1/webhooks/{webhookId}", "Webhooks", "Get a webhook endpoint"), func(context.Context, *webhookPath) (*webhookResponse, error) { return &webhookResponse{}, nil })
	huma.Register(api, authoringOperation("update-webhook", http.MethodPatch, "/studio/v1/webhooks/{webhookId}", "Webhooks", "Update a webhook endpoint"), func(context.Context, *updateWebhookInput) (*webhookResponse, error) { return &webhookResponse{}, nil })
	huma.Register(api, authoringOperation("delete-webhook", http.MethodDelete, "/studio/v1/webhooks/{webhookId}", "Webhooks", "Delete a webhook endpoint"), func(context.Context, *webhookPath) (*struct{}, error) { return &struct{}{}, nil })
	huma.Register(api, authoringOperation("list-webhook-deliveries", http.MethodGet, "/studio/v1/webhooks/{webhookId}/deliveries", "Webhooks", "List webhook deliveries"), func(context.Context, *deliveryListInput) (*deliveryPageResponse, error) {
		return &deliveryPageResponse{}, nil
	})
	huma.Register(api, authoringOperation("list-events", http.MethodGet, "/studio/v1/workspaces/{workspaceId}/events", "Events", "List domain events"), func(context.Context, *eventListInput) (*eventPageResponse, error) { return &eventPageResponse{}, nil })
}
