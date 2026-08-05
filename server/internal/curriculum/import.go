package curriculum

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// Default authorized namespaces for custom standards via import.
// Official sources (e.g. non-custom) cannot be overwritten on this path.
var DefaultAuthorizedStandardNamespaces = []string{
	"PRIMER.",
	"CUSTOM.",
}

// OfficialStandardSources cannot be mutated through the guarded import path.
var OfficialStandardSources = map[string]bool{
	"ccss": true,
	"ngss": true,
	"csta": true,
	"state": true,
}

// ImportBundle is the strict document-level curriculum import contract.
type ImportBundle struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Version       string                     `json:"version,omitempty"`
	SourceLabel   string                     `json:"sourceLabel,omitempty"`
	Standards     []StandardSeed             `json:"standards,omitempty"`
	Activities    []contracts.ActivityDocument `json:"activities,omitempty"`
	Course        *contracts.CourseDocument  `json:"course,omitempty"`
}

// ImportOptions configures plan/apply.
type ImportOptions struct {
	// AuthorizedNamespaces restricts custom standard codes (prefix match).
	AuthorizedNamespaces []string
	// DisallowCreateSubjects rejects unknown subject codes during plan.
	// When false (default), plan records subject creates and apply ensures them.
	DisallowCreateSubjects bool
	// Now overrides wall clock.
	Now time.Time
	// ActorID is the parent/admin performing the operation.
	ActorID string
	// SourceLabel overrides bundle source label for audit.
	SourceLabel string
}

// PlanAction is one planned change.
type PlanAction struct {
	Kind     string `json:"kind"` // standard | activity | course
	Slug     string `json:"slug,omitempty"`
	Code     string `json:"code,omitempty"`
	Action   string `json:"action"` // create | update | reuse | skip
	Digest   string `json:"digest,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Conflict bool   `json:"conflict,omitempty"`
}

// ImportPlan is the read-only diff for a bundle.
type ImportPlan struct {
	BundleDigest string       `json:"bundleDigest"`
	SourceLabel  string       `json:"sourceLabel,omitempty"`
	Actions      []PlanAction `json:"actions"`
	Warnings     []string     `json:"warnings,omitempty"`
	Errors       []string     `json:"errors,omitempty"`
	// Valid is true when Errors is empty.
	Valid bool `json:"valid"`
}

// DocumentResult is one per-document apply outcome.
type DocumentResult struct {
	Kind       string `json:"kind"`
	Slug       string `json:"slug,omitempty"`
	Code       string `json:"code,omitempty"`
	Status     string `json:"status"` // created | reused | updated | failed | skipped
	ID         string `json:"id,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
	Message    string `json:"message,omitempty"`
}

// ImportResultManifest is the durable apply result.
type ImportResultManifest struct {
	BundleDigest string           `json:"bundleDigest"`
	AppliedAt    time.Time        `json:"appliedAt"`
	SourceLabel  string           `json:"sourceLabel,omitempty"`
	Documents    []DocumentResult `json:"documents"`
	// EnrolledStudents is always empty — import never enrolls.
	EnrolledStudents []string `json:"enrolledStudents"`
	// AssignedStudents is always empty — import never assigns.
	AssignedStudents []string `json:"assignedStudents"`
	IdempotentReplay bool     `json:"idempotentReplay,omitempty"`
}

// BundleDigest returns a stable SHA-256 over canonical JSON of the bundle.
func BundleDigest(b *ImportBundle) (string, error) {
	if b == nil {
		return "", errors.New("bundle is nil")
	}
	// Canonicalize by re-marshaling through sorted structures.
	raw, err := json.Marshal(canonicalBundle(b))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalBundle(b *ImportBundle) any {
	// Marshal/unmarshal to map then sort activity slugs for stability is already
	// covered by deterministic json.Marshal of struct field order; sort slices.
	cp := *b
	stds := append([]StandardSeed(nil), b.Standards...)
	sort.SliceStable(stds, func(i, j int) bool {
		if stds[i].Source != stds[j].Source {
			return stds[i].Source < stds[j].Source
		}
		return stds[i].Code < stds[j].Code
	})
	cp.Standards = stds
	acts := append([]contracts.ActivityDocument(nil), b.Activities...)
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].Slug < acts[j].Slug })
	cp.Activities = acts
	return cp
}

