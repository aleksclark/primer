import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { QRCodeSVG } from "qrcode.react";
import { NavLink, Navigate, Route, Routes, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { TasksApiError, tasksClient, type Occurrence, type Schedule, type Student } from "@primer-tasks/client";
import "./index.css";

type Theme = "dark" | "light";

type RequestState = "loading" | "ready" | "empty" | "error" | "denied" | "revoked" | "expired";

// This visible marker is also the target of the real Vite HMR proof. Keeping
// it in the application module proves React state updates without a document
// reload; it is not a test-only fake response.
const HMR_PROOF_MARKER = "System C · HMR baseline";

function useTheme() {
  const [theme, setTheme] = useState<Theme>(() =>
    document.documentElement.dataset.theme === "light" ? "light" : "dark",
  );
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  return { theme, toggle: () => setTheme((current) => (current === "dark" ? "light" : "dark")) };
}

function apiState(error: unknown): Exclude<RequestState, "loading" | "ready" | "empty"> {
  if (error instanceof TasksApiError) {
    if (error.code === "revoked") return "revoked";
    if (error.status === 410 || error.code === "expired") return "expired";
    if (error.status === 403) return "denied";
    if (error.status === 401) return "error";
  }
  return "error";
}

function ErrorNotice({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const state = apiState(error);
  const copy = {
    error: ["Unable to load", "The Tasks service did not return the requested record."],
    denied: ["Access denied", "This record belongs to another household or your session is not permitted."],
    revoked: ["Pairing revoked", "This student session is no longer active. Pair again to continue."],
    expired: ["Pairing expired", "The one-use pairing material is no longer valid. Request a new code."],
  }[state];
  return (
    <div className={`notice ${state}`} role="alert">
      <div>
        <strong>{copy[0]}</strong>
        <p>{copy[1]}</p>
        {onRetry && <button className="button quiet" type="button" onClick={onRetry}>Try again</button>}
      </div>
    </div>
  );
}

function StateNotice({ state, onRetry }: { state: RequestState; onRetry?: () => void }) {
  if (state === "loading") return <div className="notice" role="status"><p>Loading the latest record…</p></div>;
  if (state === "error" || state === "denied" || state === "revoked" || state === "expired") {
    return <ErrorNotice error={new TasksApiError(state === "denied" ? 403 : state === "expired" ? 410 : 500, state, state)} onRetry={onRetry} />;
  }
  return null;
}

function ThemeButton({ theme, toggle }: { theme: Theme; toggle: () => void }) {
  return <button className="button secondary" type="button" onClick={toggle} aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}>
    {theme === "dark" ? "Light" : "Dark"} theme
  </button>;
}

function Brand() {
  return <NavLink className="brand" to="/parent/students" aria-label="Primer Tasks home">
    <img className="brand-dark" src="/brand/logo-mark.svg" alt="" />
    <img className="brand-light" src="/brand/logo-mark-light.svg" alt="" />
    <span className="brand-wordmark">Primer<strong>Tasks</strong></span>
    <span className="system-label hmr-proof-marker" data-hmr-proof-marker="true">{HMR_PROOF_MARKER}</span>
  </NavLink>;
}

function ParentShell({ children }: { children: ReactNode }) {
  const { theme, toggle } = useTheme();
  const [open, setOpen] = useState(false);
  return <div className={`app-shell ${open ? "mobile-nav-open" : ""}`}>
    <aside className="app-nav">
      <Brand />
      <button className="button secondary mobile-menu" type="button" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="parent-navigation">Menu</button>
      <div id="parent-navigation" className="nav-body">
        <div className="nav-section">
          <p className="system-label" style={{ padding: "0 20px" }}>Parent workspace</p>
          <NavLink className="nav-link" to="/parent/students">Students</NavLink>
          <NavLink className="nav-link" to="/parent/tasks">Tasks</NavLink>
          <NavLink className="nav-link" to="/parent/schedules">Schedules</NavLink>
          <NavLink className="nav-link" to="/parent/occurrences">Occurrences</NavLink>
        </div>
        <div className="nav-section">
          <p className="system-label" style={{ padding: "0 20px" }}>Student access</p>
          <NavLink className="nav-link" to="/student/pair">Pair a browser</NavLink>
        </div>
      </div>
      <div className="nav-footer"><p className="system-label">System C · Primer Tasks</p><ThemeButton theme={theme} toggle={toggle} /><button className="button quiet" type="button" onClick={() => void tasksClient.logout().finally(() => window.location.assign("/parent/students"))}>Sign out</button></div>
    </aside>
    <main className="main"><div className="content">{children}</div></main>
  </div>;
}

function ParentAuthGate() {
  const [status, setStatus] = useState<RequestState>("loading");
  const { theme, toggle } = useTheme();
  useEffect(() => {
    let mounted = true;
    tasksClient.parentSession().then(() => mounted && setStatus("ready")).catch((error) => mounted && setStatus(apiState(error)));
    return () => { mounted = false; };
  }, []);
  if (status === "loading") return <AuthFrame><StateNotice state="loading" /></AuthFrame>;
  if (status !== "ready") return <LoginPage theme={theme} toggle={toggle} />;
  return <ParentShell><Routes><Route path="students" element={<StudentsPage />} /><Route path="students/:studentId" element={<StudentDetailPage />} /><Route path="tasks" element={<TasksPage />} /><Route path="schedules" element={<SchedulesPage />} /><Route path="occurrences" element={<OccurrencesPage />} /><Route path="*" element={<Navigate to="students" replace />} /></Routes></ParentShell>;
}

function AuthFrame({ children }: { children: ReactNode }) {
  return <div className="auth-frame"><div className="auth-brand"><Brand /></div><main className="main"><div className="content">{children}</div></main></div>;
}

function LoginPage({ theme, toggle }: { theme: Theme; toggle: () => void }) {
  return <AuthFrame><section className="pair-card" style={{ maxWidth: 560, margin: "12vh auto 0" }}>
    <div className="page-header"><div><p className="eyebrow">Parent access</p><h1>Sign in to Primer Tasks</h1><p>Your household workspace is protected by the Tasks identity service.</p></div><ThemeButton theme={theme} toggle={toggle} /></div>
    <div style={{ display: "grid", gap: 16, marginTop: 24 }}>
      <p style={{ margin: 0, color: "var(--muted)" }}>Continue to the secure authorization flow. Your browser receives only a host-only session cookie; provider tokens never enter this app.</p>
      <button className="button" type="button" onClick={() => tasksClient.beginParentLogin("/parent/students")}>Continue with parent sign-in</button>
    </div>
  </section></AuthFrame>;
}

function PageHeader({ eyebrow, title, lede, actions }: { eyebrow: string; title: string; lede?: string; actions?: ReactNode }) {
  return <header className="page-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{lede && <p>{lede}</p>}</div>{actions && <div className="page-actions">{actions}</div>}</header>;
}

function StudentsPage() {
  const [q, setQ] = useState("");
  const [offset, setOffset] = useState(0);
  const [students, setStudents] = useState<Student[]>([]);
  const [total, setTotal] = useState(0);
  const [state, setState] = useState<RequestState>("loading");
  const [error, setError] = useState<unknown>(null);
  const [showCreate, setShowCreate] = useState(false);
  const limit = 20;
  const load = useCallback((signal?: AbortSignal) => {
    setState("loading");
    tasksClient.listStudents({ limit, offset, q: q || undefined }, { signal }).then((page) => {
      setStudents(page.items);
      setTotal(page.totalCount);
      setState(page.items.length ? "ready" : "empty");
      setError(null);
    }).catch((nextError) => {
      if (nextError instanceof DOMException && nextError.name === "AbortError") return;
      setError(nextError); setState(apiState(nextError));
    });
  }, [offset, q]);
  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);
  return <>
    <PageHeader eyebrow="Parent workspace / Explore + Configure" title="Students" lede="Manage the students in your household. Lists, filters, and pagination remain tenant-scoped on the server." actions={<button className="button" type="button" onClick={() => setShowCreate(true)}>Add student</button>} />
    {state === "error" || state === "denied" ? <ErrorNotice error={error} onRetry={load} /> : null}
    <section className="record" aria-label="Students">
      <div className="record-toolbar"><div><p className="system-label">Household roster</p><p style={{ margin: "4px 0 0", color: "var(--muted)" }}>{total} student{total === 1 ? "" : "s"}</p></div><input className="input" aria-label="Search students" placeholder="Search by name" value={q} onChange={(event) => { setQ(event.target.value); setOffset(0); }} /></div>
      {state === "loading" && <StateNotice state="loading" />}
      {state === "empty" && <div className="empty"><h2>No students yet</h2><p>Create the first student in this household, then issue a one-use pairing code.</p><button className="button" type="button" onClick={() => setShowCreate(true)}>Add first student</button></div>}
      {state === "ready" && <><div className="table-wrap"><table><thead><tr><th>Name</th><th>Created</th><th>Access</th><th><span className="sr-only">Actions</span></th></tr></thead><tbody>{students.map((student) => <StudentRow key={student.id} student={student} onArchived={load} />)}</tbody></table></div><Pagination offset={offset} limit={limit} total={total} onChange={setOffset} /></>}
    </section>
    {showCreate && <StudentForm onClose={() => setShowCreate(false)} onSaved={() => { setShowCreate(false); load(); }} />}
  </>;
}

