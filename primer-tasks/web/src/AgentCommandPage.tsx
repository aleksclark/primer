import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import {
  createAgentClient,
  safeAgentToolLabel,
  tasksClient,
  type AgentClient,
  type AgentClientSnapshot,
  type AgentEvent,
} from "@primer-tasks/client";
import "./index.css";

function PageHeader({ eyebrow, title, lede, actions }: { eyebrow: string; title: string; lede?: string; actions?: ReactNode }) {
  return <header className="page-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{lede && <p>{lede}</p>}</div>{actions && <div className="page-actions">{actions}</div>}</header>;
}

function ErrorNotice({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error && typeof error === "object" && "message" in error && typeof error.message === "string" ? error.message : "The agent connection did not return a usable response.";
  return <div className="notice error" role="alert"><div><strong>Agent connection problem</strong><p>{message}</p>{onRetry && <button className="button quiet" type="button" onClick={onRetry}>Try again</button>}</div></div>;
}

type SubmittedMessage = { clientMessageId: string; text: string };
type TranscriptItem = {
  key: string;
  sequence: number;
  kind: "user" | "assistant" | "thinking" | "tool" | "confirmation" | "retry" | "error" | "status";
  text: string;
  label?: string;
  confirmationId?: string;
  tone?: "active" | "attention";
};

function latestRunStatus(events: readonly AgentEvent[]): string | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.kind === "terminal") return event.status;
    if (event.kind === "tool_progress" && event.phase === "awaiting_confirmation") return "awaiting_confirmation";
    if (event.kind === "thinking_start" || event.kind === "text_start" || (event.kind === "tool_progress" && (event.phase === "started" || event.phase === "called"))) return "running";
  }
  return undefined;
}

function hasPendingConfirmation(events: readonly AgentEvent[]): boolean {
  const preview = [...events].reverse().find((event) => event.kind === "tool_progress" && event.phase === "awaiting_confirmation" && event.confirmationId);
  if (!preview || preview.kind !== "tool_progress" || preview.confirmationId === undefined) return false;
  return !events.some((event) => event.sequence > preview.sequence && (
    (event.kind === "tool_progress" && event.label === "Confirm change" && event.phase === "completed") ||
    (event.kind === "error" && event.code === "confirmation_rejected")
  ));
}

function buildTranscript(events: readonly AgentEvent[], submitted: readonly SubmittedMessage[]): TranscriptItem[] {
  const items: TranscriptItem[] = submitted.map((message, index) => ({
    key: `local-${message.clientMessageId}`,
    sequence: index - submitted.length,
    kind: "user",
    text: message.text,
    label: "Parent command",
  }));
  const assistant = new Map<string, TranscriptItem>();
  for (const event of events) {
    const streamId = event.runId || "agent";
    switch (event.kind) {
      case "text_start":
        assistant.set(streamId, { key: `assistant-${streamId}`, sequence: event.sequence, kind: "assistant", text: event.text ?? "", label: "Assistant" });
        break;
      case "text_delta": {
        const current = assistant.get(streamId) ?? { key: `assistant-${streamId}`, sequence: event.sequence, kind: "assistant" as const, text: "", label: "Assistant" };
        current.text += event.text;
        assistant.set(streamId, current);
        break;
      }
      case "text_end": {
        const current = assistant.get(streamId);
        if (current && event.text) current.text += event.text;
        break;
      }
      case "thinking_start":
        items.push({ key: `thinking-${event.sequence}`, sequence: event.sequence, kind: "thinking", text: "Thinking", label: "Progress" });
        break;
      case "tool_progress": {
        const isConfirmation = event.phase === "awaiting_confirmation";
        items.push({ key: `tool-${event.sequence}`, sequence: event.sequence, kind: isConfirmation ? "confirmation" : "tool", text: isConfirmation ? (event.summary ?? "A server preview is waiting for explicit parent confirmation.") : safeAgentToolLabel(event.label), label: isConfirmation ? "Confirmation required" : event.phase === "started" || event.phase === "called" ? "Working" : event.phase === "completed" ? "Complete" : "Progress", confirmationId: event.confirmationId, tone: isConfirmation ? "attention" : event.phase === "failed" ? "attention" : "active" });
        break;
      }
      case "retry":
        items.push({ key: `retry-${event.sequence}`, sequence: event.sequence, kind: "retry", text: `Retry ${event.retry}`, label: event.retryAfterMs ? `Bounded retry · ${event.retryAfterMs} ms` : "Bounded retry" });
        break;
      case "error":
        items.push({ key: `error-${event.sequence}`, sequence: event.sequence, kind: "error", text: "The server stopped this run before a confirmed result was produced.", label: event.code, tone: "attention" });
        break;
      case "terminal":
        items.push({ key: `terminal-${event.sequence}`, sequence: event.sequence, kind: event.status === "failed" || event.status === "disabled" ? "error" : "status", text: event.text ?? terminalCopy(event.status), label: terminalLabel(event.status), tone: event.status === "failed" || event.status === "disabled" ? "attention" : "active" });
        break;
      case "replay_gap":
        items.push({ key: `gap-${event.sequence}`, sequence: event.sequence, kind: "error", text: event.text ?? "Some live progress was unavailable; reconnect will resume from the durable cursor.", label: "Replay gap", tone: "attention" });
        break;
      default:
        break;
    }
  }
  for (const item of assistant.values()) items.push(item);
  return items.sort((a, b) => a.sequence - b.sequence);
}

