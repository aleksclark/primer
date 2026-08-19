export const STUDENT_DIALOGUE_PROTOCOL_VERSION = 1 as const;

export type StudentDialogueCommand =
  | { protocol: typeof STUDENT_DIALOGUE_PROTOCOL_VERSION; kind: "subscribe"; attemptId?: string; occurrenceId?: string; cursor?: number }
  | { protocol: typeof STUDENT_DIALOGUE_PROTOCOL_VERSION; kind: "user_message"; attemptId: string; occurrenceId: string; clientMessageId: string; text: string; expectedSequence: number }
  | { protocol: typeof STUDENT_DIALOGUE_PROTOCOL_VERSION; kind: "retry"; attemptId: string; occurrenceId: string };

export type StudentDialogueEvent = {
  protocol: typeof STUDENT_DIALOGUE_PROTOCOL_VERSION;
  kind: "hello" | "state" | "message_ack" | "progress" | "question" | "answer_evaluation" | "complete" | "error";
  attemptId?: string;
  occurrenceId?: string;
  sequence: number;
  cursor: number;
  messageId?: string;
  clientMessageId?: string;
  text?: string;
  role?: "student" | "agent";
  phase?: "evaluating" | "retry" | "complete";
  status?: string;
  code?: string;
  message?: string;
  retryable?: boolean;
  acceptedCount?: number;
  requiredCount?: number;
  questionKey?: string;
  time?: string;
};

function record(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null; }
function stringValue(value: Record<string, unknown>, key: string): string | undefined { return typeof value[key] === "string" ? value[key] : undefined; }
function numberValue(value: Record<string, unknown>, key: string): number | undefined { return typeof value[key] === "number" && Number.isFinite(value[key]) ? value[key] : undefined; }

/** Parse untrusted student socket frames; hidden reasoning/provider payloads are never accepted. */
export function parseStudentDialogueEvent(value: unknown): StudentDialogueEvent | null {
  if (!record(value) || value.protocol !== STUDENT_DIALOGUE_PROTOCOL_VERSION) return null;
  const kind = stringValue(value, "kind");
  const allowed = ["hello", "state", "message_ack", "progress", "question", "answer_evaluation", "complete", "error"];
  if (!kind || !allowed.includes(kind)) return null;
  const sequence = numberValue(value, "sequence");
  const cursor = numberValue(value, "cursor");
  if (sequence === undefined || cursor === undefined) return null;
  const phase = stringValue(value, "phase");
  return { protocol: STUDENT_DIALOGUE_PROTOCOL_VERSION, kind: kind as StudentDialogueEvent["kind"], sequence, cursor,
    attemptId: stringValue(value, "attemptId"), occurrenceId: stringValue(value, "occurrenceId"), messageId: stringValue(value, "messageId"),
    clientMessageId: stringValue(value, "clientMessageId"), text: stringValue(value, "text"), role: stringValue(value, "role") as StudentDialogueEvent["role"],
    phase: phase as StudentDialogueEvent["phase"], status: stringValue(value, "status"), code: stringValue(value, "code"), message: stringValue(value, "message"),
    retryable: typeof value.retryable === "boolean" ? value.retryable : undefined, acceptedCount: numberValue(value, "acceptedCount"),
    requiredCount: numberValue(value, "requiredCount"), questionKey: stringValue(value, "questionKey"), time: stringValue(value, "time") };
}
