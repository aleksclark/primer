import createClient from "openapi-fetch";
import type { paths, components } from "../generated/schema";
import {
  parseInspectTimeline,
  parseStudentDialogueState,
  validateOverrideInput,
  type InspectTimeline,
  type OverrideInput,
  type StudentDialogueState,
} from "./dialogue.ts";
import {
  artifactRubricRequirement,
  type ArtifactFinalizeInput,
  type ArtifactKind,
  type ArtifactRubricConfig,
  type ArtifactRubricRequirement,
  type ArtifactStudentState,
  type ArtifactUploadReservation,
} from "./artifact.ts";

export {
  AGENT_PROTOCOL_VERSION,
  parseAgentEvent,
  safeAgentToolLabel,
} from "./agent-protocol.ts";
export type {
  AgentCancelCommand,
  AgentCommand,
  AgentConfirmCommand,
  AgentEvent,
  AgentMessageCommand,
  AgentSubscribeCommand,
  AgentToolLabel,
  AgentUnsubscribeCommand,
} from "./agent-protocol.ts";
export { createAgentClient, readDurableAgentConversation, writeDurableAgentConversation } from "./agent-client.ts";
export { STUDENT_DIALOGUE_PROTOCOL_VERSION, parseStudentDialogueEvent } from "./student-dialogue-protocol.ts";
export type { StudentDialogueCommand, StudentDialogueEvent } from "./student-dialogue-protocol.ts";
export { createDialogueClient } from "./dialogue-client.ts";
export { createArtifactClient } from "./artifact-client.ts";
export type { ArtifactClient, ArtifactClientError, ArtifactClientOptions, ArtifactClientSnapshot, ArtifactConnectionState } from "./artifact-client.ts";
export type {
  AgentClient,
  AgentClientError,
  AgentClientOptions,
  AgentClientSnapshot,
  AgentConnectionState,
} from "./agent-client.ts";
export type {
  DialogueClient,
  DialogueClientError,
  DialogueClientOptions,
  DialogueClientSnapshot,
  DialogueConnectionState,
} from "./dialogue-client.ts";
export {
  AGENT_DIALOGUE_CONFIG_VERSION,
  AGENT_DIALOGUE_EXECUTOR,
  AGENT_DIALOGUE_INTERACTION,
  AGENT_DIALOGUE_KIND,
  defaultDialogueConfig,
  dialogueRequirement,
  normalizeDialogueConfig,
  parseInspectTimeline,
  parseStudentDialogueState,
  previewDialogueConfig,
  stripUnsafeDialogueFields,
  studentProgressCopy,
  validateDialogueConfig,
  validateOverrideInput,
} from "./dialogue.ts";
export type {
  DialogueConfig,
  DialogueConfigIssue,
  DialogueConfigPreview,
  DialogueRequirement,
  InspectEntry,
  InspectTimeline,
  OverrideInput,
  OverrideRecord,
  StudentDialogueState,
  StudentDialogueStatus,
} from "./dialogue.ts";
export {
  ARTIFACT_KINDS,
  ARTIFACT_LIMITS,
  ARTIFACT_RUBRIC_CONFIG_VERSION,
  ARTIFACT_RUBRIC_EXECUTOR,
  ARTIFACT_RUBRIC_INTERACTION,
  ARTIFACT_RUBRIC_KIND,
  artifactRubricRequirement,
  defaultArtifactRubricConfig,
  mediaKind,
  newArtifactIdempotencyKey,
  normalizeArtifactRubricConfig,
  parseArtifactProgressEvent,
  previewArtifactRubric,
  sha256Hex,
  validateArtifactFile,
  validateArtifactRubric,
} from "./artifact.ts";
export type {
  ArtifactCriterionEvaluation,
  ArtifactEvaluation,
  ArtifactEvaluationStatus,
  ArtifactFinalizeInput,
  ArtifactKind,
  ArtifactProgressEvent,
  ArtifactReviewPolicy,
  ArtifactRubricConfig,
  ArtifactRubricCriterion,
  ArtifactRubricIssue,
  ArtifactRubricPreview,
  ArtifactRubricRequirement,
  ArtifactStudentState,
  ArtifactSubmission,
  ArtifactSubmissionStatus,
  ArtifactUploadReservation,
} from "./artifact.ts";

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
type ArtifactReservationBody = JsonBody<"/student/occurrences/{occurrence}/artifacts/reserve", "post">;
type ArtifactFinalizeBody = JsonBody<"/student/occurrences/{occurrence}/artifacts/finalize", "post">;
export type ArtifactReservationResponse = components["schemas"]["ArtifactReservationOutput"];
export type ArtifactStateResponse = components["schemas"]["ArtifactStateResponse"];
export type ArtifactFinalizeResponse = components["schemas"]["ArtifactOutput"];
export type ArtifactRetryResponse = components["schemas"]["ArtifactStateResponse"];
export type ParentArtifactInspectResponse = components["schemas"]["ArtifactStateResponse"];
export type ArtifactRequirementBody = ArtifactRubricRequirement;
export type Task = components["schemas"]["TaskRevision"];
export type TaskPage = components["schemas"]["TaskPage2"];
export type Schedule = components["schemas"]["Schedule2"];
export type Occurrence = components["schemas"]["Occurrence2"];
export type OccurrencePage = components["schemas"]["OccurrencePage2"];
export type AgentConversation = components["schemas"]["AgentConversation"];