// PlanImport validates the bundle and computes a read-only diff. No writes.
func PlanImport(ctx context.Context, q repo.Querier, bundle *ImportBundle, opts ImportOptions) (*ImportPlan, error) {
	if bundle == nil {
		return nil, repo.ErrBadRequest{Msg: "bundle is required"}
	}
	if opts.AuthorizedNamespaces == nil {
		opts.AuthorizedNamespaces = DefaultAuthorizedStandardNamespaces
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	label := firstNonEmpty(opts.SourceLabel, bundle.SourceLabel)

	plan := &ImportPlan{SourceLabel: label, Valid: true}
	digest, err := BundleDigest(bundle)
	if err != nil {
		return nil, err
	}
	plan.BundleDigest = digest

	if err := validateBundleShape(bundle); err != nil {
		plan.Valid = false
		plan.Errors = append(plan.Errors, err.Error())
		return plan, nil
	}

	codeToID := map[string]string{}
	if err := indexExistingStandards(ctx, q, codeToID); err != nil {
		return nil, err
	}
	subjectCache := map[string]string{}

	// Standards
	for _, s := range bundle.Standards {
		src := s.Source
		if src == "" {
			src = "custom"
		}
		if OfficialStandardSources[strings.ToLower(src)] {
			plan.Valid = false
			plan.Errors = append(plan.Errors, fmt.Sprintf("standard %s: official source %q cannot be imported via this path", s.Code, src))
			plan.Actions = append(plan.Actions, PlanAction{Kind: "standard", Code: s.Code, Action: "skip", Conflict: true, Detail: "official source"})
			continue
		}
		if src == "custom" || src == "" {
			if !codeAuthorized(s.Code, opts.AuthorizedNamespaces) {
				plan.Valid = false
				plan.Errors = append(plan.Errors, fmt.Sprintf("standard %s: code not in authorized namespaces", s.Code))
				plan.Actions = append(plan.Actions, PlanAction{Kind: "standard", Code: s.Code, Action: "skip", Conflict: true, Detail: "unauthorized namespace"})
				continue
			}
		}
		if s.SubjectCode == "" {
			plan.Valid = false
			plan.Errors = append(plan.Errors, fmt.Sprintf("standard %s: subject_code is required", s.Code))
			continue
		}
		// Subject resolution (read-only existence check).
		if _, err := lookupSubject(ctx, q, s.SubjectCode, subjectCache); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				if opts.DisallowCreateSubjects {
					plan.Valid = false
					plan.Errors = append(plan.Errors, fmt.Sprintf("unknown subject code %q", s.SubjectCode))
					continue
				}
				plan.Actions = append(plan.Actions, PlanAction{Kind: "subject", Code: s.SubjectCode, Action: "create", Detail: "subject will be created"})
			} else {
				return nil, err
			}
		}
		if id, ok := codeToID[s.Code]; ok {
			plan.Actions = append(plan.Actions, PlanAction{Kind: "standard", Code: s.Code, Action: "update", Detail: "upsert existing " + id})
		} else {
			plan.Actions = append(plan.Actions, PlanAction{Kind: "standard", Code: s.Code, Action: "create"})
		}
	}

	// Activities
	for i := range bundle.Activities {
		doc := &bundle.Activities[i]
		if err := contracts.ValidateDocument(doc); err != nil {
			plan.Valid = false
			plan.Errors = append(plan.Errors, fmt.Sprintf("activity %s: %v", doc.Slug, err))
			plan.Actions = append(plan.Actions, PlanAction{Kind: "activity", Slug: doc.Slug, Action: "skip", Conflict: true, Detail: err.Error()})
			continue
		}
		// Standard refs must exist in bundle seeds or DB (after apply of standards).
		for _, ref := range doc.Standards {
			if _, ok := codeToID[ref.Code]; ok {
				continue
			}
			foundInBundle := false
			for _, s := range bundle.Standards {
				if s.Code == ref.Code {
					foundInBundle = true
					break
				}
			}
			if !foundInBundle {
				plan.Valid = false
				plan.Errors = append(plan.Errors, fmt.Sprintf("activity %s: unknown standard %s", doc.Slug, ref.Code))
			}
		}
		if doc.SubjectCode != "" {
			if _, err := lookupSubject(ctx, q, doc.SubjectCode, subjectCache); err != nil {
				if errors.Is(err, repo.ErrNotFound) && opts.DisallowCreateSubjects {
					plan.Valid = false
					plan.Errors = append(plan.Errors, fmt.Sprintf("activity %s: unknown subject %s", doc.Slug, doc.SubjectCode))
				}
			}
		}
		digest, _, err := repo.ContentSHA256(doc.Content)
		if err != nil {
			return nil, err
		}
		act, err := activityBySlugOptional(ctx, q, doc.Slug)
		if err != nil {
			return nil, err
		}
		if act == nil {
			plan.Actions = append(plan.Actions, PlanAction{Kind: "activity", Slug: doc.Slug, Action: "create", Digest: digest})
			continue
		}
		existing, err := revisionByDigestOptional(ctx, q, act.ID, digest)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			plan.Actions = append(plan.Actions, PlanAction{Kind: "activity", Slug: doc.Slug, Action: "reuse", Digest: digest, Detail: "content digest already published"})
		} else {
			plan.Actions = append(plan.Actions, PlanAction{Kind: "activity", Slug: doc.Slug, Action: "create", Digest: digest, Detail: "new revision"})
		}
	}

	// Course
	if bundle.Course != nil {
		if err := contracts.ValidateCourseDocument(bundle.Course); err != nil {
			plan.Valid = false
			plan.Errors = append(plan.Errors, fmt.Sprintf("course: %v", err))
			plan.Actions = append(plan.Actions, PlanAction{Kind: "course", Slug: bundle.Course.Slug, Action: "skip", Conflict: true, Detail: err.Error()})
		} else {
			// All activity slugs must be in bundle or already published.
			for _, a := range bundle.Course.Activities {
				inBundle := false
				for _, ad := range bundle.Activities {
					if ad.Slug == a.Slug {
						inBundle = true
						break
					}
				}
				if inBundle {
					continue
				}
				id, err := latestPublishedActivityRevisionID(ctx, q, a.Slug)
				if err != nil {
					return nil, err
				}
				if id == "" {
					plan.Valid = false
					plan.Errors = append(plan.Errors, fmt.Sprintf("course activity %q has no published revision and is not in the bundle", a.Slug))
				}
			}
			plan.Actions = append(plan.Actions, PlanAction{Kind: "course", Slug: bundle.Course.Slug, Action: "create", Detail: "publish curriculum revision"})
		}
	}

	// Never enroll/assign — explicit warning for operators.
	plan.Warnings = append(plan.Warnings, "import does not enroll students or create assignments")
	return plan, nil
}

