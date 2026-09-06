// Package projects materializes multi-subject project phases into Studio items.
// It never writes LMS mastery; reinforcement is recorded as item notes only.
package projects

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

const (
	SnapshotPhaseIDKey   = "projectPhaseId"
	SnapshotProjectIDKey = "projectId"
	BodyPhaseIDKey       = "phaseId"
	BodyToolsKey         = "toolRequirements"
	BodyEvidenceKey      = "evidenceRequirements"
	BodyReinforcementKey = "reinforcementNotes"
	BodyOffScreenKey     = "offScreen"
	BodyMasteryWriteKey  = "writesLmsMastery"
)

// Window selects a published project's phase for materialization.
type Window struct {
	ProjectID uuid.UUID
	PhaseID   string
}

// Item is a phase-scoped generated candidate. Kind is always a closed Studio
// item kind; off-screen work becomes project_task.
type Item struct {
	Kind      string
	Title     string
	Body      map[string]any
	ProjectID uuid.UUID
	PhaseID   string
	OutcomeID *uuid.UUID
}

// Result is the deterministic materialization of one project phase.
type Result struct {
	ProjectID uuid.UUID
	PhaseID   string
	Items     []Item
	Snapshot  map[string]any
}

// MaterializePhase scopes items to one project phase. Missing projects or
// unknown phase ids fail closed. Production callers still reject live models;
// this path is fully scripted and provider-free.
func MaterializePhase(g *domain.PlanGraph, window Window) (Result, error) {
	if g == nil {
		return Result{}, fmt.Errorf("plan graph is required")
	}
	project, ok := projectByID(g, window.ProjectID)
	if !ok {
		return Result{}, fmt.Errorf("project %s not found", window.ProjectID)
	}
	phases, err := domain.ParseProjectPhases(project.Phases)
	if err != nil {
		return Result{}, err
	}
	phase, ok := domain.PhaseByID(phases, window.PhaseID)
	if !ok {
		return Result{}, fmt.Errorf("phase %q not found on project %q", window.PhaseID, project.Title)
	}
	tools := toolRequirements(g, project.ID)
	evidence := evidenceForProject(g, project.ID)
	notes := reinforcementNotes(g, project.ID)
	items := make([]Item, 0)
	for _, activity := range phase.Activities {
		items = append(items, activityItem(project, phase, activity, tools, evidence, notes))
	}
	if phase.OffScreen && !hasProjectTask(items) {
		items = append(items, activityItem(project, phase, domain.ProjectActivity{
			Kind:  domain.ActivityKindOffScreen,
			Title: phase.Name + " off-screen task",
		}, tools, evidence, notes))
	}
	if len(items) == 0 {
		items = append(items, activityItem(project, phase, domain.ProjectActivity{
			Kind:  "phase",
			Title: phase.Name,
		}, tools, evidence, notes))
	}
	if len(tools) > 0 && !hasTeacherGuide(items) {
		items = append(items, Item{
			Kind:      domain.ItemKindTeacherGuide,
			Title:     project.Title + " — " + phase.Name + " teacher guide",
			ProjectID: project.ID,
			PhaseID:   phase.ID,
			Body: map[string]any{
				BodyPhaseIDKey:       phase.ID,
				BodyToolsKey:         tools,
				BodyEvidenceKey:      evidence,
				BodyReinforcementKey: notes,
				BodyMasteryWriteKey:  false,
			},
		})
	}
	return Result{
		ProjectID: project.ID,
		PhaseID:   phase.ID,
		Items:     items,
		Snapshot: map[string]any{
			SnapshotProjectIDKey: project.ID.String(),
			SnapshotPhaseIDKey:   phase.ID,
		},
	}, nil
}

