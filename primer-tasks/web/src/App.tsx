import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { QRCodeSVG } from "qrcode.react";
import { NavLink, Navigate, Route, Routes, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { TasksApiError, tasksClient, type Occurrence, type Schedule, type Student, type Task } from "@primer-tasks/client";
import AgentCommandPage from "./AgentCommandPage";
import StudentDialoguePage, { manualActionState, performManualAction } from "./StudentDialoguePage";
import OccurrenceInspectPage, { requirementName } from "./OccurrenceInspectPage";
import { TaskEditor, ScheduleForm } from "./TaskForms";
import { cadenceLabel, localDateTime } from "./schedule-presets";
import "./index.css";
import { appBase, useParentIdentity } from "./identity";

type Theme = "dark" | "light";

type RequestState = "loading" | "ready" | "empty" | "error" | "denied" | "revoked" | "expired";

// Source-backed, non-announcing attribute used by the hot-reload proof.
const HMR_PROOF_MARKER = "tasks-source-baseline";

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
    expired: ["Pairing expired", "This code has expired or already been used. Ask for a new code."],
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
    <img className="brand-dark" src={`${appBase}brand/logo-mark.svg`} alt="" />
    <img className="brand-light" src={`${appBase}brand/logo-mark-light.svg`} alt="" />
    <span className="brand-wordmark">Primer<strong>Tasks</strong></span>
    <span data-hmr-proof-marker={HMR_PROOF_MARKER} aria-hidden="true" />
  </NavLink>;
}

function ParentShell({ children }: { children: ReactNode }) {
  const identity = useParentIdentity();
  const [logoutError, setLogoutError] = useState<unknown>(null);
  const { theme, toggle } = useTheme();
  const [open, setOpen] = useState(false);
  return <div className={`app-shell ${open ? "mobile-nav-open" : ""}`}>
    <aside className="app-nav">
      <Brand />
      <button className="button secondary mobile-menu" type="button" onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls="parent-navigation">Menu</button>
      <div id="parent-navigation" className="nav-body">
        <div className="nav-section">
          <p className="system-label" style={{ padding: "0 20px" }}>For parents</p>
          <NavLink className="nav-link" to="/parent/students">Students</NavLink>
          <NavLink className="nav-link" to="/parent/tasks">Tasks</NavLink>
          <NavLink className="nav-link" to="/parent/schedules">Schedules</NavLink>
          <NavLink className="nav-link" to="/parent/occurrences">Assigned work</NavLink>
          <NavLink className="nav-link" to="/parent/agent">Parent agent</NavLink>
        </div>
        <div className="nav-section">
          <p className="system-label" style={{ padding: "0 20px" }}>Student access</p>
          <NavLink className="nav-link" to="/student/pair">Pair a browser</NavLink>
        </div>
      </div>
      <div className="nav-footer"><p className="system-label">Primer Tasks</p><ThemeButton theme={theme} toggle={toggle} /><button className="button quiet" type="button" onClick={() => void identity.signOut().catch(setLogoutError)}>Sign out</button></div>
    </aside>
    <main className="main"><div className="content">{logoutError ? <ErrorNotice error={logoutError} /> : null}{children}</div></main>
  </div>;
}

function ParentAuthGate() {
  const identity = useParentIdentity();
  const [status, setStatus] = useState<RequestState>("loading");
  const { theme, toggle } = useTheme();
  useEffect(() => {
    let mounted = true;
    setStatus("loading");
    tasksClient.parentSession().then(() => mounted && setStatus("ready")).catch((error) => mounted && setStatus(apiState(error)));
    return () => { mounted = false; };
  }, [identity.ready, identity.signedIn]);
  if (status === "loading") return <AuthFrame><StateNotice state="loading" /></AuthFrame>;
  if (status !== "ready") return <LoginPage theme={theme} toggle={toggle} denied={status === "denied"} />;
  return <ParentShell><Routes><Route path="students" element={<StudentsPage />} /><Route path="students/:studentId" element={<StudentDetailPage />} /><Route path="tasks" element={<TasksPage />} /><Route path="schedules" element={<SchedulesPage />} /><Route path="occurrences" element={<OccurrencesPage />} /><Route path="occurrences/:id/inspect" element={<OccurrenceInspectPage />} /><Route path="agent" element={<AgentCommandPage />} /><Route path="*" element={<Navigate to="students" replace />} /></Routes></ParentShell>;
}

