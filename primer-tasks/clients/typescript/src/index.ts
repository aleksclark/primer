import createClient from "openapi-fetch";
import type { paths, components } from "../generated/schema";

export type { components, paths } from "../generated/schema";
export type Student = components["schemas"]["Student"];

type Operation<Path extends keyof paths, Method extends keyof paths[Path]> = NonNullable<paths[Path][Method]>;
type JsonBody<Path extends keyof paths, Method extends keyof paths[Path]> = Operation<Path, Method> extends { requestBody?: { content: { "application/json": infer Body } } } ? Body : never;
type Query<Path extends keyof paths, Method extends keyof paths[Path]> = Operation<Path, Method> extends { parameters?: { query?: infer Parameters } } ? Parameters : never;

type CreateStudentBody = JsonBody<"/students", "post">;
type UpdateStudentBody = JsonBody<"/students/{id}", "patch">;
type PairStudentBody = JsonBody<"/student/pair", "post">;
type StudentListQuery = Query<"/students", "get">;

export interface TasksClientOptions {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export class TasksApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "TasksApiError";
    this.status = status;
    this.code = code;
  }
}

/**
 * Stable browser façade for the generated Tasks transport.
 *
 * The generated schema is intentionally a build output. Consumers import only
 * this façade; no page knows about openapi-fetch, URLs, or wire error shapes.
 * Parent and student browser sessions are host-only cookies, so credentials
 * are included and no bearer token is persisted in browser storage.
 */
export function createTasksClient(options: TasksClientOptions = {}) {
  const baseUrl = options.baseUrl ?? "/api";
  const transport = createClient<paths>({
    baseUrl,
    credentials: "include",
    fetch: options.fetch,
  });

  async function unwrap<T>(resultPromise: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
    const result = await resultPromise;
    if (result.data !== undefined) return result.data;
    if (result.response.ok) return undefined as T;

    let message = `Request failed (${result.response.status})`;
    let code: string | undefined;
    if (result.error && typeof result.error === "object") {
      const body = result.error as { detail?: string; message?: string; code?: string };
      message = body.detail ?? body.message ?? message;
      code = body.code;
    }
    throw new TasksApiError(result.response.status, message, code);
  }

  return {
    async parentSession(options: RequestOptions = {}) {
      return unwrap(transport.GET("/auth/session", { ...options }));
    },

    /** Start the real BFF authorization-code flow; provider credentials stay server-side. */
    beginParentLogin(returnTo = "/parent/students") {
      const target = new URL("/auth/login", window.location.origin);
      target.searchParams.set("return_to", returnTo);
      window.location.assign(target.toString());
    },

    async logout(options: RequestOptions = {}) {
      return unwrap(transport.POST("/auth/logout", { ...options }));
    },

    async listStudents(query: StudentListQuery, options: RequestOptions = {}) {
      return unwrap(transport.GET("/students", { ...options, params: { query } }));
    },

    async getStudent(id: string, options: RequestOptions = {}) {
      return unwrap(transport.GET("/students/{id}", { ...options, params: { path: { id } } }));
    },

    async createStudent(body: CreateStudentBody, options: RequestOptions = {}) {
      return unwrap(transport.POST("/students", { ...options, body }));
    },

    async updateStudent(id: string, body: UpdateStudentBody, options: RequestOptions = {}) {
      return unwrap(transport.PATCH("/students/{id}", { ...options, params: { path: { id } }, body }));
    },

    async archiveStudent(id: string, options: RequestOptions = {}) {
      return unwrap(transport.DELETE("/students/{id}", { ...options, params: { path: { id } } }));
    },

    async issuePairing(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/students/{id}/pairing", { ...options, params: { path: { id } } }));
    },

    async pairStudent(body: PairStudentBody, options: RequestOptions = {}) {
      return unwrap(transport.POST("/student/pair", { ...options, body }));
    },

    async studentProfile(options: RequestOptions = {}) {
      return unwrap(transport.GET("/student/profile", { ...options }));
    },

    async studentChecklist(options: RequestOptions = {}) {
      return unwrap(transport.GET("/student/checklist", { ...options }));
    },
  };
}

export type TasksClient = ReturnType<typeof createTasksClient>;
export const tasksClient = createTasksClient();