// ApplyImport applies a previously planned bundle by digest inside one DB transaction.
// Rejects when the recomputed digest does not match expectedDigest.
func ApplyImport(ctx context.Context, q repo.Querier, bundle *ImportBundle, expectedDigest string, opts ImportOptions) (*ImportResultManifest, *domain.CurriculumImportRun, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.AuthorizedNamespaces == nil {
		opts.AuthorizedNamespaces = DefaultAuthorizedStandardNamespaces
	}

	digest, err := BundleDigest(bundle)
	if err != nil {
		return nil, nil, err
	}
	if expectedDigest == "" {
		return nil, nil, repo.ErrBadRequest{Msg: "bundleDigest is required for apply"}
	}
	if !strings.EqualFold(digest, expectedDigest) {
		return nil, nil, repo.ErrBadRequest{Msg: fmt.Sprintf("bundle digest drift: got %s want %s", digest, expectedDigest)}
	}

	// Idempotent replay: prior successful apply returns stored manifest.
	if prev, err := repo.LatestAppliedImportByDigest(ctx, q, digest); err == nil {
		manifest := manifestFromMap(prev.ResultManifest)
		manifest.IdempotentReplay = true
		return &manifest, prev, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, nil, err
	}

	plan, err := PlanImport(ctx, q, bundle, opts)
	if err != nil {
		return nil, nil, err
	}
	if !plan.Valid {
		run, _ := persistImportRun(ctx, q, digest, opts, "apply", "rejected", plan, nil, strings.Join(plan.Errors, "; "))
		return nil, run, repo.ErrBadRequest{Msg: "import plan invalid: " + strings.Join(plan.Errors, "; ")}
	}

	label := firstNonEmpty(opts.SourceLabel, bundle.SourceLabel, plan.SourceLabel)
	manifest := &ImportResultManifest{
		BundleDigest:     digest,
		AppliedAt:        opts.Now,
		SourceLabel:      label,
		EnrolledStudents: []string{},
		AssignedStudents: []string{},
	}

	err = repo.WithTx(ctx, q, func(tx repo.Querier) error {
		codeToID := map[string]string{}
		if err := indexExistingStandards(ctx, tx, codeToID); err != nil {
			return err
		}
		subjectCache := map[string]string{}

		for _, s := range bundle.Standards {
			subjID, err := ensureSubject(ctx, tx, s.SubjectCode, subjectCache)
			if err != nil {
				return err
			}
			id, err := upsertStandard(ctx, tx, s, subjID)
			if err != nil {
				return err
			}
			codeToID[s.Code] = id
			manifest.Documents = append(manifest.Documents, DocumentResult{
				Kind: "standard", Code: s.Code, Status: "updated", ID: id,
			})
		}
		// Re-index after seeds.
		if err := indexExistingStandards(ctx, tx, codeToID); err != nil {
			return err
		}

		for i := range bundle.Activities {
			doc := &bundle.Activities[i]
			subjID, err := ensureSubject(ctx, tx, doc.SubjectCode, subjectCache)
			if err != nil {
				return err
			}
			var subjPtr *string
			if subjID != "" {
				subjPtr = &subjID
			}
			// Ensure all referenced standards resolve.
			for _, ref := range doc.Standards {
				if _, ok := codeToID[ref.Code]; !ok {
					return repo.ErrBadRequest{Msg: "unknown standard code: " + ref.Code}
				}
			}
			act, rev, err := repo.PublishActivityRevision(ctx, tx, doc, subjPtr, codeToID, opts.Now)
			if err != nil {
				return fmt.Errorf("publish activity %s: %w", doc.Slug, err)
			}
			status := "created"
			if rev != nil {
				// Detect reuse: if plan said reuse
				for _, a := range plan.Actions {
					if a.Kind == "activity" && a.Slug == doc.Slug && a.Action == "reuse" {
						status = "reused"
						break
					}
				}
			}
			dr := DocumentResult{Kind: "activity", Slug: doc.Slug, Status: status}
			if act != nil {
				dr.ID = act.ID
			}
			if rev != nil {
				dr.Digest = rev.ContentSHA256
				if dr.ID == "" {
					dr.ID = rev.ID
				}
			}
			manifest.Documents = append(manifest.Documents, dr)
		}

		if bundle.Course != nil {
			res, err := repo.PublishCourseDocument(ctx, tx, bundle.Course, opts.Now)
			if err != nil {
				return fmt.Errorf("publish course: %w", err)
			}
			dr := DocumentResult{Kind: "course", Slug: bundle.Course.Slug, Status: "created"}
			if res.Revision != nil {
				dr.ID = res.Revision.ID
			} else if res.Curriculum != nil {
				dr.ID = res.Curriculum.ID
			}
			manifest.Documents = append(manifest.Documents, dr)
		}
		return nil
	})
	if err != nil {
		run, _ := persistImportRun(ctx, q, digest, opts, "apply", "failed", plan, nil, err.Error())
		return nil, run, err
	}

	run, err := persistImportRun(ctx, q, digest, opts, "apply", "applied", plan, manifest, "")
	if err != nil {
		return manifest, nil, err
	}
	return manifest, run, nil
}

