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
const environment = { ...process.env, GOWORK: "off", GOFLAGS: "-mod=readonly", GIT_OPTIONAL_LOCKS: "0" };
delete environment.TASKS_CLIENT_CONTRACT_BUNDLE;
delete environment.TASKS_CLIENT_EXPECTED_CONTRACT_DIGEST;
const emit = (directory, overlay) => execFileSync("go", ["run", ...(overlay ? [`-overlay=${overlay}`] : []), "./cmd/agent-protocol-gen", "-bundle", directory], { cwd: tasksRoot, env: environment, stdio: "pipe" });
const bundleA = path.join(temporary, "bundle-a");
emit(bundleA);
const fixtureClient = path.join(temporary, "app/clients/typescript");
await mkdir(fixtureClient, { recursive: true });
for (const name of ["generate-client.mjs", "contract-bundle.mjs", "package.json"]) await cp(path.join(here, name), path.join(fixtureClient, name));
await symlink(path.join(here, "node_modules"), path.join(fixtureClient, "node_modules"), "dir");
await cp(path.join(here, "src"), path.join(fixtureClient, "src"), { recursive: true });
await cp(path.join(here, "tsconfig.json"), path.join(fixtureClient, "tsconfig.json"));
const output = path.join(fixtureClient, "generated/agent-protocol.ts");
const noGo = path.join(temporary, "no-executables");
await mkdir(noGo);
const runWithoutGo = (bundle, explicit = true, expectedDigest) => spawnSync(process.execPath, [path.join(fixtureClient, "generate-client.mjs")], {
  cwd: fixtureClient, encoding: "utf8", env: { ...environment, PATH: noGo, ...(explicit ? { TASKS_CLIENT_CONTRACT_BUNDLE: bundle } : {}), ...(expectedDigest === undefined ? {} : { TASKS_CLIENT_EXPECTED_CONTRACT_DIGEST: expectedDigest }) },
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

// Rename the ACTUAL runtime command field and qualify the same decoder under
// the real Go overlay. Variant/required metadata follows the Go JSON tag.
const studentOriginal = path.join(tasksRoot, "internal/api/student_ws_protocol.go");
const studentSource = await readFile(studentOriginal, "utf8");
const textTag = '`json:"text,omitempty" wire:"user_message!"';
assert.equal(studentSource.split(textTag).length, 2);
const studentOverlaySource = path.join(temporary, "student_ws_protocol.go");
await writeFile(studentOverlaySource, studentSource.replace(textTag, '`json:"answerText,omitempty" wire:"user_message!"'));
const runtimeTestOriginal = path.join(tasksRoot, "internal/api/student_ws_contract_test.go");
const runtimeTestOverlay = path.join(temporary, "student_ws_contract_test.go");
await writeFile(runtimeTestOverlay, (await readFile(runtimeTestOriginal, "utf8")) + `
func TestStudentContractOverlayRuntime(t *testing.T) {
 e:=contractStudentEvent("question")
 input:=map[string]any{"protocol":1,"kind":"user_message","occurrenceId":e.OccurrenceID,"attemptId":e.AttemptID,"questionId":e.QuestionID,"policyVersion":e.PolicyVersion,"snapshotDigest":e.SnapshotDigest,"expectedVersion":2,"clientMessageId":"overlay","answerText":"A saved answer."}
 raw,_:=json.Marshal(input);command,err:=decodeStudentCommand(raw)
 if err!=nil||command.Text!="A saved answer."{t.Fatal("actual renamed runtime boundary not enforced")}
 input["text"]=input["answerText"];delete(input,"answerText");raw,_=json.Marshal(input)
 if _,err=decodeStudentCommand(raw);err==nil{t.Fatal("obsolete field accepted by actual runtime decoder")}
}
`);
const studentOverlay = path.join(temporary, "student-overlay.json");
await writeFile(studentOverlay, JSON.stringify({ Replace: { [studentOriginal]: studentOverlaySource, [runtimeTestOriginal]: runtimeTestOverlay } }));
const bundleStudent = path.join(temporary, "bundle-student-overlay");
emit(bundleStudent, studentOverlay);

// Parent config bounds are shared by actual Go validation and emission.
const configOriginal = path.join(tasksRoot, "internal/domain/dialogue.go");
const configSource = await readFile(configOriginal, "utf8");
assert.match(configSource, /DialogueFocusMaxRunes\s+= 500/);
const configOverlaySource = path.join(temporary, "dialogue.go");
await writeFile(configOverlaySource, configSource.replace(/DialogueFocusMaxRunes\s+= 500/, "DialogueFocusMaxRunes = 501"));
const configRuntimeOverlay = path.join(temporary, "config-runtime_test.go");
await writeFile(configRuntimeOverlay, (await readFile(runtimeTestOriginal, "utf8")) + `
func TestDialogueConfigOverlayRuntime(t *testing.T) {
 config:=domain.DialogueConfig{SourceText:"Assigned source",LearningFocus:strings.Repeat("x",501),RequiredQuestions:3,Rubric:[]string{"source"},AllowedFollowUps:1,MaxAttempts:2,MaxTurns:8,RetentionPolicy:"retain"}
 if config.Validate()!=nil{t.Fatal("actual runtime config bound did not follow the Go change")}
}
`);
const configOverlay = path.join(temporary, "config-overlay.json");
await writeFile(configOverlay, JSON.stringify({ Replace: { [configOriginal]: configOverlaySource, [runtimeTestOriginal]: configRuntimeOverlay } }));
const bundleConfig = path.join(temporary, "bundle-config-overlay");
emit(bundleConfig, configOverlay);

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
  const student = await readContractBundle(bundleStudent);
  assert.match(student["student-dialogue.ts"].toString(), /answerText: string/);
  assert.match(student["student-dialogue.schema.json"].toString(), /answerText/);
  for (const name of ["agent-protocol.ts", "agent-protocol.schema.json", "openapi.yaml"]) assert.deepEqual(a[name], student[name], `student overlay changed parent contract ${name}`);
  execFileSync("go", ["test", `-overlay=${studentOverlay}`, "./internal/api", "-run", "^TestStudentContractOverlayRuntime$", "-count=1"], { cwd: tasksRoot, env: environment, stdio: "pipe" });
  const config = await readContractBundle(bundleConfig);
  assert.equal(JSON.parse(config["dialogue-config.schema.json"].toString()).properties.learningFocus.maxLength, 501);
  assert.match(config["dialogue-config.ts"].toString(), /501/);
  const typecheck = () => spawnSync(process.execPath, [path.join(here, "node_modules/typescript/bin/tsc"), "-p", path.join(fixtureClient, "tsconfig.json"), "--allowImportingTsExtensions"], { encoding: "utf8", env: { ...environment, PATH: noGo } });
  assert.equal(typecheck().status, 0, "fresh actual client must compile");
  assert.equal(runWithoutGo(bundleStudent).status, 0);
  const staleConsumer = typecheck();
  assert.notEqual(staleConsumer.status, 0, "actual Go wire rename must break the stale consumer");
  assert.match(staleConsumer.stdout + staleConsumer.stderr, /text/);
  assert.equal(runWithoutGo(bundleA).status, 0, "restore coherent fixture output");
  execFileSync("go", ["test", `-overlay=${configOverlay}`, "./internal/api", "-run", "^TestDialogueConfigOverlayRuntime$", "-count=1"], { cwd: tasksRoot, env: environment, stdio: "pipe" });
});

