/// <reference types="node" />
import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, writeFile, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { build } from "vite";
import react from "@vitejs/plugin-react";
import ts from "typescript";
import type { DialogueClientSnapshot, DialogueInspect, Occurrence, StudentDialogueEvent, StudentDialogueStateEvent, Task } from "@primer-tasks/client";

// Compile the actual components and owned client with the installed production
// bundler. These are form/projection/static-markup checks, NOT browser evidence.
let folder: string;
let ui: typeof import("./DialogueTaskForm") & typeof import("./StudentDialoguePage") & typeof import("./OccurrenceInspectPage") & typeof import("./TaskForms");
let client: typeof import("@primer-tasks/client");
before(async () => {
  const root = path.resolve(import.meta.dirname, "..");
  folder = await mkdtemp(path.join(tmpdir(), "primer-cp4-ui-"));
  await symlink(path.join(root, "node_modules"), path.join(folder, "node_modules"), "dir");
  await mkdir(path.join(folder, "out"));
  const entry = path.join(folder, "entry.ts");
  await writeFile(entry, ["DialogueTaskForm", "StudentDialoguePage", "OccurrenceInspectPage", "TaskForms"].map(name => `export * from ${JSON.stringify(path.join(root, "src", name + ".tsx"))};`).join("\n") + `\nexport * as client from ${JSON.stringify(path.resolve(root, "../clients/typescript/src/index.ts"))};\n`);
  await build({ configFile: false, root, logLevel: "silent", plugins: [react()], ssr: { noExternal: ["@primer-tasks/client", "openapi-fetch"] }, build: { ssr: entry, outDir: path.join(folder, "out"), emptyOutDir: false, minify: false, rollupOptions: { output: { entryFileNames: "ui.mjs" } } } });
  const module = await import(pathToFileURL(path.join(folder, "out/ui.mjs")).href);
  ui = module; client = module.client;
});
after(async () => { if (folder) await rm(folder, { recursive: true, force: true }); });

const occurrence: Occurrence = { id: "occurrence", studentId: "student", studentName: "Alice", title: "Careful reading", instructions: "Read the chapter.", status: "awaiting_verification", scheduleId: "schedule", revisionId: "revision", nominalAt: "2026-01-01T00:00:00Z", dueAt: "2026-01-01T00:00:00Z", timezone: "UTC", dueOffsetMinutes: 0, dueSemantics: "nominal", taskRevisionVersion: 1, scheduleVersion: 1, attemptNumber: 1 };
const binding = { protocol: 1, time: "2026-01-01T00:00:00Z", occurrenceId: "occurrence", attemptId: "attempt", requirementId: "requirement", policyVersion: "dialogue.v1", snapshotDigest: "0".repeat(64), version: 4, acceptedCount: 2, requiredCount: 3 } as const;
const state: StudentDialogueStateEvent = { ...binding, kind: "state", cursor: 0, sequence: 0, status: "open", occurrenceStatus: "awaiting_verification", questionId: "third", text: "Explain the third concept." };
const empty: DialogueClientSnapshot = { connectionState: "connected", cursor: 0, events: [], canAnswer: false, completed: false };

test("editable requirement updates preserve IDs, mixed arrays and unknown payloads", () => {
  const config = { ...ui.newDialogueConfig(), sourceText: "The wall needs time to dry.", learningFocus: "Explain careful work", rubric: ["Uses the source"] };
  const requirements: NonNullable<Task["requirements"]> = [
    { id: "manual", kind: "parent_approval", configVersion: 1, interaction: "parent_action", executor: "human", config: { retained: { nested: true } } },
    { ...client.dialogueRequirement(config), id: "reading" },
    { id: "other", kind: "external_supported_method", configVersion: 7, interaction: "external", executor: "other", config: { opaque: [1, 2, 3] } },
  ];
  const result = ui.replaceRequirement(requirements, 1, client.dialogueRequirement({ ...config, learningFocus: "New focus" }));
  assert.equal(result[0], requirements[0]); assert.equal(result[2], requirements[2]); assert.equal(result[1].id, "reading");
  assert.equal(result[1].config.learningFocus, "New focus"); assert.equal(requirements[1].config.learningFocus, "Explain careful work");
  assert.equal(ui.dialogueFormValid(result), true); assert.equal(ui.dialogueFormValid([]), false);
});

