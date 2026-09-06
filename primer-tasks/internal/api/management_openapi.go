package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"primer-tasks/internal/devicemanagement"
)

type IssueEnrollmentInput struct {
	Body devicemanagement.IssueEnrollmentInput `required:"true"`
}
type EnrollInputEnvelope struct {
	Body devicemanagement.EnrollInput `required:"true"`
}
type ManagedDeviceIDInput struct {
	ID string `path:"id" format:"uuid"`
}
type PolicyUpdateEnvelope struct {
	ID   string                             `path:"id"`
	Body devicemanagement.PolicyUpdateInput `required:"true"`
}
type RecoveryIntentEnvelope struct {
	ID   string                               `path:"id"`
	Body devicemanagement.RecoveryIntentInput `required:"true"`
}
type StateChangeEnvelope struct {
	ID   string                            `path:"id"`
	Body devicemanagement.StateChangeInput `required:"true"`
}
type PolicyReportEnvelope struct {
	Body devicemanagement.PolicyReportInput `required:"true"`
}
type RecoveryConfirmEnvelope struct {
	ID   string                                `path:"id"`
	Body devicemanagement.RecoveryConfirmInput `required:"true"`
}

type EnrollmentOutput struct {
	ResponseHeaders
	Body devicemanagement.Enrollment
}
type EnrollOutput struct {
	ResponseHeaders
	Body devicemanagement.EnrollResult
}
type DevicePageOutput struct {
	ResponseHeaders
	Body devicemanagement.DevicePage
}
type ManagedDeviceOutput struct {
	ResponseHeaders
	Body devicemanagement.Device
}
type PolicyRevisionOutput struct {
	ResponseHeaders
	Body devicemanagement.PolicyRevision
}
type DesiredStateOutput struct {
	ResponseHeaders
	Body devicemanagement.DesiredState
}
type PolicyReportOutput struct {
	ResponseHeaders
	Body devicemanagement.PolicyReport
}
type RecoveryIntentOutput struct {
	ResponseHeaders
	Body devicemanagement.RecoveryIntent
}

type EnrollmentIDInput struct {
	ID string `path:"id" format:"uuid"`
}