test("missing, mixed, corrupt and unsupported bundles fail before replacing outputs", async () => {
  const goodOutput = await readFile(output);
  const cases = [
    ["missing directory", async dir => rm(dir, { recursive: true })],
    ["missing completion marker", async dir => rm(path.join(dir, "manifest.sha256"))],
    ["missing OpenAPI", async dir => rm(path.join(dir, "openapi.yaml"))],
    ["missing WS schema", async dir => rm(path.join(dir, "agent-protocol.schema.json"))],
    ["missing WS types", async dir => rm(path.join(dir, "agent-protocol.ts"))],
    ["missing REST JSON", async dir => rm(path.join(dir, "openapi.json"))],
    ["missing student schema", async dir => rm(path.join(dir, "student-dialogue.schema.json"))],
    ["missing student types", async dir => rm(path.join(dir, "student-dialogue.ts"))],
    ["missing config schema", async dir => rm(path.join(dir, "dialogue-config.schema.json"))],
    ["missing config types", async dir => rm(path.join(dir, "dialogue-config.ts"))],
    ["mixed student source", async dir => cp(path.join(bundleStudent, "student-dialogue.ts"), path.join(dir, "student-dialogue.ts"))],
    ["mixed config source", async dir => cp(path.join(bundleConfig, "dialogue-config.schema.json"), path.join(dir, "dialogue-config.schema.json"))],
    ["wrong normalized digest", async dir => {
      const manifest = JSON.parse(await readFile(path.join(dir, "manifest.json"), "utf8")); manifest.contractDigest = "0".repeat(64);
      const bytes = JSON.stringify(manifest); await writeFile(path.join(dir, "manifest.json"), bytes); await writeFile(path.join(dir, "manifest.sha256"), sha(bytes));
    }],
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
  const expected = JSON.parse(await readFile(path.join(bundleStudent, "manifest.json"), "utf8")).contractDigest;
  const stale = runWithoutGo(bundleA, true, expected);
  assert.notEqual(stale.status, 0, "coherent but stale bundle must fail expected-digest binding");
  assert.match(stale.stderr, /Stale or unexpected/);
  assert.deepEqual(await readFile(output), goodOutput);
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
  const generated = ["schema.d.ts", "agent-protocol.ts", "student-dialogue.ts", "dialogue-config.ts"];
  const before = Object.fromEntries(await Promise.all(generated.map(async name => [name, await readFile(path.join(here, "generated", name))])));
  await writeFile(actual, "stale local output");
  await writeFile(path.join(tasksRoot, "build/client-contract/agent-protocol.ts"), "stale local bundle");
  generate();
  assert.deepEqual(await readFile(actual), expected);
  assert.deepEqual((await readContractBundle(path.join(tasksRoot, "build/client-contract")))["agent-protocol.ts"], expected);
  for (const name of generated) assert.deepEqual(await readFile(path.join(here, "generated", name)), before[name], `fresh double-generation drift: ${name}`);
});
