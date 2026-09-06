package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"primer-tasks/internal/devicemanagement"
)

func (s *Server) management() *devicemanagement.Service {
	if s.Management != nil {
		return s.Management
	}
	s.Management = &devicemanagement.Service{DB: s.DB}
	return s.Management
}

func managementProblem(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, devicemanagement.ErrUnauthorized):
		return &Problem{Code: "unauthorized", Message: "management credential required", Detail: "management credential required", status: http.StatusUnauthorized}
	case errors.Is(err, devicemanagement.ErrForbidden):
		return &Problem{Code: "denied", Message: "not permitted", Detail: "not permitted", status: http.StatusForbidden}
	case errors.Is(err, devicemanagement.ErrNotFound):
		return &Problem{Code: "not_found", Message: "not found", Detail: "not found", status: http.StatusNotFound}
	case errors.Is(err, devicemanagement.ErrGone):
		return &Problem{Code: "gone", Message: "enrollment is expired or consumed", Detail: "enrollment is expired or consumed", status: http.StatusGone}
	case errors.Is(err, devicemanagement.ErrConflict):
		return &Problem{Code: "conflict", Message: err.Error(), Detail: err.Error(), status: http.StatusConflict}
	case errors.Is(err, devicemanagement.ErrInvalid):
		return &Problem{Code: "invalid_request", Message: err.Error(), Detail: err.Error(), status: http.StatusBadRequest}
	case errors.Is(err, devicemanagement.ErrUnavailable):
		return &Problem{Code: "unavailable", Message: "management service unavailable", Detail: "management service unavailable", status: http.StatusServiceUnavailable}
	default:
		return &Problem{Code: "internal", Message: "internal", Detail: "internal", status: http.StatusInternalServerError}
	}
}

func (s *Server) parentManagementScope(ctx context.Context) (devicemanagement.Scope, error) {
	hctx, ok := ctx.Value(humaContextKey{}).(huma.Context)
	if !ok {
		return devicemanagement.Scope{}, managementProblem(devicemanagement.ErrUnavailable)
	}
	req, _ := humachiRequest(hctx)
	sc, err := s.parentScope(req)
	if err != nil {
		return devicemanagement.Scope{}, err
	}
	return devicemanagement.Scope{TenantID: sc.Tenant, ActorRef: sc.Subject}, nil
}

func (s *Server) requireManagementDevice(ctx context.Context) (devicemanagement.DeviceScope, error) {
	hctx, ok := ctx.Value(humaContextKey{}).(huma.Context)
	if !ok {
		return devicemanagement.DeviceScope{}, managementProblem(devicemanagement.ErrUnavailable)
	}
	req, _ := humachiRequest(hctx)
	token := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	sc, err := s.management().AuthenticateDevice(req.Context(), token)
	if err != nil {
		return devicemanagement.DeviceScope{}, managementProblem(err)
	}
	return sc, nil
}

func (s *Server) wrapParentErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return s.parentHumaError(err)
}

func (s *Server) parentHumaError(err error) error {
	rec := &capturedResponse{header: make(http.Header)}
	s.parentError(rec, err)
	return capturedError(rec)
}
