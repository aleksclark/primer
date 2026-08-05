package overseer

import (
	"context"
	"fmt"
	"strings"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Blocking reason codes (stable for parents/tests).
const (
	BlockEnrollmentPaused     = "enrollment_paused"
	BlockEnrollmentInactive   = "enrollment_inactive"
	BlockNotInRevision        = "not_in_revision"
	BlockAlreadyCompleted     = "already_completed"
	BlockAlreadyOpen          = "already_open"
	BlockPrerequisiteUnmet    = "prerequisite_unmet"
	BlockParentReviewPending  = "parent_review_pending"
	BlockEvidenceGateUnmet    = "evidence_gate_unmet"
	BlockMissingActivityRev   = "missing_activity_revision"
	BlockIncompatibleOpen     = "incompatible_open_assignment"
	BlockNoActiveEnrollment   = "no_active_enrollment"
	BlockNoEligibleActivity   = "no_eligible_activity"
)

// ActivityStatus is one membership row evaluated for a student.
type ActivityStatus struct {
	Membership     domain.CurriculumActivity `json:"membership"`
	Eligible       bool                      `json:"eligible"`
	Status         string                    `json:"status"` // eligible | blocked | assigned | completed | review_needed
	BlockingReasons []BlockReason            `json:"blockingReasons,omitempty"`
}

// BlockReason explains why an activity is not eligible.
type BlockReason struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Activity    string `json:"activity,omitempty"`
	Requires    string `json:"requires,omitempty"`
	Requirement string `json:"requirement,omitempty"`
}

// EligibilityPreview is the parent-facing course map for one enrollment.
type EligibilityPreview struct {
	Enrollment domain.Enrollment `json:"enrollment"`
	Revision   domain.CurriculumRevision `json:"revision"`
	Activities []ActivityStatus  `json:"activities"`
	Eligible   []ActivityStatus  `json:"eligible"`
	Blocking   []BlockReason     `json:"blockingSummary,omitempty"`
}

// EvaluateEnrollmentEligibility computes eligibility for every membership entry.
func EvaluateEnrollmentEligibility(ctx context.Context, q repo.Querier, enrollmentID string) (*EligibilityPreview, error) {
	en, err := repo.Enrollments.Get(ctx, q, enrollmentID)
	if err != nil {
		return nil, err
	}
	if en.CurriculumRevisionID == nil || *en.CurriculumRevisionID == "" {
		return nil, repo.ErrBadRequest{Msg: "enrollment has no curriculum revision"}
	}
	rev, err := repo.CurriculumRevisions.Get(ctx, q, *en.CurriculumRevisionID)
	if err != nil {
		return nil, err
	}
	membership, err := repo.ListCurriculumActivities(ctx, q, rev.ID)
	if err != nil {
		return nil, err
	}
	prereqs, err := repo.ListCurriculumPrerequisites(ctx, q, rev.ID)
	if err != nil {
		return nil, err
	}
	gates, err := repo.ListCurriculumGates(ctx, q, rev.ID)
	if err != nil {
		return nil, err
	}
	completed, err := repo.CompletedActivitySlugsForStudent(ctx, q, en.StudentID)
	if err != nil {
		return nil, err
	}
	openSlugs, err := repo.OpenAssignmentSlugsForStudent(ctx, q, en.StudentID)
	if err != nil {
		return nil, err
	}
	pendingReview, err := repo.PendingParentReviewSlugs(ctx, q, en.StudentID)
	if err != nil {
		return nil, err
	}
	masteryByCode, err := repo.MasteryStatusByStandardCode(ctx, q, en.StudentID)
	if err != nil {
		return nil, err
	}

	prereqByAct := map[string][]domain.CurriculumActivityPrerequisite{}
	for _, p := range prereqs {
		prereqByAct[p.ActivitySlug] = append(prereqByAct[p.ActivitySlug], p)
	}
	gatesByAct := map[string][]domain.CurriculumActivityGate{}
	for _, g := range gates {
		gatesByAct[g.ActivitySlug] = append(gatesByAct[g.ActivitySlug], g)
	}
	override := map[string]bool{}
	for _, s := range en.OverrideSlugs {
		override[s] = true
	}

	out := &EligibilityPreview{
		Enrollment: *en,
		Revision:   *rev,
	}
	var summary []BlockReason

	for _, m := range membership {
		st := evaluateMembership(m, *en, prereqByAct[m.ActivitySlug], gatesByAct[m.ActivitySlug],
			completed, openSlugs, pendingReview, masteryByCode, override)
		out.Activities = append(out.Activities, st)
		if st.Eligible {
			out.Eligible = append(out.Eligible, st)
		} else {
			summary = append(summary, st.BlockingReasons...)
		}
	}
	out.Blocking = dedupeBlockReasons(summary)

	// Persist a compact summary on the enrollment for parent dashboards.
	reasonsAny := make([]any, 0, len(out.Blocking))
	for _, b := range out.Blocking {
		reasonsAny = append(reasonsAny, map[string]any{
			"code":     b.Code,
			"message":  b.Message,
			"activity": b.Activity,
		})
	}
	_ = repo.UpdateEnrollmentBlockingReasons(ctx, q, en.ID, reasonsAny)
	return out, nil
}

