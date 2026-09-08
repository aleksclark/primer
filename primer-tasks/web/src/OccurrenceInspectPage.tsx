import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { NavLink, useParams, useSearchParams } from "react-router-dom";
import { TasksApiError, tasksClient, type DialogueInspect, type DialogueOverrideRequest, type Occurrence } from "@primer-tasks/client";

// Stable across renders, unique across inspector instances. Names describe the
// field's purpose; existing wrapping labels continue to own accessible names.
export function useInspectField(name: string) {
  const instanceId = useId();
  return { id: `${name}-${instanceId}`, name };
}

type Capability = NonNullable<Occurrence["verification"]>[number];
export function requirementName(capability: Capability, index: number): string {
  return `${capability.kind === "agent_dialogue" ? "Reading dialogue" : capability.kind === "parent_approval" ? "Parent approval" : "Other verification"} · requirement ${index + 1}`;
}
export function inspectErrorCopy(error: unknown): string {
  if (error instanceof TasksApiError) {
    if (error.status === 503) return "The server could not validate or load this history. No partial timeline is shown. Evidence is retained; retry later.";
    if (error.status === 401 || error.status === 403) return "Access denied. Check your parent session and household membership.";
    if (error.status === 404) return "The selected record or attempt is unavailable. Refresh the assigned record; it has not been treated as an empty transcript.";
    if (error.status === 409) return "The record changed, this attempt is not current, or its policy does not permit this action. Review fresh state before creating a new request.";
  }
  return "The service did not confirm this request. Evidence and your pending decision are retained. Check the connection before trying again.";
}
export function makeOverride(timeline: DialogueInspect, reason: string, accepted: boolean, requestId: string): DialogueOverrideRequest {
  return { attemptId: timeline.attemptId, expectedVersion: timeline.version, clientRequestId: requestId, accepted, reason: reason.trim() };
}