function AuthFrame({ children }: { children: ReactNode }) {
  return <div className="auth-frame"><div className="auth-brand"><Brand /></div><main className="main"><div className="content">{children}</div></main></div>;
}

function LoginPage({ theme, toggle, denied }: { theme: Theme; toggle: () => void; denied: boolean }) {
  const identity = useParentIdentity();
  const [error, setError] = useState<unknown>(null);
  return <AuthFrame><section className="pair-card" style={{ maxWidth: 560, margin: "12vh auto 0" }}>
    <div className="page-header"><div><p className="eyebrow">Parent access</p><h1>Sign in to Primer Tasks</h1><p>Sign in to manage your household’s tasks and schedules.</p></div><ThemeButton theme={theme} toggle={toggle} /></div>
    <div style={{ display: "grid", gap: 16, marginTop: 24 }}>
      <p style={{ margin: 0, color: "var(--muted)" }}>{denied ? "You’re signed in, but don’t have access to this household. Ask a parent in your household to check your membership." : "Use your parent account to continue."}</p>
      <button className="button" type="button" onClick={identity.signIn}>Continue with parent sign-in</button>
      {identity.signedIn && <button className="button quiet" type="button" onClick={() => void identity.signOut().catch(setError)}>Sign out</button>}
      {error ? <ErrorNotice error={error} /> : null}
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
    <PageHeader eyebrow="For parents" title="Students" lede="Add students, update their names, and help them open their checklists." actions={<button className="button" type="button" onClick={() => setShowCreate(true)}>Add student</button>} />
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
  if (state === "loading") return <><PageHeader eyebrow="For parents" title="Student" /><StateNotice state="loading" /></>;
  if (!student) return <><PageHeader eyebrow="For parents" title="Student" /><ErrorNotice error={error} onRetry={load} /></>;
  return <><PageHeader eyebrow="For parents" title={student.displayName} lede="Manage this student’s name and access to their checklist." actions={<button className="button secondary" type="button" onClick={() => navigate("/parent/students")}>Back to students</button>} />{error ? <ErrorNotice error={error} /> : null}
    <div className="pair-layout"><section className="pair-card"><p className="eyebrow">Student record</p><h2>{student.displayName}</h2><p style={{ color: "var(--muted)" }}>Created {formatDate(student.createdAt)} · <span className="meta">{student.id}</span></p><div className="page-actions" style={{ marginTop: 24 }}><button className="button secondary" type="button" onClick={() => setEdit(true)}>Edit name</button>{!student.archivedAt && <button className="button danger" type="button" onClick={() => void tasksClient.archiveStudent(student.id).then(() => navigate("/parent/students"))}>Archive student</button>}</div></section><section className="pair-card"><p className="eyebrow">Student browser access</p><h2>Issue a pairing QR</h2><p style={{ color: "var(--muted)" }}>Have your student scan this code to open their checklist. It works once and expires soon.</p><button className="button" type="button" onClick={() => void issue()} disabled={Boolean(student.archivedAt)}>{pairing ? "Issue a new code" : "Issue pairing QR"}</button>{pairing && <PairingDisplay pairing={pairing} />}</section></div>{edit && <StudentForm student={student} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load(); }} />}</>;
}

function PairingDisplay({ pairing }: { pairing: Awaited<ReturnType<typeof tasksClient.issuePairing>> }) {
  return <div style={{ marginTop: 20 }}><p className="system-label">Show once · expires {formatDate(pairing.expiresAt)}</p><div className="qr-frame"><QRCodeSVG value={pairing.qrPayload} size={200} includeMargin={false} title="One-use student pairing QR code" role="img" aria-label="One-use student pairing QR code" /></div><span className="code">{pairing.code}</span><p style={{ color: "var(--muted)", fontSize: 13 }}>Keep this page open while your student scans or enters the code. Keep the code private.</p></div>;
}

