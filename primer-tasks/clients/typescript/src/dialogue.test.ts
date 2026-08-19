import assert from "node:assert/strict";
import test from "node:test";
import {
  AGENT_DIALOGUE_KIND,
  defaultDialogueConfig,
  dialogueRequirement,
  parseInspectTimeline,
  parseStudentDialogueState,
  previewDialogueConfig,
  stripUnsafeDialogueFields,
  studentProgressCopy,
  validateDialogueConfig,
  validateOverrideInput,
} from "./dialogue.ts";

const valid = {
  ...defaultDialogueConfig(),
  source: "The garden wall must dry before the next course of stones.",
  learningFocus: "Recall three facts from the chapter.",
  rubric: "Accept only distinct source facts.",
};

test("valid dialogue config previews parent-owned fields only", () => {
  const issues = validateDialogueConfig(valid);
  assert.deepEqual(issues, []);
  const preview = previewDialogueConfig(valid);
  assert.ok(preview);
  assert.equal(preview.kind, AGENT_DIALOGUE_KIND);
  assert.equal(preview.hiddenPromptExposed, false);
  assert.equal(preview.requiredAcceptedQuestions, 3);
  assert.match(preview.summary, /3 accepted answers/);
  assert.doesNotMatch(JSON.stringify(preview), /systemPrompt|reasoning|score/i);
  const requirement = dialogueRequirement("agent-dialogue", valid);
  assert.equal(requirement.interaction, "chat");
  assert.equal(requirement.executor, "fantasy");
  assert.equal((requirement.config as unknown as { sourceText: string }).sourceText, valid.source);
});

test("dialogue config rejects empty source, hidden-count, and oversized fields", () => {
  assert.ok(validateDialogueConfig({ ...valid, source: "" }).some((issue) => issue.field === "source"));
  assert.ok(validateDialogueConfig({ ...valid, requiredAcceptedQuestions: 0 }).some((issue) => issue.field === "requiredAcceptedQuestions"));
  assert.ok(validateDialogueConfig({ ...valid, maxTurns: 2 }).some((issue) => issue.field === "maxTurns"));
  assert.ok(validateDialogueConfig({ ...valid, retentionDays: 0 }).some((issue) => issue.field === "retentionDays"));
});

test("inspect timeline strips reasoning and scores and keeps student text separate", () => {
  const timeline = parseInspectTimeline({
    occurrenceId: "occ-1",
    status: "awaiting_verification",
    acceptedCount: 1,
    requiredCount: 3,
    provider: "scripted",
    policyVersion: "dialogue.v1",
    reasoning: "hidden chain",
    rawScore: 97,
    entries: [{
      id: "q1",
      kind: "question",
      at: "2026-08-19T14:00:00Z",
      author: "agent",
      title: "Question",
      body: "What must dry before the next course?",
      reasoning: "do not show",
      score: 12,
    }, {
      id: "a1",
      kind: "student_answer",
      at: "2026-08-19T14:01:00Z",
      author: "student",
      title: "Student answer",
      body: "The mortar.",
    }, {
      id: "e1",
      kind: "evaluation",
      at: "2026-08-19T14:01:30Z",
      author: "agent",
      title: "Evaluation",
      body: "Accepted for naming mortar.",
      outcome: "accepted",
      rationale: "Names a source fact.",
      provider: "scripted",
      model: "fixture",
      policyVersion: "dialogue.v1",
      usage: { inputTokens: 20, outputTokens: 8 },
      chainOfThought: "secret",
    }],
    overrides: [],
  });
  assert.ok(timeline);
  assert.equal(timeline.entries.length, 3);
  assert.equal(timeline.entries[0]?.author, "agent");
  assert.equal(timeline.entries[1]?.author, "student");
  assert.equal(timeline.entries[2]?.outcome, "accepted");
  assert.equal(timeline.entries[2]?.rationale, "Names a source fact.");
  const serialized = JSON.stringify(timeline);
  assert.doesNotMatch(serialized, /hidden chain|rawScore|chainOfThought|secret|score/i);
});

test("override input is explicit and does not rewrite evidence", () => {
  assert.equal(validateOverrideInput({ accepted: true, reason: "short" }), "Override reason must be at least 8 characters.");
  assert.equal(validateOverrideInput({ accepted: true, reason: "Parent observed the reading conversation." }), null);
  const before = parseInspectTimeline({
    occurrenceId: "occ-1",
    status: "failed",
    acceptedCount: 1,
    requiredCount: 3,
    entries: [{ id: "a1", kind: "student_answer", at: "2026-08-19T14:01:00Z", author: "student", title: "Student answer", body: "The mortar." }],
    overrides: [],
  });
  const after = parseInspectTimeline({
    ...before,
    overrides: [{ id: "ov-1", accepted: true, reason: "Parent reviewed the transcript.", actorId: "parent-a", createdAt: "2026-08-19T15:00:00Z" }],
  });
  assert.equal(before?.entries[0]?.body, after?.entries[0]?.body);
  assert.equal(after?.overrides.length, 1);
  assert.equal(after?.overrides[0]?.reason, "Parent reviewed the transcript.");
});

test("student state never exposes scores or reasoning and reports progress without gamification", () => {
  const state = parseStudentDialogueState({
    occurrenceId: "occ-1",
    attemptId: "att-1",
    conversationId: "con-1",
    status: "in_progress",
    acceptedCount: 2,
    requiredCount: 3,
    currentQuestion: "What happens if the work is rushed?",
    rawScore: 88,
    reasoning: "hidden",
  });
  assert.ok(state);
  assert.equal(studentProgressCopy(state), "2 of 3 accepted answers");
  assert.equal(studentProgressCopy({ ...state, status: "complete" }), "Verification complete.");
  assert.doesNotMatch(JSON.stringify(state), /rawScore|reasoning|88/);
  assert.equal(parseStudentDialogueState({ occurrenceId: "occ-1", status: "in_progress" }), null);
});

test("unsafe fields are stripped from nested inspect payloads", () => {
  const cleaned = stripUnsafeDialogueFields({
    entries: [{ body: "ok", reasoningDelta: "nope", providerMetadata: { rawPrompt: "x" } }],
    systemPrompt: "never",
  });
  assert.deepEqual(cleaned, { entries: [{ body: "ok" }] });
});