func (s *Server) registerManagement(api huma.API) {
	register(api, huma.Operation{OperationID: "managed-devices-enrollments-create", Method: http.MethodPost, Path: "/managed-devices/enrollments", DefaultStatus: http.StatusCreated, Errors: []int{400, 401, 503}}, func(ctx context.Context, in *IssueEnrollmentInput) (*EnrollmentOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().IssueEnrollment(ctx, sc, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &EnrollmentOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-list", Method: http.MethodGet, Path: "/managed-devices", Errors: []int{401, 500}}, func(ctx context.Context, _ *struct{}) (*DevicePageOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		items, err := s.management().ListDevices(ctx, sc)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &DevicePageOutput{Body: devicemanagement.DevicePage{Items: items}}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-get", Method: http.MethodGet, Path: "/managed-devices/{id}", Errors: []int{401, 404, 500}}, func(ctx context.Context, in *ManagedDeviceIDInput) (*ManagedDeviceOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().GetDevice(ctx, sc, in.ID)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ManagedDeviceOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-desired", Method: http.MethodGet, Path: "/managed-devices/{id}/desired", Errors: []int{401, 404, 500}}, func(ctx context.Context, in *ManagedDeviceIDInput) (*DesiredStateOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		if _, err = s.management().GetDevice(ctx, sc, in.ID); err != nil {
			return nil, managementProblem(err)
		}
		out, err := s.management().DesiredState(ctx, devicemanagement.DeviceScope{TenantID: sc.TenantID, DeviceID: in.ID})
		if err != nil {
			return nil, managementProblem(err)
		}
		return &DesiredStateOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-enrollments-abandon", Method: http.MethodPost, Path: "/managed-devices/enrollments/{id}/abandon", DefaultStatus: http.StatusNoContent, Errors: []int{401, 404, 500}}, func(ctx context.Context, in *EnrollmentIDInput) (*NoContentOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		if err = s.management().AbandonEnrollment(ctx, sc, in.ID); err != nil {
			return nil, managementProblem(err)
		}
		return &NoContentOutput{}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-policy", Method: http.MethodPost, Path: "/managed-devices/{id}/policy", Errors: []int{400, 401, 404, 409, 500}}, func(ctx context.Context, in *PolicyUpdateEnvelope) (*PolicyRevisionOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().UpdatePolicy(ctx, sc, in.ID, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &PolicyRevisionOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-recovery", Method: http.MethodPost, Path: "/managed-devices/{id}/recovery", DefaultStatus: http.StatusCreated, Errors: []int{400, 401, 404, 500}}, func(ctx context.Context, in *RecoveryIntentEnvelope) (*RecoveryIntentOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().CreateRecoveryIntent(ctx, sc, in.ID, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &RecoveryIntentOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-quarantine", Method: http.MethodPost, Path: "/managed-devices/{id}/quarantine", Errors: []int{401, 404, 409, 500}}, func(ctx context.Context, in *StateChangeEnvelope) (*ManagedDeviceOutput, error) {
		return s.changeManagedState(ctx, in.ID, "quarantined", in.Body.Reason)
	})
	register(api, huma.Operation{OperationID: "managed-devices-revoke", Method: http.MethodPost, Path: "/managed-devices/{id}/revoke", Errors: []int{401, 404, 409, 500}}, func(ctx context.Context, in *StateChangeEnvelope) (*ManagedDeviceOutput, error) {
		return s.changeManagedState(ctx, in.ID, "revoked", in.Body.Reason)
	})

	register(api, huma.Operation{OperationID: "management-device-enroll", Method: http.MethodPost, Path: "/management-device/enroll", DefaultStatus: http.StatusCreated, Errors: []int{400, 410, 503}}, func(ctx context.Context, in *EnrollInputEnvelope) (*EnrollOutput, error) {
		out, err := s.management().Enroll(ctx, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &EnrollOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "management-device-desired", Method: http.MethodGet, Path: "/management-device/desired", Errors: []int{401, 503}}, func(ctx context.Context, _ *struct{}) (*DesiredStateOutput, error) {
		sc, err := s.requireManagementDevice(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.management().DesiredState(ctx, sc)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &DesiredStateOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "management-device-report", Method: http.MethodPost, Path: "/management-device/reports", Errors: []int{400, 401, 409, 503}}, func(ctx context.Context, in *PolicyReportEnvelope) (*PolicyReportOutput, error) {
		sc, err := s.requireManagementDevice(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.management().ReportPolicy(ctx, sc, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &PolicyReportOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "management-device-recovery-confirm", Method: http.MethodPost, Path: "/management-device/recovery/{id}/confirm", Errors: []int{400, 401, 404, 409, 503}}, func(ctx context.Context, in *RecoveryConfirmEnvelope) (*StatusOutput, error) {
		sc, err := s.requireManagementDevice(ctx)
		if err != nil {
			return nil, err
		}
		if err = s.management().ConfirmRecoveryIntent(ctx, sc, in.ID, in.Body.ReportID); err != nil {
			return nil, managementProblem(err)
		}
		return &StatusOutput{Body: StatusResponse{Status: "applied"}}, nil
	})
}

func (s *Server) changeManagedState(ctx context.Context, id, state, reason string) (*ManagedDeviceOutput, error) {
	sc, err := s.parentManagementScope(ctx)
	if err != nil {
		return nil, s.wrapParentErr(err)
	}
	out, err := s.management().ChangeDeviceState(ctx, sc, id, state, reason)
	if err != nil {
		return nil, managementProblem(err)
	}
	return &ManagedDeviceOutput{Body: out}, nil
}
