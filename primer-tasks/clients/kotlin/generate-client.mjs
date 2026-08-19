import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../build/openapi.yaml");
const output = path.resolve(here, "generated/PrimerTasksContract.kt");
const source = await readFile(contract, "utf8");
await mkdir(path.dirname(output), { recursive: true });
// This is intentionally a generated build artifact, not a hand-maintained
// DTO. The Android façade consumes the generated contract in a later phase.
const escaped = source.replace(/\\/g, "\\\\").replace(/"/g, '\\"').replace(/\r?\n/g, "\\n");
await writeFile(output, `package com.aleksclark.primertasks.generated\n\nobject PrimerTasksContract {\n    const val OPENAPI_JSON = "${escaped}"\n}\n`);
console.log(`generated ${path.relative(process.cwd(), output)}`);
