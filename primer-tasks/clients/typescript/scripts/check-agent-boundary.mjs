import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src");
const files = [];
async function walk(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) await walk(absolute);
    else if (entry.name.endsWith(".ts")) files.push(absolute);
  }
}
await walk(root);

const constructors = [];
for (const file of files) {
  const source = await readFile(file, "utf8");
  if (/new\s+WebSocket\s*\(/.test(source)) constructors.push(path.relative(process.cwd(), file));
}
const protocol = await readFile(path.join(root, "agent-protocol.ts"), "utf8");
for (const required of ['protocol: typeof AGENT_PROTOCOL_VERSION', 'kind: "user_message"', 'kind: "text_delta"', 'kind: "tool_progress"', 'kind: "replay_gap"']) {
  if (!protocol.includes(required)) {
    console.error(`Agent protocol projection is missing ${required}`);
    process.exit(1);
  }
}
if (/reasoning_delta|provider_metadata|rawPrompt/.test(protocol)) {
  console.error("Agent protocol projection contains an unsafe provider field.");
  process.exit(1);
}
const allowedConstructors = new Set(["src/agent-client.ts", "src/dialogue-client.ts", "src/artifact-client.ts"]);
if (constructors.length !== allowedConstructors.size || constructors.some((file) => !allowedConstructors.has(file))) {
  console.error("Client transport boundary violation: only the owned agent/dialogue façades may construct WebSocket.");
  console.error(constructors.join("\n") || "no façade constructor found");
  process.exit(1);
}
console.log("agent client boundary: ok");