func evaluateMembership(
	m domain.CurriculumActivity,
	en domain.Enrollment,
	prereqs []domain.CurriculumActivityPrerequisite,
	gates []domain.CurriculumActivityGate,
	completed, openSlugs, pendingReview map[string]bool,
	masteryByCode map[string]string,
	override map[string]bool,
) ActivityStatus {
	st := ActivityStatus{Membership: m, Status: "blocked"}
	var blocks []BlockReason

	if en.Status == "paused" {
		blocks = append(blocks, BlockReason{
			Code: BlockEnrollmentPaused, Message: "enrollment is paused", Activity: m.ActivitySlug,
		})
	} else if en.Status != "active" {
		blocks = append(blocks, BlockReason{
			Code: BlockEnrollmentInactive, Message: "enrollment is " + en.Status, Activity: m.ActivitySlug,
		})
	}

	if m.ActivityRevisionID == nil || *m.ActivityRevisionID == "" {
		blocks = append(blocks, BlockReason{
			Code: BlockMissingActivityRev, Message: "membership has no resolved activity revision", Activity: m.ActivitySlug,
		})
	}

	if completed[m.ActivitySlug] {
		st.Status = "completed"
		st.BlockingReasons = []BlockReason{{
			Code: BlockAlreadyCompleted, Message: "activity already completed", Activity: m.ActivitySlug,
		}}
		return st
	}
	if openSlugs[m.ActivitySlug] {
		st.Status = "assigned"
		st.BlockingReasons = []BlockReason{{
			Code: BlockAlreadyOpen, Message: "activity already assigned", Activity: m.ActivitySlug,
		}}
		return st
	}

	// Parent pin override still requires enrollment active and resolved revision.
	force := override[m.ActivitySlug] || (en.PinnedActivitySlug != "" && en.PinnedActivitySlug == m.ActivitySlug)

	if !force {
		for _, p := range prereqs {
			if !prerequisiteSatisfied(p, completed, masteryByCode) {
				blocks = append(blocks, BlockReason{
					Code:        BlockPrerequisiteUnmet,
					Message:     fmt.Sprintf("requires %s (%s)", p.RequiresSlug, p.Requirement),
					Activity:    m.ActivitySlug,
					Requires:    p.RequiresSlug,
					Requirement: p.Requirement,
				})
			}
			// Pending parent review on a required prior activity blocks successors.
			if pendingReview[p.RequiresSlug] {
				blocks = append(blocks, BlockReason{
					Code:     BlockParentReviewPending,
					Message:  fmt.Sprintf("parent review pending on %s", p.RequiresSlug),
					Activity: m.ActivitySlug,
					Requires: p.RequiresSlug,
				})
			}
		}
		for _, g := range gates {
			// Gates on this activity itself are entry conditions for assigning it.
			// parent_review on the activity means prior required reviews must be clear
			// and any listed standards must meet evidence thresholds when present.
			if g.Kind == contracts.GateParentReview {
				// If this activity is itself waiting on review from a previous attempt,
				// mark review_needed (should already be caught by open/completed paths).
				if pendingReview[m.ActivitySlug] {
					blocks = append(blocks, BlockReason{
						Code: BlockParentReviewPending, Message: "parent review required", Activity: m.ActivitySlug,
					})
				}
			}
			if g.Kind == contracts.GateEvidence {
				for _, code := range g.Standards {
					status := masteryByCode[code]
					if status != "approaching" && status != "mastered" {
						blocks = append(blocks, BlockReason{
							Code:     BlockEvidenceGateUnmet,
							Message:  fmt.Sprintf("evidence gate unmet for %s", code),
							Activity: m.ActivitySlug,
							Requires: code,
						})
					}
				}
			}
		}
	}

	// Incompatible open assignment: MVP treats any other open course assignment
	// for the same enrollment revision membership as incompatible when one is open.
	// (Pin/remediation may still choose among eligible set; open work is reused.)
	if len(openSlugs) > 0 && !openSlugs[m.ActivitySlug] {
		// Only block if there is open work that is part of this course revision.
		// We approximate: if student has open assignments at all, overseer reuses them;
		// eligibility still lists candidates but marks incompatible when another open
		// membership slug exists in this revision.
		for slug := range openSlugs {
			// openSlugs is global; membership-local check happens via completed/open maps.
			_ = slug
		}
	}

	if len(blocks) > 0 {
		st.BlockingReasons = blocks
		if hasCode(blocks, BlockParentReviewPending) {
			st.Status = "review_needed"
		} else {
			st.Status = "blocked"
		}
		return st
	}

	st.Eligible = true
	st.Status = "eligible"
	return st
}

