/**
 * TypeScript projection of primer-tasks/internal/agent/protocol.
 *
 * The Go tagged union is the wire authority: fields use its JSON names
 * (`protocol`/`kind`), and this module deliberately contains no provider or
 * domain DTOs. Generated REST types remain in generated/schema.d.ts.
 */

export const AGENT_PROTOCOL_VERSION = 1 as const;

export type AgentCommand =
  | AgentHelloCommand
  | AgentSubscribeCommand
  | AgentUnsubscribeCommand
  | AgentMessageCommand
  | AgentCancelCommand
  | AgentConfirmCommand;

export interface AgentHelloCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "hello";
  requestId?: string;
}

export interface AgentSubscribeCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "subscribe";
  requestId?: string;
  runId: string;
  cursor?: number;
}

export interface AgentUnsubscribeCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "unsubscribe";
  requestId?: string;
  runId?: string;
}

export interface AgentMessageCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "user_message";
  requestId?: string;
  conversationId: string;
  clientMessageId: string;
  text: string;
}

export interface AgentCancelCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "cancel";
  requestId?: string;
  runId: string;
}

export interface AgentConfirmCommand {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: "confirm";
  requestId?: string;
  runId: string;
  confirmationId: string;
}

export type AgentEventKind =
  | "hello"
  | "text_start"
  | "text_delta"
  | "text_end"
  | "thinking_start"
  | "thinking_end"
  | "tool_progress"
  | "retry"
  | "terminal"
  | "error"
  | "replay_gap";

export interface AgentEventBase {
  protocol: typeof AGENT_PROTOCOL_VERSION;
  kind: AgentEventKind;
  runId: string;
  sequence: number;
  cursor: number;
  time: string;
}

export type AgentEvent =
  | (AgentEventBase & { kind: "hello" })
  | (AgentEventBase & { kind: "text_start"; text?: string })
  | (AgentEventBase & { kind: "text_delta"; text: string })
  | (AgentEventBase & { kind: "text_end"; text?: string })
  | (AgentEventBase & { kind: "thinking_start" })
  | (AgentEventBase & { kind: "thinking_end" })
  | (AgentEventBase & { kind: "tool_progress"; label: string; phase: string })
  | (AgentEventBase & { kind: "retry"; retry: number; retryAfterMs: number })
  | (AgentEventBase & { kind: "terminal"; status: string; text?: string })
  | (AgentEventBase & { kind: "error"; code: string })
  | (AgentEventBase & { kind: "replay_gap"; text?: string });

export type AgentToolLabel =
  | "List students"
  | "Inspect student"
  | "List tasks"
  | "Inspect task"
  | "Draft task"
  | "Update task"
  | "Publish task"
  | "Retire task"
  | "List schedules"
  | "Create schedule"
  | "Update schedule"
  | "Disable schedule"
  | "List occurrences"
  | "Prepare change"
  | "Confirm change"
  | "Assistant action";

const TOOL_LABELS: Record<string, AgentToolLabel> = {
  "List students": "List students",
  "Inspect student": "Inspect student",
  "List tasks": "List tasks",
  "Inspect task": "Inspect task",
  "Draft task": "Draft task",
  "Update task": "Update task",
  "Publish task": "Publish task",
  "Retire task": "Retire task",
  "List schedules": "List schedules",
  "Create schedule": "Create schedule",
  "Update schedule": "Update schedule",
  "Disable schedule": "Disable schedule",
  "List occurrences": "List occurrences",
  "Prepare change": "Prepare change",
  "Confirm change": "Confirm change",
};

/** Go emits an allowlisted label; unknown labels become generic. */
export function safeAgentToolLabel(label: string): AgentToolLabel {
  return TOOL_LABELS[label] ?? "Assistant action";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function stringField(value: Record<string, unknown>, key: string): string | null {
  return typeof value[key] === "string" ? value[key] : null;
}
function numberField(value: Record<string, unknown>, key: string): number | null {
  return typeof value[key] === "number" && Number.isFinite(value[key]) ? value[key] : null;
}

/** Parse untrusted socket data; invalid frames never reach a page. */
export function parseAgentEvent(input: unknown): AgentEvent | null {
  if (!isRecord(input) || input.protocol !== AGENT_PROTOCOL_VERSION) return null;
  const kind = stringField(input, "kind") as AgentEventKind | null;
  const runId = stringField(input, "runId");
  const sequence = numberField(input, "sequence");
  const cursor = numberField(input, "cursor");
  const time = stringField(input, "time");
  if (!kind || (kind !== "hello" && !runId) || sequence === null || cursor === null || !time) return null;
  const base = { protocol: AGENT_PROTOCOL_VERSION, kind, runId: runId ?? "", sequence, cursor, time };
  switch (kind) {
    case "hello": return { ...base, kind };
    case "thinking_start":
    case "thinking_end": return { ...base, kind };
    case "text_start":
    case "text_end": return { ...base, kind, text: stringField(input, "text") ?? undefined };
    case "text_delta": {
      const text = stringField(input, "text");
      return text === null ? null : { ...base, kind, text };
    }
    case "tool_progress": {
      const label = stringField(input, "label");
      const phase = stringField(input, "phase");
      return label === null || phase === null ? null : { ...base, kind, label, phase };
    }
    case "retry": {
      const retry = numberField(input, "retry");
      const retryAfterMs = numberField(input, "retryAfterMs");
      return retry === null || retryAfterMs === null ? null : { ...base, kind, retry, retryAfterMs };
    }
    case "terminal": {
      const status = stringField(input, "status");
      return status === null ? null : { ...base, kind, status, text: stringField(input, "text") ?? undefined };
    }
    case "error": {
      const code = stringField(input, "code");
      return code === null ? null : { ...base, kind, code };
    }
    case "replay_gap": return { ...base, kind, text: stringField(input, "text") ?? undefined };
    default: return null;
  }
}
