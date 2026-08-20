import assert from "node:assert/strict";
import test from "node:test";
import {
  artifactRubricRequirement,
  defaultArtifactRubricConfig,
  normalizeArtifactRubricConfig,
  parseArtifactProgressEvent,
  parseArtifactStudentState,
  previewArtifactRubric,
  validateArtifactFile,
  validateArtifactRubric,
} from "./artifact.ts";

test("artifact rubric preview exposes bounded parent fields only", () => {
  const config = defaultArtifactRubricConfig();
  const preview = previewArtifactRubric(config);
  assert.ok(preview);
  assert.equal(preview.hiddenPromptExposed, false);
  assert.equal(preview.requiredCriteria, 1);
  assert.equal("reasoning" in preview, false);
  const requirement = artifactRubricRequirement("artifact-rubric", config);
  assert.equal(requirement.kind, "agent_artifact_rubric");
  assert.equal(requirement.interaction, "artifact_upload");
});

test("artifact rubric rejects missing criteria and duplicate identifiers", () => {
  const issues = validateArtifactRubric({ acceptedKinds: ["image"], maxBytes: 100, criteria: [
    { id: "same", label: "One", description: "A", required: true },
    { id: "same", label: "Two", description: "B", required: true },
  ] });
  assert.ok(issues.some((issue) => issue.field.endsWith(".id")));
  assert.equal(validateArtifactRubric({ acceptedKinds: [], criteria: [] }).length > 0, true);
});

test("file validation is media-kind and byte bounded", () => {
  const config = normalizeArtifactRubricConfig({ acceptedKinds: ["image"], maxBytes: 10, criteria: [{ id: "one", label: "One", description: "A" }] });
  assert.equal(validateArtifactFile({ name: "poem.png", type: "image/png", size: 10 }, config), null);
  assert.match(validateArtifactFile({ name: "poem.mp4", type: "video/mp4", size: 1 }, config) ?? "", /not allowed/i);
  assert.match(validateArtifactFile({ name: "large.png", type: "image/png", size: 11 }, config) ?? "", /larger/i);
});

test("artifact projection drops unsafe fields and preserves criterion feedback", () => {
  const state = parseArtifactStudentState({
    occurrenceId: "occ-1",
    status: "evaluating",
    config: { acceptedKinds: ["image"], maxBytes: 1000, criteria: [{ id: "one", label: "One", description: "Show it", required: true }] },
    submissions: [{ id: "submission-1", artifactId: "artifact-1", kind: "image", mediaType: "image/png", sizeBytes: 4, status: "evaluating", createdAt: "2026-01-01T00:00:00Z" }],
    evaluation: { status: "rejected", accepted: false, reasoning: "do not expose", criteria: [{ id: "eval-1", criterionId: "one", required: true, status: "rejected", feedback: "Try a clearer picture." }] },
  });
  assert.ok(state);
  assert.equal(state.evaluation?.criteria[0]?.feedback, "Try a clearer picture.");
  assert.equal((state.evaluation as unknown as Record<string, unknown>).reasoning, undefined);
});

test("artifact progress accepts safe lifecycle events and rejects unknown kinds", () => {
  const event = parseArtifactProgressEvent({ protocol: 1, kind: "progress", cursor: 2, sequence: 2, phase: "evaluating", rawPrompt: "hidden" });
  assert.equal(event?.phase, "evaluating");
  assert.equal((event as unknown as Record<string, unknown>).rawPrompt, undefined);
  assert.equal(parseArtifactProgressEvent({ protocol: 1, kind: "model_reasoning", cursor: 3, sequence: 3 }), null);
});
