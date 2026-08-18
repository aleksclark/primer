package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
)

// ValidationReportRepo persists validation output; it does not run validation.
type ValidationReportRepo struct{ Q Querier }

func NewValidationReportRepo(q Querier) *ValidationReportRepo { return &ValidationReportRepo{Q: q} }

// CreateReportWithFindings commits the report and all findings in one UoW.
func (r *ValidationReportRepo) CreateReportWithFindings(ctx context.Context, workspaceID uuid.UUID, in *domain.ValidationReport, findings []domain.ValidationFinding) (*domain.ValidationReport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if in == nil || workspaceID == uuid.Nil || in.PlanRevisionID == uuid.Nil {
		return nil, fmt.Errorf("workspace and plan revision are required")
	}
	summary, err := validationObject(in.Summary, "summary")
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "pending"
	}
	var out *domain.ValidationReport
	err = WithTx(ctx, r.Q, func(q Querier) error {
		const insertReport = `INSERT INTO curriculum_studio.validation_reports(plan_revision_id,status,summary) SELECT r.id,$2,$3 FROM curriculum_studio.plan_revisions r JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE r.id=$1 AND c.workspace_id=$4 RETURNING id,plan_revision_id,status,summary,generated_at,created_at`
		var e error
		out, e = scanValidationReport(q.QueryRow(ctx, insertReport, in.PlanRevisionID, status, summary, workspaceID))
		if e != nil {
			return e
		}
		const insertFinding = `INSERT INTO curriculum_studio.validation_findings(report_id,severity,code,message,node_kind,node_id,details) VALUES($1,$2,$3,$4,$5,$6,$7)`
		for _, finding := range findings {
			details, e := validationObject(finding.Details, "finding details")
			if e != nil {
				return e
			}
			if _, e = q.Exec(ctx, insertFinding, out.ID, finding.Severity, strings.TrimSpace(finding.Code), strings.TrimSpace(finding.Message), finding.NodeKind, finding.NodeID, details); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *ValidationReportRepo) GetReport(ctx context.Context, workspaceID, reportID uuid.UUID) (*domain.ValidationReport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || reportID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `SELECT v.id,v.plan_revision_id,v.status,v.summary,v.generated_at,v.created_at FROM curriculum_studio.validation_reports v JOIN curriculum_studio.plan_revisions r ON r.id=v.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE v.id=$1 AND c.workspace_id=$2`
	out, err := scanValidationReport(r.Q.QueryRow(ctx, q, reportID, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *ValidationReportRepo) ListReports(ctx context.Context, workspaceID, revisionID uuid.UUID) ([]domain.ValidationReport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || revisionID == uuid.Nil {
		return []domain.ValidationReport{}, nil
	}
	rows, err := r.Q.Query(ctx, `SELECT v.id,v.plan_revision_id,v.status,v.summary,v.generated_at,v.created_at FROM curriculum_studio.validation_reports v JOIN curriculum_studio.plan_revisions r ON r.id=v.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE v.plan_revision_id=$1 AND c.workspace_id=$2 ORDER BY v.generated_at DESC,v.id DESC`, revisionID, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.ValidationReport
	for rows.Next() {
		v, e := scanValidationReport(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.ValidationReport{}
	}
	return out, nil
}

func (r *ValidationReportRepo) Latest(ctx context.Context, workspaceID, revisionID uuid.UUID) (*domain.ValidationReport, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || revisionID == uuid.Nil {
		return nil, fmt.Errorf("%w", ErrNotFound)
	}
	const q = `SELECT v.id,v.plan_revision_id,v.status,v.summary,v.generated_at,v.created_at FROM curriculum_studio.validation_reports v JOIN curriculum_studio.plan_revisions r ON r.id=v.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE v.plan_revision_id=$1 AND c.workspace_id=$2 ORDER BY v.generated_at DESC,v.id DESC LIMIT 1`
	out, err := scanValidationReport(r.Q.QueryRow(ctx, q, revisionID, workspaceID))
	if err != nil {
		return nil, MapError(err)
	}
	return out, nil
}

func (r *ValidationReportRepo) ListFindings(ctx context.Context, workspaceID, reportID uuid.UUID) ([]domain.ValidationFinding, error) {
	if r == nil || r.Q == nil {
		return nil, fmt.Errorf("%w", ErrClosed)
	}
	if workspaceID == uuid.Nil || reportID == uuid.Nil {
		return []domain.ValidationFinding{}, nil
	}
	const q = `SELECT f.id,f.report_id,f.severity,f.code,f.message,f.node_kind,f.node_id,f.details,f.created_at FROM curriculum_studio.validation_findings f JOIN curriculum_studio.validation_reports v ON v.id=f.report_id JOIN curriculum_studio.plan_revisions r ON r.id=v.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id WHERE f.report_id=$1 AND c.workspace_id=$2 ORDER BY CASE f.severity WHEN 'error' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END,f.code,f.id`
	rows, err := r.Q.Query(ctx, q, reportID, workspaceID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	var out []domain.ValidationFinding
	for rows.Next() {
		v, e := scanValidationFinding(rows)
		if e != nil {
			return nil, MapError(e)
		}
		out = append(out, *v)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	if out == nil {
		out = []domain.ValidationFinding{}
	}
	return out, nil
}

func (r *ValidationReportRepo) GetWithFindings(ctx context.Context, workspaceID, reportID uuid.UUID) (*domain.ValidationReport, []domain.ValidationFinding, error) {
	report, err := r.GetReport(ctx, workspaceID, reportID)
	if err != nil {
		return nil, nil, err
	}
	findings, err := r.ListFindings(ctx, workspaceID, reportID)
	if err != nil {
		return nil, nil, err
	}
	return report, findings, nil
}

func validationObject(raw json.RawMessage, name string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be JSON: %w", name, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return raw, nil
}

type validationScanner interface{ Scan(...any) error }

func scanValidationReport(s validationScanner) (*domain.ValidationReport, error) {
	v := new(domain.ValidationReport)
	err := s.Scan(&v.ID, &v.PlanRevisionID, &v.Status, &v.Summary, &v.GeneratedAt, &v.CreatedAt)
	return v, err
}
func scanValidationFinding(s validationScanner) (*domain.ValidationFinding, error) {
	v := new(domain.ValidationFinding)
	err := s.Scan(&v.ID, &v.ReportID, &v.Severity, &v.Code, &v.Message, &v.NodeKind, &v.NodeID, &v.Details, &v.CreatedAt)
	return v, err
}
