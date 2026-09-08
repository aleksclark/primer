import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { NavLink } from "react-router-dom";
import { TasksApiError, tasksClient, reduceDialogueEvent, type DialogueClientSnapshot, type Occurrence, type StudentDialogueEvent, type StudentDialogueStateEvent, type TasksClient } from "@primer-tasks/client";

type Capability = NonNullable<Occurrence["verification"]>[number];

// The selected manual check, not global occurrence progress, determines which
// action the student can take. Another requirement may already be awaiting review.
export function manualActionState(occurrence: Occurrence, selected: Capability): "start" | "submit" | "waiting" | "accepted" | "closed" | "unavailable" {
  if (!occurrence.verification?.some(item => item.id === selected.id) || selected.kind !== "parent_approval" || selected.interaction !== "parent_action") return "unavailable";
  if (["completed", "canceled", "excused"].includes(occurrence.status)) return "closed";
  if (selected.attemptStatus === "accepted") return "accepted";
  if (!["", "open", "rejected"].includes(selected.attemptStatus)) return "unavailable";
  if (occurrence.status === "pending") return "start";
  if (occurrence.status === "in_progress") return "submit";
  if (occurrence.status === "awaiting_verification") return selected.attemptStatus === "open" ? "waiting" : "submit";
  return "unavailable";
}
export async function performManualAction(client: Pick<TasksClient, "startStudentOccurrence" | "submitStudentOccurrence">, occurrence: Occurrence, selected: Capability, submit: boolean) {
  const state = manualActionState(occurrence, selected);
  if (state !== (submit ? "submit" : "start")) throw new Error("Refresh the selected manual requirement before continuing.");
  const query = { requirementId: selected.id };
  return submit ? client.submitStudentOccurrence(occurrence.id, {}, query) : client.startStudentOccurrence(occurrence.id, {}, query);
}

export function dialogueErrorCopy(error: unknown): string {
  if (error instanceof TasksApiError) {
    if (error.status === 401 || error.status === 403) return "Access denied. Ask your parent to check this browser’s pairing.";
    if (error.status === 404) return "This dialogue is unavailable. It has not been replaced with manual approval. Refresh the assigned record or ask your parent.";
    if (error.status === 409) return "This record changed or its retry policy ended. Refresh and review the current question before continuing.";
  }
  return "The dialogue service did not return a usable record. Your saved evidence is retained. Try again or ask your parent.";
}
export function dialogueOutcome(snapshot: DialogueClientSnapshot): string {
  if (snapshot.completed) return "Task completed. The server has recorded the required evidence and all other required checks.";
  if (snapshot.status === "accepted") return "This dialogue requirement is accepted. The task is not complete until its other required checks are satisfied.";
  if (snapshot.occurrenceStatus === "canceled" || snapshot.occurrenceStatus === "excused") return "This assignment is closed. Earlier evidence is retained.";
  if (snapshot.status === "exhausted" || snapshot.status === "rejected") return "This attempt has ended without completing the task. Ask your parent to inspect it and decide whether another attempt is permitted.";
  return "";
}
export function transcriptLabel(event: StudentDialogueEvent): string {
  switch (event.kind) {
    case "question": return "Verifier question";
    case "message_ack": return "Your saved answer";
    case "answer_evaluation": return event.status === "accepted" ? "Evaluation · accepted answer" : "Evaluation · follow-up needed";
    case "override": return "Parent’s audited decision";
    case "complete": return "Recorded verification outcome";
    case "progress": return "Verifier progress";
    case "error": return "Verification interrupted";
    default: return "Server record";
  }
}
export function transcriptText(event: StudentDialogueEvent): string {
  if (event.kind === "progress") return event.phase === "evaluating" ? "Evaluating the saved answer." : "Thinking about the current question.";
  if (event.kind === "error") return "The verifier could not finish this step. Saved evidence is retained; review the current attempt status.";
  if (event.kind === "complete" || event.kind === "override") return `Requirement ${event.status}; assignment ${event.occurrenceStatus.replaceAll("_", " ")}.`;
  return "text" in event ? event.text ?? "" : "";
}