export interface TasksClientOptions {
  baseUrl?: string;
  fetch?: typeof globalThis.fetch;
}

export interface RequestOptions {
  signal?: AbortSignal;
  fetch?: typeof globalThis.fetch;
}

export interface ArtifactUploadOptions {
  signal?: AbortSignal;
  onProgress?: (loaded: number, total: number) => void;
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

  async function requestJSON<T>(path: string, init: RequestInit = {}, parse: (body: unknown) => T | null): Promise<T> {
    const fetchImpl = options.fetch ?? globalThis.fetch;
    const headers = new Headers(init.headers);
    if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
    const response = await fetchImpl(`${baseUrl}${path}`, { credentials: "include", ...init, headers, signal: init.signal });
    let payload: unknown;
    const text = await response.text();
    if (text) {
      try { payload = JSON.parse(text); } catch { payload = undefined; }
    }
    if (!response.ok) {
      const body = payload && typeof payload === "object" ? payload as { detail?: string; message?: string; code?: string } : undefined;
      throw new TasksApiError(response.status, body?.detail ?? body?.message ?? `Request failed (${response.status})`, body?.code);
    }
    const parsed = parse(payload);
    if (parsed === null) throw new TasksApiError(response.status, "The server returned an unusable dialogue record.");
    return parsed;
  }

  /**
   * The sole binary transport façade. Pages receive a reservation or an
   * occurrence-scoped derivative response and never choose an object-store
   * URL. Both PUT and GET targets are checked before browser transport.
   */
  function artifactTarget(value: string): URL {
    const pageOrigin = typeof window === "undefined" ? new URL(baseUrl, "http://localhost").origin : window.location.origin;
    const target = new URL(value, pageOrigin);
    const apiRoot = new URL(baseUrl, pageOrigin).pathname.replace(/\/+$/, "");
    const relativePath = target.pathname.startsWith(`${apiRoot}/`) ? target.pathname.slice(apiRoot.length) : "";
    const uuid = "[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}";
    const allowedPath = new RegExp(`^/student/artifacts/${uuid}/(?:upload|parts/[1-9][0-9]*)$`).test(relativePath)
      || new RegExp(`^/student/artifacts/${uuid}/derivative/(?:thumbnail|preview)$`).test(relativePath)
      || new RegExp(`^/occurrences/${uuid}/artifacts/${uuid}/derivative$`).test(relativePath);
    if (target.origin !== pageOrigin || !/^https?:$/.test(target.protocol) || target.username || target.password || target.hash || target.search || !allowedPath) {
      throw new TasksApiError(400, "The server returned an invalid artifact target.", "invalid_artifact_target");
    }
    return target;
  }

