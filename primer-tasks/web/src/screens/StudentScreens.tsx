import { useState, type FormEvent, type ReactNode } from "react";

export type RequestState = "loading" | "ready" | "empty" | "error" | "denied" | "revoked" | "expired";

export type ChecklistScreenItem = {
  id: string;
  title: string;
  description: string;
  status: string;
  href?: string;
};

export type StudentChecklistScreenProps = {
  name: string;
  state: "ready" | "empty";
  items: ChecklistScreenItem[];
  onRefresh?: () => void;
  renderLink?: (item: ChecklistScreenItem, children: ReactNode, className: string) => ReactNode;
};

export type StudentPairScreenProps = {
  code: string;
  state: RequestState;
  notice?: ReactNode;
  onCodeChange: (code: string) => void;
  onSubmit: (event: FormEvent) => void;
};

export type StudentOccurrenceScreenProps = {
  title: string;
  instructions: string;
  status: string;
  message?: string;
  explanation?: string;
  supported?: boolean;
  action?: "start" | "submit" | "retry";
  busy?: boolean;
  onAction: () => void;
  onRefresh: () => void;
  onBack: () => void;
};

export function ScreenHeader({ eyebrow, title, lede, actions }: { eyebrow: string; title: string; lede?: string; actions?: ReactNode }) {
  return <header className="page-header" data-review-target="screen.header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{lede && <p>{lede}</p>}</div>{actions && <div className="page-actions">{actions}</div>}</header>;
}

export function StudentPairScreen({ code, state, notice, onCodeChange, onSubmit }: StudentPairScreenProps) {
  const helpId = "pair-code-help";
  return <><ScreenHeader eyebrow="Your checklist" title="Connect this browser" lede="Enter the code from your parent to open your checklist." /><section className="pair-card" data-review-target="pair.form" style={{ maxWidth: 560, marginTop: 28 }}><form onSubmit={onSubmit} style={{ display: "grid", gap: 18 }}><div className="field"><label htmlFor="pair-code">Pairing code</label><input id="pair-code" className="input code" required inputMode="text" autoComplete="one-time-code" autoCapitalize="characters" maxLength={32} value={code} onChange={(event) => onCodeChange(event.target.value.toUpperCase())} aria-describedby={helpId} aria-invalid={state !== "ready" && state !== "loading"} /><p id={helpId}>Enter the one-use code shown by your parent.</p></div>{notice}<button className="button" data-review-target="pair.primary-action" type="submit" disabled={state === "loading" || !code.trim()}>Pair this browser</button></form></section></>;
}

function studentActionRequired(status: string) {
  return status === "Ready to start" || status === "In progress" || status === "Needs another try";
}

function terminalStatus(status: string) {
  return status === "Approved" || status === "Completed" || status === "Skipped" || status === "Canceled";
}

function statusPriority(status: string) {
  if (status === "In progress") return 0;
  if (status === "Needs another try") return 1;
  if (status === "Ready to start") return 2;
  if (status === "Sent to your parent") return 3;
  if (status === "Unavailable on this device") return 4;
  return 5;
}

function statusClass(status: string) {
  if (status === "In progress") return "status-in-progress";
  if (status === "Sent to your parent") return "status-sent";
  if (status === "Needs another try") return "status-retry";
  if (status === "Ready to start") return "status-ready";
  return "status-terminal";
}

export function StudentChecklistScreen({ name, state, items, onRefresh, renderLink }: StudentChecklistScreenProps) {
  const [showCompleted, setShowCompleted] = useState(false);
  const waiting = items.filter((item) => item.status === "Sent to your parent").length;
  const finished = items.filter((item) => item.status === "Approved" || item.status === "Completed").length;
  const actionRequired = items.filter((item) => studentActionRequired(item.status)).length;
  const terminalCount = items.filter((item) => terminalStatus(item.status)).length;
  const visibleItems = items
    .filter((item) => showCompleted || !terminalStatus(item.status))
    .map((item, index) => ({ item, index }))
    .sort((left, right) => statusPriority(left.item.status) - statusPriority(right.item.status) || left.index - right.index)
    .map(({ item }) => item);
  const summary = items.length ? `${actionRequired} active · ${finished} approved${waiting ? ` · ${waiting} sent to your parent` : ""}` : undefined;
  return <><ScreenHeader eyebrow="Your tasks" title={`Today with ${name}`} lede={summary} /><section className="checklist" data-review-target="checklist.items" aria-live="polite">{state === "empty" ? <div className="empty"><h2>You don’t have any tasks today</h2><p>You’re all caught up. Check back later or ask your parent.</p>{onRefresh && <button className="button quiet" type="button" onClick={onRefresh}>Check for updates</button>}</div> : <>{visibleItems.map((item) => {
    const requiresAction = studentActionRequired(item.status);
    const row = <><div><h2 className="checklist-title">{item.title}</h2>{item.description && <p>{item.description}</p>}</div><div className="checklist-row-end"><span className={`status ${statusClass(item.status)}`}>{item.status}</span><span className="open-affordance" aria-hidden="true">Open task →</span></div></>;
    const rowClass = `checklist-row ${requiresAction ? "requires-student-action" : "no-student-action"}`;
    return renderLink && item.href ? renderLink(item, row, rowClass) : <div className={rowClass} data-review-target={`checklist.item.${item.id}`} key={item.id}>{row}</div>;
  })}{terminalCount > 0 && <div className="checklist-terminal-toggle"><button className="button quiet" type="button" aria-expanded={showCompleted} onClick={() => setShowCompleted((current) => !current)}>{showCompleted ? "Hide completed tasks" : `Show completed tasks (${terminalCount})`}</button></div>}</>}</section></>;
}

export function StudentOccurrenceScreen({ title, instructions, status, message, explanation, supported = true, action, busy = false, onAction, onRefresh, onBack }: StudentOccurrenceScreenProps) {
  const actionLabel = action === "start" ? "Start task" : action === "submit" ? "Submit for parent approval" : action === "retry" ? "Try this task again" : undefined;
  return <><button className="button quiet back-link" type="button" onClick={onBack}>← Back to today</button><ScreenHeader eyebrow="Your tasks" title={title} /><section className={`pair-card occurrence-card ${action ? "requires-student-action" : "no-student-action"}`} data-review-target="occurrence.detail"><p className={`status ${statusClass(status)}`} data-review-target="occurrence.status" role="status">{status}</p><div data-review-target="occurrence.instructions"><p className="system-label">What to do</p><p>{instructions}</p></div>{message && <p className="notice error" role="alert">{message}</p>}{explanation && <p className={`notice ${supported ? "" : "error"}`} role={supported ? "status" : "alert"}>{explanation}</p>}{action === "submit" && <p>Your work will stay saved while your parent checks it.</p>}{actionLabel && <button className="button occurrence-primary" data-review-target="occurrence.primary-action" type="button" disabled={busy} onClick={onAction}>{actionLabel}</button>}<div className="page-actions occurrence-actions"><button className="button quiet" type="button" onClick={onRefresh}>Check for updates</button>{!action && <button className="button secondary" type="button" onClick={onBack}>Back to today’s tasks</button>}</div></section></>;
}
