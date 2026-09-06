import {
  STUDENT_DIALOGUE_PROTOCOL_VERSION, isStudentDialogueCommand, parseStudentDialogueEvent, studentDialogueTransport,
  type StudentDialogueCommand, type StudentDialogueEvent, type StudentDialogueStateEvent,
} from "./student-dialogue-protocol.ts";

type Answer = Extract<StudentDialogueCommand, { kind: "user_message" }>;
type BoundEvent = Extract<StudentDialogueEvent, { kind: "state" | "question" | "message_ack" | "answer_evaluation" }>;
export type DialogueConnectionState = "idle" | "connecting" | "connected" | "reconnecting" | "offline" | "revoked";
export interface DialogueClientSnapshot {
  connectionState: DialogueConnectionState;
  cursor: number;
  events: readonly StudentDialogueEvent[];
  binding?: BoundEvent;
  state?: StudentDialogueStateEvent;
  status?: StudentDialogueStateEvent["status"];
  occurrenceStatus?: StudentDialogueStateEvent["occurrenceStatus"];
  canAnswer: boolean;
  completed: boolean;
  unsentAnswer?: string;
  error?: { code: string; message: string; retryable: boolean };
}
export interface DialogueClientOptions {
  occurrenceId: string; attemptId: string; baseUrl?: string;
  maxEvents?: number; reconnect?: boolean;
  webSocketFactory?: (url: string, protocols: string[]) => WebSocket;
  randomId?: () => string;
  setTimeout?: typeof globalThis.setTimeout; clearTimeout?: typeof globalThis.clearTimeout;
}
export interface DialogueClient {
  connect(): void; disconnect(): void;
  subscribe(listener: (snapshot: DialogueClientSnapshot) => void): () => void;
  snapshot(): DialogueClientSnapshot;
  sendMessage(text: string): string;
  retry(): void;
}
const safeMessages: Record<string, string> = {
  conflict: "The current question changed or another tab is answering. Refresh the dialogue state.",
  invalid_request: "This command does not match the current dialogue contract.",
  revoked: "Student pairing is no longer active. Pair again to continue.",
  not_found: "This dialogue is unavailable to the paired student.",
  exhausted: "This attempt has ended. Ask a parent to review or retry it.",
  provider_exhausted: "The verifier retry budget ended. Ask a parent to retry the attempt.",
  lease_exhausted: "The verifier could not resume this attempt. Ask a parent to retry it.",
  deadline_exhausted: "This verification attempt expired. Ask a parent to retry it.",
  job_budget_exhausted: "This verification attempt exhausted its retry budget.",
  provider_unavailable: "The verifier could not finish. Saved answers remain on the server.",
};
function safeError(code: string, retryable: boolean) {
  return { code, retryable, message: Object.hasOwn(safeMessages, code) ? safeMessages[code] : "Dialogue is temporarily unavailable. Saved evidence is retained." };
}

/** UI projection only. Completion is an explicit server occurrence outcome,
 * never an answer-level accepted flag, local count or fallback text. */