test("generated configuration bounds gate save while incomplete drafts remain editable", () => {
  const draft = ui.newDialogueConfig();
  assert.ok(ui.readEditableConfig(draft)); assert.equal(client.parseDialogueConfig(draft), null);
  const invalid = { ...draft, maxTurns: client.DIALOGUE_CONFIG_SCHEMA.properties.maxTurns.maximum + 1 };
  assert.ok(ui.readEditableConfig(invalid)); assert.equal(client.parseDialogueConfig(invalid), null);
  assert.equal(ui.readEditableConfig({ ...draft, retentionDays: 30 }), null);
  assert.equal(ui.readEditableConfig({ ...draft, questionPlan: ["client question"] }), null);
  assert.equal(ui.readEditableConfig({ ...draft, requiredQuestions: 2 }), null);
  const { sourceText: _text, ...curated } = draft;
  assert.ok(client.parseDialogueConfig({ ...curated, sourceRef: client.DIALOGUE_CONFIG_SCHEMA.properties.sourceRef.const, learningFocus: "Recall", rubric: ["Evidence"] }));
});

test("TaskEditor keeps manual default and renders existing dialogue in the editable revision path", () => {
  const manual = renderToStaticMarkup(createElement(ui.TaskEditor, { onSaved() {} }));
  assert.match(manual, /Create draft/); assert.match(manual, /Parent approval/); assert.doesNotMatch(manual, /Retention days/);
  const config = { ...ui.newDialogueConfig(), sourceText: "Original source", learningFocus: "Original focus", rubric: ["Original criterion"] };
  const task: Task = { id: "revision", templateId: "template", version: 2, title: "Careful reading", instructions: "Keep instructions", status: "published", createdAt: "2026-01-01T00:00:00Z", requirements: [client.dialogueRequirement(config)] };
  const edit = renderToStaticMarkup(createElement(ui.TaskEditor, { task, onSaved() {} }));
  assert.match(edit, /Save new draft/); assert.match(edit, /Original source/); assert.match(edit, /Original criterion/); assert.match(edit, /Exactly 3/); assert.match(edit, /no automatic deletion/);
  assert.doesNotMatch(edit, /Your student starts the task and submits it for your approval/);
});

test("replayed durable answers retain order and do not turn two accepted answers into completion", () => {
  const events: StudentDialogueEvent[] = [
    { ...binding, kind: "question", sequence: 1, cursor: 1, questionId: "third", text: "Explain the third concept." },
    { ...binding, kind: "message_ack", sequence: 2, cursor: 2, questionId: "third", messageId: "answer", clientMessageId: "request", text: "My original answer" },
    { ...binding, kind: "answer_evaluation", sequence: 3, cursor: 3, questionId: "third", messageId: "answer", status: "rejected", text: "Use the source." },
  ];
  let snapshot = client.reduceDialogueEvent(empty, state);
  for (const event of events) snapshot = client.reduceDialogueEvent(snapshot, event);
  assert.equal(ui.dialogueOutcome(snapshot), ""); assert.equal(snapshot.completed, false);
  assert.deepEqual(snapshot.events.map(ui.transcriptText), ["Explain the third concept.", "My original answer", "Use the source."]);
  for (const event of events) snapshot = client.reduceDialogueEvent(snapshot, event);
  assert.equal(snapshot.events.length, 3);
  assert.equal(snapshot.binding?.questionId, "third");
});

test("only explicit server occurrence completion renders completion; other terminal states explain remaining work", () => {
  assert.match(ui.dialogueOutcome({ ...empty, status: "accepted", occurrenceStatus: "awaiting_verification" }), /not complete/);
  assert.match(ui.dialogueOutcome({ ...empty, status: "exhausted" }), /without completing/);
  assert.match(ui.dialogueOutcome({ ...empty, occurrenceStatus: "canceled" }), /closed/);
  assert.match(ui.dialogueOutcome({ ...empty, completed: true, status: "accepted", occurrenceStatus: "completed" }), /Task completed/);
  const markup = renderToStaticMarkup(createElement(ui.DialogueSession, { occurrence, initial: { ...state, status: "accepted", occurrenceStatus: "completed" } }));
  assert.match(markup, /Task completed/); assert.doesNotMatch(markup, /Send answer/);
});

