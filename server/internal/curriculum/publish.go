// Package curriculum loads standards YAML and activity documents and publishes
// them into the LMS database as immutable revisions.
package curriculum

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aleksclark/primer/server/internal/domain"
	"github.com/aleksclark/primer/server/internal/repo"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
)

// StandardSeed is one row from curriculum/standards/*.yaml.
type StandardSeed struct {
	Code        string   `yaml:"code"`
	Source      string   `yaml:"source"`
	SubjectCode string   `yaml:"subject_code"`
	GradeLevel  *int     `yaml:"grade_level"`
	Domain      string   `yaml:"domain"`
	Cluster     string   `yaml:"cluster"`
	Description string   `yaml:"description"`
	Criteria    []string `yaml:"mastery_criteria"`
}

type standardsFile struct {
	Standards []StandardSeed `yaml:"standards"`
}

// PublishOptions configures a curriculum publish run.
type PublishOptions struct {
	ActivitiesDir string
	StandardsDir  string
	Now           time.Time
}

// PublishResult summarizes what was written.
type PublishResult struct {
	StandardsUpserted int
	Activities        int
	Revisions         int
}

// Publish upserts standards and publishes activity revisions from disk.
func Publish(ctx context.Context, q repo.Querier, opts PublishOptions) (*PublishResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	out := &PublishResult{}

	codeToID := map[string]string{}
	subjectCache := map[string]string{}

	if opts.StandardsDir != "" {
		seeds, err := loadStandardsDir(opts.StandardsDir)
		if err != nil {
			return nil, err
		}
		for _, s := range seeds {
			subjID, err := ensureSubject(ctx, q, s.SubjectCode, subjectCache)
			if err != nil {
				return nil, err
			}
			id, err := upsertStandard(ctx, q, s, subjID)
			if err != nil {
				return nil, err
			}
			codeToID[s.Code] = id
			out.StandardsUpserted++
		}
	}

	// Also index any standards already in DB so activity links resolve.
	if err := indexExistingStandards(ctx, q, codeToID); err != nil {
		return nil, err
	}

	if opts.ActivitiesDir == "" {
		return out, nil
	}
	docs, errs := contracts.LoadDocumentsDir(opts.ActivitiesDir)
	for _, e := range errs {
		return nil, e
	}
	for _, doc := range docs {
		subjID, err := ensureSubject(ctx, q, doc.SubjectCode, subjectCache)
		if err != nil {
			return nil, err
		}
		var subjPtr *string
		if subjID != "" {
			subjPtr = &subjID
		}
		_, rev, err := repo.PublishActivityRevision(ctx, q, doc, subjPtr, codeToID, opts.Now)
		if err != nil {
			return nil, fmt.Errorf("publish %s: %w", doc.Slug, err)
		}
		out.Activities++
		if rev != nil {
			out.Revisions++
		}
	}
	return out, nil
}

// PublishDocument publishes a single in-memory activity document (tests/helpers).
func PublishDocument(ctx context.Context, q repo.Querier, doc *contracts.ActivityDocument, now time.Time) (*domain.LearningActivity, *domain.LearningActivityRevision, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	codeToID := map[string]string{}
	if err := indexExistingStandards(ctx, q, codeToID); err != nil {
		return nil, nil, err
	}
	subjectCache := map[string]string{}
	subjID, err := ensureSubject(ctx, q, doc.SubjectCode, subjectCache)
	if err != nil {
		return nil, nil, err
	}
	var subjPtr *string
	if subjID != "" {
		subjPtr = &subjID
	}
	// Ensure referenced standards exist (create stubs if missing for tests).
	for _, ref := range doc.Standards {
		if _, ok := codeToID[ref.Code]; ok {
			continue
		}
		std, err := repo.Standards.Create(ctx, q, map[string]any{
			"source":      "custom",
			"code":        ref.Code,
			"subject_id":  subjID,
			"domain":      "imported",
			"description": ref.Code,
		})
		if err != nil {
			var id string
			if err2 := q.QueryRow(ctx, `SELECT id FROM standards WHERE source = $1 AND code = $2`, "custom", ref.Code).Scan(&id); err2 == nil {
				codeToID[ref.Code] = id
				continue
			}
			return nil, nil, err
		}
		codeToID[ref.Code] = std.ID
	}
	return repo.PublishActivityRevision(ctx, q, doc, subjPtr, codeToID, now)
}

