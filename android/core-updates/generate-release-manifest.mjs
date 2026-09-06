import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../primer-tasks/build/openapi.json");
const outputDir = path.resolve(here, "build/generated/releaseManifest/com/aleksclark/primer/updates/generated");
const output = path.join(outputDir, "GeneratedReleaseManifest.kt");
const document = JSON.parse(await readFile(contract, "utf8"));
const schema = document.components?.schemas?.ReleaseManifest;
if (!schema?.properties) throw new Error("OpenAPI schema ReleaseManifest is required");

const required = new Set(schema.required ?? []);
function kotlinType(propertySchema) {
  const types = Array.isArray(propertySchema.type) ? propertySchema.type : [propertySchema.type];
  if (types.includes("array")) return `List<String>`;
  if (types.includes("integer")) return "Long";
  if (types.includes("string")) return "String";
  throw new Error(`unsupported ReleaseManifest field ${JSON.stringify(propertySchema)}`);
}
const fields = Object.entries(schema.properties)
  .filter(([name]) => name !== "$schema")
  .map(([name, propertySchema]) => {
    let type = kotlinType(propertySchema);
    const optional = !required.has(name);
    if (optional && !type.endsWith("?")) type = `${type}?`;
    const fallback = optional || type.endsWith("?") ? " = null" : "";
    return `    val ${name}: ${type}${fallback},`;
  });

const source = `// Generated from primer-tasks/build/openapi.json components.schemas.ReleaseManifest; DO NOT EDIT.
package com.aleksclark.primer.updates.generated

import kotlinx.serialization.Serializable

@Serializable
data class ReleaseManifest(
${fields.join("\n")}
)
`;
await mkdir(outputDir, { recursive: true });
await writeFile(output, source);
console.log("generated", output);
