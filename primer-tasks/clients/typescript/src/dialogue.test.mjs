import { test, after } from "node:test";
import assert from "node:assert/strict";
import { loadTestClient } from "../scripts/client-test-support.mjs";

const loaded = await loadTestClient();
after(loaded.cleanup);
const client = loaded.client;
const occurrenceId = "00000000-0000-0000-0000-000000000001", attemptId = "00000000-0000-0000-0000-000000000002";
const requirementId = "00000000-0000-0000-0000-000000000003", questionId = "00000000-0000-0000-0000-000000000004", messageId = "00000000-0000-0000-0000-000000000005";
const base = { protocol: 1, time: "2026-09-06T12:00:00Z", sequence: 0, cursor: 0, acceptedCount: 0, requiredCount: 3 };
const binding = { occurrenceId, attemptId, requirementId, policyVersion: "dialogue.v1", snapshotDigest: "a".repeat(64), version: 2 };
const state = { ...base, ...binding, kind: "state", questionId, text: "What is the current fact?", status: "open", occurrenceStatus: "awaiting_verification" };
const config = { sourceText: "A bounded parent source.", learningFocus: "Read precisely", requiredQuestions: 3, rubric: ["source detail"], allowedFollowUps: 1, maxAttempts: 2, maxTurns: 8, retentionPolicy: "retain" };

function browser() {
  globalThis.window = { location: new URL("https://primer.example/tasks/student") };
  globalThis.document = { cookie: "tasks_csrf=fixture-csrf" };
}
class Socket {
  readyState = 0; sent = []; closed = [];
  send(text) { this.sent.push(JSON.parse(text)); }
  open() { this.readyState = 1; this.onopen?.(); }
  receive(event) { this.onmessage?.({ data: JSON.stringify(event) }); }
  close(code = 1000) { this.readyState = 3; this.closed.push(code); this.onclose?.({ code }); }
}

test("generated runtime guards reject wrong variants, bindings, statuses and unknown properties", () => {
  assert.ok(client.parseStudentDialogueEvent(state));
  assert.ok(client.parseStudentDialogueEvent({ ...state, phase: "thinking" }));
  assert.equal(client.parseStudentDialogueEvent({ ...state, phase: "thinking", code: "provider_unavailable" }), null);
  assert.equal(client.parseStudentDialogueEvent({ ...state, phase: "failed" }), null);
  assert.equal(client.parseStudentDialogueEvent({ ...state, status: "accepted", acceptedCount: 3 }), null);
  for (const change of [{ kind: "reasoning_delta" }, { status: "completed" }, { policyVersion: "other" }, { snapshotDigest: "bad" }, { version: 0 }, { cursor: 1 }, { tenantId: "client authority" }, { sourceText: "private" }, { provider_metadata: {} }]) assert.equal(client.parseStudentDialogueEvent({ ...state, ...change }), null);
  assert.equal(client.parseStudentDialogueEvent(JSON.parse(JSON.stringify(state).replace(/}$/, ',"constructor":{}}'))), null);
  const missing = { ...state }; delete missing.requirementId; assert.equal(client.parseStudentDialogueEvent(missing), null);
  const complete = { ...base, ...binding, kind: "complete", sequence: 4, cursor: 4, status: "accepted", occurrenceStatus: "completed", decisionSource: "verification_engine", decisionId: messageId, acceptedCount: 3 };
  assert.ok(client.parseStudentDialogueEvent(complete));
  assert.equal(client.parseStudentDialogueEvent({ ...complete, acceptedCount: 2 }), null);
  assert.equal(client.parseStudentDialogueEvent({ ...complete, status: "rejected" }), null);
  assert.ok(client.parseStudentDialogueEvent({ ...complete, decisionSource: "parent_override", acceptedCount: 0 }));
  assert.ok(client.isStudentDialogueCommand({ protocol: 1, kind: "ack", cursor: 0 }));
  for (const command of [{ protocol: 1, kind: "ack", cursor: -1 }, { protocol: 1, kind: "ack", cursor: 1, text: "extra" }, { protocol: 1, kind: "complete" }, { protocol: 1, kind: "user_message", text: "missing binding" }]) assert.equal(client.isStudentDialogueCommand(command), false);
});

