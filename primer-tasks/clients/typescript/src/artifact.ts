/**
 * Phase 5 artifact rubric and student evidence projections.
 *
 * These are deliberately safe UI projections. The server remains authoritative
 * for limits, authorization, digest validation, and completion decisions.
 */

import { isRecord, stripUnsafeDialogueFields } from "./dialogue.ts";

export const ARTIFACT_RUBRIC_KIND = "agent_artifact_rubric" as const;
export const ARTIFACT_RUBRIC_CONFIG_VERSION = 1 as const;
export const ARTIFACT_RUBRIC_INTERACTION = "artifact_upload" as const;
export const ARTIFACT_RUBRIC_EXECUTOR = "fantasy" as const;

export const ARTIFACT_KINDS = ["image", "video", "audio"] as const;
export type ArtifactKind = (typeof ARTIFACT_KINDS)[number];
export type ArtifactReviewPolicy = "reject" | "parent_review";
export type ArtifactPassRule = "all_required";

export const ARTIFACT_LIMITS = {
  image: { maxBytes: 25 * 1024 * 1024, maxDurationSeconds: 0, maxPixels: 40_000_000 },
  video: { maxBytes: 100 * 1024 * 1024, maxDurationSeconds: 15 * 60, maxPixels: 40_000_000 },
  audio: { maxBytes: 50 * 1024 * 1024, maxDurationSeconds: 30 * 60, maxPixels: 0 },
} as const;

export type ArtifactRubricCriterion = {
  id: string;
  label: string;
  description: string;
  required: boolean;
};

export type ArtifactRubricConfig = {
  acceptedKinds: ArtifactKind[];
  maxBytes: number;
  maxDurationSeconds?: number;
  maxPixels?: number;
  criteria: ArtifactRubricCriterion[];
  passRule: ArtifactPassRule;
  reviewPolicy: ArtifactReviewPolicy;
  feedbackStyle: "age_appropriate" | "concise";
};

export type ArtifactRubricIssue = {
  field: string;
  message: string;
};

export type ArtifactRubricPreview = {
  kind: typeof ARTIFACT_RUBRIC_KIND;
  configVersion: typeof ARTIFACT_RUBRIC_CONFIG_VERSION;
  acceptedKinds: ArtifactKind[];
  limits: string;
  requiredCriteria: number;
  criteria: ArtifactRubricCriterion[];
  reviewPolicy: ArtifactReviewPolicy;
  hiddenPromptExposed: false;
};

export type ArtifactRubricRequirement = {
  id: string;
  kind: typeof ARTIFACT_RUBRIC_KIND;
  configVersion: typeof ARTIFACT_RUBRIC_CONFIG_VERSION;
  config: ArtifactRubricConfig;
  interaction: typeof ARTIFACT_RUBRIC_INTERACTION;
  executor: typeof ARTIFACT_RUBRIC_EXECUTOR;
};

export type ArtifactSubmissionStatus = "reserved" | "uploading" | "validating" | "evaluating" | "rejected" | "review" | "complete";
export type ArtifactEvaluationStatus = "queued" | "loading" | "evaluating" | "rejected" | "review" | "complete" | "error";

export type ArtifactCriterionEvaluation = {
  id: string;
  criterionId: string;
  required: boolean;
  status: "accepted" | "rejected" | "unavailable" | "pending";
  evidence?: string;
  feedback?: string;
};

export type ArtifactEvaluation = {
  status: ArtifactEvaluationStatus;
  accepted: boolean;
  nextStep?: string;
  provider?: string;
  model?: string;
  policyVersion?: string;
  artifactDigest?: string;
  rubricRevision?: string;
  criteria: ArtifactCriterionEvaluation[];
};

export type ArtifactSubmission = {
  id: string;
  artifactId: string;
  kind: ArtifactKind;
  filename?: string;
  mediaType: string;
  sizeBytes: number;
  digest?: string;
  status: ArtifactSubmissionStatus;
  createdAt: string;
  evaluation?: ArtifactEvaluation;
};

export type ArtifactStudentState = {
  occurrenceId: string;
  requirementId?: string;
  rubricRevision?: string;
  config: ArtifactRubricConfig;
  status: ArtifactEvaluationStatus | "ready";
  submissions: ArtifactSubmission[];
  activeSubmissionId?: string;
  evaluation?: ArtifactEvaluation;
};

export type ArtifactUploadReservation = {
  artifactId: string;
  idempotencyKey: string;
  occurrenceId: string;
  uploadUrl: string;
  expiresAt: string;
  kind: ArtifactKind;
  mediaType: string;
  maxBytes: number;
  expectedDigest?: string;
};

export type ArtifactFinalizeInput = {
  artifactId: string;
  idempotencyKey?: string;
  durationMs?: number;
  digest: string;
  sizeBytes: number;
  mediaType: string;
  filename?: string;
};

