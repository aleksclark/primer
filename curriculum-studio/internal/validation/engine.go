// Package validation contains deterministic, provider-free plan checks.
package validation

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

const (
	OutcomeUnmapped       = "OUTCOME_UNMAPPED"
	OutcomeNoEvidence     = "OUTCOME_NO_EVIDENCE"
	UnitEmpty             = "UNIT_EMPTY"
	WorkloadOverloaded    = "WORKLOAD_OVERLOAD"
	PrerequisiteCycle     = "PREREQUISITE_CYCLE"
	ProjectNoTarget       = "PROJECT_NO_TARGET"
	ProjectPhaseInvalid   = "PROJECT_PHASE_INVALID"
	PortfolioEvidence     = "PORTFOLIO_EVIDENCE"
	ReinforcementNoted    = "REINFORCEMENT_NOTED"
	ReadingMediaScheduled = "READING_MEDIA_SCHEDULED"
)

// Result is stable validation output. Findings are sorted by severity, code,
// node id, and message so identical drafts produce identical responses.
type Result struct {
	Status   string
	Findings []domain.ValidationFinding
}

func Run(g *domain.PlanGraph) Result {
	if g == nil {
		return Result{Status: "failed", Findings: []domain.ValidationFinding{{Severity: "error", Code: "REVISION_MISSING", Message: "plan revision is missing", Details: json.RawMessage(`{}`)}}}
	}
	findings := make([]domain.ValidationFinding, 0)
	mapped := make(map[uuid.UUID]bool, len(g.OutcomeStandardMappings))
	for _, m := range g.OutcomeStandardMappings {
		mapped[m.OutcomeID] = true
	}
	evidence := make(map[uuid.UUID]bool, len(g.EvidenceRequirements))
	for _, e := range g.EvidenceRequirements {
		evidence[e.OutcomeID] = true
	}
	for _, o := range g.Outcomes {
		if !mapped[o.ID] {
			findings = append(findings, finding("error", OutcomeUnmapped, fmt.Sprintf("outcome %q has no standards mapping", o.Title), "outcome", o.ID))
		}
		if !evidence[o.ID] {
			findings = append(findings, finding("warning", OutcomeNoEvidence, fmt.Sprintf("outcome %q has no evidence requirement", o.Title), "outcome", o.ID))
		}
	}
	for _, u := range g.Units {
		if len(unitOutcomesFor(g, u.ID)) == 0 {
			findings = append(findings, finding("warning", UnitEmpty, fmt.Sprintf("unit %q has no outcomes", u.Title), "unit", u.ID))
		}
	}
	if capMinutes, total := workloadCap(g), plannedMinutes(g); capMinutes > 0 && total > capMinutes {
		findings = append(findings, finding("error", WorkloadOverloaded, fmt.Sprintf("planned workload %d minutes exceeds cap %d", total, capMinutes), "revision", uuid.Nil))
	}
	if hasCycle(g) {
		findings = append(findings, finding("error", PrerequisiteCycle, "outcome prerequisites contain a cycle", "revision", uuid.Nil))
	}
	findings = append(findings, projectFindings(g)...)
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		rank := func(s string) int {
			if s == "error" {
				return 0
			}
			if s == "warning" {
				return 1
			}
			return 2
		}
		if rank(a.Severity) != rank(b.Severity) {
			return rank(a.Severity) < rank(b.Severity)
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		ai, bi := "", ""
		if a.NodeID != nil {
			ai = a.NodeID.String()
		}
		if b.NodeID != nil {
			bi = b.NodeID.String()
		}
		if ai != bi {
			return ai < bi
		}
		return a.Message < b.Message
	})
	status := "passed"
	for _, f := range findings {
		if f.Severity == "error" {
			status = "failed"
			break
		}
		if f.Severity == "warning" {
			status = "warning"
		}
	}
	return Result{Status: status, Findings: findings}
}