func activityItem(project domain.Project, phase domain.ProjectPhase, activity domain.ProjectActivity, tools, evidence, notes []string) Item {
	title := strings.TrimSpace(activity.Title)
	if title == "" {
		title = phase.Name
	}
	kind := domain.ItemKindLesson
	offScreen := phase.OffScreen || activity.Kind == domain.ActivityKindOffScreen
	if offScreen {
		kind = domain.ItemKindProjectTask
	}
	return Item{
		Kind:      kind,
		Title:     title,
		ProjectID: project.ID,
		PhaseID:   phase.ID,
		Body: map[string]any{
			BodyPhaseIDKey:       phase.ID,
			BodyOffScreenKey:     offScreen,
			BodyToolsKey:         tools,
			BodyEvidenceKey:      evidence,
			BodyReinforcementKey: notes,
			BodyMasteryWriteKey:  false,
		},
	}
}

func projectByID(g *domain.PlanGraph, id uuid.UUID) (domain.Project, bool) {
	for _, p := range g.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return domain.Project{}, false
}

func toolRequirements(g *domain.PlanGraph, projectID uuid.UUID) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, pr := range g.PlanResources {
		if pr.ProjectID == nil || *pr.ProjectID != projectID {
			continue
		}
		switch pr.ResourceKind {
		case domain.ResourceKindTool, domain.ResourceKindProjectSupply:
		default:
			continue
		}
		label := strings.TrimSpace(pr.ResourceTitle)
		if label == "" {
			label = pr.ResourceKind
		}
		label = pr.ResourceKind + ": " + label
		if _, dup := seen[label]; dup {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func evidenceForProject(g *domain.PlanGraph, projectID uuid.UUID) []string {
	wanted := map[uuid.UUID]struct{}{}
	for _, po := range g.ProjectOutcomes {
		if po.ProjectID == projectID {
			wanted[po.OutcomeID] = struct{}{}
		}
	}
	out := []string{}
	for _, e := range g.EvidenceRequirements {
		if _, ok := wanted[e.OutcomeID]; !ok {
			continue
		}
		if e.Kind == "portfolio" || e.Kind == "performance" || e.Kind == "project" {
			desc := strings.TrimSpace(e.Description)
			if desc == "" {
				desc = e.Kind
			}
			out = append(out, e.Kind+": "+desc)
		}
	}
	return out
}

func reinforcementNotes(g *domain.PlanGraph, projectID uuid.UUID) []string {
	wanted := map[uuid.UUID]struct{}{}
	for _, po := range g.ProjectOutcomes {
		if po.ProjectID == projectID {
			wanted[po.OutcomeID] = struct{}{}
		}
	}
	titles := map[uuid.UUID]string{}
	for _, o := range g.Outcomes {
		titles[o.ID] = o.Title
	}
	out := []string{}
	for _, m := range g.OutcomeStandardMappings {
		if _, ok := wanted[m.OutcomeID]; !ok || m.Alignment != "reinforces" {
			continue
		}
		title := titles[m.OutcomeID]
		if title == "" {
			title = m.OutcomeID.String()
		}
		out = append(out, "Reinforce "+title+" in Studio; do not write LMS mastery")
	}
	return out
}

func hasProjectTask(items []Item) bool {
	for _, item := range items {
		if item.Kind == domain.ItemKindProjectTask {
			return true
		}
	}
	return false
}

func hasTeacherGuide(items []Item) bool {
	for _, item := range items {
		if item.Kind == domain.ItemKindTeacherGuide {
			return true
		}
	}
	return false
}

// ApplySnapshot merges phase identity into a canonical run snapshot object.
func ApplySnapshot(snapshot json.RawMessage, result Result) (json.RawMessage, error) {
	payload := map[string]any{}
	if len(snapshot) > 0 {
		if err := json.Unmarshal(snapshot, &payload); err != nil {
			return nil, err
		}
	}
	payload[SnapshotProjectIDKey] = result.ProjectID.String()
	payload[SnapshotPhaseIDKey] = result.PhaseID
	return json.Marshal(payload)
}
