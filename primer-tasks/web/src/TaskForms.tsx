import { useEffect, useState, type FormEvent } from "react";
import { TasksApiError, tasksClient, type Task, type Student, type Schedule } from "@primer-tasks/client";
import { compilePreset, defaultSettings, localDateTime, saveScheduleTimes, settingsForSchedule, weekdays, type Preset, type SaveResult } from "./schedule-presets";

function FormError({ error }: { error: unknown }) {
  if (!error) return null;
  const message = error instanceof TasksApiError
    ? error.status === 403 ? "Access denied. Check your household membership."
      : error.status === 401 ? "Please sign in again to save your changes."
      : "We couldn’t save this change. Check that the task and student are still available, then try again."
    : error instanceof Error ? error.message : "We couldn’t save this change. Please try again.";
  return <p className="notice error" role="alert">{message}</p>;
}

const parentApproval = [{ id: "parent-approval", kind: "parent_approval", configVersion: 1, config: {}, interaction: "parent_action", executor: "human" }];

export function TaskEditor({ task, onSaved, onClose }: { task?: Task; onSaved: () => void; onClose?: () => void }) {
  const [title, setTitle] = useState(task?.title ?? "");
  const [instructions, setInstructions] = useState(task?.instructions ?? "Complete the task, then ask a parent to check it.");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape" && !busy) onClose?.(); };
    window.addEventListener("keydown", escape);
    return () => window.removeEventListener("keydown", escape);
  }, [busy, onClose]);
  const save = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError(null);
    const body = { title: title.trim(), instructions: instructions.trim(), requirements: task?.requirements?.length ? task.requirements : parentApproval };
    try {
      if (task) await tasksClient.reviseTask(task.templateId, body);
      else await tasksClient.createTask(body);
      setTitle(""); onSaved();
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const form = <form onSubmit={save} className={task ? "modal" : "task-form"} aria-label={task ? "Edit task" : "Create task"}>
    <div className="modal-header"><h2>{task ? `Edit ${task.title}` : "Create a task"}</h2></div>
    <div className="modal-body">
      {task && <p>Save your changes as a new draft, then publish when you’re ready. Work already assigned keeps its original instructions. Existing schedules keep their current task version.</p>}
      <label className="field">Task title<input className="input" required autoFocus={Boolean(task)} value={title} onChange={(e) => setTitle(e.target.value)} disabled={busy} /></label>
      <label className="field">Instructions<textarea className="input" rows={3} value={instructions} onChange={(e) => setInstructions(e.target.value)} disabled={busy} /></label>
      <p>Your student will ask you to check the task when it’s done.</p>
      <FormError error={error} />
    </div>
    <div className="modal-footer">{onClose && <button className="button quiet" type="button" disabled={busy} onClick={onClose}>Cancel</button>}<button className="button" type="submit" disabled={busy || !title.trim()}>{busy ? "Saving…" : task ? "Save new draft" : "Create draft"}</button></div>
  </form>;
  return task ? <div className="modal-backdrop"><div role="dialog" aria-modal="true" aria-label="Edit task">{form}</div></div> : form;
}

/** Bounded, server-searched student picker; no bulk fetch/client filtering. */
function StudentPicker({ value, onChange, disabled }: { value: string; onChange: (id: string) => void; disabled: boolean }) {
  const [q, setQ] = useState("");
  const [students, setStudents] = useState<Student[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [total, setTotal] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    tasksClient.listStudents({ q, limit: 20, offset: 0 }, { signal: controller.signal }).then((page) => { setStudents(page.items); setTotal(page.totalCount); setError(null); }).catch((e) => { if (!controller.signal.aborted) setError(e); });
    return () => controller.abort();
  }, [q]);
  return <><label className="field">Find a student<input className="input" value={q} disabled={disabled} placeholder="Search by name" onChange={(e) => { setQ(e.target.value); onChange(""); }} /></label>
    <label className="field">Student to schedule<select className="input" aria-label="Student to schedule" required value={value} disabled={disabled} onChange={(e) => onChange(e.target.value)}><option value="">Choose a student</option>{students.map((student) => <option key={student.id} value={student.id}>{student.displayName}</option>)}</select></label>
    {total > 20 && <p>Showing 20 students. Search by name to find another student.</p>}<FormError error={error} /></>;
}

