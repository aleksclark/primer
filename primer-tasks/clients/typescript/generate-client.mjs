import { mkdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import openapiTS, { astToString } from "openapi-typescript";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../build/openapi.yaml");
const output = path.resolve(here, "generated/schema.d.ts");

try {
  const source = await readFile(contract, "utf8");
  const ast = await openapiTS(source, { silent: true });
  const schema = astToString(ast);
  await mkdir(path.dirname(output), { recursive: true });
  await import("node:fs/promises").then(({ writeFile }) => writeFile(output, schema));
  console.log(`generated ${path.relative(process.cwd(), output)}`);
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`cannot generate the Tasks client: ${contract} is required (${message})`);
  process.exitCode = 1;
}
