import { DIALOGUE_CONFIG_SCHEMA } from "../generated/dialogue-config.ts";
import type { DialogueConfig } from "../generated/dialogue-config.ts";
import type { components } from "../generated/schema";
import { matchesDialogueSchema } from "./student-dialogue-protocol.ts";

export { DIALOGUE_CONFIG_SCHEMA } from "../generated/dialogue-config.ts";
export type { DialogueConfig } from "../generated/dialogue-config.ts";
export type DialogueInspect = components["schemas"]["DialogueInspect"];
export type DialogueOverrideRequest = components["schemas"]["DialogueOverrideRequest"];
export type DialogueOverrideReceipt = components["schemas"]["DialogueOverrideReceipt"];

/** Parent configuration only. No published question plan or student authority
 * is accepted here; unknown fields/unsupported source and retention modes fail. */
export function parseDialogueConfig(value: unknown): DialogueConfig | null {
  return matchesDialogueSchema(DIALOGUE_CONFIG_SCHEMA, value) ? value as DialogueConfig : null;
}
export function dialogueRequirement(value: DialogueConfig): components["schemas"]["Requirement"] {
  const config = parseDialogueConfig(value);
  if (!config) throw new Error("Dialogue configuration does not match the server manifest.");
  return { id: "agent-dialogue", ...DIALOGUE_CONFIG_SCHEMA["x-manifest"], config: { ...config } };
}
