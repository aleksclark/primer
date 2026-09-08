import assert from "node:assert/strict";
import test from "node:test";
import { presentStudentTask } from "./student-task-presentation.ts";

test("student task presentation is derived from raw state", () => {
  assert.deepEqual(
    [
      presentStudentTask("in_progress"),
      presentStudentTask("pending", true),
      presentStudentTask("pending"),
      presentStudentTask("awaiting_verification"),
      presentStudentTask("completed"),
    ].map((state) => [state.label, state.priority, state.terminal]),
    [
      ["In progress", 0, false],
      ["Needs another try", 1, false],
      ["Ready to start", 2, false],
      ["Sent to your parent", 3, false],
      ["Approved", 5, true],
    ],
  );
});