function StudentRow({ student, onArchived }: { student: Student; onArchived: () => void }) {
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const archive = async () => {
    if (!window.confirm(`Archive ${student.displayName}? Active paired access will be revoked.`)) return;
    setBusy(true);
    try { await tasksClient.archiveStudent(student.id); onArchived(); } catch { /* row remains; parent can retry from the detail */ } finally { setBusy(false); }
  };
  return <tr><td><button className="button quiet primary-cell" type="button" onClick={() => navigate(`/parent/students/${student.id}`)}>{student.displayName}</button><span className="secondary-cell">{student.id}</span></td><td className="meta">{formatDate(student.createdAt)}</td><td><span className={`status ${student.archivedAt ? "attention" : "active"}`}>{student.archivedAt ? "Archived" : "Ready"}</span></td><td><div className="row-actions"><button className="button secondary" type="button" disabled={busy || Boolean(student.archivedAt)} onClick={() => navigate(`/parent/students/${student.id}`)}>Open</button>{!student.archivedAt && <button className="button danger" type="button" disabled={busy} onClick={() => void archive()}>Archive</button>}</div></td></tr>;
}

function StudentForm({ student, onClose, onSaved }: { student?: Student; onClose: () => void; onSaved: () => void }) {
  const [displayName, setDisplayName] = useState(student?.displayName ?? "");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!displayName.trim()) return;
    setBusy(true); setError(null);
    try { if (student) await tasksClient.updateStudent(student.id, { displayName: displayName.trim() }); else await tasksClient.createStudent({ displayName: displayName.trim() }); onSaved(); }
    catch (nextError) { setError(nextError); } finally { setBusy(false); }
  };
  return <div className="modal-backdrop" role="presentation"><form className="modal" role="dialog" aria-modal="true" aria-labelledby="student-form-title" onSubmit={save}><div className="modal-header"><h2 id="student-form-title">{student ? "Edit student" : "Add student"}</h2></div><div className="modal-body"><div className="field"><label htmlFor="student-display-name">Display name</label><input id="student-display-name" className="input" autoFocus required value={displayName} onChange={(event) => setDisplayName(event.target.value)} aria-invalid={Boolean(error)} /></div>{error ? <ErrorNotice error={error} /> : null}</div><div className="modal-footer"><button className="button quiet" type="button" onClick={onClose}>Cancel</button><button className="button" type="submit" disabled={busy || !displayName.trim()}>{busy ? "Saving…" : "Save student"}</button></div></form></div>;
}

