package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aleksclark/primer/curriculum-studio/internal/domain"
	studioexport "github.com/aleksclark/primer/curriculum-studio/internal/export"
	"github.com/aleksclark/primer/curriculum-studio/internal/repo"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

func exportView(v *domain.Export) ExportJob {
	out := ExportJob{ID: "exp_" + compactUUID(v.ID), Format: exportWireFormat(v.Format), Status: MaterializationStatus(v.Status), ArtifactURI: v.ArtifactRef, CreatedAt: v.CreatedAt, CompletedAt: v.CompletedAt, CreatedBy: v.RequestedBySubjectRef, Checksum: v.Checksum}
	if v.PlanRevisionID != nil {
		out.RevisionID = revisionID(*v.PlanRevisionID)
	}
	if v.RunID != nil {
		out.MaterializationID = encodeMaterializationID(*v.RunID)
	}
	if v.Status == domain.ExportStatusReady {
		out.DownloadURL = "/studio/v1/exports/" + out.ID + "/download"
		out.ManifestURL = "/studio/v1/exports/" + out.ID + "/manifest"
	}
	if v.Status == domain.ExportStatusFailed {
		out.ErrorMessage = "Export rendering or artifact storage failed. Request a new export; contact the operator with the export ID if it persists."
	}
	return out
}
func exportWireFormat(f string) ExportFormat {
	switch f {
	case "csv":
		return "csv_coverage"
	case "json":
		return "json_bundle"
	default:
		return ExportFormat(f)
	}
}
func exportStorageFormat(f ExportFormat) string {
	switch f {
	case "csv_coverage":
		return "csv"
	case "json_bundle":
		return "json"
	default:
		return string(f)
	}
}
func (s *Server) exportForCaller(ctx context.Context, raw string) (*domain.Export, error) {
	id, err := decodeOpaqueUUID(raw, "exp_", "export")
	if err != nil {
		return nil, repo.ErrNotFound
	}
	for _, m := range MembershipsFromContext(ctx) {
		job, err := repo.NewExportRepo(s.querier).Get(ctx, m.WorkspaceID, id)
		if err == nil {
			return job, nil
		}
		if !errors.Is(err, repo.ErrNotFound) {
			return nil, err
		}
	}
	return nil, repo.ErrNotFound
}

type exportDownload struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	CacheControl       string `header:"Cache-Control"`
	NoSniff            string `header:"X-Content-Type-Options"`
	Body               []byte
}

func (s *Server) registerExportRoutes(api huma.API) {
	op := authoringOperation("createExport", http.MethodPost, "/studio/v1/revisions/{revisionId}/exports", "Exports", "Render and store an export")
	op.DefaultStatus = http.StatusAccepted
	huma.Register(api, op, func(ctx context.Context, in *createExportInput) (*exportResponse, error) {
		rev, ws, m, err := s.revisionForCaller(ctx, in.RevisionID)
		if err != nil {
			return nil, planError(err)
		}
		if err = requireAuthor(ctx, m); err != nil {
			return nil, err
		}
		format := exportStorageFormat(in.Body.Format)
		if _, _, err = studioexport.FormatInfo(format); err != nil {
			return nil, huma.Error400BadRequest("unsupported export format")
		}
		var runID *uuid.UUID
		if in.Body.MaterializationID != "" {
			id, err := decodeMaterializationID(in.Body.MaterializationID)
			if err != nil {
				return nil, huma.Error404NotFound("materialization not found")
			}
			runID = &id
		}
		principal, _ := AuthFromContext(ctx)
		job, err := (studioexport.Service{Q: s.querier, Store: s.artifacts}).Create(ctx, studioexport.Request{WorkspaceID: ws, RevisionID: rev.ID, RunID: runID, Format: format, CreatedBy: principal.SubjectRef})
		if err != nil {
			if job != nil {
				slog.ErrorContext(ctx, "export failed", "export_id", job.ID, "error", err)
				return nil, huma.Error503ServiceUnavailable("export failed; inspect the export metadata", &huma.ErrorDetail{Location: "exportId", Value: exportView(job).ID, Message: "Export did not complete successfully"})
			}
			if errors.Is(err, repo.ErrRunRevisionMismatch) {
				return nil, huma.Error400BadRequest("materialization belongs to another revision")
			}
			return nil, planError(err)
		}
		return &exportResponse{Body: exportView(job)}, nil
	})
	huma.Register(api, authoringOperation("getExport", http.MethodGet, "/studio/v1/exports/{exportId}", "Exports", "Get export metadata and provenance"), func(ctx context.Context, in *exportPath) (*exportResponse, error) {
		job, err := s.exportForCaller(ctx, in.ExportID)
		if err != nil {
			return nil, planError(err)
		}
		return &exportResponse{Body: exportView(job)}, nil
	})
	for _, manifest := range []bool{false, true} {
		suffix, id := "/download", "downloadExport"
		if manifest {
			suffix, id = "/manifest", "getExportManifest"
		}
		op := authoringOperation(id, http.MethodGet, "/studio/v1/exports/{exportId}"+suffix, "Exports", "Download a private export artifact")
		content := map[string]*huma.MediaType{}
		for _, f := range []string{"markdown", "pdf", "docx", "csv", "json", "ical"} {
			_, ct, _ := studioexport.FormatInfo(f)
			content[strings.Split(ct, ";")[0]] = &huma.MediaType{Schema: &huma.Schema{Type: "string", Format: "binary"}}
		}
		op.Responses = map[string]*huma.Response{"200": {Description: "Stored artifact bytes", Content: content}}
		huma.Register(api, op, func(ctx context.Context, in *exportPath) (*exportDownload, error) {
			job, err := s.exportForCaller(ctx, in.ExportID)
			if err != nil {
				return nil, planError(err)
			}
			if job.Status != domain.ExportStatusReady {
				return nil, huma.Error409Conflict("export is not ready")
			}
			ext, ct, err := studioexport.FormatInfo(job.Format)
			if err != nil {
				return nil, huma.Error503ServiceUnavailable("unknown artifact format")
			}
			// Resolve only keys created by this service, never arbitrary stored URLs.
			key := "exports/" + job.WorkspaceID.String() + "/" + job.ID.String() + "." + ext
			if job.ArtifactRef != "obj:"+key || s.artifacts == nil {
				return nil, huma.Error503ServiceUnavailable("artifact is unavailable")
			}
			if manifest {
				key += ".manifest.json"
				ext = "manifest.json"
				ct = "application/json"
			}
			r, err := s.artifacts.Get(ctx, key)
			if err != nil {
				return nil, huma.Error503ServiceUnavailable("artifact is unavailable")
			}
			data, err := io.ReadAll(r)
			closeErr := r.Close()
			if err != nil || closeErr != nil {
				return nil, huma.Error503ServiceUnavailable("artifact read failed")
			}
			if !manifest {
				sum := sha256.Sum256(data)
				if hex.EncodeToString(sum[:]) != job.Checksum {
					return nil, huma.Error503ServiceUnavailable("artifact checksum mismatch")
				}
			}
			return &exportDownload{ContentType: ct, ContentDisposition: fmt.Sprintf(`attachment; filename="%s.%s"`, exportView(job).ID, ext), CacheControl: "private, no-store", NoSniff: "nosniff", Body: data}, nil
		})
	}
}