function StudentShell() {
  const { theme, toggle } = useTheme();
  return <div className="app-shell"><aside className="app-nav"><Brand /><div className="nav-body"><div className="nav-section"><p className="system-label" style={{ padding: "0 20px" }}>Your tasks</p><NavLink className="nav-link" to="/student">Checklist</NavLink></div></div><div className="nav-footer"><ThemeButton theme={theme} toggle={toggle} /><p className="system-label">Your daily checklist</p></div></aside><main className="main"><div className="content"><Routes><Route path="" element={<StudentChecklistPage />} /><Route path="occurrences/:id" element={<StudentOccurrencePage />} /><Route path="pair" element={<StudentPairPage />} /><Route path="*" element={<Navigate to="/student" replace />} /></Routes></div></main></div>;
}

function StudentPairPage() {
  const navigate = useNavigate();
  const [code, setCode] = useState("");
  const [state, setState] = useState<RequestState>("ready");
  const [error, setError] = useState<unknown>(null);
  const submit = async (event: FormEvent) => { event.preventDefault(); if (!code.trim()) return; setState("loading"); setError(null); try { await tasksClient.pairStudent({ code: code.trim().toUpperCase() }); navigate("/student"); } catch (nextError) { setError(nextError); setState(apiState(nextError)); } };
  return <><PageHeader eyebrow="Your checklist" title="Connect this browser" lede="Enter the code your parent gives you to open your checklist on this browser." /><section className="pair-card" style={{ maxWidth: 560, marginTop: 28 }}><form onSubmit={submit} style={{ display: "grid", gap: 18 }}><div className="field"><label htmlFor="pair-code">Pairing code</label><input id="pair-code" className="input code" inputMode="text" autoComplete="one-time-code" autoCapitalize="characters" maxLength={32} value={code} onChange={(event) => setCode(event.target.value)} aria-invalid={Boolean(error)} /><p className="meta">Codes are one-use and expire quickly.</p></div>{state !== "ready" && state !== "loading" && <ErrorNotice error={error ?? new TasksApiError(state === "denied" ? 403 : state === "expired" ? 410 : 500, state, state)} />}{state === "loading" && <StateNotice state="loading" />}<button className="button" type="submit" disabled={state === "loading" || !code.trim()}>Pair this browser</button></form></section></>;
}

