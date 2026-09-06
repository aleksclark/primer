import assert from "node:assert/strict";
import { test } from "node:test";
import { IMPLEMENTED_EXTENSIONS, METADATA_EXTENSIONS, rejectUnsupportedExtensions } from "./generate-client.mjs";

test("implemented Go extensions are exactly the four client-owned constraints", () => {
  assert.deepEqual([...IMPLEMENTED_EXTENSIONS].sort(), ["x-equalFields", "x-maxBytes", "x-nonBlank", "x-uniqueNormalized"]);
  assert.deepEqual([...METADATA_EXTENSIONS].sort(), ["x-manifest", "x-transport"]);
});

test("unknown x- keys fail closed before generation", () => {
  assert.throws(
    () => rejectUnsupportedExtensions({ components: { schemas: { Example: { type: "string", "x-silentPass": true } } } }),
    /unsupported contract constraint x-silentPass/,
  );
});

test("known Go extensions and metadata are accepted", () => {
  rejectUnsupportedExtensions({
    components: {
      schemas: {
        Config: {
          type: "object",
          "x-manifest": { kind: "agent_dialogue" },
          properties: {
            sourceText: { type: "string", "x-maxBytes": 12000, "x-nonBlank": true },
            rubric: { type: "array", "x-uniqueNormalized": true },
          },
          "x-equalFields": ["sequence", "cursor"],
        },
      },
    },
    "x-transport": { protocol: "primer-tasks.student.v1" },
  });
});
