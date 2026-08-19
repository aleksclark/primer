import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import {
  createDialogueClient,
  studentProgressCopy,
  type AgentEvent,
  type DialogueClient,
  type DialogueClientSnapshot,
  type Occurrence,
  type StudentDialogueState,
} from "@primer-tasks/client";

type LocalAnswer = { clientMessageId: string; text: string };
type TranscriptItem = {
  key: string;
  sequence: number;
  kind: "question" | "answer" | "progress" | "retry" | "error" | "complete";
  text: string;
  label: string;
};

function latestStatus(events: readonly AgentEvent[]): string | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.kind === "terminal") return event.status;
    if (event.kind === "thinking_start" || event.kind === "text_start" || (event.kind === "tool_progress" && (event.phase === "started" || event.phase === "called" || event.phase === "evaluating"))) return "running";
    if (event.kind === "retry") return "retry";
    if (event.kind === "error") return "error";
  }
  return undefined;
}

function buildTranscript(events: readonly AgentEvent[], answers: readonly LocalAnswer[], dialogue: StudentDialogueState | null): TranscriptItem[] {
  const items: TranscriptItem[] = answers.map((answer, index) => ({
    key: `answer-${answer.clientMessageId}`,
    sequence: index - answers.length,
    kind: "answer",
    text: answer.text,
    label: "Your answer",
  }));
  const questions = new Map<string, TranscriptItem>();
  for (const event of events) {
    const streamId = event.runId || "dialogue";
    switch (event.kind) {
      case "text_start":
        questions.set(streamId, { key: `question-${streamId}`, sequence: event.sequence, kind: "question", text: event.text ?? "", label: "Question" });
        break;
      case "text_delta": {
        const current = questions.get(streamId) ?? { key: `question-${streamId}`, sequence: event.sequence, kind: "question" as const, text: "", label: "Question" };
        current.text += event.text;
        questions.set(streamId, current);
        break;
      }
      case "text_end": {
        const current = questions.get(streamId);
        if (current && event.text) current.text += event.text;
        break;
      }
      case "thinking_start":
      case "tool_progress":
        items.push({ key: `progress-${event.sequence}`, sequence: event.sequence, kind: "progress", text: event.kind === "tool_progress" && event.phase === "evaluating" ? "Evaluating" : "Thinking", label: "Progress" });
        break;
      case "retry":
        items.push({ key: `retry-${event.sequence}`, sequence: event.sequence, kind: "retry", text: dialogue?.retryExplanation ?? "Try again with a more complete answer.", label: "Retry" });
        break;
      case "error":
        items.push({ key: `error-${event.sequence}`, sequence: event.sequence, kind: "error", text: dialogue?.errorExplanation ?? "The verifier could not finish this turn. Your answer is saved.", label: "Error" });
        break;
      case "terminal":
        items.push({ key: `terminal-${event.sequence}`, sequence: event.sequence, kind: event.status === "failed" ? "error" : "complete", text: event.status === "succeeded" || event.status === "completed" ? (dialogue?.completionSummary ?? "Verification complete.") : (dialogue?.errorExplanation ?? "This verification did not finish."), label: event.status === "failed" ? "Error" : "Complete" });
        break;
      default:
        break;
    }
  }
  for (const item of questions.values()) items.push(item);
  return items.sort((a, b) => a.sequence - b.sequence);
}