function TasksPage() {
  const [params, setParams] = useSearchParams();
  const [tasks, setTasks] = useState<Task[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [edit, setEdit] = useState<Task>();
  const [create, setCreate] = useState(false);
  const [assign, setAssign] = useState(false);
  const [message, setMessage] = useState("");
  const q = params.get("q") ?? "";
  const status = params.get("status") ?? "active";
  const offset = Math.max(0, Number(params.get("offset")) || 0);
  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true);
    return tasksClient.listTasks({ q, status, view: "templates", limit: 20, offset, sort: "title", dir: "asc" }, { signal }).then((page) => { setTasks(page.items ?? []); setTotal(page.totalCount); setError(null); }).catch((e) => { if (!signal?.aborted) setError(e); }).finally(() => { if (!signal?.aborted) setLoading(false); });
  }, [q, status, offset]);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  const changeQuery = (key: string, value: string) => { const next = new URLSearchParams(params); next.set(key, value); if (key !== "offset") next.delete("offset"); setParams(next); };
  const act = async (task: Task, archive: boolean) => {
    if (archive && !window.confirm(`Archive ${task.title}? It won’t be available for new schedules. Existing schedules and assigned work stay unchanged.`)) return;
    setBusy(true); setError(null);
    try {
      if (archive) await tasksClient.retireTask(task.templateId);
      else await tasksClient.publishTask(task.id);
      setMessage(archive ? "Task archived." : "Task published. You can now use it for new schedules. Existing schedules keep their current version.");
      await load();
    } catch (e) { setError(e); } finally { setBusy(false); }
  };
  const saved = () => { setEdit(undefined); setCreate(false); setMessage("Draft saved. Publish it when you’re ready to assign it."); void load(); };
  return <><PageHeader eyebrow="For parents" title="Tasks" lede="Write clear instructions, publish a task, then choose when your student will do it." actions={<><button className="button" type="button" onClick={() => { setCreate(true); setAssign(false); }}>Create task</button><button className="button secondary" type="button" onClick={() => { setAssign(true); setCreate(false); }}>Schedule a task</button></>} />
    {message && <p className="notice" role="status">{message}</p>}
    {create && <section className="record"><TaskEditor onSaved={saved} onClose={() => setCreate(false)} /></section>}
    {assign && <section className="record"><ScheduleForm onSaved={() => setMessage("")} onClose={() => setAssign(false)} /></section>}
    <section className="record" aria-label="Tasks"><div className="record-toolbar"><input className="input" aria-label="Search tasks" placeholder="Search tasks" value={q} onChange={(e) => changeQuery("q", e.target.value)} /><select className="input" aria-label="Task status filter" value={status} onChange={(e) => changeQuery("status", e.target.value)}><option value="active">Current tasks</option><option value="all">Include archived tasks</option></select></div>
      {error ? <ErrorNotice error={error} onRetry={() => void load()} /> : null}
      {loading ? <StateNotice state="loading" /> : <><div className="table-wrap"><table><thead><tr><th>Task</th><th>Status</th><th>Actions</th></tr></thead><tbody>{tasks.map((task) => <tr key={task.templateId}><td><strong>{task.title}</strong><details><summary>Instructions</summary><p>{task.instructions || "No instructions yet."}</p><span className="secondary-cell">Version {task.version} · {task.templateId}</span></details></td><td><span className="status">{task.templateStatus === "retired" ? "Archived" : task.status}</span></td><td>{task.templateStatus !== "retired" && <div className="row-actions"><button className="button secondary" type="button" disabled={busy} onClick={() => setEdit(task)}>Edit</button>{task.status === "draft" && <button className="button" type="button" disabled={busy} onClick={() => void act(task, false)}>Publish</button>}<button className="button danger" type="button" disabled={busy} onClick={() => void act(task, true)}>Archive</button></div>}</td></tr>)}</tbody></table></div>{!tasks.length && !error && <div className="empty"><h2>No tasks match</h2><p>Create a task or try a different search.</p></div>}<Pagination offset={offset} limit={20} total={total} onChange={(value) => changeQuery("offset", String(value))} /></>}
    </section>{edit && <TaskEditor task={edit} onSaved={saved} onClose={() => setEdit(undefined)} />}
  </>;
}