func finding(severity, code, message, kind string, id uuid.UUID) domain.ValidationFinding {
	var ptr *uuid.UUID
	if id != uuid.Nil {
		ptr = &id
	}
	return domain.ValidationFinding{Severity: severity, Code: code, Message: message, NodeKind: kind, NodeID: ptr, Details: json.RawMessage(`{}`)}
}
func ptrUUID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
func workloadCap(g *domain.PlanGraph) int {
	for _, c := range g.SchedulingConstraints {
		if c.Kind != "workload_cap" && c.Kind != "available_minutes" {
			continue
		}
		var v map[string]any
		if json.Unmarshal(c.Payload, &v) != nil {
			continue
		}
		for _, k := range []string{"minutes", "availableMinutes", "maxMinutes"} {
			if n, ok := v[k].(float64); ok {
				return int(n)
			}
		}
	}
	return 0
}
func plannedMinutes(g *domain.PlanGraph) int {
	total := 0
	for _, u := range g.Units {
		if u.EstimatedMinutes != nil {
			total += *u.EstimatedMinutes
		}
	}
	for _, p := range g.Projects {
		if p.EstimatedMinutes != nil {
			total += *p.EstimatedMinutes
		}
	}
	return total
}
func hasCycle(g *domain.PlanGraph) bool {
	adj := map[uuid.UUID][]uuid.UUID{}
	for _, e := range g.OutcomePrerequisites {
		adj[e.PrerequisiteID] = append(adj[e.PrerequisiteID], e.OutcomeID)
	}
	state := map[uuid.UUID]uint8{}
	var visit func(uuid.UUID) bool
	visit = func(n uuid.UUID) bool {
		if state[n] == 1 {
			return true
		}
		if state[n] == 2 {
			return false
		}
		state[n] = 1
		for _, next := range adj[n] {
			if visit(next) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for _, o := range g.Outcomes {
		if visit(o.ID) {
			return true
		}
	}
	return false
}

func unitOutcomesFor(g *domain.PlanGraph, unitID uuid.UUID) []domain.UnitOutcome {
	out := []domain.UnitOutcome{}
	for _, v := range g.UnitOutcomes {
		if v.UnitID == unitID {
			out = append(out, v)
		}
	}
	return out
}

func projectFindings(g *domain.PlanGraph) []domain.ValidationFinding {
	out := []domain.ValidationFinding{}
	byProject := map[uuid.UUID][]domain.ProjectOutcome{}
	for _, po := range g.ProjectOutcomes {
		byProject[po.ProjectID] = append(byProject[po.ProjectID], po)
	}
	outcomes := map[uuid.UUID]domain.Outcome{}
	for _, o := range g.Outcomes {
		outcomes[o.ID] = o
	}
	evidenceByOutcome := map[uuid.UUID][]domain.EvidenceRequirement{}
	for _, e := range g.EvidenceRequirements {
		evidenceByOutcome[e.OutcomeID] = append(evidenceByOutcome[e.OutcomeID], e)
	}
	for _, p := range g.Projects {
		roles := byProject[p.ID]
		hasTarget := false
		for _, role := range roles {
			switch role.Role {
			case domain.ProjectOutcomeRoleTarget:
				hasTarget = true
			case domain.ProjectOutcomeRolePrior, domain.ProjectOutcomeRoleStretch:
			default:
				out = append(out, finding("error", ProjectNoTarget, fmt.Sprintf("project %q has invalid outcome role %q", p.Title, role.Role), "project", p.ID))
			}
		}
		if !hasTarget {
			out = append(out, finding("error", ProjectNoTarget, fmt.Sprintf("project %q has no target outcome", p.Title), "project", p.ID))
		}
		if _, err := domain.ParseProjectPhases(p.Phases); err != nil {
			out = append(out, finding("error", ProjectPhaseInvalid, fmt.Sprintf("project %q has invalid phases: %s", p.Title, err.Error()), "project", p.ID))
		}
		for _, role := range roles {
			if role.Role != domain.ProjectOutcomeRoleTarget {
				continue
			}
			evidence := evidenceByOutcome[role.OutcomeID]
			hasPortfolio := false
			for _, e := range evidence {
				switch e.Kind {
				case "portfolio", "performance", "project":
					hasPortfolio = true
				}
			}
			if !hasPortfolio {
				outcomeTitle := role.OutcomeID.String()
				if o, ok := outcomes[role.OutcomeID]; ok {
					outcomeTitle = o.Title
				}
				out = append(out, finding("warning", PortfolioEvidence, fmt.Sprintf("project %q target outcome %q has no portfolio or performance evidence", p.Title, outcomeTitle), "project", p.ID))
			}
		}
	}
	for _, pr := range g.PlanResources {
		switch pr.ResourceKind {
		case domain.ResourceKindBook, domain.ResourceKindVideo, domain.ResourceKindDocument:
			label := pr.ResourceTitle
			if label == "" {
				label = pr.ResourceKind
			}
			out = append(out, finding("info", ReadingMediaScheduled, fmt.Sprintf("schedule %s %q via plan_resources", pr.ResourceKind, label), "project", ptrUUID(pr.ProjectID)))
		}
	}
	for _, m := range g.OutcomeStandardMappings {
		if m.Alignment != "reinforces" {
			continue
		}
		title := m.OutcomeID.String()
		if o, ok := outcomes[m.OutcomeID]; ok {
			title = o.Title
		}
		out = append(out, finding("info", ReinforcementNoted, fmt.Sprintf("outcome %q is flagged for reinforcement; Studio will not write LMS mastery", title), "outcome", m.OutcomeID))
	}
	return out
}
