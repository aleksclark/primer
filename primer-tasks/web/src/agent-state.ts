import type { AgentEvent } from "@primer-tasks/client";

export function latestRunStatus(events: readonly AgentEvent[]): string | undefined {
  const reversed = [...events].reverse();
  const currentRun = reversed.find(event => event.kind === "user_message")?.runId
    ?? reversed.find(event => event.kind !== "error" && event.runId)?.runId;
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.runId !== currentRun) continue;
    if (event.kind === "terminal") return event.status;
    if (event.kind === "user_message") return "queued";
    if (event.kind === "tool_progress" && event.phase === "awaiting_confirmation") return "awaiting_confirmation";
    if (event.kind === "thinking_start" || event.kind === "text_start" || (event.kind === "tool_progress" && (event.phase === "started" || event.phase === "called"))) return "running";
  }
  return undefined;
}

export function assistantText(current: string, event: Extract<AgentEvent, { kind: "text_start" | "text_delta" | "text_end" }>): string {
  if (event.kind === "text_start") return event.text ?? "";
  if (event.kind === "text_delta") return current + event.text;
  return event.text || current; // authoritative replacement, never append twice
}

// A persisted terminal state makes an obsolete/canceled preview nonactionable
// after reconnect as well as live. Unrelated late errors or completed runs must
// not move the confirmation onto a different run via the client's last cursor.
export function pendingConfirmation(events: readonly AgentEvent[]) {
  const preview = [...events].reverse().find(event => event.kind === "tool_progress" && event.phase === "awaiting_confirmation" && event.confirmationId);
  if (!preview || preview.kind !== "tool_progress" || !preview.confirmationId) return undefined;
  const retired = events.some(event => event.sequence > preview.sequence && event.runId === preview.runId && (
    event.kind === "terminal" || (event.kind === "tool_progress" && event.label === "Confirm change" && event.phase === "completed")
  ));
  return retired ? undefined : preview;
}
