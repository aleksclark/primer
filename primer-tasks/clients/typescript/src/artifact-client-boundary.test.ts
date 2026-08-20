import assert from "node:assert/strict";
import test from "node:test";
import { createTasksClient } from "./index.ts";

const state = {
  occurrenceId: "00000000-0000-0000-0000-000000000001",
  requirementId: "00000000-0000-0000-0000-000000000002",
  rubricRevision: "00000000-0000-0000-0000-000000000002",
  config: {
    acceptedKinds: ["image"],
    maxBytes: 1000,
    criteria: [{ id: "shows-work", label: "Shows work", description: "The work is visible.", required: true }],
    passRule: "all_required",
    reviewPolicy: "parent_review",
  },
  status: "ready",
  submissions: [],
};

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

test("artifact JSON façade uses generated REST routes and typed wire projections", async () => {
  const requests: Request[] = [];
  const client = createTasksClient({
    baseUrl: "http://tasks.test/api",
    fetch: async (input, init) => {
      const request = new Request(input, init);
      requests.push(request);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/artifacts/reserve")) {
        return response({ reservationId: "00000000-0000-0000-0000-000000000003", artifactId: "00000000-0000-0000-0000-000000000004", idempotencyKey: "idem", uploadUrl: "/api/student/artifacts/00000000-0000-0000-0000-000000000004/upload", partCount: 1, expiresAt: "2026-01-01T00:00:00Z" }, 201);
      }
      if (url.pathname.endsWith("/finalize")) return response({ id: "00000000-0000-0000-0000-000000000004", kind: "image", contentType: "image/png", size: 4, status: "finalized" });
      return response(state);
    },
  });

  const reservation = await client.reserveArtifact(state.occurrenceId, { kind: "image", mediaType: "image/png", sizeBytes: 4, idempotencyKey: "idem" });
  assert.equal(reservation.artifactId, "00000000-0000-0000-0000-000000000004");
  assert.equal(new URL(requests[0]!.url).pathname, "/api/student/occurrences/00000000-0000-0000-0000-000000000001/artifacts/reserve");
  assert.deepEqual(JSON.parse(await requests[0]!.text()), { kind: "image", contentType: "image/png", size: 4, idempotencyKey: "idem" });

  const next = await client.finalizeArtifact(state.occurrenceId, { artifactId: reservation.artifactId, digest: "a".repeat(64), sizeBytes: 4, mediaType: "image/png" });
  assert.equal(next.occurrenceId, state.occurrenceId);
  assert.match(new URL(requests[1]!.url).pathname, /\/artifacts\/finalize$/);
  assert.deepEqual(JSON.parse(await requests[1]!.text()), { artifactId: reservation.artifactId, digest: "a".repeat(64), sizeBytes: 4, mediaType: "image/png" });
  assert.equal(new URL(requests[2]!.url).pathname, "/api/student/occurrences/00000000-0000-0000-0000-000000000001/artifacts");

  const retried = await client.retryArtifactEvaluation(state.occurrenceId);
  assert.equal(retried.status, "ready");
  const inspected = await client.inspectOccurrenceArtifacts(state.occurrenceId);
  assert.equal(inspected.requirementId, state.requirementId);
  assert.match(new URL(requests[3]!.url).pathname, /\/artifacts\/retry$/);
  assert.match(new URL(requests[4]!.url).pathname, /\/artifacts\/inspect$/);
});

test("binary artifact façade rejects foreign, signed, and non-artifact targets", async () => {
  const previousWindow = globalThis.window;
  Object.defineProperty(globalThis, "window", { configurable: true, value: { location: { origin: "http://tasks.test" } } });
  try {
    const client = createTasksClient({ baseUrl: "http://tasks.test/api", fetch: async () => response({}) });
    const reservation = {
      reservationId: "00000000-0000-0000-0000-000000000003",
      idempotencyKey: "idem",
      artifactId: "00000000-0000-0000-0000-000000000004",
      uploadUrl: "https://evil.example/api/student/artifacts/00000000-0000-0000-0000-000000000004/upload",
      partCount: 1,
      expiresAt: "2026-01-01T00:00:00Z",
      occurrenceId: state.occurrenceId,
      kind: "image" as const,
      mediaType: "image/png",
      maxBytes: 1000,
    };
    for (const uploadUrl of [
      reservation.uploadUrl,
      "http://tasks.test/api/other/artifacts/00000000-0000-0000-0000-000000000004/upload",
      "http://tasks.test/api/student/artifacts/00000000-0000-0000-0000-000000000004/upload?X-Amz-Signature=secret",
      "http://tasks.test/api/student/artifacts/00000000-0000-0000-0000-000000000004/derivative/thumbnail",
    ]) {
      await assert.rejects(
        client.uploadArtifact({ ...reservation, uploadUrl }, {} as File),
        (error: unknown) => error instanceof Error && error.message.includes("invalid artifact target"),
      );
    }
  } finally {
    if (previousWindow === undefined) delete (globalThis as { window?: unknown }).window;
    else Object.defineProperty(globalThis, "window", { configurable: true, value: previousWindow });
  }
});
