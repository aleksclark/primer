// Package profile: admission maps a validated principal to an allowed profile.
// Profile selection is server-side only; callers name a mode but cannot supply
// budgets, tools, model, system instructions, or credentials.
package profile

import (
	"fmt"

	"github.com/aleksclark/primer/agents/internal/authn"
)

// ErrAdmissionDenied is returned when a principal is not admitted to the
// requested profile or route.
var ErrAdmissionDenied = fmt.Errorf("profile: admission denied")

// ErrStudentNotEnabled is returned when the student route is called without a
// reviewed Identity-issued student credential/scope.
// This is a safe blocker: callers should enable the existing Fantasy path.
var ErrStudentNotEnabled = fmt.Errorf("profile: student scope not present; reviewed Identity student credential required (feature flag off)")

// ErrForbiddenField is returned when a request supplies a field that the
// server-selected profile forbids (e.g. profile name on the student route).
var ErrForbiddenField = fmt.Errorf("profile: request contains a forbidden field")

// AdmitParentAdmin selects a profile for a parent/admin request.
// The caller may request Tutor or Admin. Unknown/unlisted profiles are denied.
func AdmitParentAdmin(p authn.Principal, requested Name) (Name, error) {
	if requested == Student || requested == Job {
		return "", fmt.Errorf("%w: callers may not request profile %q on parent/admin routes", ErrAdmissionDenied, requested)
	}
	if !Allowed[requested] {
		return "", fmt.Errorf("%w: unknown profile %q", ErrAdmissionDenied, requested)
	}
	if !p.HasScope(authn.ScopeRunsWrite) && !p.HasScope(authn.ScopeSessionsWrite) {
		return "", ErrAdmissionDenied
	}
	return requested, nil
}

// AdmitJob selects the Job profile for machine/scheduled-job requests.
// Requires agents:jobs:write scope. Profile is always Job — callers do not
// supply a profile name.
func AdmitJob(p authn.Principal) (Name, error) {
	if !p.HasScope(authn.ScopeJobsWrite) {
		return "", fmt.Errorf("%w: agents:jobs:write scope required", ErrAdmissionDenied)
	}
	return Job, nil
}

// AdmitStudent selects the Student profile for the student tutoring route.
// Requires agents:student:session scope. Any profile/budget/tools/model
// override fields in the request body are a protocol error — callers must
// use the dedicated student DTO that contains none of those fields.
//
// Returns ErrStudentNotEnabled when the scope is absent — this is the explicit
// safe blocker that keeps the student feature off until a reviewed Identity
// credential is issued for the workstation student caller.
func AdmitStudent(p authn.Principal) (Name, error) {
	if !p.HasScope(authn.ScopeStudentSession) {
		return "", ErrStudentNotEnabled
	}
	// Re-validate invariants at admission time — defense in depth.
	spec, err := Build(Student)
	if err != nil {
		return "", fmt.Errorf("%w: student spec build failed: %v", ErrAdmissionDenied, err)
	}
	if err := ValidateStudentSpec(spec); err != nil {
		return "", fmt.Errorf("%w: student invariant check failed at admission: %v", ErrAdmissionDenied, err)
	}
	return Student, nil
}