  async function uploadArtifactBinary(reservation: ArtifactUploadReservation, file: File, options: ArtifactUploadOptions = {}): Promise<void> {
    const target = artifactTarget(reservation.uploadUrl);
    await new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("PUT", target.toString(), true);
      xhr.withCredentials = true;
      xhr.setRequestHeader("Content-Type", file.type || reservation.mediaType);
      if (reservation.expectedDigest) xhr.setRequestHeader("X-Artifact-Digest", reservation.expectedDigest);
      if (options.signal) options.signal.addEventListener("abort", () => xhr.abort(), { once: true });
      xhr.upload.onprogress = (event) => options.onProgress?.(event.loaded, event.lengthComputable ? event.total : file.size);
      xhr.onload = () => xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new TasksApiError(xhr.status, "The artifact upload was not accepted."));
      xhr.onerror = () => reject(new TasksApiError(0, "The artifact upload could not reach the server.", "upload_network"));
      xhr.onabort = () => reject(new TasksApiError(0, "The artifact upload was interrupted.", "upload_aborted"));
      xhr.send(file);
    });
  }

  async function downloadArtifactBinary(path: string, options: RequestOptions = {}): Promise<Blob> {
    const target = artifactTarget(`${baseUrl}${path}`);
    const response = await (options.fetch ?? globalThis.fetch)(target.toString(), { credentials: "include", signal: options.signal });
    if (!response.ok) throw new TasksApiError(response.status, "The authorized artifact preview was not available.");
    return response.blob();
  }

  return {
    async health(options: RequestOptions = {}) {
      return unwrap(transport.GET("/health", { ...options }));
    },
    async parentSession(options: RequestOptions = {}) {
      return unwrap(transport.GET("/auth/session", { ...options }));
    },
    async createAgentConversation(options: RequestOptions = {}) {
      return unwrap(transport.POST("/agent/conversations", { ...options }));
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
    async createArtifactRubricTask(body: { title: string; instructions: string; rubric: ArtifactRubricConfig }, options: RequestOptions = {}) {
      const requirement = artifactRubricRequirement("artifact-rubric", body.rubric);
      return unwrap(transport.POST("/tasks", { ...options, body: { title: body.title, instructions: body.instructions, requirements: [requirement] } }));
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
    async inspectOccurrence(id: string, options: RequestOptions = {}): Promise<InspectTimeline> {
      return requestJSON(`/occurrences/${encodeURIComponent(id)}/inspect`, { method: "GET", signal: options.signal }, parseInspectTimeline);
    },
    async overrideOccurrence(id: string, body: OverrideInput, options: RequestOptions = {}): Promise<InspectTimeline> {
      const invalid = validateOverrideInput(body);
      if (invalid) throw new TasksApiError(400, invalid, "invalid_request");
      return requestJSON(`/occurrences/${encodeURIComponent(id)}/override`, { method: "POST", body: JSON.stringify(body), signal: options.signal }, parseInspectTimeline);
    },
    async studentDialogue(id: string, options: RequestOptions = {}): Promise<StudentDialogueState> {
      return requestJSON(`/student/occurrences/${encodeURIComponent(id)}/dialogue`, { method: "GET", signal: options.signal }, parseStudentDialogueState);
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
    /** Phase 5 JSON operations use only generated OpenAPI request/response types. */
    async studentArtifactState(id: string, options: RequestOptions = {}): Promise<ArtifactStudentState | null> {
      try {
        return await unwrap(transport.GET("/student/occurrences/{occurrence}/artifacts", { ...options, params: { path: { occurrence: id } } })) as ArtifactStudentState;
      } catch (next) {
        if (next instanceof TasksApiError && next.status === 404) return null;
        throw next;
      }
    },
    async reserveArtifact(id: string, body: { kind: ArtifactKind; mediaType: string; sizeBytes: number; digest?: string; idempotencyKey: string; filename?: string }, options: RequestOptions = {}): Promise<ArtifactUploadReservation> {
      const payload: ArtifactReservationBody = { kind: body.kind, contentType: body.mediaType, size: body.sizeBytes, idempotencyKey: body.idempotencyKey, filename: body.filename };
      const wire: ArtifactReservationResponse = await unwrap(transport.POST("/student/occurrences/{occurrence}/artifacts/reserve", { ...options, params: { path: { occurrence: id } }, body: payload }));
      return { ...wire, occurrenceId: id, kind: body.kind, mediaType: body.mediaType, maxBytes: body.sizeBytes, expectedDigest: body.digest };
    },
    async uploadArtifact(reservation: ArtifactUploadReservation, file: File, options: ArtifactUploadOptions = {}) {
      return uploadArtifactBinary(reservation, file, options);
    },
    async finalizeArtifact(id: string, body: ArtifactFinalizeInput, options: RequestOptions = {}): Promise<ArtifactStudentState> {
      const payload: ArtifactFinalizeBody = body;
      const _finalized: ArtifactFinalizeResponse = await unwrap(transport.POST("/student/occurrences/{occurrence}/artifacts/finalize", { ...options, params: { path: { occurrence: id } }, body: payload }));
      void _finalized;
      const state = await this.studentArtifactState(id, options);
      if (!state) throw new TasksApiError(502, "The server did not return artifact state after finalization.");
      return state;
    },
    async retryArtifactEvaluation(id: string, options: RequestOptions = {}): Promise<ArtifactStudentState> {
      return await unwrap(transport.POST("/student/occurrences/{occurrence}/artifacts/retry", { ...options, params: { path: { occurrence: id } } })) as ArtifactStudentState;
    },
    async inspectOccurrenceArtifacts(id: string, options: RequestOptions = {}): Promise<ArtifactStudentState> {
      return await unwrap(transport.GET("/occurrences/{occurrence}/artifacts/inspect", { ...options, params: { path: { occurrence: id } } })) as ArtifactStudentState;
    },
    async getArtifactDerivative(occurrenceId: string, artifactId: string, options: RequestOptions = {}): Promise<Blob> {
      return downloadArtifactBinary(`/occurrences/${encodeURIComponent(occurrenceId)}/artifacts/${encodeURIComponent(artifactId)}/derivative`, options);
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
