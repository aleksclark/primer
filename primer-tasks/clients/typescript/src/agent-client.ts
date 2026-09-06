import {
  AGENT_PROTOCOL_VERSION,
  type AgentCommand,
  type AgentEvent,
  type AgentMessageCommand,
  parseAgentEvent,
} from "./agent-protocol";

export type AgentConnectionState = "idle" | "connecting" | "connected" | "reconnecting" | "offline";

export interface AgentClientError {
  code: "socket" | "protocol" | "server" | "closed";
  message: string;
  retryable: boolean;
}

export interface AgentClientSnapshot {
  connectionState: AgentConnectionState;
  cursor: number;
  runId?: string;
  events: readonly AgentEvent[];
  queuedMessages: number;
  error?: AgentClientError;
}

const durableConversationID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function durableConversationKey(tenantID: string, subjectRef: string): string {
  return `primer.tasks.agent.conversation.v1:${tenantID}:${subjectRef}`;
}

/** Browser persistence for the opaque conversation handle lives in this façade. */
export function readDurableAgentConversation(tenantID: string, subjectRef: string): string | null {
  try {
    const value = window.sessionStorage.getItem(durableConversationKey(tenantID, subjectRef));
    return value && durableConversationID.test(value) ? value : null;
  } catch {
    throw new Error("Durable agent conversation storage is unavailable.");
  }
}

export function writeDurableAgentConversation(tenantID: string, subjectRef: string, conversationID: string): void {
  if (!durableConversationID.test(conversationID)) throw new Error("The server returned an invalid agent conversation.");
  try {
    window.sessionStorage.setItem(durableConversationKey(tenantID, subjectRef), conversationID);
  } catch {
    throw new Error("Durable agent conversation storage is unavailable.");
  }
}

export interface AgentClientOptions {
  /** Same-origin `/ws` is the production default. No credential belongs in a URL. */
  url?: string;
  /** Fresh in-memory Clerk credential for each upgrade; never persisted. */
  getParentToken?: () => Promise<string | null>;
  conversationId: string;
  runId?: string;
  maxEvents?: number;
  maxQueuedCommands?: number;
  reconnect?: boolean;
  reconnectBaseMs?: number;
  reconnectMaxMs?: number;
  /** Injectable only for deterministic client tests; pages use the browser constructor. */
  webSocketFactory?: (url: string) => WebSocket;
  randomId?: () => string;
  setTimeout?: typeof globalThis.setTimeout;
  clearTimeout?: typeof globalThis.clearTimeout;
}

export interface AgentClient {
  readonly conversationId: string;
  subscribe(listener: (snapshot: AgentClientSnapshot) => void): () => void;
  connect(): void;
  disconnect(): void;
  sendMessage(text: string): string;
  cancel(runId?: string): void;
  confirm(confirmationId: string, runId?: string): void;
  snapshot(): AgentClientSnapshot;
}

const SOCKET_CONNECTING = 0;
const SOCKET_OPEN = 1;
const DEFAULT_MAX_EVENTS = 400;
const DEFAULT_MAX_QUEUE = 20;
const DEFAULT_RECONNECT_BASE_MS = 500;
const DEFAULT_RECONNECT_MAX_MS = 8_000;

