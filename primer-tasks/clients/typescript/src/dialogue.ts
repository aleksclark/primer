/**
 * Parent-authored agent_dialogue configuration and inspect DTOs.
 *
 * These types are the web-facing contract the Huma handlers should emit.
 * Pages never see hidden prompts, raw reasoning, or client-owned completion.
 */

export const AGENT_DIALOGUE_KIND = "agent_dialogue" as const;
export const AGENT_DIALOGUE_CONFIG_VERSION = 1 as const;
export const AGENT_DIALOGUE_INTERACTION = "chat" as const;
export const AGENT_DIALOGUE_EXECUTOR = "fantasy" as const;

export const DIALOGUE_SOURCE_MAX = 8_000;
export const DIALOGUE_FOCUS_MAX = 500;
export const DIALOGUE_RUBRIC_MAX = 4_000;
export const DIALOGUE_QUESTION_MIN = 1;
export const DIALOGUE_QUESTION_MAX = 8;
export const DIALOGUE_FOLLOWUP_MAX = 6;
export const DIALOGUE_TURN_MAX = 40;
export const DIALOGUE_ATTEMPT_MAX = 8;
export const DIALOGUE_RETENTION_MAX_DAYS = 365;

const UNSAFE_KEYS = [
  "reasoning",
  "reasoningDelta",
  "reasoning_delta",
  "chainOfThought",
  "chain_of_thought",
  "hiddenPrompt",
  "systemPrompt",
  "rawPrompt",
  "provider_metadata",
  "providerMetadata",
  "rawScore",
  "score",
] as const;

export type DialogueConfig = {
  source: string;
  learningFocus: string;
  requiredAcceptedQuestions: number;
  rubric: string;
  allowedFollowUps: number;
  maxTurns: number;
  maxAttempts: number;
  retentionDays: number;
};

export type DialogueConfigIssue = {
  field: keyof DialogueConfig | "config";
  message: string;
};

export type DialogueConfigPreview = {
  kind: typeof AGENT_DIALOGUE_KIND;
  configVersion: typeof AGENT_DIALOGUE_CONFIG_VERSION;
  title: string;
  summary: string;
  sourceExcerpt: string;
  requiredAcceptedQuestions: number;
  allowedFollowUps: number;
  maxTurns: number;
  maxAttempts: number;
  retentionDays: number;
  hiddenPromptExposed: false;
};

export type DialogueRequirement = {
  id: string;
  kind: typeof AGENT_DIALOGUE_KIND;
  configVersion: typeof AGENT_DIALOGUE_CONFIG_VERSION;
  config: DialogueConfig;
  interaction: typeof AGENT_DIALOGUE_INTERACTION;
  executor: typeof AGENT_DIALOGUE_EXECUTOR;
};

export type DialogueUsage = {
  inputTokens?: number;
  outputTokens?: number;
};

export type InspectEntryKind = "question" | "student_answer" | "evaluation" | "override" | "status";
export type InspectAuthor = "student" | "agent" | "parent" | "system";
export type InspectOutcome = "accepted" | "rejected";

export type InspectEntry = {
  id: string;
  kind: InspectEntryKind;
  at: string;
  author: InspectAuthor;
  title: string;
  body: string;
  outcome?: InspectOutcome;
  rationale?: string;
  provider?: string;
  model?: string;
  policyVersion?: string;
  usage?: DialogueUsage;
};

export type OverrideRecord = {
  id: string;
  accepted: boolean;
  reason: string;
  actorId: string;
  createdAt: string;
};

export type OverrideInput = {
  accepted: boolean;
  reason: string;
};

export type InspectTimeline = {
  occurrenceId: string;
  attemptId?: string;
  requirementId?: string;
  status: string;
  acceptedCount: number;
  requiredCount: number;
  provider?: string;
  policyVersion?: string;
  entries: InspectEntry[];
  overrides: OverrideRecord[];
};