export function reduceDialogueEvent(previous: DialogueClientSnapshot, event: StudentDialogueEvent, maxEvents = 400): DialogueClientSnapshot {
  if (event.sequence > 0 && event.sequence <= previous.cursor) return previous;
  const next: DialogueClientSnapshot = { ...previous, cursor: Math.max(previous.cursor, event.cursor), events: event.sequence > 0 ? [...previous.events, event].slice(-maxEvents) : previous.events };
  const currentVersion = previous.binding?.version ?? 0;
  if ("version" in event && event.version < currentVersion) return next; // retain replay history, not obsolete current state
  switch (event.kind) {
    case "state":
      next.state = event; next.binding = event; next.status = event.status; next.occurrenceStatus = event.occurrenceStatus;
      next.completed = event.occurrenceStatus === "completed" && event.status === "accepted";
      next.canAnswer = event.status === "open" && event.occurrenceStatus === "awaiting_verification" && Boolean(event.questionId) && event.phase === undefined;
      next.error = event.phase === "failed" ? safeError(event.code ?? "provider_unavailable", event.retryable ?? false) : undefined;
      break;
    case "question":
      next.binding = event; next.canAnswer = !previous.completed && previous.status !== "exhausted" && previous.status !== "rejected" && previous.occurrenceStatus !== "canceled" && previous.occurrenceStatus !== "excused";
      next.error = undefined;
      break;
    case "message_ack": next.binding = event; next.canAnswer = false; break;
    case "answer_evaluation":
      next.binding = event; next.canAnswer = event.status === "rejected" && !previous.completed && previous.status !== "exhausted"; next.error = undefined;
      break;
    case "complete":
      next.completed = event.occurrenceStatus === "completed" && event.status === "accepted";
      next.status = event.status; next.occurrenceStatus = event.occurrenceStatus; next.canAnswer = false; next.error = undefined;
      break;
    case "override":
      next.status = event.status; next.occurrenceStatus = event.occurrenceStatus; next.canAnswer = false;
      next.completed = event.status === "accepted" && event.occurrenceStatus === "completed";
      break;
    case "error":
      next.error = safeError(event.code, "retryable" in event && event.retryable === true);
      if ("status" in event) { next.status = event.status; next.occurrenceStatus = event.occurrenceStatus; next.canAnswer = false; }
      break;
  }
  return next;
}