export type ArtifactProgressEvent = {
  protocol: 1;
  kind: "hello" | "progress" | "criterion" | "complete" | "error";
  cursor: number;
  sequence: number;
  time?: string;
  phase?: "queued" | "loading" | "evaluating" | "review";
  criterionId?: string;
  status?: ArtifactCriterionEvaluation["status"];
  evidence?: string;
  feedback?: string;
  message?: string;
  code?: string;
  retryable?: boolean;
};

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value : undefined;
}
function integerValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isInteger(value) ? value : fallback;
}
function kindValue(value: unknown): ArtifactKind | undefined {
  return typeof value === "string" && (ARTIFACT_KINDS as readonly string[]).includes(value) ? value as ArtifactKind : undefined;
}

export function defaultArtifactRubricConfig(): ArtifactRubricConfig {
  return {
    acceptedKinds: ["image"],
    maxBytes: ARTIFACT_LIMITS.image.maxBytes,
    maxDurationSeconds: undefined,
    maxPixels: ARTIFACT_LIMITS.image.maxPixels,
    criteria: [
      { id: "criterion-1", label: "Required criterion", description: "Describe what the submitted work must show.", required: true },
    ],
    passRule: "all_required",
    reviewPolicy: "parent_review",
    feedbackStyle: "age_appropriate",
  };
}

export function normalizeArtifactRubricConfig(input: unknown): ArtifactRubricConfig {
  const record = isRecord(input) ? input : {};
  const acceptedKinds: ArtifactKind[] = Array.isArray(record.acceptedKinds) ? record.acceptedKinds.flatMap((value) => {
    const kind = kindValue(value);
    return kind ? [kind] : [];
  }) : ["image"];
  const criteria = Array.isArray(record.criteria) ? record.criteria.flatMap((value, index) => {
    if (!isRecord(value)) return [];
    const label = stringValue(value.label) ?? `Criterion ${index + 1}`;
    const description = stringValue(value.description) ?? "";
    return [{ id: stringValue(value.id) ?? `criterion-${index + 1}`, label, description, required: value.required !== false }];
  }) : [];
  const policy = record.reviewPolicy === "reject" ? "reject" : "parent_review";
  return {
    acceptedKinds: acceptedKinds.length ? [...new Set(acceptedKinds)] : ["image"],
    maxBytes: integerValue(record.maxBytes, ARTIFACT_LIMITS.image.maxBytes),
    maxDurationSeconds: record.maxDurationSeconds === undefined ? undefined : integerValue(record.maxDurationSeconds, 0),
    maxPixels: record.maxPixels === undefined ? undefined : integerValue(record.maxPixels, 0),
    criteria,
    passRule: "all_required",
    reviewPolicy: policy,
    feedbackStyle: record.feedbackStyle === "concise" ? "concise" : "age_appropriate",
  };
}

export function validateArtifactRubric(input: unknown): ArtifactRubricIssue[] {
  const config = normalizeArtifactRubricConfig(input);
  const issues: ArtifactRubricIssue[] = [];
  if (!config.acceptedKinds.length) issues.push({ field: "acceptedKinds", message: "Choose at least one allowed media kind." });
  if (config.maxBytes < 1 || config.maxBytes > 500 * 1024 * 1024) issues.push({ field: "maxBytes", message: "The upload limit must be between 1 byte and 500 MB." });
  if (config.maxDurationSeconds !== undefined && (config.maxDurationSeconds < 1 || config.maxDurationSeconds > 2 * 60 * 60)) issues.push({ field: "maxDurationSeconds", message: "The duration limit must be between 1 second and 2 hours." });
  if (config.maxPixels !== undefined && (config.maxPixels < 1 || config.maxPixels > 100_000_000)) issues.push({ field: "maxPixels", message: "The pixel limit must be between 1 and 100 million pixels." });
  if (!config.criteria.length) issues.push({ field: "criteria", message: "Add at least one rubric criterion." });
  const ids = new Set<string>();
  config.criteria.forEach((criterion, index) => {
    if (!/^[a-z0-9][a-z0-9_-]{1,63}$/i.test(criterion.id)) issues.push({ field: `criteria.${index}.id`, message: "Criterion IDs use letters, numbers, hyphens, or underscores." });
    if (ids.has(criterion.id)) issues.push({ field: `criteria.${index}.id`, message: "Criterion IDs must be unique." });
    ids.add(criterion.id);
    if (!criterion.label.trim()) issues.push({ field: `criteria.${index}.label`, message: "Criterion label is required." });
    if (!criterion.description.trim()) issues.push({ field: `criteria.${index}.description`, message: "Criterion description is required." });
  });
  return issues;
}