export type StudentDialogueStatus = "ready" | "in_progress" | "retry" | "error" | "offline" | "complete";

export type StudentDialogueState = {
  occurrenceId: string;
  attemptId: string;
  conversationId: string;
  status: StudentDialogueStatus;
  acceptedCount: number;
  requiredCount: number;
  currentQuestion?: string;
  completionSummary?: string;
  retryExplanation?: string;
  errorExplanation?: string;
};

export const defaultDialogueConfig = (): DialogueConfig => ({
  source: "",
  learningFocus: "",
  requiredAcceptedQuestions: 3,
  rubric: "",
  allowedFollowUps: 2,
  maxTurns: 12,
  maxAttempts: 3,
  retentionDays: 30,
});

function clipped(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function integer(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isInteger(value) ? value : fallback;
}

export function normalizeDialogueConfig(input: unknown): DialogueConfig {
  const record = isRecord(input) ? input : {};
  return {
    source: clipped(record.source).trim(),
    learningFocus: clipped(record.learningFocus).trim(),
    requiredAcceptedQuestions: integer(record.requiredAcceptedQuestions, 3),
    rubric: clipped(record.rubric).trim(),
    allowedFollowUps: integer(record.allowedFollowUps, 2),
    maxTurns: integer(record.maxTurns, 12),
    maxAttempts: integer(record.maxAttempts, 3),
    retentionDays: integer(record.retentionDays, 30),
  };
}

export function validateDialogueConfig(input: unknown): DialogueConfigIssue[] {
  const config = normalizeDialogueConfig(input);
  const issues: DialogueConfigIssue[] = [];
  if (!config.source) issues.push({ field: "source", message: "Parent-authored source is required." });
  if (config.source.length > DIALOGUE_SOURCE_MAX) issues.push({ field: "source", message: `Source must be at most ${DIALOGUE_SOURCE_MAX} characters.` });
  if (!config.learningFocus) issues.push({ field: "learningFocus", message: "Learning focus is required." });
  if (config.learningFocus.length > DIALOGUE_FOCUS_MAX) issues.push({ field: "learningFocus", message: `Learning focus must be at most ${DIALOGUE_FOCUS_MAX} characters.` });
  if (!config.rubric) issues.push({ field: "rubric", message: "Acceptance rubric is required." });
  if (config.rubric.length > DIALOGUE_RUBRIC_MAX) issues.push({ field: "rubric", message: `Rubric must be at most ${DIALOGUE_RUBRIC_MAX} characters.` });
  if (config.requiredAcceptedQuestions < DIALOGUE_QUESTION_MIN || config.requiredAcceptedQuestions > DIALOGUE_QUESTION_MAX) {
    issues.push({ field: "requiredAcceptedQuestions", message: `Required accepted questions must be between ${DIALOGUE_QUESTION_MIN} and ${DIALOGUE_QUESTION_MAX}.` });
  }
  if (config.allowedFollowUps < 0 || config.allowedFollowUps > DIALOGUE_FOLLOWUP_MAX) {
    issues.push({ field: "allowedFollowUps", message: `Allowed follow-ups must be between 0 and ${DIALOGUE_FOLLOWUP_MAX}.` });
  }
  if (config.maxTurns < config.requiredAcceptedQuestions || config.maxTurns > DIALOGUE_TURN_MAX) {
    issues.push({ field: "maxTurns", message: `Max turns must be between the required question count and ${DIALOGUE_TURN_MAX}.` });
  }
  if (config.maxAttempts < 1 || config.maxAttempts > DIALOGUE_ATTEMPT_MAX) {
    issues.push({ field: "maxAttempts", message: `Max attempts must be between 1 and ${DIALOGUE_ATTEMPT_MAX}.` });
  }
  if (config.retentionDays < 1 || config.retentionDays > DIALOGUE_RETENTION_MAX_DAYS) {
    issues.push({ field: "retentionDays", message: `Retention must be between 1 and ${DIALOGUE_RETENTION_MAX_DAYS} days.` });
  }
  return issues;
}

export function previewDialogueConfig(input: unknown): DialogueConfigPreview | null {
  const config = normalizeDialogueConfig(input);
  if (validateDialogueConfig(config).length) return null;
  const excerpt = config.source.length > 240 ? `${config.source.slice(0, 237).trimEnd()}…` : config.source;
  return {
    kind: AGENT_DIALOGUE_KIND,
    configVersion: AGENT_DIALOGUE_CONFIG_VERSION,
    title: "Dialogue verification",
    summary: `The student must give ${config.requiredAcceptedQuestions} accepted answers about the parent-authored source. ${config.allowedFollowUps} follow-up${config.allowedFollowUps === 1 ? "" : "s"} and ${config.maxAttempts} attempt${config.maxAttempts === 1 ? "" : "s"} are allowed.`,
    sourceExcerpt: excerpt,
    requiredAcceptedQuestions: config.requiredAcceptedQuestions,
    allowedFollowUps: config.allowedFollowUps,
    maxTurns: config.maxTurns,
    maxAttempts: config.maxAttempts,
    retentionDays: config.retentionDays,
    hiddenPromptExposed: false,
  };
}

export function dialogueRequirement(id: string, input: unknown): DialogueRequirement {
  const issues = validateDialogueConfig(input);
  if (issues.length) throw new Error(issues[0]?.message ?? "Dialogue configuration is invalid.");
  const config = normalizeDialogueConfig(input);
  // The UI model is intentionally friendly; the wire uses the server-owned
  // manifest names. No client field can provide policy beyond this mapping.
  const wireConfig = {
    sourceText: config.source,
    learningFocus: config.learningFocus,
    requiredQuestions: config.requiredAcceptedQuestions,
    rubric: [config.rubric],
    allowedFollowUps: config.allowedFollowUps,
    maxAttempts: config.maxAttempts,
    maxTurns: config.maxTurns,
    retentionPolicy: "retain",
  } as unknown as DialogueConfig;
  return {
    id,
    kind: AGENT_DIALOGUE_KIND,
    configVersion: AGENT_DIALOGUE_CONFIG_VERSION,
    config: wireConfig,
    interaction: AGENT_DIALOGUE_INTERACTION,
    executor: AGENT_DIALOGUE_EXECUTOR,
  };
}

export function validateOverrideInput(input: unknown): string | null {
  if (!isRecord(input) || typeof input.accepted !== "boolean") return "Override must accept or reject without rewriting evidence.";
  const reason = clipped(input.reason).trim();
  if (reason.length < 8) return "Override reason must be at least 8 characters.";
  if (reason.length > 1_000) return "Override reason must be at most 1000 characters.";
  return null;
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasUnsafeKey(key: string): boolean {
  return (UNSAFE_KEYS as readonly string[]).includes(key);
}

/** Drop hidden reasoning, prompts, and gamified scores before UI mapping. */
export function stripUnsafeDialogueFields<T>(value: T): T {
  if (Array.isArray(value)) return value.map((item) => stripUnsafeDialogueFields(item)) as T;
  if (!isRecord(value)) return value;
  const next: Record<string, unknown> = {};
  for (const [key, nested] of Object.entries(value)) {
    if (hasUnsafeKey(key)) continue;
    next[key] = stripUnsafeDialogueFields(nested);
  }
  return next as T;
}

export function parseInspectTimeline(input: unknown): InspectTimeline | null {
  const value = stripUnsafeDialogueFields(input);
  if (!isRecord(value) || typeof value.occurrenceId !== "string" || typeof value.status !== "string") return null;
  const entries = Array.isArray(value.entries) ? value.entries.flatMap((entry) => parseInspectEntry(entry) ?? []) : [];
  const overrides = Array.isArray(value.overrides) ? value.overrides.flatMap((row) => parseOverride(row) ?? []) : [];
  return {
    occurrenceId: value.occurrenceId,
    attemptId: typeof value.attemptId === "string" ? value.attemptId : undefined,
    requirementId: typeof value.requirementId === "string" ? value.requirementId : undefined,
    status: value.status,
    acceptedCount: integer(value.acceptedCount, 0),
    requiredCount: integer(value.requiredCount, 0),
    provider: typeof value.provider === "string" ? value.provider : undefined,
    policyVersion: typeof value.policyVersion === "string" ? value.policyVersion : undefined,
    entries,
    overrides,
  };
}

function parseInspectEntry(input: unknown): InspectEntry | null {
  if (!isRecord(input) || typeof input.id !== "string" || typeof input.kind !== "string" || typeof input.at !== "string") return null;
  if (input.kind !== "question" && input.kind !== "student_answer" && input.kind !== "evaluation" && input.kind !== "override" && input.kind !== "status") return null;
  if (input.author !== "student" && input.author !== "agent" && input.author !== "parent" && input.author !== "system") return null;
  return {
    id: input.id,
    kind: input.kind,
    at: input.at,
    author: input.author,
    title: clipped(input.title),
    body: clipped(input.body),
    outcome: input.outcome === "accepted" || input.outcome === "rejected" ? input.outcome : undefined,
    rationale: typeof input.rationale === "string" ? input.rationale : undefined,
    provider: typeof input.provider === "string" ? input.provider : undefined,
    model: typeof input.model === "string" ? input.model : undefined,
    policyVersion: typeof input.policyVersion === "string" ? input.policyVersion : undefined,
    usage: parseUsage(input.usage),
  };
}

function parseOverride(input: unknown): OverrideRecord | null {
  if (!isRecord(input) || typeof input.id !== "string" || typeof input.reason !== "string" || typeof input.actorId !== "string" || typeof input.createdAt !== "string") return null;
  if (typeof input.accepted !== "boolean") return null;
  return { id: input.id, accepted: input.accepted, reason: input.reason, actorId: input.actorId, createdAt: input.createdAt };
}

function parseUsage(input: unknown): DialogueUsage | undefined {
  if (!isRecord(input)) return undefined;
  const usage: DialogueUsage = {};
  if (typeof input.inputTokens === "number") usage.inputTokens = input.inputTokens;
  if (typeof input.outputTokens === "number") usage.outputTokens = input.outputTokens;
  return usage.inputTokens === undefined && usage.outputTokens === undefined ? undefined : usage;
}

export function parseStudentDialogueState(input: unknown): StudentDialogueState | null {
  const value = stripUnsafeDialogueFields(input);
  if (!isRecord(value)) return null;
  if (typeof value.occurrenceId !== "string" || typeof value.attemptId !== "string" || typeof value.conversationId !== "string") return null;
  if (value.status !== "ready" && value.status !== "in_progress" && value.status !== "retry" && value.status !== "error" && value.status !== "offline" && value.status !== "complete") return null;
  return {
    occurrenceId: value.occurrenceId,
    attemptId: value.attemptId,
    conversationId: value.conversationId,
    status: value.status,
    acceptedCount: integer(value.acceptedCount, 0),
    requiredCount: integer(value.requiredCount, 0),
    currentQuestion: typeof value.currentQuestion === "string" ? value.currentQuestion : undefined,
    completionSummary: typeof value.completionSummary === "string" ? value.completionSummary : undefined,
    retryExplanation: typeof value.retryExplanation === "string" ? value.retryExplanation : undefined,
    errorExplanation: typeof value.errorExplanation === "string" ? value.errorExplanation : undefined,
  };
}

export function studentProgressCopy(state: Pick<StudentDialogueState, "acceptedCount" | "requiredCount" | "status">): string {
  if (state.status === "complete") return "Verification complete.";
  if (state.requiredCount <= 0) return "Waiting for the next question.";
  return `${state.acceptedCount} of ${state.requiredCount} accepted answers`;
}