function terminalCopy(status: string) {
  if (status === "cancelled") return "The run was canceled. No further agent work will be performed.";
  if (status === "disabled") return "Agent mode is unavailable. Use the ordinary Tasks and Schedules pages instead.";
  if (status === "failed") return "The run failed before a confirmed result was produced.";
  return "The run completed.";
}

function terminalLabel(status: string) {
  if (status === "cancelled") return "Canceled";
  if (status === "disabled") return "Unavailable";
  if (status === "failed") return "Failed";
  return "Complete";
}

function connectionLabel(snapshot: AgentClientSnapshot) {
  if (snapshot.connectionState === "connecting") return "Connecting";
  if (snapshot.connectionState === "reconnecting") return "Reconnecting";
  if (snapshot.connectionState === "offline") return "Offline";
  if (snapshot.connectionState === "connected") return "Connected";
  return "Not connected";
}

function ConnectionState({ snapshot }: { snapshot: AgentClientSnapshot }) {
  const className = snapshot.connectionState === "connected" ? "active" : snapshot.connectionState === "offline" ? "attention" : "";
  return <span className={`status ${className}`} role="status">{connectionLabel(snapshot)}</span>;
}

function AgentTranscript({ items, client, snapshot }: { items: TranscriptItem[]; client: AgentClient; snapshot: AgentClientSnapshot }) {
  return <section className="agent-transcript" aria-label="Agent command record" aria-live="polite">
    {items.length === 0 ? <div className="empty"><h2>Ready for a parent command</h2><p>Ask for a bounded task or schedule inspection. Mutations are previewed and confirmed by the server before they take effect.</p></div> : items.map((item) => <div className={`agent-entry agent-entry-${item.kind}`} key={item.key}>
      <span className="system-label">{item.label ?? "Record"}</span>
      <p>{item.text || "Working…"}</p>
      {item.kind === "confirmation" && <span className="meta">The server owns the preview and requires an explicit confirmation command.</span>}
    </div>)}
    {hasPendingConfirmation(snapshot.events) && items.some((item) => item.kind === "confirmation") && <div className="agent-confirmation-actions" aria-label="Confirmation state"><p className="meta">No mutation is implied until you confirm this single-use server preview.</p>{[...items].reverse().find((item) => item.kind === "confirmation" && item.confirmationId)?.confirmationId && <button className="button" type="button" onClick={() => client.confirm([...items].reverse().find((item) => item.kind === "confirmation" && item.confirmationId)!.confirmationId!, snapshot.runId)}>Confirm preview</button>}<button className="button secondary" type="button" onClick={() => client.cancel(snapshot.runId)}>Cancel request</button></div>}
  </section>;
}

