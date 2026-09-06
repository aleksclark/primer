import { STUDENT_DIALOGUE_SCHEMA } from "../generated/student-dialogue.ts";
import type { StudentDialogueCommand, StudentDialogueEvent } from "../generated/student-dialogue.ts";

export { STUDENT_DIALOGUE_PROTOCOL_VERSION } from "../generated/student-dialogue.ts";
export type { StudentDialogueCommand, StudentDialogueEvent, StudentDialogueEventKind } from "../generated/student-dialogue.ts";
export type StudentDialogueStateEvent = Extract<StudentDialogueEvent, { kind: "state" }>;
export type StudentDialogueQuestionEvent = Extract<StudentDialogueEvent, { kind: "question" }>;

// Generic interpreter for the emitted Go schema subset, not a mirrored DTO or
// separately maintained list of wire fields/statuses. Unknown data fails closed.
type Schema = {
  readonly type?: string; readonly const?: unknown; readonly enum?: readonly unknown[];
  readonly oneOf?: readonly Schema[]; readonly allOf?: readonly Schema[]; readonly not?: Schema; readonly if?: Schema; readonly then?: Schema;
  readonly properties?: Readonly<Record<string, Schema>>; readonly required?: readonly string[];
  readonly additionalProperties?: boolean; readonly dependentRequired?: Readonly<Record<string, readonly string[]>>;
  readonly minimum?: number; readonly maximum?: number; readonly minLength?: number; readonly maxLength?: number;
  readonly format?: string; readonly items?: Schema; readonly minItems?: number; readonly maxItems?: number; readonly uniqueItems?: boolean;
  readonly "x-maxBytes"?: number; readonly "x-nonBlank"?: boolean; readonly "x-equalFields"?: readonly string[]; readonly "x-uniqueNormalized"?: boolean;
};
function record(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === "object" && !Array.isArray(value); }
function dateTime(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match || !Number.isFinite(Date.parse(value))) return false;
  const [year, month, day, hour, minute, second] = match.slice(1, 7).map(Number);
  const days = [31, year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > days[month - 1] || hour > 23 || minute > 59 || second > 59) return false;
  return Date.parse(value) !== Date.parse("0001-01-01T00:00:00Z") || /[1-9]/.test((/\.(\d+)/.exec(value)?.[1] ?? "").slice(0, 9));
}
export function matchesDialogueSchema(schema: Schema, value: unknown): boolean {
  if (schema.allOf?.some(part => !matchesDialogueSchema(part, value))) return false;
  if (schema.oneOf && schema.oneOf.filter(candidate => matchesDialogueSchema(candidate, value)).length !== 1) return false;
  if (schema.not && matchesDialogueSchema(schema.not, value)) return false;
  if (Object.hasOwn(schema, "const") && value !== schema.const) return false;
  if (schema.enum && !schema.enum.includes(value)) return false;
  if (schema.if && matchesDialogueSchema(schema.if, value) && schema.then && !matchesDialogueSchema(schema.then, value)) return false;
  switch (schema.type) {
    case "object": if (!record(value)) return false; break;
    case "boolean": if (typeof value !== "boolean") return false; break;
    case "integer":
      if (typeof value !== "number" || !Number.isSafeInteger(value) || (schema.minimum !== undefined && value < schema.minimum) || (schema.maximum !== undefined && value > schema.maximum)) return false;
      break;
    case "string": {
      if (typeof value !== "string") return false;
      const length = [...value].length;
      if ((schema.minLength !== undefined && length < schema.minLength) || (schema.maxLength !== undefined && length > schema.maxLength) || (schema["x-maxBytes"] !== undefined && new TextEncoder().encode(value).length > schema["x-maxBytes"]) || (schema["x-nonBlank"] && !value.trim())) return false;
      if (schema.format === "uuid" && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)) return false;
      if (schema.format === "sha256" && !/^[0-9a-f]{64}$/.test(value)) return false;
      if (schema.format === "date-time" && !dateTime(value)) return false;
      break;
    }
    case "array":
      if (!Array.isArray(value) || (schema.minItems !== undefined && value.length < schema.minItems) || (schema.maxItems !== undefined && value.length > schema.maxItems)) return false;
      if (schema.items && value.some(item => !matchesDialogueSchema(schema.items!, item))) return false;
      if (schema.uniqueItems && new Set(value.map(item => JSON.stringify(item))).size !== value.length) return false;
      if (schema["x-uniqueNormalized"] && new Set(value.map(item => String(item).trim().toLowerCase())).size !== value.length) return false;
      break;
  }
  if (record(value)) {
    if (schema.required?.some(key => !Object.hasOwn(value, key))) return false;
    for (const [key, field] of Object.entries(value)) {
      const child = schema.properties && Object.hasOwn(schema.properties, key) ? schema.properties[key] : undefined;
      if (!child) { if (schema.additionalProperties === false) return false; }
      else if (!matchesDialogueSchema(child, field)) return false;
    }
    for (const [key, dependent] of Object.entries(schema.dependentRequired ?? {})) if (Object.hasOwn(value, key) && dependent.some(name => !Object.hasOwn(value, name))) return false;
    const equal = schema["x-equalFields"];
    if (equal?.length === 2 && value[equal[0]] !== value[equal[1]]) return false;
  }
  return true;
}
export function parseStudentDialogueEvent(value: unknown): StudentDialogueEvent | null {
  return matchesDialogueSchema(STUDENT_DIALOGUE_SCHEMA.$defs.event, value) ? value as StudentDialogueEvent : null;
}
export function isStudentDialogueCommand(value: unknown): value is StudentDialogueCommand {
  return matchesDialogueSchema(STUDENT_DIALOGUE_SCHEMA.$defs.command, value);
}
export const studentDialogueTransport = STUDENT_DIALOGUE_SCHEMA["x-transport"];