function defaultUrl(): string {
  const url = new URL("/ws", window.location.origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

function defaultSocket(url: string, token?: string | null): WebSocket {
  const target = new URL(url, window.location.origin);
  if (target.host !== window.location.host || target.search || target.hash || target.username || target.password || target.protocol !== (window.location.protocol === "https:" ? "wss:" : "ws:")) throw new Error("Parent sockets require the same origin.");
  const csrf = document.cookie.split(";").map((part) => part.trim()).find((part) => part.startsWith("tasks_csrf="))?.slice("tasks_csrf=".length);
  const protocols = ["primer-tasks.v1"];
  if (csrf) protocols.push(`primer-tasks.v1.csrf.${csrf}`);
  if (token) protocols.push(`primer-tasks.v1.bearer.${token}`);
  return new WebSocket(target.toString(), protocols);
}
function defaultId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}
function clientError(code: AgentClientError["code"], message: string, retryable: boolean): AgentClientError {
  return { code, message, retryable };
}

/**
 * The only browser socket construction in the product. This façade owns
 * reconnect, durable cursor replay, bounded queues, and protocol validation;
 * React pages only consume the snapshot.
 */
export function createAgentClient(options: AgentClientOptions): AgentClient {
  const maxEvents = options.maxEvents ?? DEFAULT_MAX_EVENTS;
  const maxQueuedCommands = options.maxQueuedCommands ?? DEFAULT_MAX_QUEUE;
  const reconnectEnabled = options.reconnect ?? true;
  const reconnectBaseMs = options.reconnectBaseMs ?? DEFAULT_RECONNECT_BASE_MS;
  const reconnectMaxMs = options.reconnectMaxMs ?? DEFAULT_RECONNECT_MAX_MS;
  const makeSocket = options.webSocketFactory ?? defaultSocket;
  const schedule = options.setTimeout ?? globalThis.setTimeout;
  const cancelSchedule = options.clearTimeout ?? globalThis.clearTimeout;
  const makeId = options.randomId ?? defaultId;
  const url = options.url ?? defaultUrl();
  const listeners = new Set<(snapshot: AgentClientSnapshot) => void>();
  const queued: AgentCommand[] = [];
  const unacknowledged = new Map<string, AgentMessageCommand>();
  let socket: WebSocket | undefined;
  let reconnectTimer: ReturnType<typeof globalThis.setTimeout> | undefined;
  let reconnectAttempt = 0;
  let closedByCaller = false;
  let connecting = false;
  let generation = 0;
  let current: AgentClientSnapshot = { connectionState: "idle", cursor: 0, runId: options.runId, events: [], queuedMessages: 0 };

  const notify = () => {
    current = { ...current, queuedMessages: queued.filter((command) => command.kind === "user_message").length };
    for (const listener of listeners) listener(current);
  };
  const setConnectionState = (connectionState: AgentConnectionState, nextError?: AgentClientError) => {
    current = { ...current, connectionState, error: nextError };
    notify();
  };
  const enqueue = (command: AgentCommand) => {
    if (queued.length >= maxQueuedCommands) {
      setConnectionState(current.connectionState, clientError("closed", "The agent queue is full. Try again when connected.", true));
      return false;
    }
    queued.push(command);
    notify();
    return true;
  };
  const send = (command: AgentCommand) => {
    if (socket?.readyState === SOCKET_OPEN) {
      socket.send(JSON.stringify(command));
      return true;
    }
    return enqueue(command);
  };
  const flush = () => {
    while (queued.length && socket?.readyState === SOCKET_OPEN) {
      const command = queued.shift();
      if (command) socket.send(JSON.stringify(command));
    }
    notify();
  };
  const scheduleReconnect = () => {
    if (closedByCaller || !reconnectEnabled || reconnectTimer !== undefined) {
      if (closedByCaller) setConnectionState("offline", current.error);
      return;
    }
    const delay = Math.min(reconnectMaxMs, reconnectBaseMs * 2 ** reconnectAttempt);
    reconnectAttempt += 1;
    setConnectionState("reconnecting", current.error);
    reconnectTimer = schedule(() => {
      reconnectTimer = undefined;
      connect();
    }, delay);
  };
  const handleEvent = (raw: unknown) => {
    const event = parseAgentEvent(raw);
    if (!event) {
      setConnectionState(current.connectionState, clientError("protocol", "The agent sent an invalid event.", false));
      return;
    }
    if (event.kind === "error") {
      setConnectionState(current.connectionState, clientError("server", `The agent reported ${event.code}.`, false));
    }
    if (event.kind === "user_message") unacknowledged.delete(event.clientMessageId);
    if (event.cursor > 0 && socket?.readyState === SOCKET_OPEN) socket.send(JSON.stringify({ protocol: AGENT_PROTOCOL_VERSION, kind: "ack", cursor: event.cursor }));
    if (event.runId && event.cursor > current.cursor) current = { ...current, runId: event.runId };
    if (event.cursor <= current.cursor) {
      notify();
      return;
    }
    current = { ...current, cursor: event.cursor, events: [...current.events, event].slice(-maxEvents), error: event.kind === "error" ? current.error : undefined };
    notify();
  };
  const connect = async () => {
    // A fresh explicit connect is allowed after component unmount or an
    // intentional disconnect (also keeps React StrictMode mount probes safe).
    closedByCaller = false;
    if (connecting || socket?.readyState === SOCKET_OPEN || socket?.readyState === SOCKET_CONNECTING) return;
    connecting = true;
    const attempt = ++generation;
    setConnectionState(reconnectAttempt ? "reconnecting" : "connecting", undefined);
    try {
      // Browser WS lacks custom headers. The server never echoes the bearer
      // input protocol; it negotiates only the fixed application protocol.
      const token = options.getParentToken ? await options.getParentToken() : undefined;
      if (attempt !== generation || closedByCaller) return;
      if (options.getParentToken && !token) throw new Error("Parent sign-in required.");
      socket = makeSocket(url, token);
      connecting = false;
    } catch {
      if (attempt !== generation || closedByCaller) return;
      connecting = false;
      setConnectionState("reconnecting", clientError("socket", "The agent connection could not start.", true));
      scheduleReconnect();
      return;
    }
    const openedSocket = socket;
    socket.onopen = () => {
      if (socket !== openedSocket) return;
      reconnectAttempt = 0;
      setConnectionState("connected", undefined);
      socket?.send(JSON.stringify({ protocol: AGENT_PROTOCOL_VERSION, kind: "hello", requestId: makeId() }));
      // Subscribe by conversation on every mount. A remounted page starts
      // without a run ID, but the durable conversation still owns its event
      // cursor; reconnects add the run ID as an optimization.
      socket?.send(JSON.stringify({
        protocol: AGENT_PROTOCOL_VERSION,
        kind: "subscribe",
        requestId: makeId(),
        conversationId: options.conversationId,
        ...(current.runId ? { runId: current.runId } : {}),
        ...(current.cursor ? { cursor: current.cursor } : {}),
      }));
      flush();
      for (const command of unacknowledged.values()) socket?.send(JSON.stringify(command));
    };
    socket.onmessage = (message) => {
      if (socket !== openedSocket) return;
      try {
        handleEvent(JSON.parse(String(message.data)));
      } catch {
        setConnectionState(current.connectionState, clientError("protocol", "The agent sent unreadable data.", false));
      }
    };
    socket.onerror = () => setConnectionState(current.connectionState, clientError("socket", "The agent connection encountered an error.", true));
    socket.onclose = () => {
      if (socket !== openedSocket) return;
      socket = undefined;
      if (!closedByCaller) {
        setConnectionState("reconnecting", clientError("closed", "The agent connection closed; replay will resume from the last cursor.", true));
        scheduleReconnect();
      } else setConnectionState("offline");
    };
  };
  const disconnect = () => {
    closedByCaller = true;
    generation++;
    connecting = false;
    if (reconnectTimer !== undefined) {
      cancelSchedule(reconnectTimer);
      reconnectTimer = undefined;
    }
    socket?.close(1000, "parent closed agent view");
    socket = undefined;
    setConnectionState("offline");
  };
  const sendMessage = (text: string) => {
    const clientMessageId = makeId();
    const trimmed = text.trim();
    if (!trimmed) return clientMessageId;
    const command: AgentMessageCommand = { protocol: AGENT_PROTOCOL_VERSION, kind: "user_message", requestId: makeId(), conversationId: options.conversationId, clientMessageId, text: trimmed };
    if (unacknowledged.size >= maxQueuedCommands) {
      setConnectionState(current.connectionState, clientError("closed", "The agent queue is full. Wait for server acknowledgement.", true));
      return clientMessageId;
    }
    unacknowledged.set(clientMessageId, command);
    send(command);
    return clientMessageId;
  };
  const cancel = (runId = current.runId) => {
    if (!runId) {
      setConnectionState(current.connectionState, clientError("server", "There is no active agent run to cancel.", false));
      return;
    }
    send({ protocol: AGENT_PROTOCOL_VERSION, kind: "cancel", requestId: makeId(), runId });
  };
  const confirm = (confirmationId: string, runId = current.runId) => {
    if (!runId) {
      setConnectionState(current.connectionState, clientError("server", "There is no active agent run to confirm.", false));
      return;
    }
    send({ protocol: AGENT_PROTOCOL_VERSION, kind: "confirm", requestId: makeId(), runId, confirmationId });
  };
  const client: AgentClient = {
    conversationId: options.conversationId,
    subscribe(listener) { listeners.add(listener); listener(current); return () => listeners.delete(listener); },
    connect,
    disconnect,
    sendMessage,
    cancel,
    confirm,
    snapshot: () => current,
  };
  return client;
}
