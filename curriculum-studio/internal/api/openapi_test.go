package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aleksclark/primer/curriculum-studio/internal/api"
)

func TestOpenAPIEmissionUsesOfflineRegistrationAndAuthoringBoundary(t *testing.T) {
	t.Parallel()

	spec, _ := api.NewWithPinger(nil, api.Options{})
	paths := spec.OpenAPI().Paths

	for _, path := range []string{
		"/studio/v1/health",
		"/studio/v1/workspaces",
		"/studio/v1/workspaces/{workspaceId}/curricula",
		"/studio/v1/curricula/{curriculumId}/revisions",
		"/studio/v1/revisions/{revisionId}/graph",
		"/studio/v1/workspaces/{workspaceID}/standards-catalogs",
		"/studio/v1/workspaces/{workspaceId}/resources",
		"/studio/v1/revisions/{revisionId}/materializations",
		"/studio/v1/workspaces/{workspaceId}/learner-profiles",
		"/studio/v1/materializations/{materializationId}/bundle",
		"/studio/v1/materialized-items/{itemId}/lock",
		"/studio/v1/revisions/{revisionId}/exports",
		"/studio/v1/exports/{exportId}",
		"/studio/v1/exports/{exportId}/download",
		"/studio/v1/exports/{exportId}/manifest",
		"/studio/v1/workspaces/{workspaceId}/webhooks",
		"/studio/v1/workspaces/{workspaceId}/events",
	} {
		require.Contains(t, paths, path, "missing emitted path %s", path)
	}

	schemas := spec.OpenAPI().Components.Schemas.Map()
	require.Contains(t, schemas, "MaterializationStatus")
	require.Contains(t, schemas, "EventTypeName")
	require.Contains(t, schemas, "ErrorCode")
	require.Len(t, schemas["ExportFormat"].Enum, 6)
	require.Contains(t, schemas["ExportJob"].Properties, "createdBy")
	require.Contains(t, schemas["ExportJob"].Properties, "downloadUrl")
	download := paths["/studio/v1/exports/{exportId}/download"].Get
	require.NotEmpty(t, download.Security)
	require.Equal(t, "binary", download.Responses["200"].Content["application/pdf"].Schema.Format)
	require.NotContains(t, schemas, "MaterializationContext", "Primer integration payload must remain protobuf-only")
	require.NotContains(t, schemas, "MaterializationBundle", "Primer integration payload must remain protobuf-only")
	require.NotContains(t, schemas, "SessionSpec", "Primer integration payload must remain protobuf-only")
}