export default function StudentDialoguePage({
  occurrence,
  dialogue,
  onRetry,
}: {
  occurrence: Occurrence;
  dialogue: StudentDialogueState;
  onRetry: () => void;
}) {
  const navigate = useNavigate();
  const client = useMemo(() => createDialogueClient({ conversationId: dialogue.conversationId, attemptId: dialogue.attemptId, occurrenceId: occurrence.id }), [dialogue.attemptId, dialogue.conversationId, occurrence.id]);
  const [snapshot, setSnapshot] = useState<DialogueClientSnapshot>({ connectionState: "idle", cursor: 0, events: [], queuedMessages: 0 });
  const [answers, setAnswers] = useState<LocalAnswer[]>([]);
  const [text, setText] = useState("");
  useEffect(() => {
    const unsubscribe = client.subscribe(setSnapshot);
    client.connect();
    return () => { unsubscribe(); client.disconnect(); };
  }, [client]);
  const items = buildTranscript(snapshot.events, answers, dialogue);
  const status = latestStatus(snapshot.events) ?? dialogue.status;
  const complete = dialogue.status === "complete" || status === "succeeded" || status === "completed";
  const offline = snapshot.connectionState === "offline" || snapshot.connectionState === "reconnecting";
  const send = (event: FormEvent) => {
    event.preventDefault();
    if (!text.trim() || complete) return;
    const clientMessageId = client.sendMessage(text);
    setAnswers((current) => [...current, { clientMessageId, text: text.trim() }].slice(-20));
    setText("");
  };
  return <>
    <header className="page-header"><div><p className="eyebrow">Student workspace / Decide + Learn</p><h1>{occurrence.title}</h1><p>Answer the current question. Completion happens only after the server records enough accepted answers.</p></div><div className="page-actions"><span className={`status ${snapshot.connectionState === "connected" ? "active" : offline ? "attention" : ""}`} role="status">{snapshot.connectionState === "connected" ? "Connected" : offline ? "Offline" : "Connecting"}</span><button className="button secondary" type="button" onClick={() => navigate("/student")}>Back to today</button></div></header>
    {snapshot.error && <div className="notice error" role="alert"><div><strong>Connection problem</strong><p>{snapshot.error.message}</p><button className="button quiet" type="button" onClick={() => { client.connect(); onRetry(); }}>Try again</button></div></div>}
    {offline && <div className="notice attention" role="status"><div><strong>Offline</strong><p>Your previous answers stay on the server. Reconnect to continue the remaining questions.</p></div></div>}
    {dialogue.retryExplanation && status === "retry" && <div className="notice attention" role="status"><div><strong>Try again</strong><p>{dialogue.retryExplanation}</p></div></div>}
    {dialogue.errorExplanation && status === "error" && <div className="notice error" role="alert"><div><strong>Verifier unavailable</strong><p>{dialogue.errorExplanation}</p><button className="button quiet" type="button" onClick={onRetry}>Retry</button></div></div>}
    <div className="dialogue-layout">
      <section className="dialogue-transcript" aria-label="Verification transcript" aria-live="polite">
        {dialogue.currentQuestion && <article className="dialogue-entry dialogue-question"><span className="system-label">Current question</span><p>{dialogue.currentQuestion}</p></article>}
        {items.length === 0 && !dialogue.currentQuestion ? <div className="empty"><h2>Ready to begin</h2><p>Start the task, then answer each question in complete sentences.</p></div> : items.map((item) => <article className={`dialogue-entry dialogue-${item.kind}`} key={item.key}><span className="system-label">{item.label}</span><p>{item.text}</p></article>)}
        {complete && <article className="dialogue-entry dialogue-complete"><span className="system-label">Complete</span><p>{dialogue.completionSummary ?? "This task is checked off."}</p></article>}
      </section>
      <aside className="dialogue-progress" aria-label="Progress">
        <p className="eyebrow">Learn / progress</p>
        <p>{studentProgressCopy(dialogue)}</p>
        <p className="meta">Thinking and evaluating are shown only as generic states. Scores and model reasoning never appear here.</p>
      </aside>
    </div>
    {!complete && <DialogueComposer client={client} text={text} setText={setText} disabled={offline || status === "error"} onSubmit={send} />}
  </>;
}

function DialogueComposer({
  client: _client,
  text,
  setText,
  disabled,
  onSubmit,
}: {
  client: DialogueClient;
  text: string;
  setText: (value: string) => void;
  disabled: boolean;
  onSubmit: (event: FormEvent) => void;
}) {
  return <form className="dialogue-composer" onSubmit={onSubmit}><label className="system-label" htmlFor="student-answer">Your answer</label><textarea id="student-answer" rows={4} value={text} onChange={(event) => setText(event.target.value)} disabled={disabled} placeholder="Write a complete answer" /><div className="dialogue-composer-footer"><span className="meta">Answers are saved before the verifier runs.</span><button className="button" type="submit" disabled={disabled || !text.trim()}>Send answer</button></div></form>;
}