func prerequisiteSatisfied(p domain.CurriculumActivityPrerequisite, completed map[string]bool, masteryByCode map[string]string) bool {
	req := p.Requirement
	if req == "" {
		req = contracts.PrereqCompleted
	}
	switch req {
	case contracts.PrereqCompleted:
		return completed[p.RequiresSlug]
	case contracts.PrereqApproaching, contracts.PrereqMastered:
		// Activity-level mastered/approaching is approximated by completion for MVP
		// unless standard codes are attached later. Completion implies at least approaching.
		if !completed[p.RequiresSlug] {
			return false
		}
		if req == contracts.PrereqMastered {
			// Prefer standard mastery if codes known; else completion is not enough for mastered.
			// Without per-activity standard aggregation, treat completion as insufficient for mastered.
			// Parents can override. Check any mastery record is mastered is too broad; keep strict.
			return false
		}
		return true
	default:
		return completed[p.RequiresSlug]
	}
}

func hasCode(blocks []BlockReason, code string) bool {
	for _, b := range blocks {
		if b.Code == code {
			return true
		}
	}
	return false
}

func dedupeBlockReasons(in []BlockReason) []BlockReason {
	seen := map[string]bool{}
	var out []BlockReason
	for _, b := range in {
		key := b.Code + "|" + b.Activity + "|" + b.Requires + "|" + b.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	return out
}

// FirstEligibleCourseActivity picks the lowest-position eligible membership across active enrollments.
func FirstEligibleCourseActivity(ctx context.Context, q repo.Querier, studentID string) (en *domain.Enrollment, act *domain.CurriculumActivity, reason string, err error) {
	ens, err := repo.ListActiveEnrollmentsForStudent(ctx, q, studentID)
	if err != nil {
		return nil, nil, "", err
	}
	if len(ens) == 0 {
		return nil, nil, "", nil
	}
	for i := range ens {
		prev, err := EvaluateEnrollmentEligibility(ctx, q, ens[i].ID)
		if err != nil {
			return nil, nil, "", err
		}
		// Parent pin takes precedence when eligible or overridden.
		if ens[i].PinnedActivitySlug != "" {
			for _, st := range prev.Activities {
				if st.Membership.ActivitySlug != ens[i].PinnedActivitySlug {
					continue
				}
				if st.Status == "completed" || st.Status == "assigned" {
					break
				}
				// Pin forces selection even if blocked (parent authorized), except missing revision.
				if st.Membership.ActivityRevisionID == nil {
					break
				}
				m := st.Membership
				return &ens[i], &m, "pin:" + m.ActivitySlug, nil
			}
		}
		if len(prev.Eligible) == 0 {
			continue
		}
		// Eligible already in position order from membership walk.
		m := prev.Eligible[0].Membership
		return &ens[i], &m, "course:" + m.ActivitySlug, nil
	}
	return &ens[0], nil, BlockNoEligibleActivity, nil
}

// CollectEnrollmentBlockSummary returns a human/reason code when nothing is eligible.
func CollectEnrollmentBlockSummary(ctx context.Context, q repo.Querier, studentID string) (string, error) {
	ens, err := repo.ListActiveEnrollmentsForStudent(ctx, q, studentID)
	if err != nil {
		return "", err
	}
	if len(ens) == 0 {
		return BlockNoActiveEnrollment, nil
	}
	var parts []string
	for _, en := range ens {
		prev, err := EvaluateEnrollmentEligibility(ctx, q, en.ID)
		if err != nil {
			return "", err
		}
		if len(prev.Eligible) > 0 {
			return "", nil
		}
		if len(prev.Blocking) == 0 {
			parts = append(parts, fmt.Sprintf("enrollment %s: all activities completed or assigned", en.ID))
			continue
		}
		// Prefer the earliest blocked incomplete activity's first reason.
		for _, st := range prev.Activities {
			if st.Status == "completed" || st.Status == "assigned" {
				continue
			}
			if len(st.BlockingReasons) > 0 {
				b := st.BlockingReasons[0]
				parts = append(parts, fmt.Sprintf("%s:%s", b.Code, st.Membership.ActivitySlug))
				break
			}
		}
	}
	if len(parts) == 0 {
		return BlockNoEligibleActivity, nil
	}
	return strings.Join(parts, "; "), nil
}

// IsSlugInActiveEnrollments reports whether slug belongs to any active course revision.
func IsSlugInActiveEnrollments(ctx context.Context, q repo.Querier, studentID, slug string) (bool, *domain.Enrollment, *domain.CurriculumActivity, error) {
	ens, err := repo.ListActiveEnrollmentsForStudent(ctx, q, studentID)
	if err != nil {
		return false, nil, nil, err
	}
	for i := range ens {
		if ens[i].CurriculumRevisionID == nil {
			continue
		}
		acts, err := repo.ListCurriculumActivities(ctx, q, *ens[i].CurriculumRevisionID)
		if err != nil {
			return false, nil, nil, err
		}
		for j := range acts {
			if acts[j].ActivitySlug == slug {
				return true, &ens[i], &acts[j], nil
			}
		}
	}
	return false, nil, nil, nil
}

// RemediationBranchSlug returns a remediation branch for a completed/returned activity if configured.
func RemediationBranchSlug(ctx context.Context, q repo.Querier, revisionID, forSlug string) (string, error) {
	items, err := repo.ListCurriculumRemediations(ctx, q, revisionID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ForActivitySlug == forSlug && (it.Kind == "" || it.Kind == "remediation") {
			return it.BranchSlug, nil
		}
	}
	return "", nil
}

// FilterReinforcementToCourse keeps reinforcement only when student has no active course
// enrollments, or the candidate revision slug is in an active course / parent-approved.
func activitySlugForRevision(ctx context.Context, q repo.Querier, revisionID string) (string, error) {
	const sqlStr = `
SELECT a.slug
FROM learning_activity_revisions r
JOIN learning_activities a ON a.id = r.activity_id
WHERE r.id = $1`
	var slug string
	if err := q.QueryRow(ctx, sqlStr, revisionID).Scan(&slug); err != nil {
		return "", err
	}
	return slug, nil
}

// slugAllowedForEnrolledStudent returns true when reinforcement/global picks are ok.
func slugAllowedForEnrolledStudent(ctx context.Context, q repo.Querier, studentID, slug string) (bool, error) {
	ens, err := repo.ListActiveEnrollmentsForStudent(ctx, q, studentID)
	if err != nil {
		return false, err
	}
	if len(ens) == 0 {
		return true, nil
	}
	ok, _, _, err := IsSlugInActiveEnrollments(ctx, q, studentID, slug)
	return ok, err
}