/** Capability selection is authoritative. A failed GET is an error, never an
 * invitation to fall back to the manual submission path. */
export default function StudentDialoguePage({ occurrence, capability, onRefresh }: {
  occurrence: Occurrence; capability: Capability; onRefresh: () => void;
}) {
  const [dialogue, setDialogue] = useState<StudentDialogueStateEvent>();
  const [loading, setLoading] = useState(capability.dialogueStarted);
  const [error, setError] = useState<unknown>(null);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    if (!capability.dialogueStarted) return;
    const controller = new AbortController();
    setLoading(true); setError(null); setDialogue(undefined);
    tasksClient.studentDialogue(occurrence.id, capability.id, { signal: controller.signal }).then(state => {
      if (!controller.signal.aborted) setDialogue(state);
    }).catch(next => { if (!controller.signal.aborted) setError(next); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [occurrence.id, capability.id, capability.dialogueStarted, capability.attemptId, reload]);
  const start = async () => {
    if (loading) return;
    setLoading(true); setError(null);
    try { setDialogue(await tasksClient.startStudentDialogue(occurrence.id, { requirementId: capability.id })); }
    catch (next) { setError(next); } finally { setLoading(false); }
  };
  return <section className="student-dialogue-workspace">
    <header className="page-header"><div><p className="eyebrow">Your work / Decide + Learn</p><h1>{occurrence.title}</h1><p>{occurrence.instructions}</p></div><NavLink className="button secondary" to="/student">Back to checklist</NavLink></header>
    {error ? <div className="notice error" role="alert"><div><strong>Dialogue unavailable</strong><p>{dialogueErrorCopy(error)}</p><button className="button secondary" onClick={() => { onRefresh(); setReload(value => value + 1); }}>Refresh record</button></div></div> : loading ? <p className="notice" role="status">Loading the current dialogue…</p> : dialogue ? <DialogueSession key={dialogue.attemptId} occurrence={occurrence} initial={dialogue} /> :
      <section className="dialogue-introduction"><h2>Read, then explain</h2><p>This requirement has not started. The verifier asks three distinct questions about the assigned source. Your answers and evaluations are retained for your parent to inspect.</p>
        {capability.attemptStatus === "exhausted" || capability.attemptStatus === "rejected" ? <p className="notice">Ask your parent to inspect this attempt and decide whether retry is permitted.</p> :
          <button className="button" disabled={loading || ["completed", "canceled", "excused"].includes(occurrence.status)} onClick={() => void start()}>Start reading dialogue</button>}
      </section>}
  </section>;
}

export function DialogueSession({ occurrence, initial }: { occurrence: Occurrence; initial: StudentDialogueStateEvent }) {
  const client = useMemo(() => tasksClient.createStudentDialogueClient(occurrence.id, initial.attemptId), [occurrence.id, initial.attemptId]);
  const [snapshot, setSnapshot] = useState(() => client.snapshot());
  const [text, setText] = useState("");
  const [needsReview, setNeedsReview] = useState(false);
  const [localError, setLocalError] = useState("");
  const pending = useRef<string | undefined>(undefined);
  const previousUnsent = useRef<string | undefined>(undefined);
  useEffect(() => {
    const unsubscribe = client.subscribe(next => {
      if (next.unsentAnswer !== undefined && previousUnsent.current === undefined) {
        setText(next.unsentAnswer); setNeedsReview(true); pending.current = undefined;
      }
      previousUnsent.current = next.unsentAnswer;
      if (pending.current && next.events.some(event => event.kind === "message_ack" && event.clientMessageId === pending.current)) {
        pending.current = undefined; setText("");
      }
      setSnapshot(next);
    });
    client.connect();
    return () => { unsubscribe(); client.disconnect(); };
  }, [client]);
  const state = snapshot.state ?? initial;
  const questionId = snapshot.binding?.questionId ?? state.questionId;
  const question = snapshot.events.findLast(event => event.kind === "question" && event.questionId === questionId);
  const questionText = question?.kind === "question" ? question.text : state.questionId === questionId ? state.text : undefined;
  const outcome = dialogueOutcome(snapshot.state ? snapshot : reduceDialogueEvent(snapshot, initial));
  const connected = snapshot.connectionState === "connected";
  const canSend = connected && snapshot.canAnswer && !state.phase && !needsReview && !outcome;
  const submit = (event: FormEvent) => {
    event.preventDefault(); if (!canSend || !text.trim()) return;
    setLocalError("");
    try { pending.current = client.sendMessage(text); }
    catch { setLocalError("This answer was not sent. Keep your draft, refresh the question and try again."); }
  };
  const retry = () => { try { client.retry(); setLocalError(""); } catch { setLocalError("No retryable saved answer is available. Refresh the record or ask your parent."); } };
  return <>
    <div className="dialogue-state" aria-live="polite"><span className="status">{snapshot.connectionState}</span><p>{outcome || (state.phase === "evaluating" ? "Evaluating your saved answer…" : state.phase === "thinking" ? "Thinking…" : "Answer the current question in your own words.")}</p></div>
    {!connected && <div className="notice" role="status"><div><strong>{snapshot.connectionState === "revoked" ? "Pairing unavailable" : "Connection interrupted"}</strong><p>Saved answers stay on the server. An unacknowledged answer is replayed with its original question, version and request key—not applied to a different question.</p>{snapshot.connectionState === "revoked" ? <NavLink className="button secondary" to="/student/pair">Pair this browser again</NavLink> : <button className="button secondary" onClick={() => client.connect()}>Reconnect</button>}</div></div>}
    {snapshot.error && <div className="notice error" role="alert"><div><strong>Verification needs attention</strong><p>{snapshot.error.message}</p>{state.phase === "failed" && state.retryable && connected && <button className="button secondary" onClick={retry}>Retry saved answer</button>}</div></div>}
    {localError && <p className="notice error" role="alert">{localError}</p>}
    <section className="dialogue-current" aria-label="Current question"><p className="system-label">Current question</p><h2>{questionText || (outcome ? "This attempt is closed." : "Waiting for the server’s question…")}</h2><p className="meta">{state.acceptedCount} of {state.requiredCount} distinct answers accepted · saved server state</p></section>
    {needsReview && <div className="notice error" role="alert"><div><strong>Review the refreshed question</strong><p>Your draft was not applied. Another tab or a newer version changed this turn. Check the current question above before deciding whether this draft still answers it.</p><button className="button secondary" disabled={!connected || !snapshot.canAnswer} onClick={() => setNeedsReview(false)}>I have reviewed this question</button></div></div>}
    {!outcome && <form className="dialogue-composer" onSubmit={submit}>
      <label className="field">Your answer<textarea className="input" rows={4} value={text} readOnly={!canSend} onChange={event => setText(event.target.value)} aria-describedby="dialogue-answer-help" /></label>
      <div className="dialogue-composer-footer"><p id="dialogue-answer-help" className="meta">Enter adds a line. Tab to Send answer to submit. Text clears only after a durable acknowledgement.</p><button className="button" disabled={!canSend || !text.trim()}>Send answer</button></div>
    </form>}
    <section className="dialogue-transcript" aria-label="Saved dialogue history"><h2>Saved dialogue history</h2><p>Questions, your original answers and safe evaluations, in server order. This view keeps at most 400 recent events; your parent can page through retained evidence.</p>
      {snapshot.events.length === 0 ? <p role="status">Waiting for durable history…</p> : snapshot.events.map(event => <article className={`dialogue-entry dialogue-${event.kind}`} key={event.sequence}><header><p className="system-label">{transcriptLabel(event)}</p><time className="meta" dateTime={event.time}>{new Date(event.time).toLocaleString()}</time></header><p>{transcriptText(event)}</p></article>)}
    </section>
  </>;
}