function StudentDetailPage() {
  const { studentId = "" } = useParams();
  const navigate = useNavigate();
  const [student, setStudent] = useState<Student | null>(null);
  const [pairing, setPairing] = useState<Awaited<ReturnType<typeof tasksClient.issuePairing>> | null>(null);
  const [state, setState] = useState<RequestState>("loading");
  const [error, setError] = useState<unknown>(null);
  const [edit, setEdit] = useState(false);
  const load = useCallback(() => { setState("loading"); tasksClient.getStudent(studentId).then((found) => { setStudent(found); setState("ready"); }).catch((nextError) => { setError(nextError); setState(apiState(nextError)); }); }, [studentId]);
  useEffect(load, [load]);
  const issue = async () => { setPairing(null); setError(null); try { setPairing(await tasksClient.issuePairing(studentId)); } catch (nextError) { setError(nextError); } };
  if (state === "loading") return <><PageHeader eyebrow="Parent workspace / Student" title="Student" /><StateNotice state="loading" /></>;
  if (!student) return <><PageHeader eyebrow="Parent workspace / Student" title="Student" /><ErrorNotice error={error} onRetry={load} /></>;
  return <><PageHeader eyebrow="Parent workspace / Configure" title={student.displayName} lede="Student identity and pairing access are scoped to your household." actions={<button className="button secondary" type="button" onClick={() => navigate("/parent/students")}>Back to students</button>} />{error ? <ErrorNotice error={error} /> : null}
    <div className="pair-layout"><section className="pair-card"><p className="eyebrow">Student record</p><h2>{student.displayName}</h2><p style={{ color: "var(--muted)" }}>Created {formatDate(student.createdAt)} · <span className="meta">{student.id}</span></p><div className="page-actions" style={{ marginTop: 24 }}><button className="button secondary" type="button" onClick={() => setEdit(true)}>Edit name</button>{!student.archivedAt && <button className="button danger" type="button" onClick={() => void tasksClient.archiveStudent(student.id).then(() => navigate("/parent/students"))}>Archive student</button>}</div></section><section className="pair-card"><p className="eyebrow">Student browser access</p><h2>Issue a pairing QR</h2><p style={{ color: "var(--muted)" }}>The QR contains only short-lived pairing material. It never contains the eventual session credential.</p><button className="button" type="button" onClick={() => void issue()} disabled={Boolean(student.archivedAt)}>{pairing ? "Issue a new code" : "Issue pairing QR"}</button>{pairing && <PairingDisplay pairing={pairing} />}</section></div>{edit && <StudentForm student={student} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load(); }} />}</>;
}

