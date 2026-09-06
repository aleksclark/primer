package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/authz"
	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/aleksclark/primer/curriculum-studio/internal/validation"
)

const (
	curriculumIDPrefix = "cur_"
	revisionIDPrefix   = "prev_"
	objectiveIDPrefix  = "obj_"
	outcomeIDPrefix    = "out_"
	arcIDPrefix        = "arc_"
	unitIDPrefix       = "unit_"
	projectIDPrefix    = "proj_"
	evidenceIDPrefix   = "ev_"
	constraintIDPrefix = "sc_"
)

func encodePlanID(prefix string, id uuid.UUID) string { return prefix + compactUUID(id) }
func decodePlanID(raw string, prefix string) (uuid.UUID, error) {
	return decodeOpaqueUUID(raw, prefix, "plan resource")
}

func curriculumID(id uuid.UUID) string { return encodePlanID(curriculumIDPrefix, id) }
func revisionID(id uuid.UUID) string   { return encodePlanID(revisionIDPrefix, id) }

func curriculumStatus(status string) CurriculumStatus {
	if status == "retired" {
		return "archived"
	}
	return "active"
}
func revisionState(status string) RevisionState { return RevisionState(status) }

func curriculumView(v *domain.Curriculum) Curriculum {
	return Curriculum{ID: curriculumID(v.ID), WorkspaceID: encodeWSID(v.WorkspaceID), Name: v.Title, Description: v.Description, Template: CurriculumTemplate(v.Approach), Status: curriculumStatus(v.Status), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func revisionView(v *domain.PlanRevision) PlanRevision {
	return PlanRevision{ID: revisionID(v.ID), CurriculumID: curriculumID(v.CurriculumID), RevisionNum: v.Revision, State: revisionState(v.Status), Brief: briefView(v.Brief), ETag: revisionETag(v), PublishedBy: v.PublishedBySubjectRef, PublishedAt: v.PublishedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func briefView(raw json.RawMessage) *CurriculumBrief {
	if len(raw) == 0 || string(raw) == "{}" {
		return nil
	}
	var b CurriculumBrief
	if json.Unmarshal(raw, &b) != nil {
		return nil
	}
	return &b
}
func revisionETag(v *domain.PlanRevision) string {
	return fmt.Sprintf(`W/"%s-%d"`, v.ID.String(), v.UpdatedAt.UnixNano())
}

func (s *Server) curriculumForCaller(ctx context.Context, raw string) (*domain.Curriculum, uuid.UUID, MembershipView, error) {
	id, err := decodePlanID(raw, curriculumIDPrefix)
	if err != nil {
		return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
	}
	for _, membership := range MembershipsFromContext(ctx) {
		v, e := repo.NewCurriculumRepo(s.querier).Get(ctx, membership.WorkspaceID, id)
		if e == nil {
			return v, membership.WorkspaceID, membership, nil
		}
	}
	return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
}
func (s *Server) revisionForCaller(ctx context.Context, raw string) (*domain.PlanRevision, uuid.UUID, MembershipView, error) {
	id, err := decodePlanID(raw, revisionIDPrefix)
	if err != nil {
		return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
	}
	for _, membership := range MembershipsFromContext(ctx) {
		v, e := repo.NewPlanRevisionRepo(s.querier).Get(ctx, membership.WorkspaceID, id)
		if e == nil {
			return v, membership.WorkspaceID, membership, nil
		}
	}
	return nil, uuid.Nil, MembershipView{}, repo.ErrNotFound
}
func requireAuthor(ctx context.Context, m MembershipView) error {
	if !authz.CanMutate(m.Role) {
		return huma.Error403Forbidden("forbidden")
	}
	return nil
}
func planError(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, repo.ErrConflict):
		return huma.Error409Conflict("conflict")
	case errors.Is(err, repo.ErrImmutable), errors.Is(err, repo.ErrInvalidTransition):
		return huma.Error409Conflict("revision is not editable")
	case errors.Is(err, repo.ErrForeignKey), errors.Is(err, repo.ErrCheckViolation), errors.Is(err, repo.ErrPrerequisiteCycle):
		return huma.Error400BadRequest("invalid plan graph")
	default:
		return huma.Error503ServiceUnavailable("plan operation failed")
	}
}

func (s *Server) registerPlanRoutes(api huma.API) {
	s.registerCurriculumPlanRoutes(api)
	s.registerRevisionPlanRoutes(api)
	s.registerGraphRoutes(api)
	s.registerExportRoutes(api)
}

func (s *Server) registerCurriculumPlanRoutes(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listCurricula", Method: http.MethodGet, Path: "/studio/v1/workspaces/{workspaceId}/curricula", Tags: []string{"Curricula"}}, func(ctx context.Context, in *curriculumListInput) (*curriculumPageResponse, error) {
		ws, _, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		if limit > 100 {
			limit = 100
		}
		q := strings.TrimSpace(in.Q)
		rows, err := s.querier.Query(ctx, `SELECT c.id,c.workspace_id,c.slug,c.title,c.description,c.approach,c.grade_band,c.status,c.current_draft_revision_id,c.published_revision_id,c.metadata,c.created_at,c.updated_at FROM curriculum_studio.curricula c WHERE c.workspace_id=$1 AND ($2='' OR c.title ILIKE '%'||$2||'%' OR c.description ILIKE '%'||$2||'%') ORDER BY c.title,c.id LIMIT $3 OFFSET $4`, ws, q, limit, max(0, in.Offset))
		if err != nil {
			return nil, planError(err)
		}
		defer rows.Close()
		out := &curriculumPageResponse{Body: CurriculumPage{Items: []Curriculum{}, Limit: limit, Offset: max(0, in.Offset)}}
		for rows.Next() {
			v, e := scanPlanCurriculum(rows)
			if e != nil {
				return nil, planError(e)
			}
			out.Body.Items = append(out.Body.Items, curriculumView(v))
		}
		if err := rows.Err(); err != nil {
			return nil, planError(err)
		}
		if err := s.querier.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.curricula WHERE workspace_id=$1 AND ($2='' OR title ILIKE '%'||$2||'%' OR description ILIKE '%'||$2||'%')`, ws, q).Scan(&out.Body.TotalCount); err != nil {
			return nil, planError(err)
		}
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "create-curriculum", Method: http.MethodPost, Path: "/studio/v1/workspaces/{workspaceId}/curricula", Tags: []string{"Curricula"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createCurriculumInput) (*curriculumResponse, error) {
		ws, m, err := workspaceIDFromPathForServer(s, ctx, in.WorkspaceID)
		if err != nil {
			return nil, huma.Error404NotFound("not found")
		}
		if err := requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Body.Name) == "" {
			return nil, huma.Error400BadRequest("name is required")
		}
		template := string(in.Body.Template)
		if template == "" {
			template = "custom"
		}
		approach := template
		if approach == "homeschool_year" || approach == "single_subject" || approach == "classroom_semester" || approach == "standards_remediation" {
			approach = "custom"
		}
		brief := json.RawMessage(`{}`)
		if in.Body.Brief != nil {
			encoded, e := json.Marshal(in.Body.Brief)
			if e == nil {
				brief = encoded
			}
		}
		var created *domain.Curriculum
		err = repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
			created, err = repo.NewCurriculumRepo(q).Create(ctx, &domain.Curriculum{WorkspaceID: ws, Slug: slugify(in.Body.Name) + "-" + uuid.NewString()[:8], Title: strings.TrimSpace(in.Body.Name), Description: in.Body.Description, Approach: approach, Metadata: brief})
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]string{"curriculum_id": created.ID.String()})
			_, err = q.Exec(ctx, `INSERT INTO curriculum_studio.outbox_events(workspace_id,event_type,aggregate_kind,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ws, domain.EventCurriculumCreated, "curriculum", created.ID, payload)
			return err
		})
		if err != nil {
			return nil, planError(err)
		}
		return &curriculumResponse{Body: curriculumView(created)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "getCurriculum", Method: http.MethodGet, Path: "/studio/v1/curricula/{curriculumId}", Tags: []string{"Curricula"}}, func(ctx context.Context, in *curriculumPath) (*curriculumResponse, error) {
		v, _, _, e := s.curriculumForCaller(ctx, in.CurriculumID)
		if e != nil {
			return nil, planError(e)
		}
		return &curriculumResponse{Body: curriculumView(v)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "updateCurriculum", Method: http.MethodPatch, Path: "/studio/v1/curricula/{curriculumId}", Tags: []string{"Curricula"}}, func(ctx context.Context, in *updateCurriculumInput) (*curriculumResponse, error) {
		v, ws, m, e := s.curriculumForCaller(ctx, in.CurriculumID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if in.Body.Name != nil {
			v.Title = strings.TrimSpace(*in.Body.Name)
		}
		if in.Body.Description != nil {
			v.Description = *in.Body.Description
		}
		if in.Body.Status != nil && *in.Body.Status == "archived" {
			v.Status = "retired"
		}
		updated, e := repo.NewCurriculumRepo(s.querier).UpdateCore(ctx, ws, v.ID, v)
		if e != nil {
			return nil, planError(e)
		}
		return &curriculumResponse{Body: curriculumView(updated)}, nil
	})
}

func (s *Server) registerRevisionPlanRoutes(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "listRevisions", Method: http.MethodGet, Path: "/studio/v1/curricula/{curriculumId}/revisions", Tags: []string{"Revisions"}}, func(ctx context.Context, in *revisionListInput) (*revisionPageResponse, error) {
		_, ws, _, e := s.curriculumForCaller(ctx, in.CurriculumID)
		if e != nil {
			return nil, planError(e)
		}
		cid, _ := decodePlanID(in.CurriculumID, curriculumIDPrefix)
		values, e := repo.NewPlanRevisionRepo(s.querier).List(ctx, ws, cid)
		if e != nil {
			return nil, planError(e)
		}
		out := &revisionPageResponse{Body: PlanRevisionPage{Items: make([]PlanRevision, 0, len(values)), Limit: in.Limit, Offset: in.Offset}}
		for i := range values {
			out.Body.Items = append(out.Body.Items, revisionView(&values[i]))
		}
		out.Body.TotalCount = len(values)
		if out.Body.Limit <= 0 {
			out.Body.Limit = 50
		}
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "createRevision", Method: http.MethodPost, Path: "/studio/v1/curricula/{curriculumId}/revisions", Tags: []string{"Revisions"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *createRevisionInput) (*revisionResponse, error) {
		cur, ws, m, e := s.curriculumForCaller(ctx, in.CurriculumID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		values, e := repo.NewPlanRevisionRepo(s.querier).List(ctx, ws, cur.ID)
		if e != nil {
			return nil, planError(e)
		}
		next := len(values) + 1
		brief := json.RawMessage(`{}`)
		if in.Body.Brief != nil {
			if encoded, err := json.Marshal(in.Body.Brief); err == nil {
				brief = encoded
			}
		}
		created, e := repo.NewPlanRevisionRepo(s.querier).Create(ctx, ws, &domain.PlanRevision{CurriculumID: cur.ID, Revision: next, Title: cur.Title + " — draft " + strconv.Itoa(next), Brief: brief})
		if e != nil {
			return nil, planError(e)
		}
		if strings.TrimSpace(in.Body.ForkFromRevisionID) != "" {
			from, de := decodePlanID(in.Body.ForkFromRevisionID, revisionIDPrefix)
			if de != nil {
				return nil, huma.Error400BadRequest("invalid fork revision")
			}
			if e = copyDraftGraph(ctx, s.querier, ws, from, created.ID); e != nil {
				return nil, planError(e)
			}
		}
		return &revisionResponse{Body: revisionView(created)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "getRevision", Method: http.MethodGet, Path: "/studio/v1/revisions/{revisionId}", Tags: []string{"Revisions"}}, func(ctx context.Context, in *revisionPath) (*revisionResponse, error) {
		v, _, _, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		return &revisionResponse{Body: revisionView(v)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "updateRevision", Method: http.MethodPatch, Path: "/studio/v1/revisions/{revisionId}", Tags: []string{"Revisions"}}, func(ctx context.Context, in *updateRevisionInput) (*revisionResponse, error) {
		v, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if in.Body.Brief != nil {
			v.Brief, _ = json.Marshal(in.Body.Brief)
		}
		updated, e := repo.NewPlanRevisionRepo(s.querier).UpdateCore(ctx, ws, v.ID, v)
		if e != nil {
			return nil, planError(e)
		}
		return &revisionResponse{Body: revisionView(updated)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "validateRevision", Method: http.MethodPost, Path: "/studio/v1/revisions/{revisionId}/validate", Tags: []string{"Validation"}}, func(ctx context.Context, in *revisionActionInput) (*validationResponse, error) {
		rev, ws, _, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		graph, e := repo.NewPlanGraphRepo(s.querier).Load(ctx, ws, rev.ID)
		if e != nil {
			return nil, planError(e)
		}
		result := validation.Run(graph)
		report, e := repo.NewValidationReportRepo(s.querier).CreateReportWithFindings(ctx, ws, &domain.ValidationReport{PlanRevisionID: rev.ID, Status: result.Status}, result.Findings)
		if e != nil {
			return nil, planError(e)
		}
		findings, e := repo.NewValidationReportRepo(s.querier).ListFindings(ctx, ws, report.ID)
		if e != nil {
			return nil, planError(e)
		}
		return &validationResponse{Body: validationView(report, findings)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "publishRevision", Method: http.MethodPost, Path: "/studio/v1/revisions/{revisionId}/publish", Tags: []string{"Revisions"}}, func(ctx context.Context, in *revisionActionInput) (*revisionResponse, error) {
		v, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		graph, ge := repo.NewPlanGraphRepo(s.querier).Load(ctx, ws, v.ID)
		if ge != nil {
			return nil, planError(ge)
		}
		if result := validation.Run(graph); result.Status == "failed" {
			return nil, huma.Error409Conflict("revision validation failed")
		}
		principal, _ := AuthFromContext(ctx)
		if e = repo.NewPlanRevisionRepo(s.querier).Publish(ctx, ws, v.ID, principal.SubjectRef); e != nil {
			return nil, planError(e)
		}
		v, e = repo.NewPlanRevisionRepo(s.querier).Get(ctx, ws, v.ID)
		if e != nil {
			return nil, planError(e)
		}
		return &revisionResponse{Body: revisionView(v)}, nil
	})
}

func validationView(report *domain.ValidationReport, findings []domain.ValidationFinding) ValidationReport {
	out := ValidationReport{ID: report.ID.String(), RevisionID: revisionID(report.PlanRevisionID), Findings: make([]ValidationFinding, 0, len(findings)), Passed: report.Status == "passed", CreatedAt: report.CreatedAt}
	for _, f := range findings {
		v := ValidationFinding{Rule: ValidationRule(f.Code), Severity: ValidationSeverity(f.Severity), Message: f.Message}
		if f.NodeID != nil {
			v.NodeIDs = []string{f.NodeID.String()}
		}
		out.Findings = append(out.Findings, v)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) registerGraphRoutes(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "getRevisionGraph", Method: http.MethodGet, Path: "/studio/v1/revisions/{revisionId}/graph", Tags: []string{"Graph"}}, func(ctx context.Context, in *revisionPath) (*graphResponse, error) {
		rev, ws, _, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		graph, e := repo.NewPlanGraphRepo(s.querier).Load(ctx, ws, rev.ID)
		if e != nil {
			return nil, planError(e)
		}
		return &graphResponse{Body: graphView(graph)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "replaceRevisionGraph", Method: http.MethodPut, Path: "/studio/v1/revisions/{revisionId}/graph", Tags: []string{"Graph"}}, func(ctx context.Context, in *graphInput) (*graphResponse, error) {
		rev, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if rev.Status != "draft" {
			return nil, huma.Error409Conflict("revision is not editable")
		}
		graph, e := s.replaceGraph(ctx, ws, rev.ID, in.Body)
		if e != nil {
			return nil, planError(e)
		}
		return &graphResponse{Body: graphView(graph)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "createPlanNode", Method: http.MethodPost, Path: "/studio/v1/revisions/{revisionId}/nodes", Tags: []string{"Graph"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *nodeWriteInput) (*nodeResponse, error) {
		rev, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if rev.Status != "draft" {
			return nil, huma.Error409Conflict("revision is not editable")
		}
		node, e := s.createNode(ctx, ws, rev.ID, in.Body)
		if e != nil {
			return nil, planError(e)
		}
		return &nodeResponse{Body: node}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "createPlanEdge", Method: http.MethodPost, Path: "/studio/v1/revisions/{revisionId}/edges", Tags: []string{"Graph"}, DefaultStatus: http.StatusCreated}, func(ctx context.Context, in *edgeWriteInput) (*edgeResponse, error) {
		rev, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if rev.Status != "draft" {
			return nil, huma.Error409Conflict("revision is not editable")
		}
		switch in.Body.Kind {
		case "parent_child":
			if e = createParentChildEdge(ctx, repo.NewPlanGraphRepo(s.querier), ws, in.Body.FromNodeID, in.Body.ToNodeID, in.Body.Note); e != nil {
				return nil, planError(e)
			}
		case "uses_resource":
			if e = createUsesResourceEdge(ctx, repo.NewPlanGraphRepo(s.querier), ws, rev.ID, in.Body.FromNodeID, in.Body.ToNodeID, in.Body.Note); e != nil {
				return nil, planError(e)
			}
		case "prerequisite":
			fromID, de := decodePlanID(in.Body.FromNodeID, outcomeIDPrefix)
			if de != nil {
				return nil, huma.Error400BadRequest("invalid from node")
			}
			toID, de := decodePlanID(in.Body.ToNodeID, outcomeIDPrefix)
			if de != nil {
				return nil, huma.Error400BadRequest("invalid to node")
			}
			_, e = repo.NewPlanGraphRepo(s.querier).CreatePrerequisite(ctx, ws, &domain.OutcomePrerequisite{PlanRevisionID: rev.ID, OutcomeID: toID, PrerequisiteID: fromID, Requirement: in.Body.Note})
			if e != nil {
				return nil, planError(e)
			}
		default:
			return nil, huma.Error400BadRequest("unsupported edge kind")
		}
		return &edgeResponse{Body: PlanEdge{ID: "edge_" + compactUUID(uuid.New()), RevisionID: revisionID(rev.ID), Kind: in.Body.Kind, FromNodeID: in.Body.FromNodeID, ToNodeID: in.Body.ToNodeID, Note: in.Body.Note}}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "deletePlanNode", Method: http.MethodDelete, Path: "/studio/v1/revisions/{revisionId}/nodes/{nodeId}", Tags: []string{"Graph"}, DefaultStatus: http.StatusNoContent}, func(ctx context.Context, in *nodePath) (*struct{}, error) {
		rev, ws, m, e := s.revisionForCaller(ctx, in.RevisionID)
		if e != nil {
			return nil, planError(e)
		}
		if e = requireAuthor(ctx, m); e != nil {
			return nil, e
		}
		if rev.Status != "draft" {
			return nil, huma.Error409Conflict("revision is not editable")
		}
		id, table, e := nodeTable(in.NodeID)
		if e != nil {
			return nil, huma.Error404NotFound("not found")
		}
		_, e = s.querier.Exec(ctx, `DELETE FROM curriculum_studio.`+table+` WHERE id=$1 AND plan_revision_id=$2`, id, rev.ID)
		if e != nil {
			return nil, planError(e)
		}
		_ = ws
		return nil, nil
	})
}

func graphView(g *domain.PlanGraph) PlanGraph {
	out := PlanGraph{RevisionID: revisionID(g.Revision.ID), Nodes: []PlanNode{}, Edges: []PlanEdge{}, ETag: revisionETag(&g.Revision)}
	for _, v := range g.Objectives {
		out.Nodes = append(out.Nodes, PlanNode{ID: encodePlanID(objectiveIDPrefix, v.ID), RevisionID: out.RevisionID, Kind: "objective", Title: v.Title, Body: v.Description, Position: v.Position, Attributes: map[string]string{"code": v.Code}})
	}
	evidenceByOutcome := map[uuid.UUID]domain.EvidenceRequirement{}
	for _, e := range g.EvidenceRequirements {
		if _, ok := evidenceByOutcome[e.OutcomeID]; !ok {
			evidenceByOutcome[e.OutcomeID] = e
		}
	}
	codesByOutcome := map[uuid.UUID][]string{}
	for _, m := range g.OutcomeStandardMappings {
		if m.StandardCode == "" {
			continue
		}
		codesByOutcome[m.OutcomeID] = append(codesByOutcome[m.OutcomeID], m.StandardCode)
	}
	for _, v := range g.Outcomes {
		attrs := map[string]string{"code": v.Code, "masteryCriteria": v.MasteryCriteria}
		if e, ok := evidenceByOutcome[v.ID]; ok {
			attrs["evidenceKind"] = e.Kind
			attrs["evidenceDescription"] = e.Description
		}
		out.Nodes = append(out.Nodes, PlanNode{ID: encodePlanID(outcomeIDPrefix, v.ID), RevisionID: out.RevisionID, Kind: "outcome", Title: v.Title, Body: v.Description, Position: v.Position, StandardCodes: codesByOutcome[v.ID], Attributes: attrs})
	}
	for _, v := range g.LearningArcs {
		out.Nodes = append(out.Nodes, PlanNode{ID: encodePlanID(arcIDPrefix, v.ID), RevisionID: out.RevisionID, Kind: "learning_arc", Title: v.Title, Body: v.Description, Position: v.Position, Attributes: map[string]string{"code": v.Code}})
	}
	for _, v := range g.Units {
		out.Nodes = append(out.Nodes, PlanNode{ID: encodePlanID(unitIDPrefix, v.ID), RevisionID: out.RevisionID, Kind: "unit", Title: v.Title, Position: v.Position, Attributes: map[string]string{"code": v.Code}})
	}
	for _, v := range g.Projects {
		out.Nodes = append(out.Nodes, projectNode(out.RevisionID, v))
	}
	for _, e := range g.OutcomePrerequisites {
		out.Edges = append(out.Edges, PlanEdge{ID: "edge_" + compactUUID(e.ID), RevisionID: out.RevisionID, Kind: "prerequisite", FromNodeID: encodePlanID(outcomeIDPrefix, e.PrerequisiteID), ToNodeID: encodePlanID(outcomeIDPrefix, e.OutcomeID), Note: e.Requirement})
	}
	for _, e := range g.UnitOutcomes {
		out.Edges = append(out.Edges, PlanEdge{ID: "edge_" + compactUUID(e.UnitID) + compactUUID(e.OutcomeID), RevisionID: out.RevisionID, Kind: "parent_child", FromNodeID: encodePlanID(unitIDPrefix, e.UnitID), ToNodeID: encodePlanID(outcomeIDPrefix, e.OutcomeID), Note: e.Role})
	}
	for _, e := range g.ProjectOutcomes {
		out.Edges = append(out.Edges, PlanEdge{ID: "edge_" + compactUUID(e.ProjectID) + compactUUID(e.OutcomeID), RevisionID: out.RevisionID, Kind: "parent_child", FromNodeID: encodePlanID(projectIDPrefix, e.ProjectID), ToNodeID: encodePlanID(outcomeIDPrefix, e.OutcomeID), Note: e.Role})
	}
	for _, e := range g.PlanResources {
		if e.ProjectID == nil {
			continue
		}
		out.Edges = append(out.Edges, PlanEdge{ID: "edge_" + compactUUID(e.ID), RevisionID: out.RevisionID, Kind: "uses_resource", FromNodeID: encodePlanID(projectIDPrefix, *e.ProjectID), ToNodeID: encodeResourceID(e.ResourceID), Note: e.Role})
	}
	return out
}

func projectNode(revisionID string, v domain.Project) PlanNode {
	attrs := map[string]string{"code": v.Code}
	phases, err := domain.ParseProjectPhases(v.Phases)
	if err != nil {
		attrs["phasesJSON"] = "[]"
	} else {
		raw, e := json.Marshal(phases)
		if e != nil {
			attrs["phasesJSON"] = "[]"
		} else {
			attrs["phasesJSON"] = string(raw)
		}
	}
	return PlanNode{ID: encodePlanID(projectIDPrefix, v.ID), RevisionID: revisionID, Kind: "project", Title: v.Title, Body: v.Description, Position: v.Position, Attributes: attrs}
}

func (s *Server) replaceGraph(ctx context.Context, ws, rev uuid.UUID, in PlanGraphWrite) (*domain.PlanGraph, error) {
	var out *domain.PlanGraph
	err := repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
		clears := []string{
			`DELETE FROM curriculum_studio.plan_resources WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.scheduling_constraints WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.evidence_requirements WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.project_outcomes po USING curriculum_studio.projects p WHERE po.project_id=p.id AND p.plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.unit_outcomes uo USING curriculum_studio.units u WHERE uo.unit_id=u.id AND u.plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.projects WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.units WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.learning_arcs WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.outcome_prerequisites WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.outcome_standard_mappings osm USING curriculum_studio.outcomes o WHERE osm.outcome_id=o.id AND o.plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.outcomes WHERE plan_revision_id=$1`,
			`DELETE FROM curriculum_studio.objectives WHERE plan_revision_id=$1`,
		}
		for _, statement := range clears {
			if _, e := q.Exec(ctx, statement, rev); e != nil {
				return e
			}
		}
		graphRepo := repo.NewPlanGraphRepo(q)
		ids := map[string]string{}
		for i, n := range in.Nodes {
			node, e := s.createNodeWithRepo(ctx, graphRepo, ws, rev, n)
			if e != nil {
				return e
			}
			ids[fmt.Sprintf("%d", i)] = node.ID
			if n.ID != "" {
				ids[n.ID] = node.ID
			}
		}
		for _, edge := range in.Edges {
			from, to := edgeNodeIDs(edge, ids)
			switch edge.Kind {
			case "prerequisite":
				fromID, _ := decodePlanID(from, outcomeIDPrefix)
				toID, _ := decodePlanID(to, outcomeIDPrefix)
				_, e := graphRepo.CreatePrerequisite(ctx, ws, &domain.OutcomePrerequisite{PlanRevisionID: rev, OutcomeID: toID, PrerequisiteID: fromID, Requirement: edge.Note})
				if e != nil {
					return e
				}
			case "parent_child":
				if e := createParentChildEdge(ctx, graphRepo, ws, from, to, edge.Note); e != nil {
					return e
				}
			case "uses_resource":
				if e := createUsesResourceEdge(ctx, graphRepo, ws, rev, from, to, edge.Note); e != nil {
					return e
				}
			default:
				return fmt.Errorf("%w: unsupported edge kind %q", repo.ErrCheckViolation, edge.Kind)
			}
		}
		loaded, loadErr := graphRepo.Load(ctx, ws, rev)
		out = loaded
		return loadErr
	})
	return out, err
}
func edgeNodeIDs(e PlanEdgeWrite, ids map[string]string) (string, string) {
	resolve := func(raw string) string {
		if i, err := strconv.Atoi(raw); err == nil {
			if id, ok := ids[strconv.Itoa(i)]; ok {
				return id
			}
		}
		return raw
	}
	return resolve(e.FromNodeID), resolve(e.ToNodeID)
}
func createParentChildEdge(ctx context.Context, r *repo.PlanGraphRepo, ws uuid.UUID, from, to, note string) error {
	role := strings.TrimSpace(note)
	if role == "" {
		role = domain.ProjectOutcomeRoleTarget
	}
	switch {
	case strings.HasPrefix(from, projectIDPrefix) && strings.HasPrefix(to, outcomeIDPrefix):
		projectID, err := decodePlanID(from, projectIDPrefix)
		if err != nil {
			return err
		}
		outcomeID, err := decodePlanID(to, outcomeIDPrefix)
		if err != nil {
			return err
		}
		_, err = r.CreateProjectOutcome(ctx, ws, &domain.ProjectOutcome{ProjectID: projectID, OutcomeID: outcomeID, Role: role})
		return err
	case strings.HasPrefix(from, unitIDPrefix) && strings.HasPrefix(to, outcomeIDPrefix):
		unitID, err := decodePlanID(from, unitIDPrefix)
		if err != nil {
			return err
		}
		outcomeID, err := decodePlanID(to, outcomeIDPrefix)
		if err != nil {
			return err
		}
		_, err = r.CreateUnitOutcome(ctx, ws, &domain.UnitOutcome{UnitID: unitID, OutcomeID: outcomeID, Role: role})
		return err
	default:
		return fmt.Errorf("%w: unsupported parent_child endpoints", repo.ErrCheckViolation)
	}
}

func createUsesResourceEdge(ctx context.Context, r *repo.PlanGraphRepo, ws, rev uuid.UUID, from, to, note string) error {
	if !strings.HasPrefix(from, projectIDPrefix) {
		return fmt.Errorf("%w: uses_resource requires a project source", repo.ErrCheckViolation)
	}
	projectID, err := decodePlanID(from, projectIDPrefix)
	if err != nil {
		return err
	}
	resourceID, err := decodeResourceID(to)
	if err != nil {
		return err
	}
	role := strings.TrimSpace(note)
	if role == "" {
		role = "required"
	}
	_, err = r.CreatePlanResource(ctx, ws, &domain.PlanResource{PlanRevisionID: rev, ResourceID: resourceID, ProjectID: &projectID, Role: role})
	return err
}

func projectPhasesFromAttributes(attrs map[string]string) (json.RawMessage, error) {
	if attrs == nil {
		return json.RawMessage(`[]`), nil
	}
	raw := strings.TrimSpace(attrs["phasesJSON"])
	if raw == "" {
		return json.RawMessage(`[]`), nil
	}
	phases, err := domain.ParseProjectPhases(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}
	return json.Marshal(phases)
}

func nodeUUID(raw string) uuid.UUID {
	for _, prefix := range []string{objectiveIDPrefix, outcomeIDPrefix, arcIDPrefix, unitIDPrefix, projectIDPrefix} {
		if strings.HasPrefix(raw, prefix) {
			id, _ := decodePlanID(raw, prefix)
			return id
		}
	}
	id, _ := uuid.Parse(raw)
	return id
}

func (s *Server) createNode(ctx context.Context, ws, rev uuid.UUID, in PlanNodeWrite) (PlanNode, error) {
	var node PlanNode
	err := repo.WithTx(ctx, s.querier, func(q repo.Querier) error {
		created, e := s.createNodeWithRepo(ctx, repo.NewPlanGraphRepo(q), ws, rev, in)
		if e != nil {
			return e
		}
		node = created
		return nil
	})
	return node, err
}
func (s *Server) createNodeWithRepo(ctx context.Context, r *repo.PlanGraphRepo, ws, rev uuid.UUID, in PlanNodeWrite) (PlanNode, error) {
	code := in.Attributes["code"]
	if code == "" {
		code = strings.ToLower(string(in.Kind)) + "-" + uuid.NewString()[:8]
	}
	switch in.Kind {
	case "objective":
		v, e := r.CreateObjective(ctx, ws, &domain.Objective{PlanRevisionID: rev, Code: code, Title: in.Title, Description: in.Body, Position: in.Position})
		if e != nil {
			return PlanNode{}, e
		}
		return PlanNode{ID: encodePlanID(objectiveIDPrefix, v.ID), RevisionID: revisionID(rev), Kind: in.Kind, Title: v.Title, Body: v.Description, Position: v.Position, Attributes: map[string]string{"code": v.Code}}, nil
	case "outcome":
		v, e := r.CreateOutcome(ctx, ws, &domain.Outcome{PlanRevisionID: rev, Code: code, Title: in.Title, Description: in.Body, MasteryCriteria: in.Attributes["masteryCriteria"], Position: in.Position})
		if e != nil {
			return PlanNode{}, e
		}
		if e = mapOutcomeStandardCodes(ctx, r, ws, v.ID, in.StandardCodes); e != nil {
			return PlanNode{}, e
		}
		if kind := strings.TrimSpace(in.Attributes["evidenceKind"]); kind != "" {
			if _, e = r.CreateEvidenceRequirement(ctx, ws, &domain.EvidenceRequirement{PlanRevisionID: rev, OutcomeID: v.ID, Kind: kind, Description: in.Attributes["evidenceDescription"]}); e != nil {
				return PlanNode{}, e
			}
		}
		return PlanNode{ID: encodePlanID(outcomeIDPrefix, v.ID), RevisionID: revisionID(rev), Kind: in.Kind, Title: v.Title, Body: v.Description, Position: v.Position, StandardCodes: append([]string{}, in.StandardCodes...), Attributes: map[string]string{"code": v.Code}}, nil
	case "learning_arc":
		v, e := r.CreateArc(ctx, ws, &domain.LearningArc{PlanRevisionID: rev, Code: code, Title: in.Title, Description: in.Body, Position: in.Position})
		if e != nil {
			return PlanNode{}, e
		}
		return PlanNode{ID: encodePlanID(arcIDPrefix, v.ID), RevisionID: revisionID(rev), Kind: in.Kind, Title: v.Title, Body: v.Description, Position: v.Position, Attributes: map[string]string{"code": v.Code}}, nil
	case "unit":
		var estimated *int
		if in.EstimatedMinutes > 0 {
			estimated = &in.EstimatedMinutes
		}
		v, e := r.CreateUnit(ctx, ws, &domain.Unit{PlanRevisionID: rev, Code: code, Title: in.Title, Position: in.Position, EstimatedMinutes: estimated})
		if e != nil {
			return PlanNode{}, e
		}
		return PlanNode{ID: encodePlanID(unitIDPrefix, v.ID), RevisionID: revisionID(rev), Kind: in.Kind, Title: v.Title, Position: v.Position, Attributes: map[string]string{"code": v.Code}}, nil
	case "project":
		phases, e := projectPhasesFromAttributes(in.Attributes)
		if e != nil {
			return PlanNode{}, e
		}
		v, e := r.CreateProject(ctx, ws, &domain.Project{PlanRevisionID: rev, Code: code, Title: in.Title, Description: in.Body, Position: in.Position, Phases: phases})
		if e != nil {
			return PlanNode{}, e
		}
		return projectNode(revisionID(rev), *v), nil
	default:
		return PlanNode{}, fmt.Errorf("unsupported node kind %q", in.Kind)
	}
}
func mapOutcomeStandardCodes(ctx context.Context, r *repo.PlanGraphRepo, ws, outcomeID uuid.UUID, codes []string) error {
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		var standardID uuid.UUID
		err := r.Q.QueryRow(ctx, `SELECT s.id FROM curriculum_studio.catalog_standards s JOIN curriculum_studio.standard_frameworks f ON f.id=s.framework_id WHERE s.code=$1 AND (f.workspace_id IS NULL OR f.workspace_id=$2) ORDER BY f.workspace_id NULLS LAST, s.id LIMIT 1`, code, ws).Scan(&standardID)
		if err != nil {
			return fmt.Errorf("%w: unknown standard %q", repo.ErrNotFound, code)
		}
		if _, err := r.CreateOutcomeStandardMapping(ctx, ws, &domain.OutcomeStandardMapping{OutcomeID: outcomeID, StandardID: standardID, Alignment: "addresses"}); err != nil {
			return err
		}
	}
	return nil
}

func nodeTable(raw string) (uuid.UUID, string, error) {
	for _, p := range []struct{ prefix, table string }{{objectiveIDPrefix, "objectives"}, {outcomeIDPrefix, "outcomes"}, {arcIDPrefix, "learning_arcs"}, {unitIDPrefix, "units"}, {projectIDPrefix, "projects"}} {
		if strings.HasPrefix(raw, p.prefix) {
			id, e := decodePlanID(raw, p.prefix)
			return id, p.table, e
		}
	}
	return uuid.Nil, "", fmt.Errorf("unknown node")
}

// copyDraftGraph creates a new editable graph from a published revision. The
// copy is table-backed; outcome/objective relationships are rejoined by their
// stable codes rather than leaking source UUIDs into the new draft.
func copyDraftGraph(ctx context.Context, q repo.Querier, ws, from, to uuid.UUID) error {
	return repo.WithTx(ctx, q, func(tx repo.Querier) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT r.status FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$2`, from, ws).Scan(&state); err != nil {
			return err
		}
		if state != "published" {
			return fmt.Errorf("%w: fork source is not published", repo.ErrInvalidTransition)
		}
		statements := []string{
			`INSERT INTO curriculum_studio.objectives(plan_revision_id,code,title,description,position) SELECT $2,code,title,description,position FROM curriculum_studio.objectives WHERE plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.outcomes(plan_revision_id,objective_id,code,title,description,mastery_criteria,position) SELECT $2,no.id,o.code,o.title,o.description,o.mastery_criteria,o.position FROM curriculum_studio.outcomes o LEFT JOIN curriculum_studio.objectives oo ON oo.id=o.objective_id LEFT JOIN curriculum_studio.objectives no ON no.plan_revision_id=$2 AND no.code=oo.code WHERE o.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.outcome_standard_mappings(outcome_id,standard_id,alignment,notes) SELECT no.id,m.standard_id,m.alignment,m.notes FROM curriculum_studio.outcome_standard_mappings m JOIN curriculum_studio.outcomes oo ON oo.id=m.outcome_id JOIN curriculum_studio.outcomes no ON no.plan_revision_id=$2 AND no.code=oo.code WHERE oo.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.learning_arcs(plan_revision_id,code,title,description,position) SELECT $2,code,title,description,position FROM curriculum_studio.learning_arcs WHERE plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.units(plan_revision_id,learning_arc_id,code,title,essential_questions,estimated_minutes,position,blueprint) SELECT $2,na.id,u.code,u.title,u.essential_questions,u.estimated_minutes,u.position,u.blueprint FROM curriculum_studio.units u LEFT JOIN curriculum_studio.learning_arcs oa ON oa.id=u.learning_arc_id LEFT JOIN curriculum_studio.learning_arcs na ON na.plan_revision_id=$2 AND na.code=oa.code WHERE u.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.projects(plan_revision_id,unit_id,code,title,description,phases,estimated_minutes,position) SELECT $2,nu.id,p.code,p.title,p.description,p.phases,p.estimated_minutes,p.position FROM curriculum_studio.projects p LEFT JOIN curriculum_studio.units ou ON ou.id=p.unit_id LEFT JOIN curriculum_studio.units nu ON nu.plan_revision_id=$2 AND nu.code=ou.code WHERE p.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.outcome_prerequisites(plan_revision_id,outcome_id,prerequisite_id,requirement) SELECT $2,no.id,np.id,pr.requirement FROM curriculum_studio.outcome_prerequisites pr JOIN curriculum_studio.outcomes oo ON oo.id=pr.outcome_id JOIN curriculum_studio.outcomes op ON op.id=pr.prerequisite_id JOIN curriculum_studio.outcomes no ON no.plan_revision_id=$2 AND no.code=oo.code JOIN curriculum_studio.outcomes np ON np.plan_revision_id=$2 AND np.code=op.code WHERE pr.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.evidence_requirements(plan_revision_id,outcome_id,kind,description,criteria) SELECT $2,no.id,e.kind,e.description,e.criteria FROM curriculum_studio.evidence_requirements e JOIN curriculum_studio.outcomes oo ON oo.id=e.outcome_id JOIN curriculum_studio.outcomes no ON no.plan_revision_id=$2 AND no.code=oo.code WHERE e.plan_revision_id=$1`,
			`INSERT INTO curriculum_studio.scheduling_constraints(plan_revision_id,kind,payload) SELECT $2,kind,payload FROM curriculum_studio.scheduling_constraints WHERE plan_revision_id=$1`,
		}
		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement, from, to); err != nil {
				return err
			}
		}
		return nil
	})
}

type planScanner interface{ Scan(...any) error }

func scanPlanCurriculum(s planScanner) (*domain.Curriculum, error) {
	v := new(domain.Curriculum)
	err := s.Scan(&v.ID, &v.WorkspaceID, &v.Slug, &v.Title, &v.Description, &v.Approach, &v.GradeBand, &v.Status, &v.CurrentDraftRevisionID, &v.PublishedRevisionID, &v.Metadata, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

var _ = time.Time{}