func persistImportRun(ctx context.Context, q repo.Querier, digest string, opts ImportOptions, mode, status string, plan *ImportPlan, manifest *ImportResultManifest, errMsg string) (*domain.CurriculumImportRun, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	run := &domain.CurriculumImportRun{
		BundleDigest: digest,
		SourceLabel:  firstNonEmpty(opts.SourceLabel, planSource(plan)),
		Mode:         mode,
		Status:       status,
		ErrorMessage: errMsg,
		CreatedAt:    now,
	}
	if opts.ActorID != "" {
		id := opts.ActorID
		run.ActorID = &id
	}
	if plan != nil {
		raw, _ := json.Marshal(plan)
		_ = json.Unmarshal(raw, &run.Plan)
	}
	if manifest != nil {
		raw, _ := json.Marshal(manifest)
		_ = json.Unmarshal(raw, &run.ResultManifest)
		if status == "applied" {
			t := manifest.AppliedAt
			run.AppliedAt = &t
		}
	}
	return repo.InsertCurriculumImportRun(ctx, q, run)
}

func planSource(p *ImportPlan) string {
	if p == nil {
		return ""
	}
	return p.SourceLabel
}

func manifestFromMap(m map[string]any) ImportResultManifest {
	raw, _ := json.Marshal(m)
	var out ImportResultManifest
	_ = json.Unmarshal(raw, &out)
	if out.EnrolledStudents == nil {
		out.EnrolledStudents = []string{}
	}
	if out.AssignedStudents == nil {
		out.AssignedStudents = []string{}
	}
	return out
}