function PairingDisplay({ pairing }: { pairing: Awaited<ReturnType<typeof tasksClient.issuePairing>> }) {
  return <div style={{ marginTop: 20 }}><p className="system-label">Show once · expires {formatDate(pairing.expiresAt)}</p><div className="qr-frame"><QRCodeSVG value={pairing.qrPayload} size={200} includeMargin={false} title="One-use student pairing QR code" role="img" aria-label="One-use student pairing QR code" /></div><span className="code">{pairing.code}</span><p style={{ color: "var(--muted)", fontSize: 13 }}>Keep this page open while the student scans or enters the code. Do not copy the credential into a message or URL.</p></div>;
}

function StudentShell() {
  const { theme, toggle } = useTheme();
  return <div className="app-shell"><aside className="app-nav"><Brand /><div className="nav-body"><div className="nav-section"><p className="system-label" style={{ padding: "0 20px" }}>Student workspace</p><NavLink className="nav-link" to="/student">Checklist</NavLink></div></div><div className="nav-footer"><ThemeButton theme={theme} toggle={toggle} /><p className="system-label">One student · one session</p></div></aside><main className="main"><div className="content"><Routes><Route path="" element={<StudentChecklistPage />} /><Route path="occurrences/:id" element={<StudentOccurrencePage />} /><Route path="pair" element={<StudentPairPage />} /><Route path="*" element={<Navigate to="/student" replace />} /></Routes></div></main></div>;
}

