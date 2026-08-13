package curriculum_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/curriculum"
	"github.com/aleksclark/primer/server/internal/studentclient/contracts"
	"github.com/aleksclark/primer/server/internal/testutil"
)

func TestPublishStandardsDirErrorsAndEmptySubject(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()

	// Missing standards dir.
	_, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir: filepath.Join(t.TempDir(), "missing"),
		Now:          time.Now().UTC(),
	})
	require.Error(t, err)

	// Bad YAML in standards dir.
	stdDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stdDir, "bad.yaml"), []byte("standards: [\n  - code: x\n    domain: ["), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stdDir, "skip.txt"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(stdDir, "subdir"), 0o755))
	_, err = curriculum.Publish(ctx, q, curriculum.PublishOptions{StandardsDir: stdDir, Now: time.Now().UTC()})
	require.Error(t, err)

	// Valid standards YAML with subject_code so subject_id is a real UUID.
	stdDir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stdDir2, "s.yaml"), []byte(`
standards:
  - code: PRIMER.DL.6.NAV.99
    subject_code: digital-literacy
    domain: navigation
    description: edge
`), 0o644))
	res, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{StandardsDir: stdDir2, Now: time.Now().UTC()})
	require.NoError(t, err)
	assert.Equal(t, 1, res.StandardsUpserted)

	// Re-publish updates same code.
	res2, err := curriculum.Publish(ctx, q, curriculum.PublishOptions{StandardsDir: stdDir2})
	require.NoError(t, err)
	assert.Equal(t, 1, res2.StandardsUpserted)

	// ActivitiesDir load error.
	actDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(actDir, "broken"), 0o755))
	_, err = curriculum.Publish(ctx, q, curriculum.PublishOptions{
		StandardsDir:  stdDir2,
		ActivitiesDir: actDir,
		Now:           time.Now().UTC(),
	})
	require.Error(t, err)
}

func TestPublishCourseAndDocumentZeroNow(t *testing.T) {
	t.Parallel()
	q := testutil.NewSavepointQuerier(testutil.Tx(t))
	ctx := context.Background()

	// Seed activity via PublishDocument with zero Now.
	doc := sampleBundle().Activities[0]
	doc.Slug = "publish-edge-act"
	_, rev, err := curriculum.PublishDocument(ctx, q, &doc, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, rev)

	// Course path missing.
	_, err = curriculum.PublishCourse(ctx, q, filepath.Join(t.TempDir(), "no.json"), time.Time{})
	require.Error(t, err)

	// Valid course.json referencing published activity.
	coursePath := filepath.Join(t.TempDir(), "course.json")
	course := &contracts.CourseDocument{
		SchemaVersion: "1",
		Slug:          "publish-edge-course",
		Title:         "Edge Course",
		SubjectCode:   "digital-literacy",
		Activities:    []contracts.CourseActivityRef{{Order: 1, Slug: "publish-edge-act"}},
	}
	// Write via contracts encoding — simple JSON.
	raw := []byte(`{
  "schemaVersion": "1",
  "slug": "publish-edge-course",
  "title": "Edge Course",
  "subjectCode": "digital-literacy",
  "activities": [{"order": 1, "slug": "publish-edge-act"}]
}`)
	require.NoError(t, os.WriteFile(coursePath, raw, 0o644))
	cres, err := curriculum.PublishCourse(ctx, q, coursePath, time.Time{})
	require.NoError(t, err)
	require.NotNil(t, cres)
	_ = course
}
