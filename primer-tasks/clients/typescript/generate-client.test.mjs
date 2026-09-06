import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, writeFile, cp, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readContractBundle } from "./contract-bundle.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const tasksRoot = path.resolve(here, "../..");
const temporary = await mkdtemp(path.join(tmpdir(), "primer-tasks-contract-test-"));
after(() => rm(temporary, { recursive: true, force: true }));
const environment = { ...process.env, GOWORK: "off" };
delete environment.TASKS_CLIENT_CONTRACT_BUNDLE;
const emit = (directory, overlay) => execFileSync("go", ["run", ...(overlay ? [`-overlay=${overlay}`] : []), "./cmd/agent-protocol-gen", "-bundle", directory], { cwd: tasksRoot, env: environment, stdio: "pipe" });
const bundleA = path.join(temporary, "bundle-a");
emit(bundleA);
const fixtureClient = path.join(temporary, "app/clients/typescript");
await mkdir(fixtureClient, { recursive: true });
for (const name of ["generate-client.mjs", "contract-bundle.mjs", "package.json"]) await cp(path.join(here, name), path.join(fixtureClient, name));
await symlink(path.join(here, "node_modules"), path.join(fixtureClient, "node_modules"), "dir");
const output = path.join(fixtureClient, "generated/agent-protocol.ts");
const noGo = path.join(temporary, "no-executables");
await mkdir(noGo);
const runWithoutGo = (bundle, explicit = true) => spawnSync(process.execPath, [path.join(fixtureClient, "generate-client.mjs")], {
  cwd: fixtureClient, encoding: "utf8", env: { ...environment, PATH: noGo, ...(explicit ? { TASKS_CLIENT_CONTRACT_BUNDLE: bundle } : {}) },
});
const sha = bytes => createHash("sha256").update(bytes).digest("hex");

// This uses a real Go compiler source overlay, not handwritten schema content.
// A boundary field changes BOTH emitted WS shapes through production reflection.
const original = path.join(tasksRoot, "internal/api/agent_ws.go");
const overlaid = path.join(temporary, "agent_ws.go");
const source = await readFile(original, "utf8");
assert.equal(source.split("type wireAgentEvent struct {").length, 2);
await writeFile(overlaid, source.replace("type wireAgentEvent struct {", 'type wireAgentEvent struct {\n BundleProbe string `json:"bundleProbe,omitempty"`'));
const overlay = path.join(temporary, "overlay.json");
await writeFile(overlay, JSON.stringify({ Replace: { [original]: overlaid } }));
const bundleB = path.join(temporary, "bundle-b");
emit(bundleB, overlay);

test("Node-only explicit consumption validates and generates actual Go-derived contracts", async () => {
  const a = await readContractBundle(bundleA);
  const result = runWithoutGo(bundleA);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /verified pre-emitted/);
  assert.deepEqual(await readFile(output), a["agent-protocol.ts"]);
  assert.match(await readFile(path.join(fixtureClient, "generated/schema.d.ts"), "utf8"), /export interface paths/);
  const b = await readContractBundle(bundleB);
  assert.notDeepEqual(a["agent-protocol.ts"], b["agent-protocol.ts"]);
  assert.match(b["agent-protocol.ts"].toString(), /bundleProbe/);
  assert.match(b["agent-protocol.schema.json"].toString(), /bundleProbe/);
});

test("missing, mixed, corrupt and unsupported bundles fail before replacing outputs", async () => {
  const goodOutput = await readFile(output);
  const cases = [
    ["missing directory", async dir => rm(dir, { recursive: true })],
    ["missing completion marker", async dir => rm(path.join(dir, "manifest.sha256"))],
    ["missing OpenAPI", async dir => rm(path.join(dir, "openapi.yaml"))],
    ["missing WS schema", async dir => rm(path.join(dir, "agent-protocol.schema.json"))],
    ["missing WS types", async dir => rm(path.join(dir, "agent-protocol.ts"))],
    ["corrupt WS types", async dir => writeFile(path.join(dir, "agent-protocol.ts"), "corrupt")],
    ["mixed source types", async dir => cp(path.join(bundleB, "agent-protocol.ts"), path.join(dir, "agent-protocol.ts"))],
    ["mixed source manifest", async dir => { for (const file of ["manifest.json", "manifest.sha256"]) await cp(path.join(bundleB, file), path.join(dir, file)); }],
    ["corrupt manifest", async dir => writeFile(path.join(dir, "manifest.json"), "{}")],
    ["unsupported coherent format", async dir => {
      const manifest = JSON.parse(await readFile(path.join(dir, "manifest.json"), "utf8"));
      manifest.format = "unknown";
      const bytes = JSON.stringify(manifest);
      await writeFile(path.join(dir, "manifest.json"), bytes);
      await writeFile(path.join(dir, "manifest.sha256"), sha(bytes));
    }],
  ];
  for (const [name, mutate] of cases) {
    const directory = path.join(temporary, name.replaceAll(" ", "-"));
    await cp(bundleA, directory, { recursive: true });
    await mutate(directory);
    const result = runWithoutGo(directory);
    assert.notEqual(result.status, 0, name);
    assert.doesNotMatch(result.stderr, /spawnSync go/, name); // no hidden fallback
    assert.deepEqual(await readFile(output), goodOutput, name);
  }
  assert.notEqual(runWithoutGo("").status, 0, "empty explicit mode must fail closed");
});

test("default local generation always invokes Go, never silently reuses existing bundles", async () => {
  await cp(bundleA, path.resolve(fixtureClient, "../../build/client-contract"), { recursive: true });
  const unavailable = runWithoutGo(undefined, false);
  assert.notEqual(unavailable.status, 0);
  assert.match(unavailable.stderr, /spawnSync go ENOENT/);
  // Run the real default entry point twice with actual Go. Corrupt its existing
  // output AND existing local bundle between runs; both must be regenerated.
  const generate = () => execFileSync(process.execPath, [path.join(here, "generate-client.mjs")], { cwd: here, env: environment, stdio: "pipe" });
  generate();
  const actual = path.join(here, "generated/agent-protocol.ts");
  const expected = await readFile(actual);
  await writeFile(actual, "stale local output");
  await writeFile(path.join(tasksRoot, "build/client-contract/agent-protocol.ts"), "stale local bundle");
  generate();
  assert.deepEqual(await readFile(actual), expected);
  assert.deepEqual((await readContractBundle(path.join(tasksRoot, "build/client-contract")))["agent-protocol.ts"], expected);
});