export function previewArtifactRubric(input: unknown): ArtifactRubricPreview | null {
  const config = normalizeArtifactRubricConfig(input);
  if (validateArtifactRubric(config).length) return null;
  const names = config.acceptedKinds.join(", ");
  const limit = `${Math.round(config.maxBytes / (1024 * 1024))} MB max${config.maxDurationSeconds ? ` · ${config.maxDurationSeconds}s max` : ""}`;
  return { kind: ARTIFACT_RUBRIC_KIND, configVersion: ARTIFACT_RUBRIC_CONFIG_VERSION, acceptedKinds: config.acceptedKinds, limits: `${names} · ${limit}`, requiredCriteria: config.criteria.filter((criterion) => criterion.required).length, criteria: config.criteria, reviewPolicy: config.reviewPolicy, hiddenPromptExposed: false };
}

export function artifactRubricRequirement(id: string, input: unknown): ArtifactRubricRequirement {
  const issues = validateArtifactRubric(input);
  if (issues.length) throw new Error(issues[0]!.message);
  return { id, kind: ARTIFACT_RUBRIC_KIND, configVersion: ARTIFACT_RUBRIC_CONFIG_VERSION, config: normalizeArtifactRubricConfig(input), interaction: ARTIFACT_RUBRIC_INTERACTION, executor: ARTIFACT_RUBRIC_EXECUTOR };
}

export function validateArtifactFile(file: Pick<File, "name" | "type" | "size">, config: ArtifactRubricConfig): string | null {
  const kind = mediaKind(file.type);
  if (!kind || !config.acceptedKinds.includes(kind)) return "This file type is not allowed for this task.";
  if (file.size < 1) return "Choose a file with content.";
  if (file.size > config.maxBytes) return `This file is larger than the ${Math.round(config.maxBytes / (1024 * 1024))} MB task limit.`;
  return null;
}

export function mediaKind(mediaType: string): ArtifactKind | undefined {
  if (mediaType.startsWith("image/")) return "image";
  if (mediaType.startsWith("video/")) return "video";
  if (mediaType.startsWith("audio/")) return "audio";
  return undefined;
}

const SHA256_K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);
const SHA256_INITIAL = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]);
function rotateRight(value: number, bits: number): number { return (value >>> bits) | (value << (32 - bits)); }

/** Browser-safe SHA-256 fallback for non-secure Stacklane HTTP origins. */
export function sha256HexFallback(bytes: Uint8Array): string {
  const bitLength = bytes.length * 8;
  const paddedLength = Math.ceil((bytes.length + 9) / 64) * 64;
  const padded = new Uint8Array(paddedLength);
  padded.set(bytes);
  padded[bytes.length] = 0x80;
  const view = new DataView(padded.buffer);
  view.setUint32(paddedLength - 4, bitLength >>> 0);
  view.setUint32(paddedLength - 8, Math.floor(bitLength / 0x100000000));
  const hash = new Uint32Array(SHA256_INITIAL);
  const schedule = new Uint32Array(64);
  for (let offset = 0; offset < paddedLength; offset += 64) {
    for (let i = 0; i < 16; i++) schedule[i] = view.getUint32(offset + i * 4);
    for (let i = 16; i < 64; i++) {
      const a = schedule[i - 15];
      const b = schedule[i - 2];
      const smallSigma0 = rotateRight(a, 7) ^ rotateRight(a, 18) ^ (a >>> 3);
      const smallSigma1 = rotateRight(b, 17) ^ rotateRight(b, 19) ^ (b >>> 10);
      schedule[i] = (schedule[i - 16] + smallSigma0 + schedule[i - 7] + smallSigma1) >>> 0;
    }
    let [a, b, c, d, e, f, g, h] = hash;
    for (let i = 0; i < 64; i++) {
      const bigSigma1 = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25);
      const choose = (e & f) ^ (~e & g);
      const temp1 = (h + bigSigma1 + choose + SHA256_K[i] + schedule[i]) >>> 0;
      const bigSigma0 = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22);
      const majority = (a & b) ^ (a & c) ^ (b & c);
      const temp2 = (bigSigma0 + majority) >>> 0;
      h = g; g = f; f = e; e = (d + temp1) >>> 0;
      d = c; c = b; b = a; a = (temp1 + temp2) >>> 0;
    }
    hash[0] = (hash[0] + a) >>> 0; hash[1] = (hash[1] + b) >>> 0;
    hash[2] = (hash[2] + c) >>> 0; hash[3] = (hash[3] + d) >>> 0;
    hash[4] = (hash[4] + e) >>> 0; hash[5] = (hash[5] + f) >>> 0;
    hash[6] = (hash[6] + g) >>> 0; hash[7] = (hash[7] + h) >>> 0;
  }
  return [...hash].map((word) => word.toString(16).padStart(8, "0")).join("");
}

