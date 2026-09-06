import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src");
const forbidden = [
  /\bfetch\s*\(/,
  /XMLHttpRequest/,
  /axios\b/,
  /\/api\//,
  /localStorage/,
  /sessionStorage/,
  /\bWebSocket\b/,
];

async function walk(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await walk(absolute));
    else if (/\.(tsx?|jsx?)$/.test(entry.name)) files.push(absolute);
  }
  return files;
}

const violations = [];
for (const file of await walk(root)) {
  const source = await readFile(file, "utf8");
  for (const pattern of forbidden) {
    if (pattern.test(source)) violations.push(`${path.relative(process.cwd(), file)} matches ${pattern}`);
  }
}
if (violations.length) {
  console.error("Web transport boundary violation. Use @primer-tasks/client:");
  console.error(violations.join("\n"));
  process.exit(1);
}
console.log("client boundary: ok");
