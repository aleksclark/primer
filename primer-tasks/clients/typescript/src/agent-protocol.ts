/** Runtime validation/redaction façade. Wire types are generated from Go. */
import { AGENT_PROTOCOL_VERSION } from "../generated/agent-protocol";
import type { AgentEvent, AgentEventKind } from "../generated/agent-protocol";
export { AGENT_PROTOCOL_VERSION } from "../generated/agent-protocol";
export type { AgentCommand, AgentEvent, AgentHelloCommand, AgentSubscribeCommand, AgentUnsubscribeCommand, AgentMessageCommand, AgentCancelCommand, AgentConfirmCommand } from "../generated/agent-protocol";

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

/** Parent-facing, allowlisted error copy; never display provider errors or raw codes. */
export function safeAgentErrorMessage(code: string): string {
  if (code === "confirmation_stale") return "The task or schedule changed. Request a fresh preview. No changes from this request were applied.";
  if (code === "confirmation_rejected") return "Confirmation was not applied. Reconnect if your sign-in expired, or cancel this request and ask for a fresh preview.";
  return "The request could not be completed. Check the current task or schedule before trying again.";
}

/** Parse untrusted socket data; invalid frames never reach a page. */
export function parseAgentEvent(input: unknown): AgentEvent | null {
  if (!isRecord(input) || input.protocol !== AGENT_PROTOCOL_VERSION) return null;
  const kind = stringField(input, "kind") as AgentEventKind | null;
  const runId = stringField(input, "runId");
  const sequence = numberField(input, "sequence");
  const cursor = numberField(input, "cursor");
  const time = stringField(input, "time");
  if (!kind || (kind !== "hello" && kind !== "error" && !runId) || sequence === null || cursor === null || !time) return null;
  const base = { protocol: AGENT_PROTOCOL_VERSION, kind, runId: runId ?? "", sequence, cursor, time };
  switch (kind) {
    case "hello": return { ...base, kind };
    case "user_message": {
      const text = stringField(input, "text");
      const clientMessageId = stringField(input, "clientMessageId");
      return text === null || clientMessageId === null ? null : { ...base, kind, text, clientMessageId };
    }
    case "thinking_start":
    case "thinking_end": return { ...base, kind };
    case "text_start":
    case "text_end": return { ...base, kind, text: stringField(input, "text") ?? undefined, source: input.source === "domain" ? "domain" : undefined };
    case "text_delta": {
      const text = stringField(input, "text");
      return text === null ? null : { ...base, kind, text, source: input.source === "domain" ? "domain" : undefined };
    }
    case "tool_progress": {
      const label = stringField(input, "label");
      const phase = stringField(input, "phase");
      return label === null || phase === null ? null : { ...base, kind, label, phase, confirmationId: stringField(input, "confirmationId") ?? undefined, summary: stringField(input, "summary") ?? undefined, expiresAt: stringField(input, "expiresAt") ?? undefined };
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
