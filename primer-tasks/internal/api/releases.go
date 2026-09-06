package api

import (
	"context"
	"encoding/hex"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"primer-tasks/internal/devicemanagement"
)

type ReleaseListOutput struct {
	ResponseHeaders
	Body devicemanagement.ReleasePage
}
type ReleaseOutput struct {
	ResponseHeaders
	Body devicemanagement.Release
}
type ReleaseTargetEnvelope struct {
	ID   string                              `path:"id"`
	Body devicemanagement.ReleaseTargetInput `required:"true"`
}
type ReleaseTargetOutput struct {
	ResponseHeaders
	Body devicemanagement.ReleaseTarget
}
type ReleaseReceiptEnvelope struct {
	Body devicemanagement.ReleaseReceiptInput `required:"true"`
}
type ReleaseReceiptOutput struct {
	ResponseHeaders
	Body devicemanagement.ReleaseReceipt
}
type ArtifactIDInput struct {
	ID string `path:"id" format:"uuid"`
}
type ArtifactOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	ContentLength      int64  `header:"Content-Length"`
	Body               []byte
}

func (s *Server) registerReleases(api huma.API) {
	register(api, huma.Operation{OperationID: "managed-releases-list", Method: http.MethodGet, Path: "/managed-releases", Errors: []int{401, 503}}, func(ctx context.Context, _ *struct{}) (*ReleaseListOutput, error) {
		if _, err := s.parentManagementScope(ctx); err != nil {
			return nil, s.wrapParentErr(err)
		}
		items, err := s.management().ListPublishedReleases(ctx)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ReleaseListOutput{Body: devicemanagement.ReleasePage{Items: items}}, nil
	})
	register(api, huma.Operation{OperationID: "managed-releases-get", Method: http.MethodGet, Path: "/managed-releases/{id}", Errors: []int{401, 404, 503}}, func(ctx context.Context, in *ManagedDeviceIDInput) (*ReleaseOutput, error) {
		if _, err := s.parentManagementScope(ctx); err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().GetPublishedRelease(ctx, in.ID)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ReleaseOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "managed-devices-release-target", Method: http.MethodPost, Path: "/managed-devices/{id}/releases", Errors: []int{400, 401, 404, 409, 503}}, func(ctx context.Context, in *ReleaseTargetEnvelope) (*ReleaseTargetOutput, error) {
		sc, err := s.parentManagementScope(ctx)
		if err != nil {
			return nil, s.wrapParentErr(err)
		}
		out, err := s.management().SetReleaseTarget(ctx, sc, in.ID, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ReleaseTargetOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "management-device-release-receipt", Method: http.MethodPost, Path: "/management-device/release-receipts", Errors: []int{400, 401, 404, 409, 503}}, func(ctx context.Context, in *ReleaseReceiptEnvelope) (*ReleaseReceiptOutput, error) {
		sc, err := s.requireManagementDevice(ctx)
		if err != nil {
			return nil, err
		}
		out, err := s.management().ReportRelease(ctx, sc, in.Body)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ReleaseReceiptOutput{Body: out}, nil
	})
	register(api, huma.Operation{OperationID: "management-device-artifact", Method: http.MethodGet, Path: "/management-device/artifacts/{id}", Errors: []int{401, 403, 404, 503}}, func(ctx context.Context, in *ArtifactIDInput) (*ArtifactOutput, error) {
		sc, err := s.requireManagementDevice(ctx)
		if err != nil {
			return nil, err
		}
		data, rel, err := s.management().ArtifactBytes(ctx, sc, in.ID)
		if err != nil {
			return nil, managementProblem(err)
		}
		return &ArtifactOutput{
			ContentType:        "application/vnd.android.package-archive",
			ContentDisposition: `attachment; filename="` + rel.PackageName + `-` + hex.EncodeToString([]byte(rel.SHA256[:8])) + `.apk"`,
			ContentLength:      int64(len(data)),
			Body:               data,
		}, nil
	})
}
