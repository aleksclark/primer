package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/server/internal/testutil"
	"github.com/aleksclark/primer/server/internal/testutil/factory"
)

// decode unmarshals a JSON response body into T, failing the test on error.
func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(body, &out), "unmarshal response: %s", string(body))
	return out
}

type objMap = map[string]any

func TestHealth(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	resp := h.Get("/health")
	require.Equal(t, http.StatusOK, resp.Code)
	body := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "ok", body["status"])
}

func TestStudentCRUD(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	// Create
	resp := h.Post("/students", objMap{
		"firstName":  "Ada",
		"lastName":   "Lovelace",
		"gradeLevel": 7,
		"notes":      "strong in math",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	created := decode[objMap](t, resp.Body.Bytes())
	require.NotEmpty(t, created["id"])
	assert.Equal(t, "Ada", created["firstName"])
	assert.Equal(t, float64(7), created["gradeLevel"])

	id := created["id"].(string)

	// Get
	resp = h.Get("/students/" + id)
	require.Equal(t, http.StatusOK, resp.Code)
	got := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "Lovelace", got["lastName"])

	// Update (partial)
	resp = h.Patch("/students/"+id, objMap{"gradeLevel": 8})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	updated := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, float64(8), updated["gradeLevel"])
	assert.Equal(t, "Ada", updated["firstName"], "unspecified fields must be unchanged")

	// Empty patch is a no-op returning current state
	resp = h.Patch("/students/"+id, objMap{})
	require.Equal(t, http.StatusOK, resp.Code)

	// Delete
	resp = h.Delete("/students/" + id)
	require.Equal(t, http.StatusNoContent, resp.Code)

	// Get after delete -> 404
	resp = h.Get("/students/" + id)
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Delete again -> 404
	resp = h.Delete("/students/" + id)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestStudentListPaginationSearchSortFilter(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	for i := range 5 {
		factory.Student(t, tx, factory.Override{
			"first_name":  fmt.Sprintf("Zeta%02d", i),
			"grade_level": 6,
		})
	}
	factory.Student(t, tx, factory.Override{"first_name": "Xylophone", "grade_level": 8})

	// Pagination
	resp := h.Get("/students?limit=2&offset=0&q=Zeta&sort=first_name&dir=asc")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	page := decode[struct {
		Items      []objMap `json:"items"`
		TotalCount int      `json:"totalCount"`
		Limit      int      `json:"limit"`
		Offset     int      `json:"offset"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 5, page.TotalCount)
	assert.Len(t, page.Items, 2)
	assert.Equal(t, 2, page.Limit)
	assert.Equal(t, "Zeta00", page.Items[0]["firstName"])

	// Second page
	resp = h.Get("/students?limit=2&offset=4&q=Zeta&sort=first_name&dir=asc")
	require.Equal(t, http.StatusOK, resp.Code)
	page2 := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	assert.Len(t, page2.Items, 1)
	assert.Equal(t, "Zeta04", page2.Items[0]["firstName"])

	// Descending sort
	resp = h.Get("/students?q=Zeta&sort=first_name&dir=desc")
	require.Equal(t, http.StatusOK, resp.Code)
	desc := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, "Zeta04", desc.Items[0]["firstName"])

	// Search matches a different student
	resp = h.Get("/students?q=Xylo")
	require.Equal(t, http.StatusOK, resp.Code)
	search := decode[struct {
		Items      []objMap `json:"items"`
		TotalCount int      `json:"totalCount"`
	}](t, resp.Body.Bytes())
	require.Equal(t, 1, search.TotalCount)
	assert.Equal(t, "Xylophone", search.Items[0]["firstName"])

	// Exact filter
	resp = h.Get("/students?filter=grade_level:8")
	require.Equal(t, http.StatusOK, resp.Code)
	filtered := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, "Xylophone", filtered.Items[0]["firstName"])
}

func TestListValidation(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	// Disallowed sort column -> 400
	resp := h.Get("/students?sort=notes;drop")
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Disallowed filter column -> 400
	resp = h.Get("/students?filter=nope:1")
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Malformed filter -> 400
	resp = h.Get("/students?filter=justacolumn")
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Invalid dir -> huma validation error
	resp = h.Get("/students?dir=sideways")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// Limit above maximum -> huma validation error
	resp = h.Get("/students?limit=9999")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// Invalid uuid in path -> huma validation error
	resp = h.Get("/students/not-a-uuid")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// Missing required create field -> huma validation error
	resp = h.Post("/students", objMap{"firstName": "OnlyFirst"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
}

func TestEducatorCRUDAndDuplicateEmail(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	resp := h.Post("/educators", objMap{"email": "mom@example.com", "name": "Mom"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	created := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "parent", created["role"], "role should default to parent")

	// Duplicate email -> 409
	resp = h.Post("/educators", objMap{"email": "mom@example.com", "name": "Duplicate"})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Search by name
	factory.Educator(t, tx, factory.Override{"name": "Grandpa Joe"})
	resp = h.Get("/educators?q=Grandpa")
	require.Equal(t, http.StatusOK, resp.Code)
	page := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, page.TotalCount)

	// Filter by role
	factory.Educator(t, tx, factory.Override{"role": "admin"})
	resp = h.Get("/educators?filter=role:admin")
	require.Equal(t, http.StatusOK, resp.Code)
	admins := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, admins.Items, 1)
	assert.Equal(t, "admin", admins.Items[0]["role"])
}

func TestSubjectCRUD(t *testing.T) {
	t.Parallel()
	h, _ := testutil.API(t)

	resp := h.Post("/subjects", objMap{"code": "math", "name": "Mathematics", "description": "All of math"})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	subject := decode[objMap](t, resp.Body.Bytes())

	// Duplicate code -> 409
	resp = h.Post("/subjects", objMap{"code": "math", "name": "Other Math"})
	assert.Equal(t, http.StatusConflict, resp.Code)

	resp = h.Patch("/subjects/"+subject["id"].(string), objMap{"name": "Maths"})
	require.Equal(t, http.StatusOK, resp.Code)
	updated := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "Maths", updated["name"])
}

func TestStandardCRUDAndHierarchy(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	subject := factory.Subject(t, tx, factory.Override{"code": "math"})

	resp := h.Post("/standards", objMap{
		"source":      "tennessee",
		"subjectId":   subject.ID,
		"code":        "TN.MATH.6.RP.A.3",
		"gradeLevel":  6,
		"domain":      "ratios-proportional",
		"cluster":     "understand-ratio-concepts",
		"description": "Use ratio and rate reasoning to solve problems",
		"tcapWeight":  "high",
		"metadata":    objMap{"ccssEquivalent": "6.RP.A.3"},
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	parent := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "TN.MATH.6.RP.A.3", parent["code"])
	meta := parent["metadata"].(map[string]any)
	assert.Equal(t, "6.RP.A.3", meta["ccssEquivalent"])

	// Child standard referencing the parent
	resp = h.Post("/standards", objMap{
		"source":   "tennessee",
		"parentId": parent["id"],
		"code":     "TN.MATH.6.RP.A.3.a",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	// Duplicate (source, code) -> 409
	resp = h.Post("/standards", objMap{"source": "tennessee", "code": "TN.MATH.6.RP.A.3"})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Same code from a different source is allowed
	resp = h.Post("/standards", objMap{"source": "custom", "code": "TN.MATH.6.RP.A.3"})
	assert.Equal(t, http.StatusCreated, resp.Code)

	// Filter by parent
	resp = h.Get("/standards?filter=parent_id:" + parent["id"].(string))
	require.Equal(t, http.StatusOK, resp.Code)
	children := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, children.Items, 1)
	assert.Equal(t, "TN.MATH.6.RP.A.3.a", children.Items[0]["code"])

	// Search across description
	resp = h.Get("/standards?q=rate reasoning")
	require.Equal(t, http.StatusOK, resp.Code)
	found := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, found.TotalCount)
}

func TestCurriculumAndSequencing(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	resp := h.Post("/curricula", objMap{
		"slug":        "grade-6-mastery-math",
		"name":        "Grade 6 Mastery Math",
		"approach":    "mastery_based",
		"gradeLevel":  6,
		"description": "Mastery-based math sequence",
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	curriculum := decode[objMap](t, resp.Body.Bytes())

	// Invalid approach -> validation error
	resp = h.Post("/curricula", objMap{"slug": "bad", "name": "Bad", "approach": "montessori"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// Sequence two standards into the curriculum
	std1 := factory.Standard(t, tx)
	std2 := factory.Standard(t, tx)
	curriculumID := curriculum["id"].(string)

	resp = h.Post("/curriculum-standards", objMap{
		"curriculumId": curriculumID,
		"standardId":   std1.ID,
		"unit":         "Unit 1",
		"position":     1,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	resp = h.Post("/curriculum-standards", objMap{
		"curriculumId": curriculumID,
		"standardId":   std2.ID,
		"unit":         "Unit 1",
		"position":     2,
	})
	require.Equal(t, http.StatusCreated, resp.Code)

	// Duplicate placement -> 409
	resp = h.Post("/curriculum-standards", objMap{
		"curriculumId": curriculumID,
		"standardId":   std1.ID,
	})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Placement with unknown curriculum -> 422 FK violation
	resp = h.Post("/curriculum-standards", objMap{
		"curriculumId": "00000000-0000-0000-0000-000000000000",
		"standardId":   std1.ID,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// List ordered by position
	resp = h.Get("/curriculum-standards?filter=curriculum_id:" + curriculumID + "&sort=position&dir=asc")
	require.Equal(t, http.StatusOK, resp.Code)
	seq := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, seq.Items, 2)
	assert.Equal(t, std1.ID, seq.Items[0]["standardId"])
	assert.Equal(t, std2.ID, seq.Items[1]["standardId"])
}

func TestEnrollmentLifecycle(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	student := factory.Student(t, tx)
	curriculum := factory.Curriculum(t, tx)

	resp := h.Post("/enrollments", objMap{
		"studentId":    student.ID,
		"curriculumId": curriculum.ID,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	enrollment := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "active", enrollment["status"])

	// Duplicate enrollment -> 409
	resp = h.Post("/enrollments", objMap{
		"studentId":    student.ID,
		"curriculumId": curriculum.ID,
	})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Complete the enrollment
	resp = h.Patch("/enrollments/"+enrollment["id"].(string), objMap{
		"status":  "completed",
		"endedOn": "2026-06-01T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	done := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "completed", done["status"])
	assert.NotNil(t, done["endedOn"])

	// A second student can enroll in the same curriculum
	other := factory.Enrollment(t, tx, factory.Override{"curriculum_id": curriculum.ID})
	resp = h.Get("/enrollments?filter=curriculum_id:" + curriculum.ID)
	require.Equal(t, http.StatusOK, resp.Code)
	page := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 2, page.TotalCount)
	assert.NotEqual(t, student.ID, other.StudentID)
}

func TestMasteryTracking(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	student := factory.Student(t, tx)
	standard := factory.Standard(t, tx)

	resp := h.Post("/mastery-records", objMap{
		"studentId":  student.ID,
		"standardId": standard.ID,
		"status":     "in_progress",
		"confidence": 0.4,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	record := decode[objMap](t, resp.Body.Bytes())
	recordID := record["id"].(string)

	// Confidence outside [0,1] -> validation error
	resp = h.Post("/mastery-records", objMap{
		"studentId":  factory.Student(t, tx).ID,
		"standardId": standard.ID,
		"confidence": 1.5,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// One record per (student, standard)
	resp = h.Post("/mastery-records", objMap{
		"studentId":  student.ID,
		"standardId": standard.ID,
	})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Progress to mastered
	resp = h.Patch("/mastery-records/"+recordID, objMap{
		"status":              "mastered",
		"confidence":          0.92,
		"lastAssessedAt":      "2026-07-01T10:00:00Z",
		"nextReinforcementAt": "2026-07-15T10:00:00Z",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	mastered := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "mastered", mastered["status"])
	assert.InDelta(t, 0.92, mastered["confidence"], 0.001)

	// Attach evidence of three kinds
	for _, kind := range []string{"continuous", "formal", "project"} {
		resp = h.Post("/mastery-evidence", objMap{
			"masteryRecordId": recordID,
			"kind":            kind,
			"context":         "evidence: " + kind,
		})
		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	}

	resp = h.Get("/mastery-evidence?filter=mastery_record_id:" + recordID)
	require.Equal(t, http.StatusOK, resp.Code)
	evidence := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 3, evidence.TotalCount)

	resp = h.Get("/mastery-evidence?filter=mastery_record_id:" + recordID + "&filter=kind:formal")
	require.Equal(t, http.StatusOK, resp.Code)
	formal := decode[struct {
		Items []objMap `json:"items"`
	}](t, resp.Body.Bytes())
	require.Len(t, formal.Items, 1)
	assert.Equal(t, "evidence: formal", formal.Items[0]["context"])

	// Filter records by student and status
	resp = h.Get("/mastery-records?filter=student_id:" + student.ID + "&filter=status:mastered")
	require.Equal(t, http.StatusOK, resp.Code)
	records := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, records.TotalCount)

	// Deleting the record cascades to evidence
	resp = h.Delete("/mastery-records/" + recordID)
	require.Equal(t, http.StatusNoContent, resp.Code)
	resp = h.Get("/mastery-evidence?filter=mastery_record_id:" + recordID)
	require.Equal(t, http.StatusOK, resp.Code)
	after := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 0, after.TotalCount)
}

func TestAssessmentAuthoringAndAttempt(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	subject := factory.Subject(t, tx)
	standard := factory.Standard(t, tx, factory.Override{"subject_id": subject.ID})

	// Author an assessment
	resp := h.Post("/assessments", objMap{
		"title":      "Ratios Quick Check",
		"kind":       "quick_check",
		"subjectId":  subject.ID,
		"gradeLevel": 6,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	assessment := decode[objMap](t, resp.Body.Bytes())
	assessmentID := assessment["id"].(string)

	// Add a multi-select item aligned to a standard
	resp = h.Post("/assessment-items", objMap{
		"assessmentId": assessmentID,
		"standardId":   standard.ID,
		"itemType":     "multi_select",
		"difficulty":   "on_track",
		"position":     1,
		"stem":         "A recipe calls for 2 cups flour for every 3 cups sugar. Which statements are true?",
		"points":       2,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	item := decode[objMap](t, resp.Body.Bytes())
	itemID := item["id"].(string)

	// Add options
	options := []objMap{
		{"itemId": itemID, "position": 1, "text": "The ratio of flour to sugar is 2:3", "correct": true},
		{"itemId": itemID, "position": 2, "text": "For 6 cups sugar, you need 4 cups flour", "correct": true},
		{"itemId": itemID, "position": 3, "text": "The ratio of sugar to flour is 2:3", "correct": false},
	}
	for _, opt := range options {
		resp = h.Post("/assessment-item-options", opt)
		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	}

	resp = h.Get("/assessment-item-options?filter=item_id:" + itemID + "&filter=correct:true")
	require.Equal(t, http.StatusOK, resp.Code)
	correct := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 2, correct.TotalCount)

	// Student attempts the assessment
	student := factory.Student(t, tx)
	resp = h.Post("/assessment-attempts", objMap{
		"assessmentId": assessmentID,
		"studentId":    student.ID,
		"maxScore":     2,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	attempt := decode[objMap](t, resp.Body.Bytes())
	attemptID := attempt["id"].(string)
	assert.Equal(t, "in_progress", attempt["status"])

	// Record a response
	resp = h.Post("/item-responses", objMap{
		"attemptId": attemptID,
		"itemId":    itemID,
		"response":  objMap{"selected": []string{"1", "2"}},
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	response := decode[objMap](t, resp.Body.Bytes())

	// One response per item per attempt
	resp = h.Post("/item-responses", objMap{
		"attemptId": attemptID,
		"itemId":    itemID,
		"response":  objMap{"selected": []string{"3"}},
	})
	assert.Equal(t, http.StatusConflict, resp.Code)

	// Score the response
	resp = h.Patch("/item-responses/"+response["id"].(string), objMap{
		"isCorrect":     true,
		"pointsAwarded": 2,
		"feedback":      "Correct on both counts.",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	scored := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, true, scored["isCorrect"])

	// Submit and score the attempt
	resp = h.Patch("/assessment-attempts/"+attemptID, objMap{
		"status":      "scored",
		"score":       2,
		"submittedAt": "2026-07-01T11:00:00Z",
		"scoredAt":    "2026-07-01T11:05:00Z",
	})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	final := decode[objMap](t, resp.Body.Bytes())
	assert.Equal(t, "scored", final["status"])
	assert.Equal(t, float64(2), final["score"])

	// Attempts are filterable by student and status
	resp = h.Get("/assessment-attempts?filter=student_id:" + student.ID + "&filter=status:scored")
	require.Equal(t, http.StatusOK, resp.Code)
	attempts := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, attempts.TotalCount)

	// Deleting the assessment cascades to items, options, attempts, responses
	resp = h.Delete("/assessments/" + assessmentID)
	require.Equal(t, http.StatusNoContent, resp.Code)
	resp = h.Get("/assessment-items?filter=assessment_id:" + assessmentID)
	require.Equal(t, http.StatusOK, resp.Code)
	items := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 0, items.TotalCount)
}

func TestAssessmentKindsAndSearch(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	kinds := []string{"continuous", "quick_check", "comprehensive", "tcap_practice", "quiz", "project_rubric"}
	for _, kind := range kinds {
		factory.Assessment(t, tx, factory.Override{"kind": kind, "title": "Kind " + kind})
	}

	for _, kind := range kinds {
		resp := h.Get("/assessments?filter=kind:" + kind)
		require.Equal(t, http.StatusOK, resp.Code)
		page := decode[struct {
			TotalCount int `json:"totalCount"`
		}](t, resp.Body.Bytes())
		assert.Equal(t, 1, page.TotalCount, "kind %s", kind)
	}

	// Invalid kind rejected at validation
	resp := h.Post("/assessments", objMap{"title": "Bad", "kind": "pop_quiz"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	resp = h.Get("/assessments?q=tcap")
	require.Equal(t, http.StatusOK, resp.Code)
	found := decode[struct {
		TotalCount int `json:"totalCount"`
	}](t, resp.Body.Bytes())
	assert.Equal(t, 1, found.TotalCount)
}

func TestFactoriesProduceCoherentGraph(t *testing.T) {
	t.Parallel()
	h, tx := testutil.API(t)

	// Deep factories auto-create their association chains.
	evidence := factory.MasteryEvidence(t, tx)
	option := factory.AssessmentItemOption(t, tx)
	response := factory.ItemResponse(t, tx)
	placement := factory.CurriculumStandard(t, tx)

	for path, id := range map[string]string{
		"/mastery-evidence/":        evidence.ID,
		"/assessment-item-options/": option.ID,
		"/item-responses/":          response.ID,
		"/curriculum-standards/":    placement.ID,
	} {
		resp := h.Get(path + id)
		assert.Equal(t, http.StatusOK, resp.Code, "GET %s%s", path, id)
	}

	// The response's attempt and item belong to the same assessment.
	attemptResp := h.Get("/assessment-attempts/" + response.AttemptID)
	require.Equal(t, http.StatusOK, attemptResp.Code)
	attempt := decode[objMap](t, attemptResp.Body.Bytes())

	itemResp := h.Get("/assessment-items/" + response.ItemID)
	require.Equal(t, http.StatusOK, itemResp.Code)
	item := decode[objMap](t, itemResp.Body.Bytes())

	assert.Equal(t, attempt["assessmentId"], item["assessmentId"])
}
