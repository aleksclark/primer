package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	baseapi "github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/tv/domain"
	tvrepo "github.com/aleksclark/primer/server/internal/tv/repo"
)

// primerTag groups the instructional-hours export operations in the spec.
const primerTag = "Primer Reports"

// PrimerRunResponse summarizes one reporting pass.
type PrimerRunResponse struct {
	Scanned   int `json:"scanned" doc:"Unreported viewings the pass picked up."`
	Reported  int `json:"reported" doc:"Viewings the LMS recorded as new instructional time."`
	Duplicate int `json:"duplicate" doc:"Viewings the LMS already held under the same reference."`
	Failed    int `json:"failed" doc:"Viewings that could not be reported and stay queued for the next pass."`
}

// registerPrimerAdmin wires the export ledger and the manual reporting run.
//
// The ledger is exposed read-mostly: the parent needs to see what was counted,
// and deleting a row is the one meaningful edit — it makes the viewing
// eligible again, which is how a session reported against the wrong LMS gets
// re-sent. There is nothing to update and nothing to create by hand, so those
// two operations are left off.
func (s *Server) registerPrimerAdmin() {
	baseapi.RegisterCRUD[domain.PrimerReport, struct{}, struct{}](
		s.api, s.q, tvrepo.PrimerReports, "primer-report", "primer-reports", "/primer-reports",
		s.adminGuard(), baseapi.SkipCreate(), baseapi.SkipUpdate())

	huma.Register(s.api, s.adminOp(huma.Operation{
		OperationID: "run-primer-report",
		Method:      http.MethodPost,
		Path:        "/primer-reports/run",
		Summary:     "Report pending viewings now",
		Description: "Runs one export pass immediately instead of waiting for the scheduled one. Safe to repeat: the export ledger and the LMS both deduplicate, so a viewing already counted is not counted again.",
		Tags:        []string{primerTag},
		Errors:      []int{http.StatusServiceUnavailable},
	}), s.runPrimerReport)
}

// primerRunOutput wraps the pass summary.
type primerRunOutput struct {
	Body PrimerRunResponse
}

func (s *Server) runPrimerReport(ctx context.Context, _ *struct{}) (*primerRunOutput, error) {
	if s.reporter == nil {
		return nil, huma.Error503ServiceUnavailable("primer reporting is not configured")
	}
	summary, err := s.reporter.RunOnce(ctx, s.q)
	if err != nil {
		return nil, baseapi.MapError(err)
	}
	return &primerRunOutput{Body: PrimerRunResponse{
		Scanned:   summary.Scanned,
		Reported:  summary.Reported,
		Duplicate: summary.Duplicate,
		Failed:    summary.Failed,
	}}, nil
}