function SchedulesPage() {
  const [params, setParams] = useSearchParams();
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [edit, setEdit] = useState<Schedule>();
  const [busy, setBusy] = useState(false);
  const status = params.get("status") ?? "active";
  const offset = Math.max(0, Number(params.get("offset")) || 0);
  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true);
    return tasksClient.listSchedules({ limit: 20, offset, status }, { signal }).then((page) => { setSchedules(page.items ?? []); setTotal(page.totalCount); setError(null); }).catch((e) => { if (!signal?.aborted) setError(e); }).finally(() => { if (!signal?.aborted) setLoading(false); });
  }, [status, offset]);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  const cancel = async (schedule: Schedule) => {
    if (!window.confirm(`Cancel ${schedule.title ?? "this task"} for ${schedule.studentName ?? "this student"} at ${cadenceLabel(schedule)}? Already assigned work stays on the checklist. Cancel individual assignments from Assigned work if needed.`)) return;
    setBusy(true); setError(null);
    try { await tasksClient.retireSchedule(schedule.id); setEdit(undefined); await load(); } catch (e) { setError(e); } finally { setBusy(false); }
  };
  return <><PageHeader eyebrow="For parents" title="Schedules" lede="Choose when tasks repeat. If a task happens several times a day, each time has its own schedule to edit or cancel." actions={<button className="button secondary" type="button" onClick={() => void load()}>Refresh</button>} />
    {edit && <section className="record"><ScheduleForm key={edit.id} schedule={edit} onSaved={() => void load()} onClose={() => setEdit(undefined)} /></section>}
    {error ? <ErrorNotice error={error} onRetry={() => void load()} /> : null}
    <section className="record"><div className="record-toolbar"><label className="field">Show<select className="input" aria-label="Schedule status filter" value={status} onChange={(e) => { const next = new URLSearchParams(params); next.set("status", e.target.value); next.delete("offset"); setParams(next); }}><option value="active">Active schedules</option><option value="all">All schedules</option></select></label></div>
      {loading ? <StateNotice state="loading" /> : <><div className="table-wrap"><table><thead><tr><th>Task / student</th><th>Repeat</th><th>Time zone</th><th>Status</th><th>Actions</th></tr></thead><tbody>{schedules.map((schedule) => <tr key={schedule.id}><td><strong>{schedule.title ?? "Task title unavailable"}</strong><span className="secondary-cell">{schedule.studentName ?? "Student name unavailable"}</span></td><td>{cadenceLabel(schedule)}<span className="secondary-cell">Starts {localDateTime(schedule.startAt, schedule.timezone).slice(0, 10)}</span></td><td>{schedule.timezone}</td><td>{schedule.enabled ? "Active" : "Canceled"}</td><td>{schedule.enabled && <div className="row-actions"><button className="button secondary" type="button" disabled={busy} onClick={() => setEdit(schedule)}>Edit schedule</button><button className="button danger" type="button" disabled={busy} onClick={() => void cancel(schedule)}>Cancel schedule</button></div>}</td></tr>)}</tbody></table></div>{!schedules.length && !error && <div className="empty"><h2>No schedules match</h2><p>Publish a task, then choose Schedule a task on the Tasks page.</p></div>}<Pagination offset={offset} limit={20} total={total} onChange={(value) => { const next = new URLSearchParams(params); next.set("offset", String(value)); setParams(next); }} /></>}
    </section></>;
}

function OccurrencesPage() {
  const [params, setParams] = useSearchParams();
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [total, setTotal] = useState(0);
  const status = params.get("status") ?? "";
  const offset = Math.max(0, Number(params.get("offset")) || 0);
  const dir = params.get("dir") === "desc" ? "desc" : "asc";
  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true); setError(null);
    return tasksClient.listOccurrences({ limit: 20, offset, sort: "nominalAt", dir, status: status || undefined }, { signal }).then(page => {
      if (!signal?.aborted) { setOccurrences(page.items ?? []); setTotal(page.totalCount); }
    }).catch(next => { if (!signal?.aborted) setError(next); }).finally(() => { if (!signal?.aborted) setLoading(false); });
  }, [dir, status, offset]);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  const decide = (id: string, accepted: boolean, requirementId: string) => void tasksClient.decideOccurrence(id, { requirementId, accepted, reason: accepted ? "Parent observed completion." : "Try again with care." }).then(() => load()).catch(setError);
  const setCollection = (key: string, value: string) => { const next = new URLSearchParams(params); if (value) next.set(key, value); else next.delete(key); if (key !== "offset") next.delete("offset"); setParams(next); };
  return <><PageHeader eyebrow="For parents" title="Assigned work" lede="Review assigned work and check tasks your student has finished." actions={<button className="button secondary" type="button" onClick={() => void load()}>Refresh</button>} />{error ? <ErrorNotice error={error} onRetry={() => void load()} /> : null}{loading && <StateNotice state="loading" />}<section className="record"><div className="record-toolbar"><label className="field"><span className="system-label">Status filter</span><select className="input" aria-label="Assigned work status filter" value={status} onChange={(e) => setCollection("status", e.target.value)}><option value="">All statuses</option><option value="pending">Not started</option><option value="awaiting_verification">Awaiting verification</option><option value="completed">Completed</option></select></label><label className="field"><span className="system-label">Sort direction</span><select className="input" aria-label="Assigned work sort direction" value={dir} onChange={(e) => setCollection("dir", e.target.value)}><option value="asc">Soonest first</option><option value="desc">Latest first</option></select></label></div><div className="table-wrap"><table><thead><tr><th>Task</th><th>Student</th><th>Due</th><th>Status</th><th>Decision</th></tr></thead><tbody>{!loading && !error && occurrences.map((o) => <tr key={o.id}><td><strong>{o.title}</strong><span className="secondary-cell">{o.id}</span></td><td>{o.studentName ?? "Student name unavailable"}</td><td className="meta">{formatOccurrenceTime(o)}</td><td><span className="status">{verificationStatusLabel(o)}</span></td><td>{occurrenceActions(o, decide, () => void load(), setError)}</td></tr>)}</tbody></table></div>{!loading && !error && occurrences.length === 0 && <div className="empty"><h2>No assigned work</h2><p>Publish a task and create a schedule to add work here.</p></div>}{!loading && !error && <Pagination offset={offset} limit={20} total={total} onChange={value => setCollection("offset", String(value))} />}</section></>;
}