test("typed inspect503 and dialogue404 are failures, never empty history or a manual fallback", () => {
  assert.match(ui.inspectErrorCopy(new client.TasksApiError(503, "private payload")), /No partial timeline/);
  assert.doesNotMatch(ui.inspectErrorCopy(new client.TasksApiError(503, "private payload")), /private payload/);
  assert.match(ui.dialogueErrorCopy(new client.TasksApiError(404, "not found")), /not been replaced with manual approval/);
  assert.match(ui.dialogueErrorCopy(new client.TasksApiError(403, "denied")), /Access denied/);
  const capability = { id: "reading", kind: "agent_dialogue", interaction: "chat", attemptId: "", attemptStatus: "", dialogueStarted: false, historyAttemptId: "" };
  // The dedicated page's not-started state is based on capability, not a 404.
  assert.equal(ui.requirementName(capability, 0), "Reading dialogue · requirement 1");
});

test("actual UI manual action sends selected ID through the generated client for both mixed orders", async () => {
  const savedDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", { value: { cookie: "tasks_csrf=unit-request-protection" }, configurable: true });
  const requests: Request[] = [];
  const api = client.createTasksClient({ baseUrl: "https://tasks.invalid/tasks/api", getParentToken: async () => { throw new Error("Student manual actions must not request a parent credential"); }, fetch: async input => {
    assert.ok(input instanceof Request); requests.push(input);
    return Response.json({ status: "awaiting_verification" });
  } });
  const manual = { id: "selected-manual", kind: "parent_approval", interaction: "parent_action", attemptId: "", attemptStatus: "", dialogueStarted: false, historyAttemptId: "" };
  const dialogue = { ...manual, id: "unselected-dialogue", kind: "agent_dialogue", interaction: "chat" };
  try {
    for (const verification of [[dialogue, manual], [manual, dialogue]]) {
      const pending = { ...occurrence, status: "pending", verification };
      assert.equal(ui.manualActionState(pending, manual), "start");
      await ui.performManualAction(api, pending, manual, false);
      const active = { ...pending, status: "in_progress" };
      assert.equal(ui.manualActionState(active, manual), "submit");
      await ui.performManualAction(api, active, manual, true);
      // Another requirement awaiting/accepted does not pretend this one submitted.
      const waitingElsewhere = { ...active, status: "awaiting_verification" };
      assert.equal(ui.manualActionState(waitingElsewhere, manual), "submit");
      await ui.performManualAction(api, waitingElsewhere, manual, true);
      assert.equal(ui.manualActionState(waitingElsewhere, { ...manual, attemptId: "manual-attempt", attemptStatus: "open" }), "waiting");
      assert.equal(ui.manualActionState(waitingElsewhere, { ...manual, attemptStatus: "accepted" }), "accepted");
      assert.equal(ui.manualActionState({ ...waitingElsewhere, status: "canceled" }, manual), "closed");
      await assert.rejects(ui.performManualAction(api, waitingElsewhere, dialogue, true));
    }
    assert.equal(requests.length, 6);
    for (const request of requests) {
      const url = new URL(request.url);
      assert.equal(url.searchParams.get("requirementId"), manual.id);
      assert.match(url.pathname, /^\/tasks\/api\/student\/occurrences\/occurrence\/(start|submit)$/);
      assert.equal(request.headers.get("X-CSRF-Token"), "unit-request-protection");
      assert.equal(request.headers.get("Authorization"), null);
    }
  } finally {
    if (savedDocument) Object.defineProperty(globalThis, "document", savedDocument);
    else Reflect.deleteProperty(globalThis, "document");
  }
});

test("override request binds original version/attempt/key and does not mutate the timeline", () => {
  const timeline: DialogueInspect = { occurrenceId: "occurrence", attemptId: "attempt", requirementId: "reading", version: 8, title: "Careful reading", studentName: "Alice", status: "awaiting_verification", acceptedCount: 2, requiredCount: 3, sourceVersion: "v1", sourceSha256: "0".repeat(64), entries: [], attempts: [], attemptOffset: 0, attemptLimit: 10, attemptTotal: 1, nextCursor: 0, hasMore: false };
  const request = ui.makeOverride(timeline, "  Parent examined the original work.  ", true, "stable-request");
  assert.deepEqual(request, { attemptId: "attempt", expectedVersion: 8, clientRequestId: "stable-request", accepted: true, reason: "Parent examined the original work." });
  assert.equal(timeline.status, "awaiting_verification"); assert.equal(timeline.entries?.length, 0);
});

