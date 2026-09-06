import { mkdtemp, mkdir, readdir, readFile, writeFile, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const here = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
/** Compile the real client facade/generated internals for Node conformance
 * tests. This is transport qualification, not another DTO/generator source. */
export async function loadTestClient() {
  const folder = await mkdtemp(path.join(tmpdir(), "primer-tasks-client-runtime-"));
  await writeFile(path.join(folder, "package.json"), '{"type":"module"}');
  await symlink(path.join(here, "node_modules"), path.join(folder, "node_modules"), "dir");
  for (const directory of ["src", "generated"]) {
    await mkdir(path.join(folder, directory));
    for (const file of await readdir(path.join(here, directory))) {
      if (!file.endsWith(".ts") || file.endsWith(".d.ts") || file.endsWith(".test.ts")) continue;
      const source = await readFile(path.join(here, directory, file), "utf8");
      const result = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2023 } });
      // Node requires explicit runtime extensions; product Vite/TS retains its
      // normal package facade. No server paths or DTO values are rewritten.
      const output = result.outputText.replace(/(from\s+|import\s*)["'](\.{1,2}\/[^"']+)["']/g, (_all, prefix, specifier) => `${prefix}"${specifier.replace(/\.ts$/, "").replace(/(?<!\.js)$/, ".js")}"`);
      await writeFile(path.join(folder, directory, file.replace(/\.ts$/, ".js")), output);
    }
  }
  return { client: await import(pathToFileURL(path.join(folder, "src/index.js"))), cleanup: () => rm(folder, { recursive: true, force: true }) };
}