function StudentOccurrencePage() {
  const { id = "" } = useParams();
  return <StudentOccurrenceRecord key={id} id={id} />;
}
function StudentOccurrenceRecord({ id }: { id: string }) {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [occurrence, setOccurrence] = useState<Occurrence | null>(null);
  const [error, setError] = useState<unknown>(null);
  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true); setError(null);
    return tasksClient.studentOccurrence(id, { signal }).then(record => { if (!signal?.aborted) setOccurrence(record); }).catch(next => { if (!signal?.aborted) setError(next); }).finally(() => { if (!signal?.aborted) setLoading(false); });
  }, [id]);
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  const manual = async (submit: boolean) => {
    if (busy || !occurrence || !selected) return;
    setBusy(true);
    try { await performManualAction(tasksClient, occurrence, selected, submit); await load(); }
    catch (next) { setError(next); } finally { setBusy(false); }
  };
  if (loading) return <StateNotice state="loading" />;
  if (error) return <ErrorNotice error={error} onRetry={() => void load()} />;
  if (!occurrence) return <><PageHeader eyebrow="Your tasks" title="Task detail" />{error ? <ErrorNotice error={error} onRetry={() => void load()} /> : <StateNotice state="loading" />}</>;
  const capabilities = occurrence.verification;
  if (!capabilities?.length) return <p className="notice error" role="alert">Verification capabilities are unavailable. No manual or dialogue action can be inferred.</p>;
  const selected = capabilities.find(item => item.id === params.get("requirementId")) ?? (params.has("requirementId") ? undefined : capabilities[0]);
  const selection = <label className="field inspect-selector">Verification requirement<select className="input" value={selected?.id ?? ""} onChange={event => setParams({ requirementId: event.target.value })}><option value="" disabled>Choose a requirement</option>{capabilities.map((item, index) => <option value={item.id} key={item.id}>{requirementName(item, index)}</option>)}</select></label>;
  if (!selected) return <>{selection}<p className="notice error" role="alert">The selected requirement does not belong to this assignment.</p></>;
  if (selected.kind === "agent_dialogue" && selected.interaction === "chat") return <>{selection}<StudentDialoguePage key={selected.id} occurrence={occurrence} capability={selected} onRefresh={() => void load()} /></>;
  if (selected.kind !== "parent_approval" || selected.interaction !== "parent_action") return <>{selection}<p className="notice">This requirement has no supported action here. Ask your parent; it has not been replaced with manual approval.</p></>;
  const manualState = manualActionState(occurrence, selected);
  return <>{selection}<PageHeader eyebrow="Your tasks" title={occurrence.title} lede="Start when you’re ready. When you finish, ask your parent to check your work." actions={<button className="button secondary" type="button" onClick={() => navigate("/student")}>Back to today</button>} /><section className="pair-card"><p>{occurrence.instructions}</p><p className="status">{statusLabel(occurrence.status)}</p>{manualState === "start" && <button className="button" type="button" disabled={busy} onClick={() => void manual(false)}>Start task</button>}{manualState === "submit" && <><p>This selected requirement has not been approved. Finish its work, then submit it for parent approval.</p><button className="button" type="button" disabled={busy} onClick={() => void manual(true)}>Submit for parent approval</button></>}{manualState === "waiting" && <p className="meta">This requirement is waiting for parent approval.</p>}{manualState === "accepted" && <p className="meta">Parent approval recorded for this requirement. Other required work is still outstanding.</p>}{manualState === "unavailable" && <p className="notice error" role="alert">The selected requirement is unavailable. Refresh the record or ask your parent.</p>}{manualState === "closed" && <p className="status">{occurrence.status === "completed" ? "All required work completed" : "This assignment is closed"}</p>}</section></>;
}

