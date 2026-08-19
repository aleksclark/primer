/**
 * primer-agents TypeScript client facade.
 *
 * All request/response types come from types.gen.ts which is generated from
 * openapi.yaml by openapi-typescript v7.4.0. Do not duplicate DTOs here.
 *
 * Regeneration: make clients-ts (from the primer-agents module root)
 *
 * This facade requires `openapi-fetch` (npm install openapi-fetch).
 * Using openapi-fetch@0.12 or later for full OAS 3.1 path-type support.
 */

import createClient, { type Client as OpenApiFetchClient } from "openapi-fetch";
import type { paths, components } from "./types.gen.js";

export type RunResponse = components["schemas"]["RunResponse"];
export type SessionResponse = components["schemas"]["SessionResponse"];
export type EventResponse = components["schemas"]["EventResponse"];
export type CreateRunBody = components["schemas"]["CreateRunInputBody"];
export type CreateSessionBody = components["schemas"]["CreateSessionInputBody"];
export type CancelRunBody = components["schemas"]["CancelRunInputBody"];
export type AppendTurnBody = components["schemas"]["AppendTurnInputBody"];
export type TurnResponse = components["schemas"]["TurnResponse"];
export type AppendTurnResult = components["schemas"]["AppendTurnOutBody"];

/** SSEEventFrame is the durable event frame received over the SSE stream. */
export interface SSEEventFrame {
  schemaVersion: number;
  sequence: number;
  runId: string;
  rootRunId?: string;
  parentRunId?: string;
  agentId?: string;
  agentType?: string;
  agentDepth?: number;
  kind: string;
  payload?: string;
  createdAt: string;
}

/** AgentsClient wraps openapi-fetch with bearer auth and typed helpers. */
export class AgentsClient {
  private readonly http: OpenApiFetchClient<paths>;

  constructor(baseUrl: string, bearer: string) {
    this.baseUrl = baseUrl;
    this.bearer = bearer;
    this.http = createClient<paths>({
      baseUrl,
      headers: { Authorization: `Bearer ${bearer}` },
    });
  }

  /** Create a run (idempotent on idempotencyKey). Returns the durable queued record. */
  async createRun(idempotencyKey: string, body: CreateRunBody): Promise<RunResponse> {
    const { data, error } = await this.http.POST("/agents/v1/runs", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body,
    });
    if (!data) throw new Error(`createRun failed: ${JSON.stringify(error)}`);
    return data;
  }

  /** Get a run by server-generated ID. */
  async getRun(id: string): Promise<RunResponse> {
    const { data, error } = await this.http.GET("/agents/v1/runs/{id}", {
      params: { path: { id } },
    });
    if (!data) throw new Error(`getRun failed: ${JSON.stringify(error)}`);
    return data;
  }

  /** List runs owned by the authenticated caller. */
  async listRuns(limit?: number): Promise<RunResponse[]> {
    const { data, error } = await this.http.GET("/agents/v1/runs", {
      params: { query: limit !== undefined ? { limit } : {} },
    });
    if (!data) throw new Error(`listRuns failed: ${JSON.stringify(error)}`);
    return data.runs ?? [];
  }

  /** Request cancellation of a run (idempotent). */
  async cancelRun(id: string, body: CancelRunBody = {}): Promise<RunResponse> {
    const { data, error } = await this.http.POST("/agents/v1/runs/{id}/cancel", {
      params: { path: { id } },
      body,
    });
    if (!data) throw new Error(`cancelRun failed: ${JSON.stringify(error)}`);
    return data;
  }

  /** Page run events by sequence cursor. */
  async listEvents(runId: string, afterSeq = 0, limit?: number): Promise<EventResponse[]> {
    const { data, error } = await this.http.GET("/agents/v1/runs/{id}/events", {
      params: { path: { id: runId }, query: { afterSeq, ...(limit !== undefined && { limit }) } },
    });
    if (!data) throw new Error(`listEvents failed: ${JSON.stringify(error)}`);
    return data.events ?? [];
  }

  /** Create a durable open session. */
  async createSession(body: CreateSessionBody): Promise<SessionResponse> {
    const { data, error } = await this.http.POST("/agents/v1/sessions", { body });
    if (!data) throw new Error(`createSession failed: ${JSON.stringify(error)}`);
    return data;
  }

  /** Get a session by server-generated ID. */
  async getSession(id: string): Promise<SessionResponse> {
    const { data, error } = await this.http.GET("/agents/v1/sessions/{id}", {
      params: { path: { id } },
    });
    if (!data) throw new Error(`getSession failed: ${JSON.stringify(error)}`);
    return data;
  }

  /** Append a turn to a session (idempotent with expectedRevision CAS). */
  async appendTurn(sessionId: string, body: AppendTurnBody): Promise<AppendTurnResult> {
    const { data, error } = await this.http.POST("/agents/v1/sessions/{id}/turns", {
      params: { path: { id: sessionId } },
      body,
    });
    if (!data) throw new Error(`appendTurn failed: ${JSON.stringify(error)}`);
    return data;
  }

  /**
   * streamRunEvents opens a replay-first SSE connection and calls onFrame for
   * each committed event frame. The stream ends when the run is terminal.
   * Returns an AbortController the caller can use to disconnect without
   * cancelling the run.
   *
   * lastEventId: resume cursor (Last-Event-ID from a prior connection).
   * Disconnecting (abort.abort()) does NOT cancel the run.
   */
  streamRunEvents(
    runId: string,
    onFrame: (frame: SSEEventFrame) => void,
    lastEventId = "0",
  ): AbortController {
    const abort = new AbortController();
    const url = `${this.baseUrl}/agents/v1/runs/${encodeURIComponent(runId)}/events/stream`;

    const connect = async () => {
      const resp = await fetch(url, {
        signal: abort.signal,
        headers: {
          Authorization: `Bearer ${this.bearer}`,
          "Last-Event-ID": lastEventId,
          Accept: "text/event-stream",
        },
      });
      if (!resp.ok || !resp.body) return;
      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buf = "";
      let curId = "";
      let curData = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const lines = buf.split("\n");
        buf = lines.pop() ?? "";
        for (const line of lines) {
          if (line.startsWith("id: ")) {
            curId = line.slice(4);
          } else if (line.startsWith("data: ")) {
            curData = line.slice(6);
          } else if (line === "") {
            if (curData) {
              try {
                const frame = JSON.parse(curData) as SSEEventFrame;
                lastEventId = curId || String(frame.sequence);
                onFrame(frame);
              } catch { /* ignore malformed */ }
            }
            curId = "";
            curData = "";
          }
        }
      }
    };

    connect().catch(() => {/* disconnected */});
    return abort;
  }

  private readonly baseUrl: string;
  private readonly bearer: string;
}
