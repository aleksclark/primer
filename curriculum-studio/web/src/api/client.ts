import createClient from "openapi-fetch";
import type { components, paths } from "../../../clients/ts-rest/generated/schema";

export type ExportFormat = components["schemas"]["ExportFormat"];

export const studioClient = createClient<paths>({ baseUrl: "/" });

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

export async function createPlanNode(revisionId: string, body: { kind: string; title: string; body?: string; standardCodes?: string[]; attributes?: Record<string, string> }) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/nodes", { credentials: "include", params: { path: { revisionId } }, body: body as never });
}

export async function createPlanEdge(revisionId: string, body: { kind: string; fromNodeId: string; toNodeId: string; note?: string }) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/edges", { credentials: "include", params: { path: { revisionId } }, body: body as never });
}

export async function createResource(workspaceId: string, body: { kind: string; title: string }) {
  return studioClient.POST("/studio/v1/workspaces/{workspaceId}/resources", { credentials: "include", params: { path: { workspaceId } }, body: body as never });
}

export async function materializeRevision(revisionId: string, body: { window: { availableMinutes: number }; attributes?: Record<string, string> }) {
  return studioClient.POST("/studio/v1/revisions/{revisionId}/materializations", { credentials: "include", params: { path: { revisionId } }, body: body as never });
}

export async function listMaterializedItems(materializationId: string) {
  return studioClient.GET("/studio/v1/materializations/{materializationId}/items", { credentials: "include", params: { path: { materializationId }, query: { limit: 50, offset: 0 } } });
}

export async function listStandardsCatalogs(workspaceId: string) {
  return studioClient.GET("/studio/v1/workspaces/{workspaceID}/standards-catalogs", { credentials: "include", params: { path: { workspaceID: workspaceId }, query: { limit: 25, offset: 0 } } });
}

export async function importStandardsCatalog(workspaceId: string, body: { source: string; title: string; standards: { code: string; source: string; description: string }[] }) {
  return studioClient.POST("/studio/v1/workspaces/{workspaceID}/standards-catalogs", { credentials: "include", params: { path: { workspaceID: workspaceId } }, body: body as never });
}

export async function listCatalogStandards(catalogId: string) {
  return studioClient.GET("/studio/v1/standards-catalogs/{catalogId}/standards", { credentials: "include", params: { path: { catalogId }, query: { limit: 50, offset: 0 } } });
}

export async function createCatalogStandard(catalogId: string, body: { code: string; source: string; description: string }) {
  return studioClient.POST("/studio/v1/standards-catalogs/{catalogId}/standards", { credentials: "include", params: { path: { catalogId } }, body: body as never });
}

export async function deletePlanNode(revisionId: string, nodeId: string) {
  return studioClient.DELETE("/studio/v1/revisions/{revisionId}/nodes/{nodeId}", { credentials: "include", params: { path: { revisionId, nodeId } } });
}
