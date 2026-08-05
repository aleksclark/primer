package contracts

import (
	"fmt"
	"path"
	"strings"
)

// ValidateCourseDocument validates a course manifest.
func ValidateCourseDocument(doc *CourseDocument) error {
	if doc == nil {
		return fmt.Errorf("course document is nil")
	}
	if doc.SchemaVersion != CourseSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q (want %q)", doc.SchemaVersion, CourseSchemaVersion)
	}
	if !slugRE.MatchString(doc.Slug) {
		return fmt.Errorf("slug %q is invalid", doc.Slug)
	}
	if strings.TrimSpace(doc.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(doc.SubjectCode) == "" {
		return fmt.Errorf("subjectCode is required")
	}
	if doc.RevisionPolicy != "" {
		switch doc.RevisionPolicy {
		case RevisionPolicyLatestPublished, RevisionPolicyPinnedDigest:
		default:
			return fmt.Errorf("revisionPolicy %q is unknown", doc.RevisionPolicy)
		}
	}
	if doc.PacingReference != nil {
		p := doc.PacingReference
		if p.NominalWeeks < 0 || p.NominalDaysPerWeek < 0 || p.NominalMinutesPerDay < 0 {
			return fmt.Errorf("pacingReference: nominal values must be non-negative")
		}
	}
	if err := validateContinuity(doc.ContinuityDefaults, "continuityDefaults"); err != nil {
		return err
	}
	if len(doc.Activities) == 0 {
		return fmt.Errorf("activities must not be empty")
	}

	seenOrder := map[int]bool{}
	seenSlug := map[string]bool{}
	for i, a := range doc.Activities {
		prefix := fmt.Sprintf("activities[%d]", i)
		if a.Order < 1 {
			return fmt.Errorf("%s.order must be >= 1", prefix)
		}
		if seenOrder[a.Order] {
			return fmt.Errorf("%s.order %d is duplicate", prefix, a.Order)
		}
		seenOrder[a.Order] = true
		if !slugRE.MatchString(a.Slug) {
			return fmt.Errorf("%s.slug %q is invalid", prefix, a.Slug)
		}
		if seenSlug[a.Slug] {
			return fmt.Errorf("%s.slug %q is duplicate", prefix, a.Slug)
		}
		seenSlug[a.Slug] = true
		if a.File != "" {
			if path.IsAbs(a.File) || strings.Contains(a.File, "..") {
				return fmt.Errorf("%s.file must be a relative path without ..", prefix)
			}
		}
		if err := validateContinuity(a.Continuity, prefix+".continuity"); err != nil {
			return err
		}
	}
	// Contiguous order starting at 1 is required for stable sequencing.
	for i := 1; i <= len(doc.Activities); i++ {
		if !seenOrder[i] {
			return fmt.Errorf("activities: missing order %d (orders must be contiguous from 1)", i)
		}
	}

	moduleIDs := map[string]bool{}
	for i, m := range doc.Modules {
		prefix := fmt.Sprintf("modules[%d]", i)
		if !idRE.MatchString(m.ID) {
			return fmt.Errorf("%s.id %q is invalid", prefix, m.ID)
		}
		if moduleIDs[m.ID] {
			return fmt.Errorf("%s.id %q is duplicate", prefix, m.ID)
		}
		moduleIDs[m.ID] = true
		if strings.TrimSpace(m.Title) == "" {
			return fmt.Errorf("%s.title is required", prefix)
		}
		for j, slug := range m.Activities {
			if !seenSlug[slug] {
				return fmt.Errorf("%s.activities[%d]: unknown activity slug %q", prefix, j, slug)
			}
		}
	}

	// Build adjacency for cycle detection on prerequisites.
	adj := map[string][]string{}
	for i, p := range doc.Prerequisites {
		prefix := fmt.Sprintf("prerequisites[%d]", i)
		if !seenSlug[p.Activity] {
			return fmt.Errorf("%s.activity %q is not in activities", prefix, p.Activity)
		}
		if len(p.Requires) == 0 {
			return fmt.Errorf("%s.requires must not be empty", prefix)
		}
		if p.Requirement != "" {
			switch p.Requirement {
			case PrereqCompleted, PrereqApproaching, PrereqMastered:
			default:
				return fmt.Errorf("%s.requirement %q is unknown", prefix, p.Requirement)
			}
		}
		for j, req := range p.Requires {
			if !seenSlug[req] {
				return fmt.Errorf("%s.requires[%d]: unknown activity slug %q", prefix, j, req)
			}
			if req == p.Activity {
				return fmt.Errorf("%s: activity cannot require itself", prefix)
			}
			adj[p.Activity] = append(adj[p.Activity], req)
		}
	}
	if err := detectSlugCycles(adj); err != nil {
		return fmt.Errorf("prerequisites: %w", err)
	}

	for i, g := range doc.Gates {
		prefix := fmt.Sprintf("gates[%d]", i)
		if !seenSlug[g.Activity] {
			return fmt.Errorf("%s.activity %q is not in activities", prefix, g.Activity)
		}
		switch g.Kind {
		case GateEvidence, GateParentReview:
		default:
			return fmt.Errorf("%s.kind %q is unknown", prefix, g.Kind)
		}
		for j, code := range g.Standards {
			if !standardRE.MatchString(code) {
				return fmt.Errorf("%s.standards[%d]: invalid code %q", prefix, j, code)
			}
		}
	}

	for i, r := range doc.Remediation {
		prefix := fmt.Sprintf("remediation[%d]", i)
		if !seenSlug[r.ForActivity] {
			return fmt.Errorf("%s.forActivity %q is not in activities", prefix, r.ForActivity)
		}
		if !slugRE.MatchString(r.BranchSlug) {
			return fmt.Errorf("%s.branchSlug %q is invalid", prefix, r.BranchSlug)
		}
		if r.Kind != "" {
			switch r.Kind {
			case "remediation", "reinforcement":
			default:
				return fmt.Errorf("%s.kind %q is unknown", prefix, r.Kind)
			}
		}
	}

	return nil
}

func validateContinuity(c *ContinuityPolicy, field string) error {
	if c == nil {
		return nil
	}
	switch c.Mode {
	case ContinuityFresh, ContinuityOptionalPrevious, ContinuityRequiredProject, ContinuityPortfolioReview:
		return nil
	case "":
		return fmt.Errorf("%s.mode is required", field)
	default:
		return fmt.Errorf("%s.mode %q is unknown", field, c.Mode)
	}
}

func detectSlugCycles(adj map[string][]string) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(string) error
	visit = func(n string) error {
		if color[n] == gray {
			return fmt.Errorf("cycle involving %q", n)
		}
		if color[n] == black {
			return nil
		}
		color[n] = gray
		for _, m := range adj[n] {
			if err := visit(m); err != nil {
				return err
			}
		}
		color[n] = black
		return nil
	}
	for n := range adj {
		if err := visit(n); err != nil {
			return err
		}
	}
	return nil
}
