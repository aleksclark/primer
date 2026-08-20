import { parseArtifactProgressEvent, type ArtifactProgressEvent } from "./artifact";
import type { AgentClientError, AgentClientSnapshot } from "./agent-client";

export type ArtifactConnectionState = AgentClientSnapshot["connectionState"];
export type ArtifactClientError = AgentClientError;
export type ArtifactClientSnapshot = {
  connectionState: ArtifactConnectionState;
  cursor: number;
  events: readonly ArtifactProgressEvent[];
  error?: ArtifactClientError;
};

export interface ArtifactClientOptions {
  occurrenceId: string;
  submissionId?: string;
  url?: string;
  reconnect?: boolean;
  reconnectBaseMs?: number;
  reconnectMaxMs?: number;
  maxEvents?: number;
  webSocketFactory?: (url: string, protocols?: string | string[]) => WebSocket;
  setTimeout?: typeof globalThis.setTimeout;
  clearTimeout?: typeof globalThis.clearTimeout;
}

export interface ArtifactClient {
  subscribe(listener: (snapshot: ArtifactClientSnapshot) => void): () => void;
  connect(): void;
  disconnect(): void;
  snapshot(): ArtifactClientSnapshot;
}

const DEFAULT_RECONNECT_BASE_MS = 500;
const DEFAULT_RECONNECT_MAX_MS = 8_000;

function defaultUrl(): string {
  const url = new URL("/api/student/ws", window.location.origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

function defaultProtocols(): string[] {
  const csrf = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith("tasks_csrf="))?.slice("tasks_csrf=".length);
  return csrf ? [`primer-tasks.v1.csrf.${csrf}`, "primer-tasks.student.v1"] : ["primer-tasks.student.v1"];
}

function error(code: ArtifactClientError["code"], message: string, retryable: boolean): ArtifactClientError {
  return { code, message, retryable };
}

/**
 * No-chat student event façade. It subscribes to an occurrence-scoped artifact
 * stream and only projects safe progress/criterion fields. Raw model events,
 * tool arguments, and reasoning never reach the page.
 */
export function createArtifactClient(options: ArtifactClientOptions): ArtifactClient {
  const schedule = options.setTimeout ?? globalThis.setTimeout;
  const clear = options.clearTimeout ?? globalThis.clearTimeout;
  const makeSocket = options.webSocketFactory ?? ((url: string, protocols?: string | string[]) => new WebSocket(url, protocols));
  const maxEvents = options.maxEvents ?? 200;
  const listeners = new Set<(snapshot: ArtifactClientSnapshot) => void>();
  let socket: WebSocket | undefined;
  let timer: ReturnType<typeof globalThis.setTimeout> | undefined;
  let attempts = 0;
  let closed = false;
  let current: ArtifactClientSnapshot = { connectionState: "idle", cursor: 0, events: [] };
  const notify = () => listeners.forEach((listener) => listener(current));
  const setState = (connectionState: ArtifactConnectionState, nextError?: ArtifactClientError) => {
    current = { ...current, connectionState, error: nextError };
    notify();
  };
  const connect = () => {
    if (closed) closed = false;
    if (socket) return;
    setState(attempts ? "reconnecting" : "connecting");
    try {
      socket = makeSocket(options.url ?? defaultUrl(), defaultProtocols());
      socket.onopen = () => {
        attempts = 0;
        setState("connected");
        socket?.send(JSON.stringify({ protocol: 1, kind: "artifact_subscribe", occurrenceId: options.occurrenceId, submissionId: options.submissionId, cursor: current.cursor }));
      };
      socket.onmessage = (message) => {
        const event = parseArtifactProgressEvent(JSON.parse(String(message.data)));
        if (!event) {
          setState(current.connectionState, error("protocol", "The artifact verifier sent invalid progress.", false));
          return;
        }
        if (event.kind !== "hello" && event.sequence <= current.cursor) return;
        current = { ...current, cursor: Math.max(current.cursor, event.cursor, event.sequence), events: [...current.events, event].slice(-maxEvents), error: undefined };
        notify();
      };
      socket.onerror = () => setState(current.connectionState, error("socket", "The artifact progress connection failed.", true));
      socket.onclose = () => {
        socket = undefined;
        if (closed) return;
        setState("offline", current.error);
        if (options.reconnect === false || timer !== undefined) return;
        const delay = Math.min(options.reconnectMaxMs ?? DEFAULT_RECONNECT_MAX_MS, (options.reconnectBaseMs ?? DEFAULT_RECONNECT_BASE_MS) * 2 ** attempts++);
        setState("reconnecting", current.error);
        timer = schedule(() => { timer = undefined; connect(); }, delay);
      };
    } catch {
      socket = undefined;
      setState("offline", error("socket", "Unable to connect to artifact progress.", true));
    }
  };
  return {
    subscribe(listener) { listeners.add(listener); listener(current); return () => listeners.delete(listener); },
    connect,
    disconnect() { closed = true; if (timer !== undefined) clear(timer); timer = undefined; socket?.close(); socket = undefined; setState("offline", current.error); },
    snapshot: () => current,
  };
}