func loadStandardsDir(dir string) ([]StandardSeed, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read standards dir: %w", err)
	}
	var all []StandardSeed
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var f standardsFile
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		all = append(all, f.Standards...)
	}
	return all, nil
}

func ensureSubject(ctx context.Context, q repo.Querier, code string, cache map[string]string) (string, error) {
	if code == "" {
		return "", nil
	}
	if id, ok := cache[code]; ok {
		return id, nil
	}
	page, err := repo.Subjects.List(ctx, q, repo.ListParams{
		Limit:   1,
		Filters: map[string]any{"code": code},
	})
	if err != nil {
		return "", err
	}
	if page.TotalCount > 0 {
		cache[code] = page.Items[0].ID
		return page.Items[0].ID, nil
	}
	s, err := repo.Subjects.Create(ctx, q, map[string]any{
		"code": code,
		"name": code,
	})
	if err != nil {
		page, err2 := repo.Subjects.List(ctx, q, repo.ListParams{
			Limit: 1, Filters: map[string]any{"code": code},
		})
		if err2 == nil && page.TotalCount > 0 {
			cache[code] = page.Items[0].ID
			return page.Items[0].ID, nil
		}
		return "", err
	}
	cache[code] = s.ID
	return s.ID, nil
}

func upsertStandard(ctx context.Context, q repo.Querier, s StandardSeed, subjectID string) (string, error) {
	source := s.Source
	if source == "" {
		source = "custom"
	}
	page, err := repo.Standards.List(ctx, q, repo.ListParams{
		Limit: 1,
		Filters: map[string]any{
			"source": source,
			"code":   s.Code,
		},
	})
	if err != nil {
		// filter may not support multi; fall back to scan
		page = nil
	}
	if page != nil && page.TotalCount > 0 {
		id := page.Items[0].ID
		_, err := repo.Standards.Update(ctx, q, id, map[string]any{
			"subject_id":  subjectID,
			"grade_level": s.GradeLevel,
			"domain":      s.Domain,
			"cluster":     s.Cluster,
			"description": s.Description,
		})
		return id, err
	}

	// Manual lookup by code when filter combination fails.
	const sqlStr = `SELECT id FROM standards WHERE source = $1 AND code = $2`
	var id string
	err = q.QueryRow(ctx, sqlStr, source, s.Code).Scan(&id)
	if err == nil {
		_, err := repo.Standards.Update(ctx, q, id, map[string]any{
			"subject_id":  subjectID,
			"grade_level": s.GradeLevel,
			"domain":      s.Domain,
			"cluster":     s.Cluster,
			"description": s.Description,
		})
		return id, err
	}

	values := map[string]any{
		"source":      source,
		"code":        s.Code,
		"domain":      s.Domain,
		"cluster":     s.Cluster,
		"description": s.Description,
	}
	if subjectID != "" {
		values["subject_id"] = subjectID
	}
	if s.GradeLevel != nil {
		values["grade_level"] = *s.GradeLevel
	}
	std, err := repo.Standards.Create(ctx, q, values)
	if err != nil {
		// Concurrent insert of the same code: re-select.
		err2 := q.QueryRow(ctx, sqlStr, source, s.Code).Scan(&id)
		if err2 == nil {
			return id, nil
		}
		return "", err
	}
	return std.ID, nil
}

func indexExistingStandards(ctx context.Context, q repo.Querier, codeToID map[string]string) error {
	page, err := repo.Standards.List(ctx, q, repo.ListParams{Limit: 200, Offset: 0})
	if err != nil {
		return err
	}
	for {
		for _, s := range page.Items {
			codeToID[s.Code] = s.ID
		}
		if page.Offset+len(page.Items) >= page.TotalCount {
			break
		}
		page, err = repo.Standards.List(ctx, q, repo.ListParams{
			Limit:  200,
			Offset: page.Offset + len(page.Items),
		})
		if err != nil {
			return err
		}
	}
	return nil
}
