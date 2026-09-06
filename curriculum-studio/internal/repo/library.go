package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Pool-backed library saves read every graph table from the same MVCC snapshot.
// Existing transactions retain their caller's isolation level.
func withLibrarySnapshot(ctx context.Context, q Querier, fn func(Querier) error) error {
	if beginner, ok := q.(interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	}); ok {
		tx, err := beginner.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if err = fn(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	return WithTx(ctx, q, fn)
}

// SaveUnit captures a self-contained unit: its outcomes, evidence, standards,
// projects and resource links. Only prerequisites internal to the unit are
// copied; unrelated graph nodes and collaboration records are not library data.
func (r *UnitLibraryRepo) SaveUnit(ctx context.Context, ws, revision, unitID uuid.UUID, name string) (*domain.UnitLibraryEntry, error) {
	if r == nil || r.Q == nil {
		return nil, ErrClosed
	}
	var entry *domain.UnitLibraryEntry
	err := withLibrarySnapshot(ctx, r.Q, func(q Querier) error {
		graph, err := NewPlanGraphRepo(q).Load(ctx, ws, revision)
		if err != nil {
			return err
		}
		var snapshot domain.UnitLibrarySnapshot
		for _, u := range graph.Units {
			if u.ID == unitID {
				snapshot.Unit = u
			}
		}
		if snapshot.Unit.ID == uuid.Nil {
			return ErrNotFound
		}
		if strings.TrimSpace(name) == "" {
			name = snapshot.Unit.Title
		}
		for _, a := range graph.LearningArcs {
			if snapshot.Unit.LearningArcID != nil && a.ID == *snapshot.Unit.LearningArcID {
				v := a
				snapshot.Arc = &v
			}
		}
		roles := map[uuid.UUID]string{}
		projects := map[uuid.UUID]bool{}
		for _, link := range graph.UnitOutcomes {
			if link.UnitID == unitID {
				roles[link.OutcomeID] = link.Role
			}
		}
		for _, p := range graph.Projects {
			if p.UnitID != nil && *p.UnitID == unitID {
				snapshot.Projects = append(snapshot.Projects, p)
				projects[p.ID] = true
			}
		}
		for _, link := range graph.ProjectOutcomes {
			if projects[link.ProjectID] {
				snapshot.ProjectOutcomes = append(snapshot.ProjectOutcomes, link)
				if _, ok := roles[link.OutcomeID]; !ok {
					roles[link.OutcomeID] = ""
				}
			}
		}
		objectives := map[uuid.UUID]bool{}
		for _, o := range graph.Outcomes {
			role, ok := roles[o.ID]
			if !ok {
				continue
			}
			item := domain.LibraryOutcome{Outcome: o, Role: role}
			if o.ObjectiveID != nil {
				objectives[*o.ObjectiveID] = true
			}
			for _, s := range graph.OutcomeStandardMappings {
				if s.OutcomeID == o.ID {
					item.Standards = append(item.Standards, s)
				}
			}
			for _, e := range graph.EvidenceRequirements {
				if e.OutcomeID == o.ID {
					item.Evidence = append(item.Evidence, e)
				}
			}
			snapshot.Outcomes = append(snapshot.Outcomes, item)
		}
		for _, o := range graph.Objectives {
			if objectives[o.ID] {
				snapshot.Objectives = append(snapshot.Objectives, o)
			}
		}
		for _, p := range graph.OutcomePrerequisites {
			_, a := roles[p.OutcomeID]
			_, b := roles[p.PrerequisiteID]
			if a && b {
				snapshot.Prerequisites = append(snapshot.Prerequisites, p)
			}
		}
		for _, link := range graph.PlanResources {
			if (link.UnitID != nil && *link.UnitID == unitID) || (link.ProjectID != nil && projects[*link.ProjectID]) {
				snapshot.Resources = append(snapshot.Resources, link)
			}
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		entry, err = NewUnitLibraryRepo(q).Create(ctx, &domain.UnitLibraryEntry{WorkspaceID: ws, Name: name, Blueprint: raw})
		return err
	})
	return entry, MapError(err)
}

// CopyIntoRevision remaps every copied plan ID inside a single transaction.
// Library rows are workspace-owned, even if a curriculum has a read-only share.
func (r *UnitLibraryRepo) CopyIntoRevision(ctx context.Context, ws, revision, entryID uuid.UUID) (*domain.Unit, error) {
	if r == nil || r.Q == nil {
		return nil, ErrClosed
	}
	var unit *domain.Unit
	err := WithTx(ctx, r.Q, func(q Querier) error {
		rev, err := NewPlanRevisionRepo(q).Get(ctx, ws, revision)
		if err != nil {
			return err
		}
		if rev.Status != "draft" {
			return ErrImmutable
		}
		entry, err := NewUnitLibraryRepo(q).Get(ctx, ws, entryID)
		if err != nil {
			return err
		}
		var snap domain.UnitLibrarySnapshot
		if err = json.Unmarshal(entry.Blueprint, &snap); err != nil {
			return fmt.Errorf("%w: invalid library snapshot", ErrCheckViolation)
		}
		if strings.TrimSpace(snap.Unit.Title) == "" {
			return ErrCheckViolation
		}
		g := NewPlanGraphRepo(q)
		// Codes are unique in each revision; copies retain display names but receive
		// new codes so repeated imports cannot alias an existing node.
		suffix := "-" + uuid.NewString()
		ids := map[uuid.UUID]uuid.UUID{}
		for _, v := range snap.Objectives {
			old := v.ID
			v.PlanRevisionID = revision
			v.Code += suffix
			obj, e := g.CreateObjective(ctx, ws, &v)
			if e != nil {
				return e
			}
			ids[old] = obj.ID
		}
		snap.Unit.PlanRevisionID = revision
		snap.Unit.Code += suffix
		snap.Unit.LearningArcID = nil
		if snap.Arc != nil {
			a := *snap.Arc
			a.PlanRevisionID = revision
			a.Code += suffix
			arc, e := g.CreateArc(ctx, ws, &a)
			if e != nil {
				return e
			}
			snap.Unit.LearningArcID = &arc.ID
		}
		unit, err = g.CreateUnit(ctx, ws, &snap.Unit)
		if err != nil {
			return err
		}
		for _, item := range snap.Outcomes {
			o := item.Outcome
			old := o.ID
			o.PlanRevisionID = revision
			o.Code += suffix
			if o.ObjectiveID != nil {
				id, ok := ids[*o.ObjectiveID]
				if !ok {
					return ErrCheckViolation
				}
				o.ObjectiveID = &id
			}
			created, e := g.CreateOutcome(ctx, ws, &o)
			if e != nil {
				return e
			}
			ids[old] = created.ID
			if item.Role != "" {
				if _, e = g.CreateUnitOutcome(ctx, ws, &domain.UnitOutcome{UnitID: unit.ID, OutcomeID: created.ID, Role: item.Role}); e != nil {
					return e
				}
			}
			for _, s := range item.Standards {
				s.OutcomeID = created.ID
				if _, e = g.CreateMapping(ctx, ws, &s); e != nil {
					return e
				}
			}
			for _, evidence := range item.Evidence {
				evidence.PlanRevisionID = revision
				evidence.OutcomeID = created.ID
				if _, e = g.CreateEvidenceRequirement(ctx, ws, &evidence); e != nil {
					return e
				}
			}
		}
		for _, p := range snap.Prerequisites {
			p.PlanRevisionID = revision
			p.OutcomeID = ids[p.OutcomeID]
			p.PrerequisiteID = ids[p.PrerequisiteID]
			if _, err = g.CreatePrerequisite(ctx, ws, &p); err != nil {
				return err
			}
		}
		for _, p := range snap.Projects {
			old := p.ID
			p.PlanRevisionID = revision
			p.UnitID = &unit.ID
			p.Code += suffix
			v, e := g.CreateProject(ctx, ws, &p)
			if e != nil {
				return e
			}
			ids[old] = v.ID
		}
		for _, link := range snap.ProjectOutcomes {
			link.ProjectID = ids[link.ProjectID]
			link.OutcomeID = ids[link.OutcomeID]
			if _, err = g.CreateProjectOutcome(ctx, ws, &link); err != nil {
				return err
			}
		}
		for _, link := range snap.Resources {
			link.PlanRevisionID = revision
			if link.UnitID != nil {
				link.UnitID = &unit.ID
			}
			if link.ProjectID != nil {
				id, ok := ids[*link.ProjectID]
				if !ok {
					return ErrCheckViolation
				}
				link.ProjectID = &id
			}
			if _, err = g.CreatePlanResource(ctx, ws, &link); err != nil {
				return err
			}
		}
		return nil
	})
	return unit, MapError(err)
}

func (r *UnitLibraryRepo) ListPage(ctx context.Context, ws uuid.UUID, limit, offset int) ([]domain.UnitLibraryEntry, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, ErrClosed
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := r.Q.QueryRow(ctx, `SELECT count(*) FROM curriculum_studio.unit_library_entries WHERE workspace_id=$1`, ws).Scan(&total); err != nil {
		return nil, 0, MapError(err)
	}
	entries, err := listRows(ctx, r.Q, `SELECT id,workspace_id,name,blueprint,created_at,updated_at FROM curriculum_studio.unit_library_entries WHERE workspace_id=$1 ORDER BY name,id LIMIT $2 OFFSET $3`, []any{ws, limit, offset}, func(s scanner) (*domain.UnitLibraryEntry, error) { return scanUnitLibrary(s) })
	if entries == nil {
		entries = []domain.UnitLibraryEntry{}
	}
	return entries, total, err
}

func (r *TemplateRepo) Create(ctx context.Context, ws uuid.UUID, code, name, briefType string, seed domain.TemplateSeed) (*domain.PlanTemplate, error) {
	if r == nil || r.Q == nil {
		return nil, ErrClosed
	}
	if ws == uuid.Nil || strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" {
		return nil, ErrCheckViolation
	}
	if err := ValidateTemplateSeed(seed); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(seed)
	if err != nil {
		return nil, err
	}
	v, err := scanTemplate(r.Q.QueryRow(ctx, `INSERT INTO curriculum_studio.plan_templates(workspace_id,code,name,brief_type,seed) VALUES($1,$2,$3,$4,$5) RETURNING id,workspace_id,code,name,brief_type,seed,created_at`, ws, strings.TrimSpace(code), strings.TrimSpace(name), briefType, raw))
	return v, MapError(err)
}

func ValidateTemplateSeed(seed domain.TemplateSeed) error {
	if len(seed.Outcomes)+len(seed.Units) == 0 || len(seed.Outcomes) > 200 || len(seed.Units) > 200 {
		return ErrCheckViolation
	}
	codes := map[string]bool{}
	for _, o := range seed.Outcomes {
		if o.DisplayName() == "" {
			return ErrCheckViolation
		}
		if o.Code != "" {
			if codes[o.Code] {
				return ErrCheckViolation
			}
			codes[o.Code] = true
		}
	}
	units := map[string]bool{}
	for _, u := range seed.Units {
		if u.DisplayName() == "" {
			return ErrCheckViolation
		}
		if u.Code != "" {
			if units[u.Code] {
				return ErrCheckViolation
			}
			units[u.Code] = true
		}
		seen := map[string]bool{}
		for _, code := range u.OutcomeCodes {
			if !codes[code] || seen[code] {
				return ErrCheckViolation
			}
			seen[code] = true
		}
	}
	return nil
}

func (r *TemplateRepo) ListPage(ctx context.Context, ws uuid.UUID, limit, offset int) ([]domain.PlanTemplate, int, error) {
	if r == nil || r.Q == nil {
		return nil, 0, ErrClosed
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	const visible = ` FROM curriculum_studio.plan_templates t WHERE workspace_id=$1 OR (workspace_id IS NULL AND NOT EXISTS(SELECT 1 FROM curriculum_studio.plan_templates w WHERE w.workspace_id=$1 AND w.code=t.code))`
	var total int
	if err := r.Q.QueryRow(ctx, `SELECT count(*)`+visible, ws).Scan(&total); err != nil {
		return nil, 0, MapError(err)
	}
	items, err := listRows(ctx, r.Q, `SELECT id,workspace_id,code,name,brief_type,seed,created_at`+visible+` ORDER BY name,id LIMIT $2 OFFSET $3`, []any{ws, limit, offset}, func(s scanner) (*domain.PlanTemplate, error) { return scanTemplate(s) })
	if items == nil {
		items = []domain.PlanTemplate{}
	}
	return items, total, err
}