test("parent configuration derives Go limits and cannot provide a published plan or student authority", () => {
  assert.ok(client.parseDialogueConfig(config));
  assert.ok(client.parseDialogueConfig({ ...config, sourceText: "A different inline chapter." }));
  for (const change of [{ retentionDays: 30 }, { retentionPolicy: "redact" }, { requiredQuestions: 1 }, { allowedFollowUps: 6 }, { questions: [] }, { questionPlanVersion: "forged" }, { sourceRef: "https://foreign/source" }, { rubric: ["Fact", " fact "] }, { sourceText: "é".repeat(6001) }]) assert.equal(client.parseDialogueConfig({ ...config, ...change }), null);
  assert.equal(client.dialogueRequirement(config).kind, "agent_dialogue");
  assert.equal(client.dialogueRequirement(config).config.retentionPolicy, "retain");
});

test("reducer never converts partial acceptance or exhausted/rejected state into completion", () => {
  let value = { connectionState: "connected", cursor: 0, events: [], canAnswer: false, completed: false };
  value = client.reduceDialogueEvent(value, state);
  value = client.reduceDialogueEvent(value, { ...base, ...binding, kind: "answer_evaluation", sequence: 2, cursor: 2, version: 4, questionId, messageId, text: "Safe feedback.", status: "accepted", acceptedCount: 1 });
  assert.equal(value.completed, false); assert.equal(value.canAnswer, false);
  value = client.reduceDialogueEvent(value, { ...state, version: 4, status: "exhausted", occurrenceStatus: "pending", decisionSource: "verification_engine" });
  assert.equal(value.completed, false); assert.equal(value.canAnswer, false);
  const replay = client.reduceDialogueEvent(value, { ...state, version: 2 });
  assert.equal(replay.status, "exhausted");
});

test("student facade owns /tasks/api socket auth, current CAS and identical pending replay", () => {
  browser(); const sockets = [], transports = [];
  const transport = client.createDialogueClient({ occurrenceId, attemptId, baseUrl: "/tasks/api", reconnect: false, randomId: () => "stable-client-key", webSocketFactory: (url, protocols) => { transports.push({ url, protocols }); const socket = new Socket(); sockets.push(socket); return socket; } });
  transport.connect(); sockets[0].open(); sockets[0].receive(state);
  assert.equal(transports[0].url, "wss://primer.example/tasks/api/student/ws");
  assert.deepEqual(transports[0].protocols, ["primer-tasks.student.v1", "primer-tasks.v1.csrf.fixture-csrf"]);
  transport.sendMessage("A complete answer.");
  const command = sockets[0].sent.find(value => value.kind === "user_message");
  assert.equal(command.expectedVersion, 2); assert.equal(command.questionId, questionId); assert.equal(command.snapshotDigest, binding.snapshotDigest);
  assert.throws(() => transport.sendMessage("concurrent"));
  transport.disconnect(); transport.connect(); sockets[1].open();
  assert.deepEqual(sockets[1].sent.find(value => value.kind === "user_message"), command);
  sockets[1].receive({ ...base, ...binding, kind: "message_ack", version: 3, sequence: 1, cursor: 1, questionId, messageId, clientMessageId: "stable-client-key", text: "A complete answer." });
  sockets[1].receive({ ...base, ...binding, kind: "answer_evaluation", version: 4, sequence: 2, cursor: 2, questionId, messageId, status: "rejected", text: "Add a source detail." });
  transport.sendMessage("Another complete answer.");
  assert.equal(sockets[1].sent.filter(value => value.kind === "user_message").at(-1).expectedVersion, 4);
  sockets[1].close(1008); assert.equal(transport.snapshot().connectionState, "revoked");
});

