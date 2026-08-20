import { parseStudentDialogueEvent, STUDENT_DIALOGUE_PROTOCOL_VERSION, type StudentDialogueCommand, type StudentDialogueEvent } from "./student-dialogue-protocol.ts";
import type { AgentClientError, AgentClientSnapshot } from "./agent-client.ts";
import type { AgentEvent } from "./agent-protocol.ts";

export type DialogueConnectionState = AgentClientSnapshot["connectionState"];
export type DialogueClientError = AgentClientError;
export type DialogueClientSnapshot = AgentClientSnapshot;
export interface DialogueClientOptions {
  url?: string; conversationId: string; attemptId?: string; occurrenceId?: string;
  maxEvents?: number; maxQueuedMessages?: number; reconnect?: boolean; reconnectBaseMs?: number; reconnectMaxMs?: number;
  webSocketFactory?: (url: string, protocols?: string | string[]) => WebSocket; randomId?: () => string;
  setTimeout?: typeof globalThis.setTimeout; clearTimeout?: typeof globalThis.clearTimeout;
}
export interface DialogueClient { readonly conversationId: string; subscribe(listener: (snapshot: DialogueClientSnapshot) => void): () => void; connect(): void; disconnect(): void; sendMessage(text: string): string; retry(): void; snapshot(): DialogueClientSnapshot; }
const OPEN = 1;
const id = () => typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
function url(explicit?: string): string { if (explicit) return explicit; const value = new URL("/api/student/ws", window.location.origin); value.protocol = value.protocol === "https:" ? "wss:" : "ws:"; return value.toString(); }
function protocols(): string[] {
  const csrf = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith("tasks_csrf="))?.slice("tasks_csrf=".length);
  return csrf ? [`primer-tasks.v1.csrf.${csrf}`, "primer-tasks.student.v1"] : ["primer-tasks.student.v1"];
}
function clientError(code: DialogueClientError["code"], message: string, retryable: boolean): DialogueClientError { return { code, message, retryable }; }
function project(event: StudentDialogueEvent, runId: string): AgentEvent[] {
  const base = { protocol: 1 as const, runId, sequence: event.sequence, cursor: event.cursor, time: event.time ?? new Date().toISOString() };
  switch (event.kind) {
    case "hello": return [{ ...base, kind: "hello" }];
    case "question": return [{ ...base, kind: "text_start", text: event.text }, { ...base, kind: "text_end", text: event.text }];
    case "progress":
      return event.phase === "retry"
        ? [{ ...base, kind: "retry", retry: 1, retryAfterMs: 0 }]
        : [{ ...base, kind: "tool_progress", label: event.phase === "evaluating" ? "Evaluating" : "Thinking", phase: event.phase ?? "working" }];
    case "answer_evaluation": return event.status === "rejected" ? [{ ...base, kind: "retry", retry: 1, retryAfterMs: 0 }] : [];
    case "complete": return [{ ...base, kind: "terminal", status: "completed", text: "Verification complete." }];
    case "error": return [{ ...base, kind: "error", code: event.code ?? "student_dialogue" }];
    default: return [];
  }
}
/** Generated-client façade for the authenticated, occurrence-scoped student socket. */
export function createDialogueClient(options: DialogueClientOptions): DialogueClient {
  const maxEvents = options.maxEvents ?? 400, maxQueue = options.maxQueuedMessages ?? 20;
  const factory = options.webSocketFactory ?? ((value, protocols) => new WebSocket(value, protocols));
  const schedule = options.setTimeout ?? globalThis.setTimeout, clear = options.clearTimeout ?? globalThis.clearTimeout, makeId = options.randomId ?? id;
  const listeners = new Set<(snapshot: DialogueClientSnapshot) => void>(); const queue: StudentDialogueCommand[] = [];
  let socket: WebSocket | undefined, timer: ReturnType<typeof globalThis.setTimeout> | undefined, attempts = 0, closed = false;
  let current: DialogueClientSnapshot = { connectionState: "idle", cursor: 0, events: [], queuedMessages: 0 };
  const notify = () => { current = { ...current, queuedMessages: queue.filter((x) => x.kind === "user_message").length }; listeners.forEach((listener) => listener(current)); };
  const state = (connectionState: DialogueConnectionState, nextError?: DialogueClientError) => { current = { ...current, connectionState, error: nextError }; notify(); };
  const send = (command: StudentDialogueCommand) => { if (socket?.readyState === OPEN) { socket.send(JSON.stringify(command)); return true; } if (queue.length >= maxQueue) { state(current.connectionState, clientError("closed", "The answer queue is full. Reconnect before sending more.", true)); return false; } queue.push(command); notify(); return true; };
  const connect = () => {
    // React Strict Mode runs effect cleanup/setup once in development. A
    // cleanup disconnect must therefore not permanently poison the same
    // memoized client before its second setup can establish the socket.
    if (closed) closed = false;
    if (socket) return; state("connecting");
    try {
      socket = factory(url(options.url), protocols());
      socket.onopen = () => { attempts = 0; state("connected"); send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "subscribe", attemptId: options.attemptId, occurrenceId: options.occurrenceId ?? options.conversationId, cursor: current.cursor }); while (socket?.readyState === OPEN && queue.length) { const command = queue.shift(); if (command) socket.send(JSON.stringify(command)); } notify(); };
      socket.onmessage = (message) => { try { const event = parseStudentDialogueEvent(JSON.parse(message.data as string)); if (!event) throw new Error("invalid event");
        // Conflict errors are deliberately ephemeral (not durable replay
        // events), so the server may attach the current cursor to one. Do not
        // discard that typed recovery instruction merely because a concurrent
        // tab already advanced the durable cursor.
        if (event.kind !== "hello" && event.kind !== "error" && event.sequence > 0 && event.sequence <= current.cursor) return;
        current = { ...current, cursor: Math.max(current.cursor, event.cursor, event.sequence), events: [...current.events, ...project(event, options.attemptId ?? options.conversationId)].slice(-maxEvents) };
        if (event.kind === "error") current.error = clientError("server", event.message ?? "The verifier reported an error.", event.retryable ?? false);
        else if (event.kind === "progress" || event.kind === "question" || event.kind === "complete") current.error = undefined;
        notify();
      } catch { state(current.connectionState, clientError("protocol", "The student verifier sent invalid data.", false)); } };
      socket.onerror = () => state(current.connectionState, clientError("socket", "The student verifier connection failed.", true));
      socket.onclose = () => { socket = undefined; if (closed) return; state("offline", current.error); if (options.reconnect === false || timer !== undefined) return; const delay = Math.min(options.reconnectMaxMs ?? 8_000, (options.reconnectBaseMs ?? 500) * 2 ** attempts++); state("reconnecting", current.error); timer = schedule(() => { timer = undefined; connect(); }, delay); };
    } catch { socket = undefined; state("offline", clientError("socket", "Unable to connect to the student verifier.", true)); }
  };
  return {
    conversationId: options.conversationId,
    subscribe(listener) { listeners.add(listener); listener(current); return () => listeners.delete(listener); }, connect,
    disconnect() { closed = true; if (timer !== undefined) clear(timer); timer = undefined; socket?.close(); socket = undefined; state("offline", current.error); },
    sendMessage(text) { const clientMessageId = makeId(); send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "user_message", attemptId: options.attemptId ?? options.conversationId, occurrenceId: options.occurrenceId ?? options.conversationId, clientMessageId, text: text.trim(), expectedSequence: 0 }); return clientMessageId; },
    // Retry is a narrow, authenticated command for the durable failed job;
    // it neither re-sends student text nor carries a client decision.
    retry() { send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "retry", attemptId: options.attemptId ?? options.conversationId, occurrenceId: options.occurrenceId ?? options.conversationId }); },
    snapshot: () => current,
  };
}