function AgentComposer({ client, disabled }: { client: AgentClient; disabled: boolean }) {
  const [text, setText] = useState("");
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!text.trim() || disabled) return;
    client.sendMessage(text);
    setText("");
  };
  return <form className="agent-composer" onSubmit={submit}><label className="system-label" htmlFor="agent-command">Parent command</label><textarea id="agent-command" rows={3} value={text} onChange={(event) => setText(event.target.value)} placeholder="List the tasks for this week" disabled={disabled} /><div className="agent-composer-footer"><span className="meta">Enter a request; the server decides scope and available tools.</span><button className="button" type="submit" disabled={disabled || !text.trim()}>Send command</button></div></form>;
}

export default function AgentCommandPage() {
  const [conversationId, setConversationId] = useState<string | null>(null);
  const [conversationError, setConversationError] = useState<unknown>(null);
  const [submitted, setSubmitted] = useState<SubmittedMessage[]>([]);
  useEffect(() => {
    let active = true;
    tasksClient.createAgentConversation().then((conversation) => active && setConversationId(conversation.id)).catch((error) => active && setConversationError(error));
    return () => { active = false; };
  }, []);
  const client = useMemo(() => conversationId ? createAgentClient({ conversationId }) : null, [conversationId]);
  const [snapshot, setSnapshot] = useState<AgentClientSnapshot>({ connectionState: "idle", cursor: 0, events: [], queuedMessages: 0 });
  useEffect(() => {
    if (!client) return;
    const unsubscribe = client.subscribe(setSnapshot);
    client.connect();
    return () => { unsubscribe(); client.disconnect(); };
  }, [client]);
  const sendWrappedClient = useMemo(() => client && ({
    ...client,
    sendMessage(text: string) {
      const clientMessageId = client.sendMessage(text);
      if (text.trim()) setSubmitted((messages) => [...messages, { clientMessageId, text: text.trim() }].slice(-20));
      return clientMessageId;
    },
  }), [client]);
  if (!client || !sendWrappedClient) return <><PageHeader eyebrow="Parent workspace / Command + Inspect" title="Parent agent" lede="Preparing an authenticated conversation…" />{conversationError ? <ErrorNotice error={conversationError} /> : <div className="notice" role="status"><p>Opening the durable parent conversation.</p></div>}</>;
  const items = buildTranscript(snapshot.events, submitted);
  const status = latestRunStatus(snapshot.events);
  const disabled = status === "disabled";
  const active = status === "queued" || status === "running" || status === "awaiting_confirmation";
  return <><PageHeader eyebrow="Parent workspace / Command + Inspect" title="Parent agent" lede="A bounded command surface for inspecting and preparing Tasks changes. The ordinary task and schedule pages remain the source of truth." actions={<><ConnectionState snapshot={snapshot} />{active && <button className="button danger" type="button" onClick={() => sendWrappedClient.cancel(snapshot.events.find((event) => event.runId)?.runId)}>Cancel run</button>}</>} />
    {snapshot.error && <ErrorNotice error={snapshot.error} onRetry={() => client.connect()} />}
    {disabled && <div className="notice attention" role="status"><div><strong>Agent unavailable</strong><p>Provider-backed commands are disabled. No chat response or mutation is simulated. Continue in <a href="/parent/tasks">Tasks</a> or <a href="/parent/schedules">Schedules</a>.</p></div></div>}
    {snapshot.queuedMessages > 0 && <div className="agent-queued" role="status"><span className="status">Queued · {snapshot.queuedMessages}</span><span>Waiting for the server connection; commands will replay with their idempotency keys.</span></div>}
    <div className="agent-layout"><div><AgentTranscript items={items} client={sendWrappedClient} snapshot={snapshot} /><AgentComposer client={sendWrappedClient} disabled={disabled} /></div><aside className="agent-inspector" aria-label="Agent run boundaries"><p className="eyebrow">Inspect / boundaries</p><dl><div><dt>Transport</dt><dd>Authenticated WebSocket</dd></div><div><dt>Cursor</dt><dd className="meta">{snapshot.cursor || "—"}</dd></div><div><dt>Run state</dt><dd>{status ? status.replaceAll("_", " ") : "No active run"}</dd></div><div><dt>Authority</dt><dd>Server-owned Tasks and Schedules</dd></div></dl><p className="meta">Thinking is shown only as a generic state. Tool activity uses an allowlisted label; provider reasoning and raw tool arguments never render here.</p></aside></div>
  </>;
}

