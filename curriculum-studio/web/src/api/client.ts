import createClient from "openapi-fetch";
import type { paths } from "../../../clients/ts-rest/generated/schema";

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
