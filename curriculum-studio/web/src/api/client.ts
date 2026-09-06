import { createClient } from "../../../clients/ts-rest/src/client";
import type { components, paths } from "../../../clients/ts-rest/src/client";
type JSONBody<P extends keyof paths> = NonNullable<paths[P]['post']> extends {requestBody: {content: {'application/json': infer B}}} ? B : never;
export type { components } from "../../../clients/ts-rest/src/client";

export type ExportFormat = components["schemas"]["ExportFormat"];

export const studioClient = createClient({ baseUrl: "/" });

export async function currentSession() {
  return studioClient.GET("/studio/v1/auth/me", { credentials: "include" });
}

export async function listCurricula(workspaceId: string, q = "") {
  return studioClient.GET("/studio/v1/workspaces/{workspaceId}/curricula", {
    credentials: "include",
    params: { path: { workspaceId }, query: { q, limit: 25, offset: 0 } },
  });
}

export async function createCurriculum(workspaceId: string, name: string) {
  return studioClient.POST("/studio/v1/workspaces/{workspaceId}/curricula", {
    credentials: "include",
    params: { path: { workspaceId } },
    body: { name, description: "", template: "custom" },
  });
}

export async function listRevisions(curriculumId: string) {
  return studioClient.GET("/studio/v1/curricula/{curriculumId}/revisions", { credentials: "include", params: { path: { curriculumId }, query: { limit: 25, offset: 0 } } });
}
export async function createRevision(curriculumId: string) {
  return studioClient.POST("/studio/v1/curricula/{curriculumId}/revisions", { credentials: "include", params: { path: { curriculumId } }, body: {} });
}
export async function validateRevision(revisionId: string) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/validate", { credentials: "include", params: { path: { revisionId } } });
}
export async function publishRevision(revisionId: string) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/publish", { credentials: "include", params: { path: { revisionId } } });
}
export async function exportRevision(revisionId: string, format: ExportFormat) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/exports", { credentials: "include", params: { path: { revisionId } }, body: { format } });
}
export async function downloadExport(exportId: string) {
  return studioClient.GET("/studio/v1/exports/{exportId}/download", { credentials: "include", params: { path: { exportId } }, parseAs: "blob" });
}

export async function getRevisionGraph(revisionId: string) {
  return studioClient.GET("/studio/v1/revisions/{revisionId}/graph", { credentials: "include", params: { path: { revisionId } } });
}

export async function createPlanNode(revisionId: string, body: JSONBody<'/studio/v1/revisions/{revisionId}/nodes'>) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/nodes", { credentials: "include", params: { path: { revisionId } }, body });
}

export async function createPlanEdge(revisionId: string, body: JSONBody<'/studio/v1/revisions/{revisionId}/edges'>) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/edges", { credentials: "include", params: { path: { revisionId } }, body });
}

export async function createResource(workspaceId: string, body: JSONBody<'/studio/v1/workspaces/{workspaceId}/resources'>) {
  return studioClient.POST("/studio/v1/workspaces/{workspaceId}/resources", { credentials: "include", params: { path: { workspaceId } }, body });
}

export async function materializeRevision(revisionId: string, body: JSONBody<'/studio/v1/revisions/{revisionId}/materializations'>) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/materializations", { credentials: "include", params: { path: { revisionId } }, body });
}

export async function listMaterializedItems(materializationId: string) {
  return studioClient.GET("/studio/v1/materializations/{materializationId}/items", { credentials: "include", params: { path: { materializationId }, query: { limit: 50, offset: 0 } } });
}

export async function listStandardsCatalogs(workspaceId: string, offset = 0, limit = 100) {
  return studioClient.GET("/studio/v1/workspaces/{workspaceID}/standards-catalogs", { credentials: "include", params: { path: { workspaceID: workspaceId }, query: { limit, offset } } });
}

export async function importStandardsCatalog(workspaceId: string, body: JSONBody<'/studio/v1/workspaces/{workspaceID}/standards-catalogs'>) {
  return studioClient.POST("/studio/v1/workspaces/{workspaceID}/standards-catalogs", { credentials: "include", params: { path: { workspaceID: workspaceId } }, body });
}

export async function listCatalogStandards(catalogId: string, offset = 0, limit = 100, q = "") {
  return studioClient.GET("/studio/v1/standards-catalogs/{catalogId}/standards", { credentials: "include", params: { path: { catalogId }, query: { limit, offset, q } } });
}

export async function createCatalogStandard(catalogId: string, body: JSONBody<'/studio/v1/standards-catalogs/{catalogId}/standards'>) {
  return studioClient.POST("/studio/v1/standards-catalogs/{catalogId}/standards", { credentials: "include", params: { path: { catalogId } }, body });
}

export async function deletePlanNode(revisionId: string, nodeId: string) {
  return studioClient.DELETE("/studio/v1/revisions/{revisionId}/nodes/{nodeId}", { credentials: "include", params: { path: { revisionId, nodeId } } });
}
