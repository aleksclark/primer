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
