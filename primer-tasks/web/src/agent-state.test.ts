/// <reference types="node" />
import { test } from "node:test";
import assert from "node:assert/strict";
import { assistantText, latestRunStatus, pendingConfirmation } from "./agent-state.ts";
import type { AgentEvent } from "@primer-tasks/client";

const base = { protocol: 1 as const, sequence: 1, cursor: 1, time: "2026-09-06T00:00:00Z", runId: "run-one" };
const preview: AgentEvent = { ...base, kind: "tool_progress", label: "Prepare change", phase: "awaiting_confirmation", confirmationId: "opaque-preview" };

test("durable stale/canceled terminal removes obsolete confirmation after live delivery or replay", () => {
  assert.equal(pendingConfirmation([preview])?.confirmationId, "opaque-preview");
  for (const status of ["failed", "cancelled", "completed"]) {
    const events: AgentEvent[] = [preview, { ...base, sequence: 2, cursor: 2, kind: "terminal", status }];
    assert.equal(pendingConfirmation(events), undefined);
    assert.equal(pendingConfirmation(JSON.parse(JSON.stringify(events))), undefined);
    assert.equal(latestRunStatus(events), status);
  }
});

test("credential-refreshable rejection leaves version-valid preview actionable", () => {
  const events: AgentEvent[] = [preview, { ...base, sequence: 2, cursor: 2, kind: "error", code: "confirmation_rejected" }];
  assert.equal(pendingConfirmation(events)?.confirmationId, "opaque-preview");
  assert.equal(latestRunStatus(events), "awaiting_confirmation");
});

test("old handle errors/terminal cannot retarget or retire a fresh proposal", () => {
  const events: AgentEvent[] = [preview,
    { ...base, sequence: 2, cursor: 2, kind: "terminal", status: "failed" },
    { ...base, sequence: 3, cursor: 3, runId: "run-new", kind: "user_message", clientMessageId: "new", text: "Fresh request" },
    { ...preview, sequence: 4, cursor: 4, runId: "run-new", confirmationId: "fresh-preview" },
    { ...base, sequence: 5, cursor: 5, kind: "error", code: "confirmation_rejected" },
    { ...base, sequence: 6, cursor: 6, kind: "terminal", status: "failed" },
  ];
  assert.equal(pendingConfirmation(events)?.runId, "run-new");
  assert.equal(pendingConfirmation(events)?.confirmationId, "fresh-preview");
  assert.equal(latestRunStatus(events), "awaiting_confirmation");
});

test("committed receipt clauses stream and authoritative final text replaces rather than duplicates", () => {
  let text = assistantText("", { ...base, kind: "text_start", source: "domain" });
  text = assistantText(text, { ...base, kind: "text_delta", text: 'Published "Careful work".', source: "domain" });
  text = assistantText(text, { ...base, kind: "text_delta", text: ' Scheduled it for Alice.', source: "domain" });
  const complete = 'Published "Careful work". Scheduled it for Alice.';
  assert.equal(text, complete);
  const end: AgentEvent = { ...base, kind: "text_end", text: complete, source: "domain" };
  assert.equal(assistantText(text, end), complete);
  assert.equal(assistantText(assistantText(text, end), end), complete);
  assert.equal(assistantText("only the last delta", end), complete);
});