function occurrenceActions(o: Occurrence, decide: (id: string, accepted: boolean, requirementId: string) => void, load: () => void, setError: (error: unknown) => void) {
  const terminal = o.status === "completed" || o.status === "canceled" || o.status === "excused";
  const only = o.verification?.length === 1 ? o.verification[0] : undefined;
  return <div className="row-actions"><NavLink className="button secondary" to={`/parent/occurrences/${o.id}/inspect`}>Inspect</NavLink>
    {!terminal && <>
      {o.status === "awaiting_verification" && only?.kind === "parent_approval" && only.interaction === "parent_action" && only.attemptStatus === "open" && <><button className="button" type="button" onClick={() => decide(o.id, true, only.id)}>Approve</button><button className="button danger" type="button" onClick={() => decide(o.id, false, only.id)}>Reject</button></>}
      {o.status === "pending" && only?.kind === "parent_approval" && only.interaction === "parent_action" && only.attemptStatus === "rejected" && <button className="button secondary" type="button" onClick={() => void tasksClient.retryOccurrence(o.id).then(load).catch(setError)}>Retry</button>}
      <button className="button quiet" type="button" onClick={() => void tasksClient.skipOccurrence(o.id).then(load).catch(setError)}>Skip</button>
      <button className="button quiet" type="button" onClick={() => void tasksClient.cancelOccurrence(o.id).then(load).catch(setError)}>Cancel</button>
    </>}
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
  if (state === "loading") return <><PageHeader eyebrow="Your tasks" title="Today" /><StateNotice state="loading" /></>;
  if (state === "error" || state === "denied" || state === "revoked" || state === "expired") return <><PageHeader eyebrow="Your tasks" title="Today" /><ErrorNotice error={error} onRetry={load} /><p className="meta">If access was revoked or expired, ask a parent for a new pairing code.</p></>;
  return <><PageHeader eyebrow="Your tasks" title={`Today with ${profile?.displayName ?? "you"}`} lede="Choose a task to get started, then follow its assigned checks. Saved evidence stays with your work." /><section className="checklist" aria-live="polite">{state === "empty" ? <div className="empty"><h2>Nothing assigned yet</h2><p>Your parent has not scheduled anything for today. This empty checklist is ready for the next task.</p></div> : <>{occurrences.map((item) => <NavLink className="checklist-row" to={`/student/occurrences/${item.id}`} key={item.id}><div><h3>{item.title}</h3><p>{item.instructions}</p></div><span className="status">{verificationStatusLabel(item)}</span></NavLink>)}{items.map((item) => <div className="checklist-row" key={item.id}><div><h3>{item.title}</h3>{item.description && <p>{item.description}</p>}</div><span className="status">{statusLabel(item.status)}</span></div>)}</>}</section></>;
}

function Pagination({ offset, limit, total, onChange }: { offset: number; limit: number; total: number; onChange: (offset: number) => void }) {
  if (total <= limit) return null;
  return <div className="record-toolbar"><span className="meta">{offset + 1}–{Math.min(offset + limit, total)} of {total}</span><div className="page-actions"><button className="button secondary" type="button" disabled={offset === 0} onClick={() => onChange(Math.max(0, offset - limit))}>Previous</button><button className="button secondary" type="button" disabled={offset + limit >= total} onClick={() => onChange(offset + limit)}>Next</button></div></div>;
}

function verificationStatusLabel(occurrence: Occurrence) {
  if (occurrence.status === "awaiting_verification" && occurrence.verification?.some(item => item.kind === "agent_dialogue")) return "Verification in progress";
  return statusLabel(occurrence.status);
}

function statusLabel(status: string) {
  const names: Record<string, string> = { pending: "Not started", in_progress: "In progress", awaiting_verification: "Waiting for parent", completed: "Completed", excused: "Skipped", canceled: "Canceled" };
  return names[status] ?? status.replaceAll("_", " ");
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
