import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";

const members = ["agent-protocol.schema.json", "agent-protocol.ts", "dialogue-config.schema.json", "dialogue-config.ts", "openapi.json", "openapi.yaml", "student-dialogue.schema.json", "student-dialogue.ts"];
const contractMembers = ["agent-protocol.schema.json", "dialogue-config.schema.json", "openapi.json", "student-dialogue.schema.json"];
function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value !== null && typeof value === "object") return Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])]));
  return value;
}
export function normalizedContractBytes(bundle) {
  const contracts = Object.fromEntries(contractMembers.map(name => [name, JSON.parse(bundle[name].toString("utf8"))]));
  // Match Go encoding/json's deterministic object order and string escaping;
  // no arrays, required fields, validation rules or other semantics are erased.
  return Buffer.from(JSON.stringify(canonical(contracts)).replaceAll("<", "\\u003c").replaceAll(">", "\\u003e").replaceAll("&", "\\u0026").replaceAll("\u2028", "\\u2028").replaceAll("\u2029", "\\u2029"));
}
const sha256 = data => createHash("sha256").update(data).digest("hex");

/** Validate a complete Go-emitted bundle before exposing any of its members.
 * Integrity is not authenticity: the production trust/freshness boundary is
 * Docker's current-source Go emission stage and COPY --from, never host output.
 * This manifest contains hashes only, not a second DTO/schema definition.
 */
export async function readContractBundle(directory, expectedContractDigest) {
  const manifestBytes = await readFile(path.join(directory, "manifest.json"));
  const expectedManifest = (await readFile(path.join(directory, "manifest.sha256"), "utf8")).trim();
  if (!/^[0-9a-f]{64}$/.test(expectedManifest) || sha256(manifestBytes) !== expectedManifest) {
    throw new Error("Client contract bundle manifest integrity mismatch");
  }
  const manifest = JSON.parse(manifestBytes.toString("utf8"));
  if (manifest.format !== "primer-tasks/client-contract-v2" || manifest.normalization !== "json-recursive-object-key-order-v1" || !manifest.files ||
      JSON.stringify(Object.keys(manifest.files).sort()) !== JSON.stringify(members)) {
    throw new Error("Unsupported or incomplete client contract bundle");
  }
  const result = {};
  for (const name of members) {
    const data = await readFile(path.join(directory, name));
    if (!data.length || !/^[0-9a-f]{64}$/.test(manifest.files[name]) || sha256(data) !== manifest.files[name]) {
      throw new Error(`Client contract bundle integrity mismatch: ${name}`);
    }
    result[name] = data;
  }
  // Detect a malformed schema rather than treating it as an arbitrary file;
  // wire fields/variants still come exclusively from the actual Go boundary.
  const socketSchema = JSON.parse(result["agent-protocol.schema.json"].toString("utf8"));
  if (!socketSchema.$defs?.command || !socketSchema.$defs?.event) throw new Error("Incomplete socket contract schema");
  const studentSchema = JSON.parse(result["student-dialogue.schema.json"].toString("utf8"));
  if (!studentSchema.$defs?.command?.oneOf || !studentSchema.$defs?.event?.oneOf || studentSchema["x-transport"]?.authentication !== "host-only-student-cookie") throw new Error("Incomplete student socket contract schema");
  const config = JSON.parse(result["dialogue-config.schema.json"].toString("utf8"));
  if (!config.properties || !config.oneOf || config.additionalProperties !== false) throw new Error("Incomplete parent dialogue config contract");
  if (!/^[0-9a-f]{64}$/.test(manifest.contractDigest) || sha256(normalizedContractBytes(result)) !== manifest.contractDigest) throw new Error("Normalized contract digest mismatch");
  if (expectedContractDigest !== undefined && (!/^[0-9a-f]{64}$/.test(expectedContractDigest) || manifest.contractDigest !== expectedContractDigest)) throw new Error("Stale or unexpected client contract bundle");
  return result;
}
