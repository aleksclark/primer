import { readdir, readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
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
  if (/\bfetch\s*\(|XMLHttpRequest|axios\b|server\/internal|internal\/api/.test(source)) { console.error("Client source must use its generated transport, not a parallel service boundary."); process.exit(1); }
  if (/(?:interface\s+(?:StudentDialogueEvent|StudentDialogueCommand|DialogueConfig)\b|type\s+(?:StudentDialogueEvent|StudentDialogueCommand|DialogueConfig)\s*=)/.test(source)) { console.error("Student wire/config DTOs must be generated, not copied into client source."); process.exit(1); }
}
const facade = await readFile(path.join(root, "agent-protocol.ts"), "utf8");
if (!facade.includes('../generated/agent-protocol') || /export (interface Agent|type AgentCommand\s*=|type AgentEvent\s*=)/.test(facade)) {
  console.error("Wire DTOs must be generated from the actual Go socket boundary, not copied into the façade.");
  process.exit(1);
}
const protocol = await readFile(path.join(root, "../generated/agent-protocol.ts"), "utf8");
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
const studentFacade = await readFile(path.join(root, "student-dialogue-protocol.ts"), "utf8");
if (!studentFacade.includes('../generated/student-dialogue.ts') || /export (interface StudentDialogueEvent|type StudentDialogueCommand\s*=|type StudentDialogueEvent\s*=)/.test(studentFacade)) {
  console.error("Student DTOs must be generated from the actual Go runtime boundary."); process.exit(1);
}
const student = await readFile(path.join(root, "../generated/student-dialogue.ts"), "utf8");
for (const required of ['kind: "user_message"', 'kind: "complete"', 'expectedVersion:', 'snapshotDigest:', 'STUDENT_DIALOGUE_SCHEMA']) if (!student.includes(required)) { console.error(`Missing generated student contract: ${required}`); process.exit(1); }
if (/reasoning_delta|provider_metadata|rawPrompt/.test(student)) { console.error("Unsafe student provider field in generated contract."); process.exit(1); }
const allowedConstructors = ["src/agent-client.ts", "src/dialogue-client.ts"];
if (constructors.length !== allowedConstructors.length || !allowedConstructors.every(name => constructors.some(file => file.endsWith(name)))) {
  console.error("Transport boundary violation: only the named parent/student client facades may construct WebSocket.");
  console.error(constructors.join("\n") || "no façade constructor found");
  process.exit(1);
}
const tracked = execFileSync("git", ["ls-files", "--", "primer-tasks/clients/typescript/generated", "primer-tasks/build"], { cwd: path.resolve(root, "../../../.."), encoding: "utf8", env: { ...process.env, GIT_OPTIONAL_LOCKS: "0" } }).trim();
const generatedTracked = tracked.split("\n").filter(file => file && file !== "primer-tasks/clients/typescript/generated/.gitkeep");
if (generatedTracked.length) { console.error("Generated Tasks client/contract output must not be tracked."); process.exit(1); }
console.log("parent/student generated client boundaries: ok");