test("busy state and stale-CAS conflict recover without silently rebasing or losing text", () => {
  browser(); const socket = new Socket(); let id = 0;
  const value = client.createDialogueClient({ occurrenceId, attemptId, reconnect: false, randomId: () => `attempt-${++id}`, webSocketFactory: () => socket });
  value.connect(); socket.open(); socket.receive({ ...state, phase: "evaluating" });
  assert.equal(value.snapshot().canAnswer, false); assert.throws(() => value.sendMessage("while busy"));
  socket.receive(state); value.sendMessage("Keep this unsaved answer.");
  socket.receive({ ...base, requiredCount: 0, kind: "error", code: "conflict", retryable: true });
  assert.equal(value.snapshot().unsentAnswer, "Keep this unsaved answer.");
  assert.equal(socket.sent.filter(command => command.kind === "subscribe").length, 2);
  socket.receive({ ...state, version: 5 });
  value.sendMessage("Explicitly revised answer.");
  const latest = socket.sent.filter(command => command.kind === "user_message").at(-1);
  assert.equal(latest.expectedVersion, 5); assert.equal(latest.clientMessageId, "attempt-2");
  assert.equal(value.snapshot().unsentAnswer, undefined);
});

test("invalid/cross-attempt frames and URL credential lanes fail closed", () => {
  browser(); let created = false;
  const bad = client.createDialogueClient({ occurrenceId, attemptId, baseUrl: "https://foreign.example/api", webSocketFactory: () => { created = true; return new Socket(); } });
  bad.connect(); assert.equal(created, false); assert.equal(bad.snapshot().connectionState, "offline");
  const socket = new Socket(); const value = client.createDialogueClient({ occurrenceId, attemptId, reconnect: false, webSocketFactory: () => socket });
  value.connect(); socket.open(); socket.receive({ ...state, attemptId: requirementId });
  assert.deepEqual(socket.closed, [1002]); assert.equal(value.snapshot().completed, false);
});

test("REST facade uses generated operations and existing Clerk middleware, with student-only CSRF", async () => {
  browser(); const requests = []; let tokens = 0;
  const api = client.createTasksClient({ baseUrl: "https://primer.example/tasks/api", getParentToken: async () => { tokens++; return "ephemeral-parent"; }, fetch: async request => {
    requests.push(request);
    if (request.url.endsWith("/dialogue")) return new Response(JSON.stringify({ $schema: "https://primer.example/DialogueEvent.json", ...state }), { status: 200, headers: { "Content-Type": "application/json" } });
    if (request.url.endsWith("/override")) return new Response(JSON.stringify({ occurrenceId, attemptId, overrideId: messageId, accepted: true, resultStatus: "completed", resultVersion: 3 }), { status: 200, headers: { "Content-Type": "application/json" } });
    return new Response(JSON.stringify({ code: "not_found", message: "Unavailable" }), { status: 404, headers: { "Content-Type": "application/json" } });
  } });
  assert.equal((await api.startStudentDialogue(occurrenceId)).kind, "state");
  assert.equal(requests[0].headers.get("Authorization"), null); assert.equal(requests[0].headers.get("X-CSRF-Token"), "fixture-csrf");
  const result = await api.overrideOccurrence(occurrenceId, { attemptId, clientRequestId: "request", expectedVersion: 2, accepted: true, reason: "Parent reviewed the work." });
  assert.equal(result.overrideId, messageId); assert.equal(requests[1].headers.get("Authorization"), "Bearer ephemeral-parent");
  await assert.rejects(api.inspectOccurrence(occurrenceId), error => error instanceof client.TasksApiError && error.status === 404);
  assert.equal(tokens, 2); assert.equal(requests[2].credentials, "include");
});