let fallbackIdCounter = 0;
export function newArtifactIdempotencyKey(): string {
  const bytes = new Uint8Array(16);
  const randomValues = globalThis.crypto?.getRandomValues;
  if (randomValues) {
    randomValues.call(globalThis.crypto, bytes);
  } else {
    // This is an idempotency key, not an authentication credential. Preserve
    // uniqueness on old HTTP origins without pretending Math.random is secure.
    const seed = `${Date.now()}-${fallbackIdCounter++}-${Math.random()}`;
    for (let i = 0; i < bytes.length; i++) bytes[i] = seed.charCodeAt(i % seed.length) & 0xff;
  }
  return `artifact-${[...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

export async function sha256Hex(value: Blob): Promise<string> {
  const bytes = new Uint8Array(await value.arrayBuffer());
  const subtle = globalThis.crypto?.subtle;
  if (subtle) {
    const digest = await subtle.digest("SHA-256", bytes);
    return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
  }
  return sha256HexFallback(bytes);
}

export function parseArtifactStudentState(input: unknown): ArtifactStudentState | null {
  const value = stripUnsafeDialogueFields(input);
  if (!isRecord(value) || typeof value.occurrenceId !== "string") return null;
  const config = normalizeArtifactRubricConfig(value.config);
  const submissions = Array.isArray(value.submissions) ? value.submissions.flatMap((item) => parseArtifactSubmission(item) ?? []) : [];
  const evaluation = parseArtifactEvaluation(value.evaluation);
  const status = typeof value.status === "string" ? value.status as ArtifactStudentState["status"] : "ready";
  return { occurrenceId: value.occurrenceId, requirementId: stringValue(value.requirementId), rubricRevision: stringValue(value.rubricRevision), config, status, submissions, activeSubmissionId: stringValue(value.activeSubmissionId), evaluation: evaluation ?? undefined };
}

function parseArtifactSubmission(input: unknown): ArtifactSubmission | null {
  if (!isRecord(input) || typeof input.id !== "string" || typeof input.artifactId !== "string" || typeof input.mediaType !== "string" || typeof input.status !== "string") return null;
  const kind = kindValue(input.kind) ?? mediaKind(input.mediaType);
  if (!kind || typeof input.sizeBytes !== "number" || typeof input.createdAt !== "string") return null;
  const evaluation = parseArtifactEvaluation(input.evaluation);
  return { id: input.id, artifactId: input.artifactId, kind, filename: stringValue(input.filename), mediaType: input.mediaType, sizeBytes: input.sizeBytes, digest: stringValue(input.digest), status: input.status as ArtifactSubmissionStatus, createdAt: input.createdAt, evaluation: evaluation ?? undefined };
}

function parseArtifactEvaluation(input: unknown): ArtifactEvaluation | null {
  if (!isRecord(input) || typeof input.status !== "string") return null;
  const criteria = Array.isArray(input.criteria) ? input.criteria.flatMap((item) => {
    if (!isRecord(item) || typeof item.id !== "string" || typeof item.criterionId !== "string" || typeof item.status !== "string") return [];
    return [{ id: item.id, criterionId: item.criterionId, required: item.required !== false, status: item.status as ArtifactCriterionEvaluation["status"], evidence: stringValue(item.evidence), feedback: stringValue(item.feedback) }];
  }) : [];
  return { status: input.status as ArtifactEvaluationStatus, accepted: input.accepted === true, nextStep: stringValue(input.nextStep), provider: stringValue(input.provider), model: stringValue(input.model), policyVersion: stringValue(input.policyVersion), artifactDigest: stringValue(input.artifactDigest), rubricRevision: stringValue(input.rubricRevision), criteria };
}

export function parseArtifactProgressEvent(input: unknown): ArtifactProgressEvent | null {
  const value = stripUnsafeDialogueFields(input);
  if (!isRecord(value) || value.protocol !== 1 || typeof value.kind !== "string" || typeof value.cursor !== "number" || typeof value.sequence !== "number") return null;
  if (!["hello", "progress", "criterion", "complete", "error"].includes(value.kind)) return null;
  return { protocol: 1, kind: value.kind as ArtifactProgressEvent["kind"], cursor: value.cursor, sequence: value.sequence, time: stringValue(value.time), phase: ["queued", "loading", "evaluating", "review"].includes(String(value.phase)) ? value.phase as ArtifactProgressEvent["phase"] : undefined, criterionId: stringValue(value.criterionId), status: ["accepted", "rejected", "unavailable", "pending"].includes(String(value.status)) ? value.status as ArtifactCriterionEvaluation["status"] : undefined, evidence: stringValue(value.evidence), feedback: stringValue(value.feedback), message: stringValue(value.message), code: stringValue(value.code), retryable: value.retryable === true };
}
