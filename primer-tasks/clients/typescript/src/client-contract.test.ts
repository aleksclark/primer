import assert from "node:assert/strict";
import test from "node:test";
import { createTasksClient, TasksApiError } from "./index.ts";

const occurrenceId = "00000000-0000-0000-0000-000000000001";
const taskId = "00000000-0000-0000-0000-000000000002";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const inspect = {
  occurrenceId,
  status: "awaiting_verification",
  acceptedCount: 1,
  requiredCount: 2,
  entries: [],
  overrides: [],
};

test("parent contract façade uses generated inspect, fallback, retry, and cancel operations", async () => {
  const requests: Request[] = [];
  const client = createTasksClient({
    baseUrl: "http://tasks.test/api",
    fetch: async (input, init) => {
      const request = new Request(input, init);
      requests.push(request);
      const path = new URL(request.url).pathname;
      if (path.endsWith("/inspect") || path.endsWith("/override")) return response(inspect);
      return response({ occurrenceId, status: "pending" });
    },
  });

  const inspected = await client.inspectOccurrence(occurrenceId);
  assert.equal(inspected.occurrenceId, inspect.occurrenceId);
  assert.equal(inspected.status, inspect.status);
  assert.equal(inspected.acceptedCount, inspect.acceptedCount);
  const overridden = await client.overrideOccurrence(occurrenceId, { accepted: true, reason: "Parent reviewed the saved evidence." });
  assert.equal(overridden.status, inspect.status);
  assert.equal(overridden.requiredCount, inspect.requiredCount);
  await client.retryOccurrence(occurrenceId);
  await client.cancelOccurrence(occurrenceId);

  assert.deepEqual(requests.map((request) => `${request.method} ${new URL(request.url).pathname}`), [
    `GET /api/occurrences/${occurrenceId}/inspect`,
    `POST /api/occurrences/${occurrenceId}/override`,
    `POST /api/occurrences/${occurrenceId}/retry`,
    `POST /api/occurrences/${occurrenceId}/cancel`,
  ]);
  assert.deepEqual(JSON.parse(await requests[1]!.text()), { accepted: true, reason: "Parent reviewed the saved evidence." });
});

test("student dialogue state and task revision stay on generated REST contracts", async () => {
  const requests: Request[] = [];
  const client = createTasksClient({
    baseUrl: "http://tasks.test/api",
    fetch: async (input, init) => {
      const request = new Request(input, init);
      requests.push(request);
      if (new URL(request.url).pathname.endsWith("/dialogue")) {
        return response({ occurrenceId, attemptId: "attempt-1", conversationId: "attempt-1", status: "in_progress", acceptedCount: 1, requiredCount: 2, currentQuestion: "What happened?" });
      }
      return response({ id: "revision-2", templateId: taskId, version: 2, title: "Revised task", instructions: "Finish it.", status: "draft", requirements: [], createdAt: "2026-01-01T00:00:00Z" }, 201);
    },
  });

  const state = await client.studentDialogue(occurrenceId);
  assert.equal(state.status, "in_progress");
  await client.reviseTask(taskId, { title: "Revised task", instructions: "Finish it.", requirements: [] });

  assert.equal(new URL(requests[0]!.url).pathname, `/api/student/occurrences/${occurrenceId}/dialogue`);
  assert.equal(new URL(requests[1]!.url).pathname, `/api/tasks/${taskId}/revisions`);
  assert.deepEqual(JSON.parse(await requests[1]!.text()), { title: "Revised task", instructions: "Finish it.", requirements: [] });
});

test("unsafe or malformed contract projections fail closed", async () => {
  const client = createTasksClient({
    baseUrl: "http://tasks.test/api",
    fetch: async () => response({ occurrenceId, status: "awaiting_verification", entries: [{ id: "x", kind: "evaluation", at: "2026-01-01T00:00:00Z", author: "agent", body: "safe", rawPrompt: "hidden" }], overrides: [] }),
  });

  const timeline = await client.inspectOccurrence(occurrenceId);
  assert.equal(timeline.entries.length, 1);
  await assert.rejects(
    createTasksClient({ baseUrl: "http://tasks.test/api", fetch: async () => response({}) }).inspectOccurrence(occurrenceId),
    (error: unknown) => error instanceof TasksApiError && error.status === 502,
  );
});
