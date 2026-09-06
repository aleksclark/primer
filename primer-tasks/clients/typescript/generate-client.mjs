import { mkdir, writeFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import openapiTS, { astToString } from "openapi-typescript";
import { readContractBundle } from "./contract-bundle.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const tasksRoot = path.resolve(here, "../..");
const output = path.resolve(here, "generated/schema.d.ts");

try {
  const preEmitted = Object.hasOwn(process.env, "TASKS_CLIENT_CONTRACT_BUNDLE");
  if (preEmitted && !process.env.TASKS_CLIENT_CONTRACT_BUNDLE.trim()) throw new Error("Explicit client contract bundle path is empty");
  const bundlePath = preEmitted ? path.resolve(process.env.TASKS_CLIENT_CONTRACT_BUNDLE) : path.join(tasksRoot, "build/client-contract");
  if (!preEmitted) {
    // Default local generation is ALWAYS fresh, even when old output exists.
    // A missing Go tool is an error, never an implicit pre-emitted fallback.
    execFileSync("go", ["run", "./cmd/agent-protocol-gen", "-bundle", bundlePath], { cwd: tasksRoot, stdio: "inherit" });
  }
  const bundle = await readContractBundle(bundlePath);
  const schema = astToString(await openapiTS(bundle["openapi.yaml"].toString("utf8"), { silent: true }));
  // Validate the entire bundle and REST generation before replacing outputs.
  await mkdir(path.dirname(output), { recursive: true });
  await mkdir(path.join(tasksRoot, "build"), { recursive: true });
  await writeFile(output, schema);
  await writeFile(path.join(here, "generated/agent-protocol.ts"), bundle["agent-protocol.ts"]);
  await writeFile(path.join(tasksRoot, "build/openapi.yaml"), bundle["openapi.yaml"]);
  await writeFile(path.join(tasksRoot, "build/agent-protocol.schema.json"), bundle["agent-protocol.schema.json"]);
  console.log(`generated ${path.relative(process.cwd(), output)} from ${preEmitted ? "verified pre-emitted" : "fresh Go-emitted"} contracts`);
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`cannot generate the Tasks client: ${message}`);
  process.exitCode = 1;
}