func validateBundleShape(b *ImportBundle) error {
	if b.SchemaVersion == "" {
		b.SchemaVersion = "1"
	}
	if len(b.Standards) == 0 && len(b.Activities) == 0 && b.Course == nil {
		return errors.New("bundle is empty")
	}
	// Secrets / paths must never enter the bundle — soft scan.
	raw, _ := json.Marshal(b)
	lower := strings.ToLower(string(raw))
	for _, bad := range []string{"\"password\"", "\"api_key\"", "\"apikey\"", "\"secret\"", "file://", "/home/", "c:\\\\"} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("bundle must not contain secrets or local filesystem paths (%s)", bad)
		}
	}
	seen := map[string]bool{}
	for _, a := range b.Activities {
		if seen[a.Slug] {
			return fmt.Errorf("duplicate activity slug %q", a.Slug)
		}
		seen[a.Slug] = true
	}
	return nil
}

func codeAuthorized(code string, namespaces []string) bool {
	for _, ns := range namespaces {
		if ns == "*" {
			return true
		}
		if strings.HasPrefix(code, ns) {
			return true
		}
	}
	return false
}

func lookupSubject(ctx context.Context, q repo.Querier, code string, cache map[string]string) (string, error) {
	if code == "" {
		return "", nil
	}
	if id, ok := cache[code]; ok {
		return id, nil
	}
	page, err := repo.Subjects.List(ctx, q, repo.ListParams{
		Limit: 1, Filters: map[string]any{"code": code},
	})
	if err != nil {
		return "", err
	}
	if page.TotalCount == 0 || len(page.Items) == 0 {
		return "", repo.ErrNotFound
	}
	cache[code] = page.Items[0].ID
	return page.Items[0].ID, nil
}

func activityBySlugOptional(ctx context.Context, q repo.Querier, slug string) (*domain.LearningActivity, error) {
	page, err := repo.LearningActivities.List(ctx, q, repo.ListParams{
		Limit: 1, Filters: map[string]any{"slug": slug},
	})
	if err != nil {
		return nil, err
	}
	if page.TotalCount == 0 || len(page.Items) == 0 {
		return nil, nil
	}
	return &page.Items[0], nil
}

func revisionByDigestOptional(ctx context.Context, q repo.Querier, activityID, digest string) (*domain.LearningActivityRevision, error) {
	page, err := repo.LearningActivityRevisions.List(ctx, q, repo.ListParams{
		Limit: 1,
		Filters: map[string]any{
			"activity_id":    activityID,
			"content_sha256": digest,
		},
	})
	if err != nil {
		// Fallback SQL
		const sqlStr = `SELECT id FROM learning_activity_revisions WHERE activity_id = $1 AND content_sha256 = $2`
		var id string
		if err2 := q.QueryRow(ctx, sqlStr, activityID, digest).Scan(&id); err2 == nil {
			return &domain.LearningActivityRevision{ID: id, ContentSHA256: digest}, nil
		}
		return nil, nil
	}
	if page.TotalCount == 0 || len(page.Items) == 0 {
		return nil, nil
	}
	return &page.Items[0], nil
}

func latestPublishedActivityRevisionID(ctx context.Context, q repo.Querier, slug string) (string, error) {
	const sqlStr = `
SELECT r.id
FROM learning_activity_revisions r
JOIN learning_activities a ON a.id = r.activity_id
WHERE a.slug = $1
ORDER BY r.revision DESC
LIMIT 1`
	var id string
	err := q.QueryRow(ctx, sqlStr, slug).Scan(&id)
	if err != nil {
		// no rows
		return "", nil
	}
	return id, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// BuildBundleFromDirs loads standards YAML + activity documents + optional course.json.
func BuildBundleFromDirs(standardsDir, activitiesDir, coursePath, sourceLabel string) (*ImportBundle, error) {
	b := &ImportBundle{SchemaVersion: "1", SourceLabel: sourceLabel}
	if standardsDir != "" {
		seeds, err := loadStandardsDir(standardsDir)
		if err != nil {
			return nil, err
		}
		b.Standards = seeds
	}
	if activitiesDir != "" {
		docs, errs := contracts.LoadDocumentsDir(activitiesDir)
		if len(errs) > 0 {
			return nil, errs[0]
		}
		for _, d := range docs {
			if d != nil {
				b.Activities = append(b.Activities, *d)
			}
		}
	}
	if coursePath != "" {
		doc, err := contracts.LoadCourseDocument(coursePath)
		if err != nil {
			return nil, err
		}
		b.Course = doc
	}
	return b, nil
}