function TaskPicker({ value, onChange, disabled }: { value?: Task; onChange: (task?: Task) => void; disabled: boolean }) {
  const [q, setQ] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    const controller = new AbortController();
    tasksClient.listTasks({ q, view: "templates", status: "published", limit: 20, offset: 0, sort: "title", dir: "asc" }, { signal: controller.signal }).then((page) => { setTasks(page.items ?? []); setTotal(page.totalCount); setError(null); }).catch((e) => { if (!controller.signal.aborted) setError(e); });
    return () => controller.abort();
  }, [q]);
  return <><label className="field">Find a published task<input className="input" value={q} disabled={disabled} placeholder="Search by title" onChange={(e) => { setQ(e.target.value); onChange(undefined); }} /></label>
    <label className="field">Task to schedule<select className="input" aria-label="Task to schedule" value={value?.id ?? ""} disabled={disabled} required onChange={(e) => onChange(tasks.find((task) => task.id === e.target.value))}><option value="">Choose a published task</option>{tasks.map((task) => <option key={task.id} value={task.id}>{task.title}</option>)}</select></label>
    {total > 20 && <p>Showing 20 tasks. Search by title to find another task.</p>}<FormError error={error} /></>;
}

export function ScheduleForm({ schedule, onSaved, onClose }: { schedule?: Schedule; onSaved: () => void; onClose?: () => void }) {
  const [settings, setSettings] = useState(() => schedule ? settingsForSchedule(schedule) : defaultSettings());
  const [task, setTask] = useState<Task>();
  const [studentId, setStudentId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [results, setResults] = useState<SaveResult[] | null>(null);
  const [updated, setUpdated] = useState(false);
  const locked = busy || results !== null || updated;
  const save = async (event: FormEvent) => {
    event.preventDefault(); setBusy(true); setError(null);
    try {
      const cadence = compilePreset(settings);
      if (schedule) {
        await tasksClient.updateSchedule(schedule.id, { studentId: schedule.studentId, templateId: schedule.templateId, revisionId: schedule.revisionId, dueOffsetMinutes: schedule.dueOffsetMinutes, ...cadence[0] });
        setUpdated(true); onSaved();
      } else if (task && studentId) {
        const next = await saveScheduleTimes(cadence.map((time) => ({ ...time, studentId, templateId: task.templateId, revisionId: task.id, dueOffsetMinutes: 0 })), (body) => tasksClient.createSchedule(body));
        setResults(next); onSaved();
      }
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const changeTime = (index: number, value: string) => setSettings((s) => ({ ...s, times: s.times.map((time, i) => i === index ? value : time) }));
  return <form onSubmit={save} className="task-form" aria-label={schedule ? "Edit schedule" : "Schedule a task"}>
    <div className="modal-header"><h2>{schedule ? `Edit schedule · ${schedule.title ?? "Task"}` : "Schedule a task"}</h2></div>
    <div className="modal-body">
      {schedule ? <><p>{schedule.studentName ?? "Student"} · {schedule.timezone}</p><p>Future dates follow the new cadence. Existing assigned times and instructions stay unchanged, including dates already on the checklist. This changes only this schedule.</p></> : <><TaskPicker value={task} onChange={setTask} disabled={locked} /><StudentPicker value={studentId} onChange={setStudentId} disabled={locked} /></>}
      <label className="field">Repeat<select className="input" aria-label="Repeat" value={settings.preset} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, preset: e.target.value as Preset }))}>
        <option value="once">Once</option><option value="daily">Daily</option><option value="weekly">Weekly</option><option value="interval">Every N days</option>{!schedule && <option value="multiple">N times per day</option>}{settings.preset === "advanced" && <option value="advanced">Custom repeat</option>}
      </select></label>
      <label className="field">Start date<input className="input" type="date" required value={settings.date} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, date: e.target.value }))} /></label>
      {settings.preset === "interval" && <label className="field">Days between repeats<input className="input" type="number" min={1} max={30} required value={settings.interval} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, interval: Number(e.target.value) }))} /></label>}
      {settings.preset === "weekly" && <fieldset disabled={locked}><legend>Weekdays</legend><div className="page-actions">{weekdays.map(([code, name]) => <label key={code}><input type="checkbox" checked={settings.weekdays.includes(code)} onChange={(e) => setSettings((s) => ({ ...s, weekdays: e.target.checked ? [...s.weekdays, code] : s.weekdays.filter((d) => d !== code) }))} /> {name}</label>)}</div></fieldset>}
      {settings.preset === "multiple" && <><p>Each time becomes a separate daily schedule. You can edit or cancel each one on Schedules.</p><label className="field">Times per day<input className="input" type="number" min={1} max={12} required value={settings.times.length} disabled={locked} onChange={(e) => { const n = Math.max(1, Math.min(12, Number(e.target.value))); setSettings((s) => ({ ...s, times: Array.from({ length: n }, (_, i) => s.times[i] ?? `${String(9 + i).padStart(2, "0")}:00`) })); }} /></label></>}
      {(settings.preset === "multiple" ? settings.times : settings.times.slice(0, 1)).map((time, i) => <label className="field" key={i}>{settings.preset === "multiple" ? `Time ${i + 1}` : "Time"}<input className="input" type="time" required value={time} disabled={locked} onChange={(e) => changeTime(i, e.target.value)} /></label>)}
      {settings.preset !== "once" && settings.preset !== "advanced" && <label className="field">{settings.preset === "multiple" ? "Repeat limit for each time (optional)" : "Repeat limit (optional)"}<input className="input" type="number" min={1} max={366} value={settings.count} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, count: e.target.value }))} /></label>}
      <p>Times are in {settings.timezone}. Change the time zone under Advanced.</p>
      <details open={settings.preset === "advanced" || undefined}><summary>Advanced</summary><div className="modal-body">
        <label className="field">Time zone<input className="input" required value={settings.timezone} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, timezone: e.target.value }))} /></label>
        <label className="field">Schedule start<input className="input" type="datetime-local" required value={`${settings.date}T${settings.times[0]}`} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, date: e.target.value.slice(0, 10), times: [e.target.value.slice(11), ...s.times.slice(1)] }))} /></label>
        <label className="field">End date and time (optional)<input className="input" type="datetime-local" value={settings.endAt} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, endAt: e.target.value }))} /></label>
        <label className="field">Custom repeat rule (RRULE)<input className="input" placeholder="FREQ=DAILY;COUNT=7" value={settings.rrule} disabled={locked} onChange={(e) => setSettings((s) => ({ ...s, preset: "advanced", rrule: e.target.value }))} /></label><p>Leave the rule empty for a single assignment. Daylight-saving changes skip missing clock times and repeat both instances of a repeated time.</p>
      </div></details>
      <FormError error={error} />
      {updated && <p role="status">Schedule saved. Already assigned work has not changed.</p>}
      {results && <div role="status"><h3>Schedule results</h3><ul>{results.map((result, i) => <li key={i}>{localDateTime(result.startAt, settings.timezone).replace("T", " ")} — {result.id ? "Saved" : "Could not confirm this time was saved"}</li>)}</ul>{results.some((result) => !result.id) && <p>Some times could not be confirmed. Saved times will not be retried. Check Schedules before trying failed times again, because a lost response can still mean a schedule was saved.</p>}<p>Manage each time separately on Schedules.</p></div>}
    </div>
    <div className="modal-footer">{onClose && <button className="button quiet" type="button" disabled={busy} onClick={onClose}>{results || updated ? "Done" : "Cancel"}</button>}{!results && !updated && <button className="button" type="submit" disabled={busy || (!schedule && (!task || !studentId))}>{busy ? "Saving…" : schedule ? "Save schedule" : "Schedule task"}</button>}</div>
  </form>;
}
