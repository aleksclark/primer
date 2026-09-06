/// <reference types="node" />
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { compilePreset, defaultSettings, localDateTime, saveScheduleTimes, settingsForSchedule, zonedStart, type ScheduleSettings, type ScheduleInput } from "./schedule-presets.ts";
import type { Schedule } from "@primer-tasks/client";

type Fixture = { name: string; settings: ScheduleSettings; expected: { kind: string; startAt: string; rrule: string }[] };
const fixtures: Fixture[] = JSON.parse(readFileSync(new URL("./schedule-preset-cases.json", import.meta.url), "utf8"));
for (const fixture of fixtures) {
  test(`compiles ${fixture.name} to the same contract exercised by PostgreSQL materialization tests`, () => {
    assert.deepEqual(compilePreset(fixture.settings).map(({ kind, startAt, rrule }) => ({ kind, startAt, rrule })), fixture.expected);
  });
}

test("wall time belongs to the chosen zone, with a stable DST policy", () => {
  assert.equal(zonedStart("2026-03-07T09:00", "America/New_York"), "2026-03-07T14:00:00.000Z");
  assert.equal(zonedStart("2026-03-09T09:00", "America/New_York"), "2026-03-09T13:00:00.000Z");
  assert.throws(() => zonedStart("2026-03-08T02:30", "America/New_York"), /clocks move forward/);
  assert.equal(zonedStart("2026-11-01T01:30", "America/New_York"), "2026-11-01T05:30:00.000Z");
  assert.equal(localDateTime("2026-11-01T05:30:00Z", "America/New_York"), "2026-11-01T01:30");
  assert.throws(() => zonedStart("2026-02-30T09:00", "UTC"), /valid date/);
  assert.throws(() => zonedStart("2026-02-01T09:00", "No/Such"), /valid time zone/);
});

test("invalid presets fail before any schedules are created", () => {
  const base = defaultSettings("UTC");
  assert.throws(() => compilePreset({ ...base, preset: "weekly", weekdays: [] }), /weekday/);
  assert.throws(() => compilePreset({ ...base, preset: "interval", interval: 31 }), /interval/);
  assert.throws(() => compilePreset({ ...base, preset: "daily", count: "367" }), /repeat limit/);
  assert.throws(() => compilePreset({ ...base, preset: "multiple", times: ["09:00", "09:00"] }), /different clock times/);
  assert.throws(() => compilePreset({ ...base, date: "2026-03-07", endAt: "2026-03-06T09:00" }), /end must be after/);
});

test("schedule editing round-trips supported presets and preserves advanced rules", () => {
  for (const fixture of fixtures) {
    for (const input of compilePreset(fixture.settings)) {
      const schedule: Schedule = { ...input, id: "schedule", studentId: "student", templateId: "template", revisionId: "revision", dueOffsetMinutes: 5, enabled: true, version: 1 };
      assert.deepEqual(compilePreset(settingsForSchedule(schedule)), [input]);
    }
  }
  const advanced: Schedule = { id: "schedule", studentId: "student", templateId: "template", revisionId: "revision", dueOffsetMinutes: 5, enabled: true, version: 1, kind: "recurrence", timezone: "America/New_York", startAt: "2026-03-07T14:00:00.000Z", rrule: "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;UNTIL=20261201T000000Z", endAt: "2026-11-30T23:00:00.000Z" };
  const settings = settingsForSchedule(advanced);
  assert.equal(settings.preset, "advanced");
  const [compiled] = compilePreset(settings);
  assert.equal(compiled.rrule, advanced.rrule);
  assert.equal(compiled.endAt, advanced.endAt);
});

test("multiple times report each success and failure and never retry a created time", async () => {
  const inputs: ScheduleInput[] = compilePreset(fixtures[4].settings).map((input) => ({ ...input, studentId: "student", templateId: "template", revisionId: "revision", dueOffsetMinutes: 0 }));
  const calls: string[] = [];
  const results = await saveScheduleTimes(inputs, async (input) => {
    calls.push(input.startAt);
    if (calls.length === 2) throw new Error("connection lost");
    return { id: "saved-morning" };
  });
  assert.deepEqual(calls, inputs.map((input) => input.startAt));
  assert.equal(results[0].id, "saved-morning");
  assert.equal(results[1].id, undefined);
  assert.match(String(results[1].error), /connection lost/);
  const followingCalls: string[] = [];
  const following = await saveScheduleTimes(inputs, async (input) => { followingCalls.push(input.startAt); if (followingCalls.length === 1) throw new Error("first failed"); return { id: "saved-evening" }; });
  assert.equal(following[1].id, "saved-evening");
  assert.equal(followingCalls.length, 2);
});