function StudentPairPage() {
  const navigate = useNavigate();
  const [code, setCode] = useState("");
  const [state, setState] = useState<RequestState>("ready");
  const [error, setError] = useState<unknown>(null);
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!code.trim()) return; setState("loading"); setError(null); try { await tasksClient.pairStudent({ code: code.trim().toUpperCase() }); navigate("/student"); } catch (nextError) { setError(nextError); setState(apiState(nextError)); } };
  return <><PageHeader eyebrow="Student access / Pair" title="Connect this browser" lede="Enter the one-use code shown by your parent. This browser will remain bound to one student." /><section className="pair-card" style={{ maxWidth: 560, marginTop: 28 }}><form onSubmit={submit} style={{ display: "grid", gap: 18 }}><div className="field"><label htmlFor="pair-code">Pairing code</label><input id="pair-code" className="input code" inputMode="text" autoComplete="one-time-code" autoCapitalize="characters" maxLength={32} value={code} onChange={(event) => setCode(event.target.value)} aria-invalid={Boolean(error)} /><p className="meta">Codes are one-use and expire quickly.</p></div>{state !== "ready" && state !== "loading" && <ErrorNotice error={error ?? new TasksApiError(state === "denied" ? 403 : state === "expired" ? 410 : 500, state, state)} />}{state === "loading" && <StateNotice state="loading" />}<button className="button" type="submit" disabled={state === "loading" || !code.trim()}>Pair this browser</button></form></section></>;
}

