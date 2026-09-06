import type { Schedule, tasksClient } from "@primer-tasks/client";

export type ScheduleInput = Parameters<typeof tasksClient.createSchedule>[0];
export type Preset = "once" | "daily" | "weekly" | "interval" | "multiple" | "advanced";
export type ScheduleSettings = {
  preset: Preset;
  date: string;
  times: string[];
  timezone: string;
  interval: number;
  weekdays: string[];
  count: string;
  rrule: string;
  endAt: string;
};
export const weekdays = [
  ["MO", "Monday"], ["TU", "Tuesday"], ["WE", "Wednesday"], ["TH", "Thursday"],
  ["FR", "Friday"], ["SA", "Saturday"], ["SU", "Sunday"],
] as const;

export function localDateTime(instant: string, timezone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: timezone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).formatToParts(new Date(instant));
  const part = (name: string) => parts.find((p) => p.type === name)?.value;
  return `${part("year")}-${part("month")}-${part("day")}T${part("hour")}:${part("minute")}`;
}

/** Interpret wall-clock input in the selected zone, not the browser's zone.
 * Reject a missing spring-forward time. Pick the first fold instant as the
 * anchor; the server emits both fold instants for recurring schedules.
 */
export function zonedStart(local: string, timezone: string): string {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(local)) throw new Error("Choose a start date and time.");
  const wall = Date.parse(`${local}:00Z`);
  if (!Number.isFinite(wall) || new Date(wall).toISOString().slice(0, 16) !== local) throw new Error("Choose a valid date and time.");
  const offsets = new Set<number>();
  try {
    for (let hours = -36; hours <= 36; hours += 6) {
      const sample = wall + hours * 3600000;
      offsets.add(Date.parse(`${localDateTime(new Date(sample).toISOString(), timezone)}:00Z`) - sample);
    }
  } catch { throw new Error("Choose a valid time zone, such as America/New_York."); }
  const matches = [...offsets].map((offset) => wall - offset).filter((instant) => localDateTime(new Date(instant).toISOString(), timezone) === local).sort((a, b) => a - b);
  if (!matches.length) throw new Error("That clock time does not exist on this date because the clocks move forward. Choose another time.");
  return new Date(matches[0]).toISOString();
}

export function defaultSettings(timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"): ScheduleSettings {
  return { preset: "once", date: localDateTime(new Date().toISOString(), timezone).slice(0, 10), times: ["09:00"], timezone, interval: 2, weekdays: ["MO"], count: "7", rrule: "", endAt: "" };
}

export function compilePreset(settings: ScheduleSettings): Pick<ScheduleInput, "kind" | "timezone" | "startAt" | "rrule" | "endAt">[] {
  const { preset, date, timezone, interval, count } = settings;
  const times = preset === "multiple" ? settings.times : settings.times.slice(0, 1);
  if (!times.length || times.length > 12 || new Set(times).size !== times.length) throw new Error("Choose between 1 and 12 different clock times.");
  let rrule = "";
  if (preset === "advanced") rrule = settings.rrule.trim();
  else if (preset !== "once") {
    rrule = preset === "weekly" ? "FREQ=WEEKLY" : "FREQ=DAILY";
    if (preset === "interval") {
      if (!Number.isInteger(interval) || interval < 1 || interval > 30) throw new Error("Choose an interval from 1 to 30 days.");
      rrule += `;INTERVAL=${interval}`;
    }
    if (preset === "weekly") {
      if (!settings.weekdays.length || settings.weekdays.some((day) => !weekdays.some(([code]) => code === day))) throw new Error("Choose at least one weekday.");
      rrule += `;BYDAY=${weekdays.filter(([day]) => settings.weekdays.includes(day)).map(([day]) => day).join(",")}`;
    }
    if (count) {
      if (!/^\d+$/.test(count) || Number(count) < 1 || Number(count) > 366) throw new Error("Choose a repeat limit from 1 to 366, or leave it empty.");
      rrule += `;COUNT=${Number(count)}`;
    }
  }
  const endAt = settings.endAt ? zonedStart(settings.endAt, timezone) : undefined;
  return times.map((time) => {
    const startAt = zonedStart(`${date}T${time}`, timezone);
    if (endAt && endAt < startAt) throw new Error("The end must be after the start.");
    return { kind: rrule ? "recurrence" : "one_off", timezone, startAt, rrule, endAt };
  });
}

export function settingsForSchedule(schedule: Schedule): ScheduleSettings {
  const local = localDateTime(schedule.startAt, schedule.timezone);
  const base = { ...defaultSettings(schedule.timezone), date: local.slice(0, 10), times: [local.slice(11)], count: "", rrule: schedule.rrule ?? "", endAt: schedule.endAt ? localDateTime(schedule.endAt, schedule.timezone) : "" };
  if (schedule.kind === "one_off") return base;
  const rule = schedule.rrule ?? "";
  // Only decode the subset our friendly controls can round-trip exactly.
  if (!/^(FREQ=(DAILY|WEEKLY))(;INTERVAL=\d+)?(;BYDAY=(MO|TU|WE|TH|FR|SA|SU)(,(MO|TU|WE|TH|FR|SA|SU))*)?(;COUNT=\d+)?$/.test(rule)) return { ...base, preset: "advanced" };
  const fields = Object.fromEntries(rule.split(";").map((clause) => clause.split("=")));
  if (fields.FREQ === "WEEKLY" && fields.INTERVAL && fields.INTERVAL !== "1") return { ...base, preset: "advanced" };
  const day = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"][new Date(`${base.date}T12:00:00Z`).getUTCDay()];
  return { ...base, preset: fields.FREQ === "WEEKLY" ? "weekly" : fields.INTERVAL ? "interval" : "daily", interval: Number(fields.INTERVAL ?? 1), weekdays: fields.BYDAY?.split(",") ?? [day], count: fields.COUNT ?? "" };
}

export function cadenceLabel(schedule: Schedule): string {
  const settings = settingsForSchedule(schedule);
  const label = settings.preset === "once" ? "Once" : settings.preset === "daily" ? "Daily" : settings.preset === "interval" ? `Every ${settings.interval} days` : settings.preset === "weekly" ? `Weekly · ${settings.weekdays.map((day) => weekdays.find(([code]) => code === day)?.[1]).join(", ")}` : "Custom repeat";
  return `${label} · ${settings.times[0]}${settings.count && settings.preset !== "once" ? ` · ${settings.count} times` : ""}`;
}

export type SaveResult = { startAt: string; id?: string; error?: unknown };
/** Each time is attempted once. A failed response may still have been saved;
 * never automatically retry the batch (or hide its successful members).
 */
export async function saveScheduleTimes(inputs: ScheduleInput[], create: (input: ScheduleInput) => Promise<{ id: string }>): Promise<SaveResult[]> {
  const results: SaveResult[] = [];
  for (const input of inputs) {
    try { const saved = await create(input); results.push({ startAt: input.startAt, id: saved.id }); }
    catch (error) { results.push({ startAt: input.startAt, error }); }
  }
  return results;
}
