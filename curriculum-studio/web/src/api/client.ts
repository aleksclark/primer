import createClient from "openapi-fetch";
import type { paths } from "../../../clients/ts-rest/generated/schema";

export const studioClient = createClient<paths>({ baseUrl: "/" });

export async function currentSession() {
  return studioClient.GET("/studio/v1/auth/me", { credentials: "include" });
}