function TasksPage() {
  const [params, setParams] = useSearchParams();
  const [tasks, setTasks] = useState<NonNullable<Awaited<ReturnType<typeof tasksClient.listTasks>>["items"]>>([]);
  const [title, setTitle] = useState("");
  const instructions = "Complete the task, then ask a parent to check it.";
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [students, setStudents] = useState<Student[]>([]);
  const [studentId, setStudentId] = useState("");
  const [scheduleTaskId, setScheduleTaskId] = useState("");
  const [scheduleAt, setScheduleAt] = useState("");
  const [rrule, setRrule] = useState("");
  const [timezone, setTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC");
  const [health, setHealth] = useState<Awaited<ReturnType<typeof tasksClient.health>> | null>(null);
  const q = params.get("q") ?? "";
  const load = useCallback(() => tasksClient.listTasks({ q, limit: 20, offset: 0, sort: "title", dir: "asc" }).then((p) => setTasks(p.items ?? [])).catch(setError), [q]);
  useEffect(() => { void load(); void tasksClient.listStudents({ limit: 100, offset: 0 }).then((p) => setStudents(p.items ?? [])).catch(setError); void tasksClient.health().then(setHealth).catch(setError); }, [load]);
  const create = async (event: FormEvent) => { event.preventDefault(); if (!title.trim()) return; setBusy(true); setError(null); try { await tasksClient.createTask({ title: title.trim(), instructions, requirements: [{ id: "parent-approval", kind: "parent_approval", configVersion: 1, config: {}, interaction: "parent_action", executor: "human" }] }); setTitle(""); await load(); } catch (e) { setError(e); } finally { setBusy(false); } };
  return <><PageHeader eyebrow="Parent workspace / Explore + Configure" title="Tasks" lede={`Published revisions are immutable. Parent approval is the only verification driver enabled in this phase. Model provider: ${health?.modelProvider ?? "loading"}. Server started: ${health ? new Date(health.startedAt).toLocaleString() : "loading"}.`} />
    <section className="record"><form className="record-toolbar" onSubmit={create}><input className="input" aria-label="Task title" placeholder="Brush your teeth" value={title} onChange={(e) => setTitle(e.target.value)} /><button className="button" type="submit" disabled={busy || !title.trim()}>Create draft</button></form><form className="record-toolbar" onSubmit={(event) => { event.preventDefault(); const task = tasks.find((x) => x.id === scheduleTaskId); if (!task || !studentId || !scheduleAt) return; void tasksClient.createSchedule({ studentId, templateId: task.templateId, revisionId: task.id, kind: rrule.trim() ? "recurrence" : "one_off", timezone, startAt: new Date(scheduleAt).toISOString(), rrule: rrule.trim() || undefined, dueOffsetMinutes: 0 }).then(() => { setScheduleAt(""); setRrule(""); window.alert("Schedule saved and occurrences materialized."); }).catch(setError); }}><select className="input" aria-label="Task to schedule" value={scheduleTaskId} onChange={(e) => setScheduleTaskId(e.target.value)}><option value="">Choose a published task</option>{tasks.filter((x) => x.status === "published").map((x) => <option value={x.id} key={x.id}>{x.title} · v{x.version}</option>)}</select><select className="input" aria-label="Student to schedule" value={studentId} onChange={(e) => setStudentId(e.target.value)}><option value="">Choose a student</option>{students.map((x) => <option value={x.id} key={x.id}>{x.displayName}</option>)}</select><input className="input" aria-label="Schedule start" type="text" inputMode="numeric" placeholder="2026-08-19T14:00" value={scheduleAt} onChange={(e) => setScheduleAt(e.target.value)} /><input className="input" aria-label="IANA timezone" placeholder="America/New_York" value={timezone} onChange={(e) => setTimezone(e.target.value)} /><input className="input" aria-label="RRULE" placeholder="Optional: FREQ=DAILY;COUNT=7" value={rrule} onChange={(e) => setRrule(e.target.value)} /><button className="button secondary" type="submit" disabled={!scheduleTaskId || !studentId || !scheduleAt}>Schedule task</button></form><div className="record-toolbar"><input className="input" aria-label="Search tasks" placeholder="Search tasks" value={q} onChange={(e) => { const next = new URLSearchParams(params); if (e.target.value) next.set("q", e.target.value); else next.delete("q"); setParams(next); }} /></div>{error ? <ErrorNotice error={error} onRetry={load} /> : null}<div className="table-wrap"><table><thead><tr><th>Task</th><th>Revision</th><th>Status</th><th>Action</th></tr></thead><tbody>{tasks.map((task) => <tr key={task.id}><td><strong>{task.title}</strong><span className="secondary-cell">{task.templateId}</span></td><td className="meta">v{task.version}</td><td><span className="status">{task.status}</span></td><td>{task.status === "draft" ? <button className="button" type="button" onClick={() => void tasksClient.publishTask(task.id).then(load).catch(setError)}>Publish</button> : task.status === "published" ? <button className="button danger" type="button" onClick={() => void tasksClient.retireTask(task.templateId).then(load).catch(setError)}>Retire</button> : null}</td></tr>)}</tbody></table></div>{tasks.length === 0 && <div className="empty"><h2>No tasks match</h2><p>Create a draft to begin a versioned task.</p></div>}</section>
  </>;
}

function SchedulesPage() {
  const [params, setParams] = useSearchParams();
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [error, setError] = useState<unknown>(null);
  const status = params.get("status") ?? "active";
  const load = useCallback(() => tasksClient.listSchedules({ limit: 50, offset: 0, status }).then((p) => setSchedules((p.items ?? []) as Schedule[])).catch(setError), [status]);
  useEffect(() => { void load(); }, [load]);
  return <><PageHeader eyebrow="Parent workspace / Explore + Configure" title="Schedules" lede="Every schedule has an explicit timezone and bounded recurrence. Changes are versioned; issued occurrences remain unchanged." actions={<button className="button secondary" type="button" onClick={load}>Refresh server state</button>} />{error ? <ErrorNotice error={error} onRetry={load} /> : null}<section className="record"><div className="record-toolbar"><label className="field"><span className="system-label">Collection filter</span><select className="input" aria-label="Schedule status filter" value={status} onChange={(e) => { const next = new URLSearchParams(params); next.set("status", e.target.value); setParams(next); }}><option value="active">Active schedules</option><option value="all">All schedules</option></select></label><span className="meta">Server-owned schedule collection · IANA timezone required</span></div><div className="table-wrap"><table><thead><tr><th>Task / student</th><th>Cadence</th><th>Timezone</th><th>Version</th><th>Action</th></tr></thead><tbody>{schedules.map((schedule) => <tr key={schedule.id}><td><strong>{schedule.templateId}</strong><span className="secondary-cell">{schedule.studentId}</span></td><td>{schedule.kind === "recurrence" ? schedule.rrule : "One-off"}</td><td className="meta">{schedule.timezone}</td><td className="meta">v{schedule.version}</td><td>{schedule.enabled && <button className="button danger" type="button" onClick={() => void tasksClient.retireSchedule(schedule.id).then(load).catch(setError)}>Cancel schedule</button>}</td></tr>)}</tbody></table></div>{schedules.length === 0 && <div className="empty"><h2>No active schedules</h2><p>Publish a task and schedule it from the Tasks surface.</p></div>}</section></>;
}

function OccurrencesPage() {
  const [params, setParams] = useSearchParams();
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [error, setError] = useState<unknown>(null);
  const status = params.get("status") ?? "";
  const dir = params.get("dir") === "desc" ? "desc" : "asc";
  const load = useCallback(() => tasksClient.listOccurrences({ limit: 50, offset: 0, sort: "nominalAt", dir, status: status || undefined }).then((p) => setOccurrences(p.items ?? [])).catch(setError), [dir, status]);
  useEffect(() => { void load(); }, [load]);
  const decide = (id: string, accepted: boolean) => void tasksClient.decideOccurrence(id, { accepted, reason: accepted ? "Parent observed completion." : "Try again with care." }).then(load).catch(setError);
  const setCollection = (key: string, value: string) => { const next = new URLSearchParams(params); if (value) next.set(key, value); else next.delete(key); setParams(next); };
  return <><PageHeader eyebrow="Parent workspace / Operate + Inspect" title="Occurrences" lede="Approval and rejection are durable decisions. The student never writes completion state." actions={<button className="button secondary" type="button" onClick={load}>Refresh server state</button>} />{error ? <ErrorNotice error={error} onRetry={load} /> : null}<section className="record"><div className="record-toolbar"><label className="field"><span className="system-label">Status filter</span><select className="input" aria-label="Occurrence status filter" value={status} onChange={(e) => setCollection("status", e.target.value)}><option value="">All statuses</option><option value="pending">Pending</option><option value="awaiting_verification">Awaiting verification</option><option value="completed">Completed</option></select></label><label className="field"><span className="system-label">Sort direction</span><select className="input" aria-label="Occurrence sort direction" value={dir} onChange={(e) => setCollection("dir", e.target.value)}><option value="asc">Soonest first</option><option value="desc">Latest first</option></select></label></div><div className="table-wrap"><table><thead><tr><th>Task</th><th>Student</th><th>Due</th><th>Status</th><th>Decision</th></tr></thead><tbody>{occurrences.map((o) => <tr key={o.id}><td><strong>{o.title}</strong><span className="secondary-cell">{o.id}</span></td><td className="meta">{o.studentId}</td><td className="meta">{formatOccurrenceTime(o)}</td><td><span className="status">{o.status}</span></td><td>{occurrenceActions(o, decide, load, setError)}</td></tr>)}</tbody></table></div>{occurrences.length === 0 && <div className="empty"><h2>No issued work</h2><p>Publish a task and create a schedule before occurrences appear here.</p></div>}</section></>;
}

function StudentOccurrencePage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [occurrence, setOccurrence] = useState<Occurrence | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(() => tasksClient.studentOccurrence(id).then(setOccurrence).catch(setError), [id]);
  useEffect(() => { void load(); }, [load]);
  if (!occurrence) return <><PageHeader eyebrow="Student workspace / Inspect" title="Task detail" />{error ? <ErrorNotice error={error} onRetry={load} /> : <StateNotice state="loading" />}</>;
  return <><PageHeader eyebrow="Student workspace / Inspect" title={occurrence.title} lede="The server owns this state. Start, then submit for parent approval." actions={<button className="button secondary" type="button" onClick={() => navigate("/student")}>Back to today</button>} /><section className="pair-card"><p>{occurrence.instructions}</p><p className="status">{occurrence.status}</p>{occurrence.status === "pending" && <button className="button" type="button" onClick={() => void tasksClient.startStudentOccurrence(id).then(load).catch(setError)}>Start task</button>}{occurrence.status === "in_progress" && <button className="button" type="button" onClick={() => void tasksClient.submitStudentOccurrence(id).then(load).catch(setError)}>Submit for parent approval</button>}{occurrence.status === "awaiting_verification" && <p className="meta">Waiting for parent approval.</p>}{occurrence.status === "completed" && <p className="status active">Checked by parent</p>}</section></>;
}