test("every inspector data-entry field uses its descriptive instance identity", async () => {
  const text = await readFile(path.join(import.meta.dirname, "OccurrenceInspectPage.tsx"), "utf8");
  const source = ts.createSourceFile("OccurrenceInspectPage.tsx", text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const identities = new Map<string, string>();
  const fields: Array<ts.JsxOpeningElement | ts.JsxSelfClosingElement> = [];
  const visit = (node: ts.Node) => {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer && ts.isCallExpression(node.initializer) && node.initializer.expression.getText(source) === "useInspectField") {
      const name = node.initializer.arguments[0];
      assert.ok(name && ts.isStringLiteral(name), "field names must be stable descriptive literals");
      identities.set(node.name.text, name.text);
    }
    if ((ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) && ["input", "select", "textarea"].includes(node.tagName.getText(source))) fields.push(node);
    ts.forEachChild(node, visit);
  };
  visit(source);
  const names = fields.map(field => {
    const identity = field.attributes.properties.filter(ts.isJsxSpreadAttribute).map(attribute => identities.get(attribute.expression.getText(source))).filter(name => name !== undefined);
    assert.equal(identity.length, 1, `${field.tagName.getText(source)} must attach its id/name identity, including conditional fields`);
    assert.ok(!field.attributes.properties.some(attribute => ts.isJsxAttribute(attribute) && ["id", "name"].includes(attribute.name.getText(source))), "do not override an instance identity with a duplicate constant");
    return identity[0];
  });
  assert.deepEqual(names.sort(), ["inspect-attempt", "inspect-requirement", "override-decision", "override-reason"]);
});

test("inspector field identities stay stable and unique across multiple component instances", () => {
  const names = ["inspect-requirement", "inspect-attempt", "override-decision", "override-reason"];
  function FieldInstance({ name, value, disabled }: { name: string; value: string; disabled: boolean }) {
    const identity = ui.useInspectField(name);
    return createElement("label", null, name, createElement(name === "override-reason" ? "textarea" : "select", { ...identity, value, disabled, onChange() {} },
      name === "override-reason" ? undefined : ["first", "second"].map(value => createElement("option", { key: value, value }, value))));
  }
  const render = (value: string, disabled: boolean) => renderToStaticMarkup(createElement("div", null,
    [0, 1].map(instance => createElement("section", { key: instance }, names.map(name => createElement(FieldInstance, { key: name, name, value, disabled }))))));
  const readFields = (markup: string) => [...markup.matchAll(/<(?:select|textarea)\b[^>]*>/g)].map(([tag]) => {
    const id = tag.match(/\bid="([^"]+)"/)?.[1];
    const name = tag.match(/\bname="([^"]+)"/)?.[1];
    assert.ok(id && name && id.startsWith(`${name}-`) && !/\s/.test(id));
    return { id, name };
  });
  const initial = render("first", false);
  const changed = render("second", true);
  const fields = readFields(initial);
  assert.equal(fields.length, 8);
  assert.equal(new Set(fields.map(field => field.id)).size, 8, "instances must not duplicate IDs");
  assert.deepEqual(fields.map(field => field.name), [...names, ...names]);
  assert.deepEqual(readFields(changed), fields, "changing values/disabled state must not change field identity");
  assert.doesNotMatch(initial, /disabled=""/);
  assert.equal([...changed.matchAll(/disabled=""/g)].length, 8);
  assert.match(initial, /value="first" selected=""/);
  assert.match(changed, /value="second" selected=""/);
});

test("original student text is escaped, distinct from provenance and evaluations", () => {
  const entry: NonNullable<DialogueInspect["entries"]>[number] = { sequence: 2, kind: "message_ack", author: "student", at: "2026-01-01T00:00:00Z", text: '<img src=x onerror="alert(1)">', criteria: [], inputTokens: 0, outputTokens: 0, policyVersion: "dialogue.v1" };
  const markup = renderToStaticMarkup(createElement(MemoryRouter, null, createElement(ui.InspectEntry, { entry })));
  assert.match(markup, /Student · original answer/); assert.match(markup, /&lt;img/); assert.doesNotMatch(markup, /<img/); assert.match(markup, /Provenance and usage/);
});
