import { useState, type FormEvent, type ReactNode } from "react";
import { presentStudentTask, type StudentTaskStatus } from "./student-task-presentation";

export type RequestState = "loading" | "ready" | "empty" | "error" | "denied" | "revoked" | "expired";
export type ChecklistScreenItem = {
  id: string;
  title: string;
  description: string;
  status: StudentTaskStatus;
  retried?: boolean;
  href?: string;
};

export type StudentChecklistScreenProps = {
  name: string;
  state: "ready" | "empty";
  items: ChecklistScreenItem[];
  onRefresh?: () => void;
  renderLink?: (item: ChecklistScreenItem, children: ReactNode, className: string, accessibleName: string) => ReactNode;
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
  status: StudentTaskStatus;
  retried?: boolean;
  message?: string;
  explanation?: string;
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

export function StudentChecklistScreen({ name, state, items, onRefresh, renderLink }: StudentChecklistScreenProps) {
  const [showCompleted, setShowCompleted] = useState(false);
  const presented = items.map((item, index) => ({ item, index, presentation: presentStudentTask(item.status, item.retried) }));
  const waiting = presented.filter(({ item }) => item.status === "awaiting_verification").length;
  const finished = presented.filter(({ item }) => item.status === "completed").length;
  const active = presented.filter(({ presentation }) => presentation.requiresAction).length;
  const terminalCount = presented.filter(({ presentation }) => presentation.terminal).length;
  const visibleItems = presented
    .filter(({ presentation }) => showCompleted || !presentation.terminal)
    .sort((left, right) => left.presentation.priority - right.presentation.priority || left.index - right.index);
  const summary = items.length ? `${active} active · ${finished} approved${waiting ? ` · ${waiting} sent to your parent` : ""}` : undefined;
  return <><ScreenHeader eyebrow="Your tasks" title={`Today with ${name}`} lede={summary} /><section className="checklist" data-review-target="checklist.items" aria-live="polite">{state === "empty" ? <div className="empty"><h2>You don’t have any tasks today</h2><p>You’re all caught up. Check back later or ask your parent.</p>{onRefresh && <button className="button quiet" type="button" onClick={onRefresh}>Check for updates</button>}</div> : <>{visibleItems.map(({ item, presentation }) => {
    const row = <><div><h2 className="checklist-title">{item.title}</h2>{item.description && <p>{item.description}</p>}</div><div className="checklist-row-end"><span className={`status ${presentation.className}`}>{presentation.label}</span><span className="open-affordance" aria-hidden="true">Open task →</span></div></>;
    const rowClass = `checklist-row ${presentation.requiresAction ? "requires-student-action" : "no-student-action"}`;
    const accessibleName = `Open ${item.title}, ${presentation.label}`;
    return renderLink && item.href ? renderLink(item, row, rowClass, accessibleName) : <div className={rowClass} data-review-target={`checklist.item.${item.id}`} key={item.id}>{row}</div>;
  })}{terminalCount > 0 && <div className="checklist-terminal-toggle"><button className="button quiet" type="button" aria-expanded={showCompleted} onClick={() => setShowCompleted((current) => !current)}>{showCompleted ? "Hide completed tasks" : `Show completed tasks (${terminalCount})`}</button></div>}</>}</section></>;
}

export function StudentOccurrenceScreen({ title, instructions, status, retried = false, message, explanation, action, busy = false, onAction, onRefresh, onBack }: StudentOccurrenceScreenProps) {
  const actionLabel = action === "start" ? "Start task" : action === "submit" ? "Submit for parent approval" : action === "retry" ? "Try this task again" : undefined;
  const presentation = presentStudentTask(status, retried);
  return <><button className="button quiet back-link" type="button" onClick={onBack}>← Back to today</button><ScreenHeader eyebrow="Your tasks" title={title} /><section className={`pair-card occurrence-card ${presentation.requiresAction ? "requires-student-action" : "no-student-action"}`} data-review-target="occurrence.detail"><p className={`status ${presentation.className}`} data-review-target="occurrence.status" role="status">{presentation.label}</p><div data-review-target="occurrence.instructions"><p className="system-label">What to do</p><p>{instructions}</p></div>{message && <p className="notice error" role="alert">{message}</p>}{explanation && <p className={`notice ${status === "unavailable" ? "error" : ""}`} role={status === "unavailable" ? "alert" : "status"}>{explanation}</p>}{action === "submit" && <p>Your work will stay saved while your parent checks it.</p>}{actionLabel && <button className="button occurrence-primary" data-review-target="occurrence.primary-action" type="button" disabled={busy} onClick={onAction}>{actionLabel}</button>}<div className="page-actions occurrence-actions"><button className="button quiet" type="button" onClick={onRefresh}>Check for updates</button>{!action && <button className="button secondary" type="button" onClick={onBack}>Back to today’s tasks</button>}</div></section></>;
}