export default function OccurrenceInspectPage() {
  const { id = "" } = useParams();
  return <OccurrenceRecord key={id} id={id} />;
}
function OccurrenceRecord({ id }: { id: string }) {
  const requirementField = useInspectField("inspect-requirement");
  const [params, setParams] = useSearchParams();
  const [occurrence, setOccurrence] = useState<Occurrence>();
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [locked, setLocked] = useState(false);
  const refresh = useCallback(() => setRevision(value => value + 1), []);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError(null);
    tasksClient.getOccurrence(id, { signal: controller.signal }).then(record => { if (!controller.signal.aborted) setOccurrence(record); })
      .catch(next => { if (!controller.signal.aborted) setError(next); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [id, revision]);
  const capabilities = occurrence?.verification;
  const selected = capabilities?.find(item => item.id === params.get("requirementId")) ?? (params.has("requirementId") ? undefined : capabilities?.[0]);
  const choose = (requirementId: string) => { const next = new URLSearchParams(); next.set("requirementId", requirementId); setParams(next); };
  return <section className="occurrence-inspect">
    <header className="page-header"><div><p className="eyebrow">For parents / Inspect</p><h1>{occurrence?.title ?? "Assigned work"}</h1><p>{occurrence?.studentName ?? "Loading student name…"}</p></div><div className="page-actions"><button className="button secondary" disabled={locked || loading} onClick={refresh}>Refresh record</button><NavLink className="button secondary" to="/parent/occurrences">Back to assigned work</NavLink></div></header>
    {error ? <div className="notice error" role="alert"><p>{inspectErrorCopy(error)}</p><button className="button secondary" onClick={refresh}>Refresh record</button></div> : loading ? <p className="notice" role="status">Loading issued verification requirements…</p> : occurrence && <>
      <p className="status">Assignment: {occurrence.status.replaceAll("_", " ")}</p><p>{occurrence.instructions}</p>
      {!capabilities?.length ? <p className="notice error" role="alert">This record has no usable verification capabilities. No approval or dialogue action can be inferred.</p> : <>
        <label className="field inspect-selector">Inspect requirement<select {...requirementField} className="input" disabled={locked} value={selected?.id ?? ""} onChange={event => choose(event.target.value)}><option value="" disabled>Choose a requirement</option>{capabilities.map((item, index) => <option key={item.id} value={item.id}>{requirementName(item, index)}</option>)}</select></label>
        {!selected ? <p className="notice error" role="alert">The selected requirement does not belong to this issued task.</p> : selected.kind === "agent_dialogue" ? <DialogueInspection key={`${selected.id}:${selected.attemptId}`} occurrence={occurrence} capability={selected} onChanged={refresh} onPending={setLocked} /> :
          <ManualInspection key={selected.id} occurrence={occurrence} capability={selected} onChanged={refresh} />}
      </>}
      <details className="inspect-identities"><summary>Issued record identity</summary><dl className="ruled-metadata"><div><dt>Assignment</dt><dd>{occurrence.id}</dd></div><div><dt>Task version</dt><dd>{occurrence.taskRevisionVersion}</dd></div><div><dt>Revision</dt><dd>{occurrence.revisionId}</dd></div></dl></details>
    </>}
  </section>;
}

function ManualInspection({ occurrence, capability, onChanged }: { occurrence: Occurrence; capability: Capability; onChanged: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const act = async (accepted: boolean) => {
    if (busy) return; setBusy(true); setError(null);
    try { await tasksClient.decideOccurrence(occurrence.id, { requirementId: capability.id, accepted, reason: accepted ? "Parent observed completion." : "Try again with care." }); onChanged(); }
    catch (next) { setError(next); } finally { setBusy(false); }
  };
  const retry = async () => { if (busy) return; setBusy(true); setError(null); try { await tasksClient.retryOccurrence(occurrence.id, {}, { requirementId: capability.id, attemptId: capability.attemptId }); onChanged(); } catch (next) { setError(next); } finally { setBusy(false); } };
  if (capability.kind !== "parent_approval" || capability.interaction !== "parent_action") return <p className="notice">This verification method has no supported action in this inspector. It is not treated as parent approval.</p>;
  return <section className="inspect-manual"><h2>Parent approval</h2><p>Attempt: {capability.attemptStatus || "not submitted"}. Approval applies only to this requirement; the server checks all required work before completing the assignment.</p>
    {occurrence.status === "awaiting_verification" && capability.attemptStatus === "open" && <div className="page-actions"><button className="button" disabled={busy} onClick={() => void act(true)}>Approve</button><button className="button danger" disabled={busy} onClick={() => void act(false)}>Reject</button></div>}
    {["pending", "awaiting_verification"].includes(occurrence.status) && capability.attemptStatus === "rejected" && <button className="button secondary" disabled={busy} onClick={() => void retry()}>Retry this requirement</button>}
    {error ? <p className="notice error" role="alert">{inspectErrorCopy(error)}</p> : null}
  </section>;
}

function DialogueInspection({ occurrence, capability, onChanged, onPending }: { occurrence: Occurrence; capability: Capability; onChanged: () => void; onPending: (pending: boolean) => void }) {
  const attemptField = useInspectField("inspect-attempt");
  const decisionField = useInspectField("override-decision");
  const reasonField = useInspectField("override-reason");
  const [params, setParams] = useSearchParams();
  const attemptId = params.get("attemptId") || capability.historyAttemptId || capability.attemptId;
  const after = Math.max(0, Number(params.get("after")) || 0);
  const attemptOffset = Math.max(0, Number(params.get("attemptOffset")) || 0);
  const [timeline, setTimeline] = useState<DialogueInspect>();
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);
  const [busy, setBusy] = useState(false);
  const [reason, setReason] = useState("");
  const [accepted, setAccepted] = useState(true);
  const [pending, setPending] = useState<DialogueOverrideRequest>();
  const [mutationError, setMutationError] = useState<unknown>(null);
  const [receipt, setReceipt] = useState("");
  const live = useRef(true);
  useEffect(() => { live.current = true; return () => { live.current = false; }; }, []);
  const refresh = () => setReload(value => value + 1);
  const notStarted = !capability.historyAttemptId && !params.has("attemptId");
  useEffect(() => {
    if (notStarted) { setLoading(false); return; }
    const controller = new AbortController(); setLoading(true); setError(null); setTimeline(undefined);
    tasksClient.inspectOccurrence(occurrence.id, { attemptId, after, limit: 20, attemptOffset }, { signal: controller.signal }).then(page => {
      if (!controller.signal.aborted) setTimeline(page);
    }).catch(next => { if (!controller.signal.aborted) setError(next); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [occurrence.id, attemptId, after, attemptOffset, reload, notStarted]);
  const changePage = (key: string, value: number) => { const next = new URLSearchParams(params); next.set(key, String(value)); setParams(next); };
  const override = async (event: FormEvent) => {
    event.preventDefault(); if (busy || !timeline) return;
    const request = pending ?? makeOverride(timeline, reason, accepted, crypto.randomUUID());
    setPending(request); onPending(true); setBusy(true); setMutationError(null); setReceipt("");
    try {
      await tasksClient.overrideOccurrence(occurrence.id, request);
      if (live.current) { setPending(undefined); onPending(false); setReceipt("Override recorded. Reloading durable state; other requirements may still be outstanding."); onChanged(); }
    } catch (next) { if (live.current) setMutationError(next); }
    finally { if (live.current) setBusy(false); }
  };
  const retry = async () => {
    if (busy) return; setBusy(true); setMutationError(null);
    try { await tasksClient.retryOccurrence(occurrence.id, {}, { requirementId: capability.id, attemptId: capability.attemptId }); if (live.current) onChanged(); }
    catch (next) { if (live.current) setMutationError(next); } finally { if (live.current) setBusy(false); }
  };
  const current = attemptId === capability.attemptId;
  const closed = ["completed", "canceled", "excused"].includes(timeline?.status ?? occurrence.status);
  if (notStarted) return <section className="inspect-manual"><h2>No dialogue history yet</h2><p>The authoritative issued capability says this attempt has not started. There is no transcript to inspect yet; the student can start it from their checklist.</p></section>;
  return <>
    {receipt && <p className="notice" role="status">{receipt}</p>}
    {mutationError ? <p className="notice error" role="alert">{inspectErrorCopy(mutationError)}</p> : null}
    {error ? <div className="notice error" role="alert"><p>{inspectErrorCopy(error)}</p><button className="button secondary" disabled={busy} onClick={refresh}>Reload history</button></div> : loading ? <p className="notice" role="status">Loading the server-ordered evidence page…</p> : timeline && <>
      <div className="record-toolbar"><label className="field">Attempt history<select {...attemptField} className="input" value={timeline.attemptId} disabled={busy || Boolean(pending)} onChange={event => {
        const attempt = timeline.attempts?.find(item => item.id === event.target.value); if (!attempt) return;
        const next = new URLSearchParams(params); next.set("attemptId", attempt.id); next.set("requirementId", attempt.requirementId); next.delete("after"); setParams(next);
      }}><option value={timeline.attemptId}>Selected {current ? "current" : "historic"} attempt</option>{timeline.attempts?.map(attempt => {
        const index = occurrence.verification?.findIndex(item => item.id === attempt.requirementId) ?? -1;
        const item = occurrence.verification?.[index];
        return <option key={attempt.id} value={attempt.id} disabled={attempt.kind !== "agent_dialogue" || (item?.attemptId === attempt.id && !item.dialogueStarted)}>{item ? requirementName(item, index) : "Requirement name unavailable"} · attempt {attempt.number} · {item?.attemptId === attempt.id && !item.dialogueStarted ? "dialogue not started" : attempt.status}</option>;
      })}</select></label>
        <div className="page-actions"><button className="button secondary" disabled={busy || Boolean(pending) || attemptOffset === 0} onClick={() => changePage("attemptOffset", Math.max(0, attemptOffset - timeline.attemptLimit))}>Earlier attempt page</button><button className="button secondary" disabled={busy || Boolean(pending) || attemptOffset + timeline.attemptLimit >= timeline.attemptTotal} onClick={() => changePage("attemptOffset", attemptOffset + timeline.attemptLimit)}>More attempts</button></div>
      </div>
      <div className="inspect-layout"><section className="inspect-timeline" aria-label="Verification timeline"><h2>Retained evidence</h2>
        {!timeline.entries?.length ? <p>No events on this page.</p> : timeline.entries.map(entry => <InspectEntry key={entry.sequence} entry={entry} />)}
        <div className="page-actions"><button className="button secondary" disabled={busy || Boolean(pending) || after === 0} onClick={() => changePage("after", 0)}>First events</button><button className="button secondary" disabled={busy || Boolean(pending) || !timeline.hasMore} onClick={() => changePage("after", timeline.nextCursor)}>Next events</button></div>
      </section><aside className="inspect-panel" aria-label="Evidence context"><h2>{timeline.title}</h2><p>{timeline.studentName}</p><dl className="ruled-metadata"><div><dt>Assignment state</dt><dd>{timeline.status.replaceAll("_", " ")}</dd></div><div><dt>Accepted questions</dt><dd>{timeline.acceptedCount} of {timeline.requiredCount}</dd></div><div><dt>Attempt version</dt><dd>{timeline.version}</dd></div><div><dt>Source version</dt><dd>{timeline.sourceVersion}</dd></div><div><dt>Source SHA-256</dt><dd>{timeline.sourceSha256}</dd></div></dl>
        <p>Evidence is retained. Original answers and evaluations cannot be edited or deleted here. Overrides append a separate audited decision.</p>
        {closed && <p className="notice">This assignment is closed. No reversal action is available here; original evidence remains retained.</p>}
        {!current && <p className="notice">Historical attempt: read only. Select the current requirement attempt to act.</p>}
        {current && !closed && <form className="inspect-override-form" onSubmit={override}><h3>Audited override</h3><label className="field">Decision<select {...decisionField} className="input" value={accepted ? "accept" : "reject"} disabled={busy || Boolean(pending)} onChange={event => setAccepted(event.target.value === "accept")}><option value="accept">Accept this requirement</option><option value="reject">Reject this requirement</option></select></label><label className="field">Reason<textarea {...reasonField} className="input" rows={4} required value={reason} disabled={busy || Boolean(pending)} onChange={event => setReason(event.target.value)} /></label><button className="button" disabled={busy || !reason.trim()}>{busy ? "Recording…" : pending ? "Retry identical decision request" : "Record override"}</button>
          {pending && <button type="button" className="button secondary" disabled={busy} onClick={() => { setPending(undefined); onPending(false); setMutationError(null); refresh(); }}>Refresh and review before a new request</button>}
          <p className="meta">An acknowledgement is not a timeline or proof of whole-task completion. Other required checks remain authoritative.</p>
        </form>}
        {current && !closed && ["rejected", "exhausted"].includes(capability.attemptStatus) && <button className="button secondary" disabled={busy || Boolean(pending)} onClick={() => void retry()}>Retry selected requirement attempt</button>}
      </aside></div>
    </>}
  </>;
}
export function InspectEntry({ entry }: { entry: NonNullable<DialogueInspect["entries"]>[number] }) {
  const label = entry.kind === "message_ack" ? "Student · original answer" : entry.kind === "question" ? "Verifier · question" : entry.kind === "answer_evaluation" ? "Verifier · evaluation" : entry.kind === "override" ? "Parent · audited override" : "System · recorded outcome";
  return <article className={`inspect-entry inspect-${entry.kind}`}><header><p className="system-label">{label}</p><time className="meta" dateTime={entry.at}>{new Date(entry.at).toLocaleString()}</time></header><p>{entry.text}</p>{entry.status && <p className="status">{entry.status}</p>}
    {Boolean(entry.criteria?.length) && <><p className="system-label">Recorded criteria</p><ul>{entry.criteria?.map(criterion => <li key={criterion}>{criterion}</li>)}</ul></>}
    <details><summary>Provenance and usage</summary><dl className="ruled-metadata"><div><dt>Policy</dt><dd>{entry.policyVersion}</dd></div>{entry.provider && <div><dt>Provider / model</dt><dd>{entry.provider} / {entry.model || "not recorded"}</dd></div>}<div><dt>Tokens in / out</dt><dd>{entry.inputTokens} / {entry.outputTokens}</dd></div><div><dt>Sequence</dt><dd>{entry.sequence}</dd></div>{entry.questionId && <div><dt>Question</dt><dd>{entry.questionId}</dd></div>}{entry.messageId && <div><dt>Message</dt><dd>{entry.messageId}</dd></div>}{entry.decisionSource && <div><dt>Decision source</dt><dd>{entry.decisionSource}</dd></div>}</dl></details>
  </article>;
}
