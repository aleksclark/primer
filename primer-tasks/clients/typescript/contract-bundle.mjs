import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";

const members = ["agent-protocol.schema.json", "agent-protocol.ts", "openapi.yaml"];
const sha256 = data => createHash("sha256").update(data).digest("hex");

/** Validate a complete Go-emitted bundle before exposing any of its members.
 * Integrity is not authenticity: the production trust/freshness boundary is
 * Docker's current-source Go emission stage and COPY --from, never host output.
 * This manifest contains hashes only, not a second DTO/schema definition.
 */
export async function readContractBundle(directory) {
  const manifestBytes = await readFile(path.join(directory, "manifest.json"));
  const expectedManifest = (await readFile(path.join(directory, "manifest.sha256"), "utf8")).trim();
  if (!/^[0-9a-f]{64}$/.test(expectedManifest) || sha256(manifestBytes) !== expectedManifest) {
    throw new Error("Client contract bundle manifest integrity mismatch");
  }
  const manifest = JSON.parse(manifestBytes.toString("utf8"));
  if (manifest.format !== "primer-tasks/client-contract-v1" || !manifest.files ||
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
  return result;
}