export function studentCSRFToken(): string {
  const value = document.cookie.split(";").map(part => part.trim()).find(part => part.startsWith("tasks_csrf="))?.slice("tasks_csrf=".length);
  if (!value || /[\s,]/.test(value)) throw new Error("Student CSRF protection is unavailable.");
  return value;
}
function studentSocketURL(baseUrl: string): string {
  const url = new URL(`${baseUrl.replace(/\/$/, "")}${studentDialogueTransport.path}`, window.location.origin);
  if (url.origin !== window.location.origin || url.username || url.password || url.search || url.hash) throw new Error("Student sockets require the same origin without URL credentials.");
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

/** The sole student socket constructor. No Clerk token, query credential or
 * parallel REST transport belongs here. Disconnect closes delivery, not work. */
export function createDialogueClient(options: DialogueClientOptions): DialogueClient {
  const listeners = new Set<(value: DialogueClientSnapshot) => void>();
  const schedule = options.setTimeout ?? globalThis.setTimeout, clear = options.clearTimeout ?? globalThis.clearTimeout;
  const makeId = options.randomId ?? (() => crypto.randomUUID());
  const factory = options.webSocketFactory ?? ((url, protocols) => new WebSocket(url, protocols));
  const maxEvents = options.maxEvents ?? 400;
  if (!Number.isInteger(maxEvents) || maxEvents < studentDialogueTransport.acknowledgmentWindow || maxEvents > 2000) throw new Error("Invalid dialogue event bound.");
  let current: DialogueClientSnapshot = { connectionState: "idle", cursor: 0, events: [], canAnswer: false, completed: false };
  let socket: WebSocket | undefined, pending: Answer | undefined, timer: ReturnType<typeof globalThis.setTimeout> | undefined;
  let closed = false, generation = 0, retries = 0;
  const notify = () => listeners.forEach(listener => listener(current));
  const send = (command: StudentDialogueCommand) => {
    if (!isStudentDialogueCommand(command)) throw new Error("Student command failed generated contract validation.");
    if (!socket || socket.readyState !== 1) throw new Error("Reconnect before sending this command.");
    socket.send(JSON.stringify(command));
  };
  const connect = () => {
    if (socket || current.connectionState === "revoked") return;
    closed = false; const epoch = ++generation;
    current = { ...current, connectionState: "connecting" }; notify();
    try { socket = factory(studentSocketURL(options.baseUrl ?? "/api"), [studentDialogueTransport.protocol, `primer-tasks.v1.csrf.${studentCSRFToken()}`]); }
    catch { current = { ...current, connectionState: "offline", error: safeError("unavailable", true) }; notify(); return; }
    const active = socket;
    active.onopen = () => {
      if (generation !== epoch || socket !== active) return;
      retries = 0; current = { ...current, connectionState: "connected" }; notify();
      send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "subscribe", occurrenceId: options.occurrenceId, attemptId: options.attemptId, cursor: current.cursor });
      if (pending) send(pending); // identical original version/key/text, never rebased to a new question
    };
    active.onmessage = message => {
      if (generation !== epoch || socket !== active) return;
      let event: StudentDialogueEvent | null = null;
      try { event = parseStudentDialogueEvent(JSON.parse(String(message.data))); } catch { /* fail closed below */ }
      if (!event || ("attemptId" in event && (event.attemptId !== options.attemptId || event.occurrenceId !== options.occurrenceId))) {
        closed = true; current = { ...current, error: safeError("invalid_request", false), canAnswer: false }; notify(); active.close(1002, "invalid student protocol"); return;
      }
      if (event.kind === "message_ack" && event.clientMessageId === pending?.clientMessageId) pending = undefined;
      const refreshConflict = event.kind === "error" && event.code === "conflict" && pending !== undefined;
      if (event.kind === "error" && pending && ["conflict", "invalid_request", "not_found", "exhausted"].includes(event.code)) { current = { ...current, unsentAnswer: pending.text }; pending = undefined; }
      if (event.cursor > current.cursor) send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "ack", cursor: event.cursor });
      current = reduceDialogueEvent(current, event, maxEvents);
      if (pending) current = { ...current, canAnswer: false };
      notify();
      if (refreshConflict) send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "subscribe", occurrenceId: options.occurrenceId, attemptId: options.attemptId, cursor: current.cursor });
    };
    active.onerror = () => { if (generation === epoch) { current = { ...current, error: safeError("unavailable", true) }; notify(); } };
    active.onclose = event => {
      if (generation !== epoch || socket !== active) return;
      socket = undefined;
      if (event.code === 1008) { pending = undefined; current = { ...current, connectionState: "revoked", canAnswer: false, error: safeError("revoked", false) }; notify(); return; }
      current = { ...current, connectionState: "offline", canAnswer: false }; notify();
      if (closed || options.reconnect === false || timer !== undefined) return;
      current = { ...current, connectionState: "reconnecting" }; notify();
      timer = schedule(() => { timer = undefined; connect(); }, Math.min(8000, 500 * 2 ** retries++));
    };
  };
  return {
    connect,
    disconnect() { closed = true; generation++; if (timer !== undefined) clear(timer); timer = undefined; const active = socket; socket = undefined; active?.close(); current = { ...current, connectionState: "offline", canAnswer: false }; notify(); },
    subscribe(listener) { listeners.add(listener); listener(current); return () => { listeners.delete(listener); }; },
    snapshot: () => current,
    sendMessage(text) {
      const binding = current.binding;
      if (!current.canAnswer || current.completed || pending || !binding?.questionId || current.connectionState !== "connected") throw new Error("Wait for the current server question before answering.");
      const command: Answer = { protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "user_message", occurrenceId: options.occurrenceId, attemptId: options.attemptId, questionId: binding.questionId, policyVersion: binding.policyVersion, snapshotDigest: binding.snapshotDigest, expectedVersion: binding.version, clientMessageId: makeId(), text };
      send(command); pending = command; current = { ...current, canAnswer: false, unsentAnswer: undefined }; notify(); return command.clientMessageId;
    },
    retry() { const state = current.state; if (!state || state.phase !== "failed" || !state.retryable || current.connectionState !== "connected") throw new Error("No retryable saved answer is available."); send({ protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: "retry", occurrenceId: options.occurrenceId, attemptId: options.attemptId, expectedVersion: state.version }); },
  };
}
