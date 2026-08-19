import {
  createAgentClient,
  type AgentClient,
  type AgentClientError,
  type AgentClientOptions,
  type AgentClientSnapshot,
  type AgentConnectionState,
} from "./agent-client";

export type DialogueConnectionState = AgentConnectionState;
export type DialogueClientError = AgentClientError;
export type DialogueClientSnapshot = AgentClientSnapshot;

export interface DialogueClientOptions extends Omit<AgentClientOptions, "conversationId"> {
  conversationId: string;
  attemptId?: string;
  occurrenceId?: string;
}

export interface DialogueClient {
  readonly conversationId: string;
  subscribe(listener: (snapshot: DialogueClientSnapshot) => void): () => void;
  connect(): void;
  disconnect(): void;
  sendMessage(text: string): string;
  snapshot(): DialogueClientSnapshot;
}

/**
 * Student dialogue transport. Pages never construct a WebSocket; this façade
 * reuses the single allowlisted agent socket constructor and never puts a
 * token, attempt, or tenant identifier in the URL.
 */
export function createDialogueClient(options: DialogueClientOptions): DialogueClient {
  const client: AgentClient = createAgentClient(options);
  return {
    conversationId: client.conversationId,
    subscribe: client.subscribe,
    connect: client.connect,
    disconnect: client.disconnect,
    sendMessage: client.sendMessage,
    snapshot: client.snapshot,
  };
}