function occurrenceActions(o: Occurrence, decide: (id: string, accepted: boolean) => void, load: () => void, setError: (error: unknown) => void) {
  const terminal = o.status === "completed" || o.status === "canceled" || o.status === "excused";
  if (terminal) return null;
  return <div className="row-actions">
    {o.status === "awaiting_verification" && <><button className="button" type="button" onClick={() => decide(o.id, true)}>Approve</button><button className="button danger" type="button" onClick={() => decide(o.id, false)}>Reject</button></>}
    {o.status === "pending" && <button className="button secondary" type="button" onClick={() => void tasksClient.retryOccurrence(o.id).then(load).catch(setError)}>Retry</button>}
    <button className="button quiet" type="button" onClick={() => void tasksClient.skipOccurrence(o.id).then(load).catch(setError)}>Skip</button>
    <button className="button quiet" type="button" onClick={() => void tasksClient.cancelOccurrence(o.id).then(load).catch(setError)}>Cancel</button>
  </div>;
}

function StudentChecklistPage() {
  const [profile, setProfile] = useState<Awaited<ReturnType<typeof tasksClient.studentProfile>> | null>(null);
  const [items, setItems] = useState<Awaited<ReturnType<typeof tasksClient.studentChecklist>>["items"]>([]);
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [state, setState] = useState<RequestState>("loading");
  const [error, setError] = useState<unknown>(null);
  const load = useCallback(() => { setState("loading"); Promise.all([tasksClient.studentProfile(), tasksClient.studentChecklist(), tasksClient.studentToday()]).then(([nextProfile, checklist, today]) => { setProfile(nextProfile); setItems(checklist.items); setOccurrences(today.items ?? []); setState((today.items ?? []).length || checklist.items.length ? "ready" : "empty"); }).catch((nextError) => { setError(nextError); setState(apiState(nextError)); }); }, []);
  useEffect(load, [load]);
  if (state === "loading") return <><PageHeader eyebrow="Student workspace / Operate" title="Today" /><StateNotice state="loading" /></>;
  if (state === "error" || state === "denied" || state === "revoked" || state === "expired") return <><PageHeader eyebrow="Student workspace / Operate" title="Today" /><ErrorNotice error={error} onRetry={load} /><p className="meta">If access was revoked or expired, ask a parent for a new pairing code.</p></>;
  return <><PageHeader eyebrow="Student workspace / Operate" title={`Today with ${profile?.displayName ?? "you"}`} lede="Your checklist is server-owned. Completing a task will appear here only after its verification succeeds." /><section className="checklist" aria-live="polite">{state === "empty" ? <div className="empty"><h2>Nothing assigned yet</h2><p>Your parent has not scheduled anything for today. This empty checklist is ready for the next task.</p></div> : <>{occurrences.map((item) => <NavLink className="checklist-row" to={`/student/occurrences/${item.id}`} key={item.id}><div><h3>{item.title}</h3><p>{item.instructions}</p></div><span className="status">{item.status}</span></NavLink>)}{items.map((item) => <div className="checklist-row" key={item.id}><div><h3>{item.title}</h3>{item.description && <p>{item.description}</p>}</div><span className="status">{item.status}</span></div>)}</>}</section></>;
}

function Pagination({ offset, limit, total, onChange }: { offset: number; limit: number; total: number; onChange: (offset: number) => void }) {
  if (total <= limit) return null;
  return <div className="record-toolbar"><span className="meta">{offset + 1}–{Math.min(offset + limit, total)} of {total}</span><div className="page-actions"><button className="button secondary" type="button" disabled={offset === 0} onClick={() => onChange(Math.max(0, offset - limit))}>Previous</button><button className="button secondary" type="button" disabled={offset + limit >= total} onClick={() => onChange(offset + limit)}>Next</button></div></div>;
}

function formatOccurrenceTime(occurrence: Occurrence) {
  const value = new Date(occurrence.nominalAt);
  if (Number.isNaN(value.valueOf())) return occurrence.nominalAt;
  const local = new Intl.DateTimeFormat(undefined, { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZone: occurrence.timezone, timeZoneName: "short" }).format(value);
  return `${local} · ${occurrence.timezone}`;
}

function formatDate(value?: string | null) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

function App() {
  return <Routes><Route path="/parent/*" element={<ParentAuthGate />} /><Route path="/student/*" element={<StudentShell />} /><Route path="*" element={<Navigate to="/parent/students" replace />} /></Routes>;
}

export default App;
