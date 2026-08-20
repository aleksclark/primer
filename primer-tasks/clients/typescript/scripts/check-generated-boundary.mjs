import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const generated = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../generated/schema.d.ts");
let source;
try {
  source = await readFile(generated, "utf8");
} catch (error) {
  console.error(`Generated client contract is missing: ${generated}`);
  console.error("Run npm run generate before checking the generated boundary.");
  process.exit(1);
}

// Generated types must not promote provider internals or object-store
// capabilities into a browser-facing contract. This is intentionally a field
// check rather than a prose scan: descriptions may discuss the boundary.
for (const field of [
  "reasoning_delta",
  "provider_metadata",
  "raw_prompt",
  "tool_input",
  "tool_arguments",
  "x-amz-signature",
  "s3://",
  "minio://",
]) {
  if (source.toLowerCase().includes(field)) {
    console.error(`Generated client contract exposes protected field: ${field}`);
    process.exit(1);
  }
}
console.log("generated client security boundary: ok");
