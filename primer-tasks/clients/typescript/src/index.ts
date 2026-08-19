import createClient from "openapi-fetch";
import type { paths, components } from "../generated/schema";

export type { components, paths } from "../generated/schema";
export type Student = components["schemas"]["Student"];
export type Health = components["schemas"]["Health"];

type Operation<Path extends keyof paths, Method extends keyof paths[Path]> = NonNullable<paths[Path][Method]>;
type JsonBody<Path extends keyof paths, Method extends keyof paths[Path]> = Operation<Path, Method> extends { requestBody?: { content: { "application/json": infer Body } } } ? Body : never;
type Query<Path extends keyof paths, Method extends keyof paths[Path]> = Operation<Path, Method> extends { parameters?: { query?: infer Parameters } } ? Parameters : never;

type CreateStudentBody = JsonBody<"/students", "post">;
type UpdateStudentBody = JsonBody<"/students/{id}", "patch">;
type PairStudentBody = JsonBody<"/student/pair", "post">;
type StudentListQuery = Query<"/students", "get">;
type TaskListQuery = Query<"/tasks", "get">;
type OccurrenceListQuery = Query<"/occurrences", "get">;
type ScheduleListQuery = Query<"/schedules", "get">;
type ScheduleInputBody = JsonBody<"/schedules", "post">;
type TaskInputBody = JsonBody<"/tasks", "post">;
type DecisionInputBody = JsonBody<"/occurrences/{id}/decision", "post">;
export type Task = components["schemas"]["TaskRevision"];
export type TaskPage = components["schemas"]["TaskPage2"];
export type Schedule = components["schemas"]["Schedule2"];
export type Occurrence = components["schemas"]["Occurrence2"];
export type OccurrencePage = components["schemas"]["OccurrencePage2"];

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
    async health(options: RequestOptions = {}) {
      return unwrap(transport.GET("/health", { ...options }));
    },
    async parentSession(options: RequestOptions = {}) {
      return unwrap(transport.GET("/auth/session", { ...options }));
    },

    /** Start the real BFF authorization-code flow; provider credentials stay server-side. */
    beginParentLogin(returnTo = "/parent/students") {
      // Keep the browser on the same-origin BFF namespace. Vite (and the
      // production reverse proxy) forwards /api/auth to the Tasks API's
      // internal /auth routes without exposing a cross-origin URL.
      const target = new URL("/api/auth/login", window.location.origin);
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
    async listTasks(query: TaskListQuery = {}, options: RequestOptions = {}) {
      return unwrap(transport.GET("/tasks", { ...options, params: { query } }));
    },
    async createTask(body: TaskInputBody, options: RequestOptions = {}) {
      return unwrap(transport.POST("/tasks", { ...options, body }));
    },
    async publishTask(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/tasks/{id}/publish", { ...options, params: { path: { id } } }));
    },
    async retireTask(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/tasks/{id}/retire", { ...options, params: { path: { id } } }));
    },
    async createSchedule(body: ScheduleInputBody, options: RequestOptions = {}) {
      return unwrap(transport.POST("/schedules", { ...options, body }));
    },
    async listSchedules(query: ScheduleListQuery = {}, options: RequestOptions = {}) {
      return unwrap(transport.GET("/schedules", { ...options, params: { query } }));
    },
    async updateSchedule(id: string, body: ScheduleInputBody, options: RequestOptions = {}) {
      return unwrap(transport.PATCH("/schedules/{id}", { ...options, params: { path: { id } }, body }));
    },
    async retireSchedule(id: string, options: RequestOptions = {}) {
      return unwrap(transport.DELETE("/schedules/{id}", { ...options, params: { path: { id } } }));
    },
    async listOccurrences(query: OccurrenceListQuery = {}, options: RequestOptions = {}) {
      return unwrap(transport.GET("/occurrences", { ...options, params: { query } }));
    },
    async getOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.GET("/occurrences/{id}", { ...options, params: { path: { id } } }));
    },
    async decideOccurrence(id: string, body: DecisionInputBody, options: RequestOptions = {}) {
      return unwrap(transport.POST("/occurrences/{id}/decision", { ...options, params: { path: { id } }, body }));
    },
    async retryOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/occurrences/{id}/retry", { ...options, params: { path: { id } } }));
    },
    async skipOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/occurrences/{id}/skip", { ...options, params: { path: { id } } }));
    },
    async cancelOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/occurrences/{id}/cancel", { ...options, params: { path: { id } } }));
    },
    async studentToday(options: RequestOptions = {}) {
      return unwrap(transport.GET("/student/today", { ...options }));
    },
    async studentUpcoming(options: RequestOptions = {}) {
      return unwrap(transport.GET("/student/upcoming", { ...options }));
    },
    async studentOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.GET("/student/occurrences/{id}", { ...options, params: { path: { id } } }));
    },
    async startStudentOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/student/occurrences/{id}/start", { ...options, params: { path: { id } } }));
    },
    async deviceToday(options: RequestOptions = {}) {
      return unwrap(transport.GET("/device/today", { ...options }));
    },
    async deviceUpcoming(options: RequestOptions = {}) {
      return unwrap(transport.GET("/device/upcoming", { ...options }));
    },
    async deviceOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.GET("/device/occurrences/{id}", { ...options, params: { path: { id } } }));
    },
    async startDeviceOccurrence(id: string, options: RequestOptions = {}) {
      return unwrap(transport.POST("/device/occurrences/{id}/start", { ...options, params: { path: { id } } }));
    },
  };
}

export type TasksClient = ReturnType<typeof createTasksClient>;
export const tasksClient = createTasksClient();
