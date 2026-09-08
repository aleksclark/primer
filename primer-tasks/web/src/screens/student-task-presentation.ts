export type StudentTaskStatus = string;

export type StudentTaskPresentation = {
  label: string;
  className: string;
  priority: number;
  terminal: boolean;
  requiresAction: boolean;
};

export function presentStudentTask(status: StudentTaskStatus, retried = false): StudentTaskPresentation {
  if (status === "in_progress") return { label: "In progress", className: "status-in-progress", priority: 0, terminal: false, requiresAction: true };
  if (status === "pending" && retried) return { label: "Needs another try", className: "status-retry", priority: 1, terminal: false, requiresAction: true };
  if (status === "pending") return { label: "Ready to start", className: "status-ready", priority: 2, terminal: false, requiresAction: true };
  if (status === "awaiting_verification") return { label: "Sent to your parent", className: "status-sent", priority: 3, terminal: false, requiresAction: false };
  if (status === "unavailable") return { label: "Unavailable on this device", className: "status-terminal", priority: 4, terminal: false, requiresAction: false };
  if (status === "completed") return { label: "Approved", className: "status-terminal", priority: 5, terminal: true, requiresAction: false };
  if (status === "excused") return { label: "Skipped", className: "status-terminal", priority: 5, terminal: true, requiresAction: false };
  return { label: "Canceled", className: "status-terminal", priority: 5, terminal: true, requiresAction: false };
}
