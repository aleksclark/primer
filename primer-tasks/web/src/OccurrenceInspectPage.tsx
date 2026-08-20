import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  tasksClient,
  validateOverrideInput,
  type InspectEntry,
  type ArtifactStudentState,
  type ExternalState,
  type InspectTimeline,
} from "@primer-tasks/client";

function formatWhen(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

function ArtifactInspectPanel({ state }: { state: ArtifactStudentState }) {
  const submission = state.submissions.at(-1);
  const evaluation = state.evaluation;
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const showDerivative = async () => {
    if (!submission) return;
    setPreviewError(null);
    try {
      const blob = await tasksClient.getArtifactDerivative(state.occurrenceId, submission.artifactId);
      const reader = new FileReader();
      reader.onload = () => { if (typeof reader.result === "string") setPreviewUrl(reader.result); };
      reader.readAsDataURL(blob);
    } catch (next) { setPreviewError(next instanceof Error ? next.message : "The authorized derivative could not be loaded."); }
  };
  return <section className="artifact-inspect" aria-label="Artifact and rubric evaluation"><div className="record-toolbar"><div><p className="system-label">Artifact evidence</p><p className="meta">Authorized metadata and derivative review only · no object-store URL is exposed.</p></div><span className={`status ${evaluation?.accepted ? "active" : evaluation?.status === "review" ? "attention" : ""}`}>{evaluation?.status ?? state.status}</span></div>{!submission ? <div className="empty"><p>No media submission has been finalized.</p></div> : <div className="artifact-inspect-body"><p className="system-label">Snapshotted rubric</p><ul className="artifact-inspect-rubric">{state.config.criteria.map((criterion) => <li key={criterion.id}><strong>{criterion.label}</strong> — {criterion.description}{criterion.required ? " (required)" : " (supporting)"}</li>)}</ul><dl className="inspect-provenance"><div><dt>Media</dt><dd>{submission.kind} · {submission.mediaType}</dd></div><div><dt>Size</dt><dd>{Math.ceil(submission.sizeBytes / 1024)} KB</dd></div><div><dt>Digest</dt><dd>{submission.digest ?? "Pending validation"}</dd></div><div><dt>Submission</dt><dd>{submission.status}</dd></div></dl><div className="page-actions"><button className="button secondary" type="button" onClick={() => void showDerivative()}>View authorized derivative</button></div>{previewError && <p className="meta">{previewError}</p>}{previewUrl && submission.kind === "image" && <img className="artifact-file-preview" src={previewUrl} alt="Authorized student work derivative" />}{previewUrl && submission.kind === "video" && <video className="artifact-file-preview" src={previewUrl} controls aria-label="Authorized student video derivative" />}{previewUrl && submission.kind === "audio" && <audio src={previewUrl} controls aria-label="Authorized student audio derivative" />}{evaluation && <div className="artifact-criteria-results">{evaluation.criteria.map((criterion) => <article className={`artifact-criterion-result artifact-result-${criterion.status}`} key={criterion.id}><span className="status">{criterion.status}</span><strong>{criterion.criterionId}</strong>{criterion.feedback && <p>{criterion.feedback}</p>}{criterion.evidence && <p className="meta">Evidence: {criterion.evidence}</p>}</article>)}</div>}<p className="meta">Provider: {evaluation?.provider ?? "—"} · Model: {evaluation?.model ?? "—"} · Policy: {evaluation?.policyVersion ?? "—"}</p></div>}</section>;
}

function Entry({ entry }: { entry: InspectEntry }) {
  return <article className={`inspect-entry inspect-${entry.kind} inspect-author-${entry.author}`}>
    <header>
      <p className="system-label">{entry.title || entry.kind.replaceAll("_", " ")}</p>
      <p className="meta">{entry.author} · {formatWhen(entry.at)}</p>
    </header>
    <p>{entry.body}</p>
    {entry.outcome && <p className={`status ${entry.outcome === "accepted" ? "active" : "attention"}`}>{entry.outcome}</p>}
    {entry.rationale && <p className="inspect-rationale">{entry.rationale}</p>}
    {(entry.provider || entry.model || entry.policyVersion || entry.usage) && <dl className="inspect-provenance">
      {entry.provider && <div><dt>Provider</dt><dd>{entry.provider}</dd></div>}
      {entry.model && <div><dt>Model</dt><dd>{entry.model}</dd></div>}
      {entry.policyVersion && <div><dt>Policy</dt><dd>{entry.policyVersion}</dd></div>}
      {entry.usage && <div><dt>Usage</dt><dd>{[entry.usage.inputTokens !== undefined ? `${entry.usage.inputTokens} in` : null, entry.usage.outputTokens !== undefined ? `${entry.usage.outputTokens} out` : null].filter(Boolean).join(" · ") || "—"}</dd></div>}
    </dl>}
  </article>;
}

export default function OccurrenceInspectPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [timeline, setTimeline] = useState<InspectTimeline | null>(null);
  const [artifact, setArtifact] = useState<ArtifactStudentState | null>(null);
  const [external, setExternal] = useState<ExternalState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [accepted, setAccepted] = useState(true);
  const [busy, setBusy] = useState(false);
  const [actionBusy, setActionBusy] = useState<"retry" | "cancel" | null>(null);
  const load = useCallback(() => {
    setError(null);
    tasksClient.inspectOccurrence(id).then(setTimeline).catch((next) => setError(next instanceof Error ? next.message : "Unable to inspect this occurrence."));
    tasksClient.inspectOccurrenceArtifacts(id).then(setArtifact).catch(() => setArtifact(null));
    tasksClient.inspectExternal(id).then(setExternal).catch(() => setExternal(null));
  }, [id]);
  useEffect(load, [load]);
  const runOccurrenceAction = async (action: "retry" | "cancel") => {
    if (!timeline || timeline.status === "completed" || timeline.status === "canceled") return;
    if (action === "cancel" && !window.confirm("Cancel this occurrence? The saved evidence will remain inspectable.")) return;
    setActionBusy(action);
    setError(null);
    try {
      if (action === "retry") await tasksClient.retryOccurrence(id);
      else await tasksClient.cancelOccurrence(id);
      load();
    } catch (next) {
      setError(next instanceof Error ? next.message : `The occurrence could not be ${action === "retry" ? "retried" : "canceled"}.`);
    } finally {
      setActionBusy(null);
    }
  };
  const override = async (event: FormEvent) => {
    event.preventDefault();
    const invalid = validateOverrideInput({ accepted, reason });
    if (invalid) {
      setError(invalid);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await tasksClient.overrideOccurrence(id, { accepted, reason: reason.trim() });
      // The override response is only a decision acknowledgement. Reload the
      // authoritative inspect projection so prior evidence remains visible.
      setTimeline(await tasksClient.inspectOccurrence(id));
      setReason("");
    } catch (next) {
      setError(next instanceof Error ? next.message : "The override could not be recorded.");
    } finally {
      setBusy(false);
    }
  };
  return <>
    <header className="page-header"><div><p className="eyebrow">Parent workspace / Inspect</p><h1>Occurrence inspect</h1><p>Questions, answers, evaluations, and overrides stay append-only. Prior evidence is never rewritten.</p></div><div className="page-actions"><button className="button secondary" type="button" onClick={() => navigate("/parent/occurrences")}>Back to occurrences</button>{timeline && timeline.status !== "completed" && timeline.status !== "canceled" && <><button className="button secondary" type="button" onClick={() => void runOccurrenceAction("retry")} disabled={actionBusy !== null}>{actionBusy === "retry" ? "Retrying…" : "Retry verification"}</button><button className="button quiet" type="button" onClick={() => void runOccurrenceAction("cancel")} disabled={actionBusy !== null}>{actionBusy === "cancel" ? "Canceling…" : "Cancel occurrence"}</button></>}</div></header>
    {error && <div className="notice error" role="alert"><div><strong>Inspect problem</strong><p>{error}</p><button className="button quiet" type="button" onClick={load}>Try again</button></div></div>}
    {!timeline && !error && <div className="notice" role="status"><p>Loading the inspect timeline…</p></div>}
    {external && <section className="record" aria-label="External verifier delivery"><div className="record-toolbar"><div><p className="system-label">External verifier</p><p className="meta">Source: {external.source} · status: {external.status}</p></div><span className={`status ${external.status === "completed" ? "active" : "attention"}`}>{external.status}</span></div>{external.progress?.length ? <ol>{external.progress.map((item, index) => <li key={`${String(item.sequence ?? index)}-${index}`}>{String(item.message ?? item.status ?? "Verifier update")}</li>)}</ol> : <p className="meta">No safe progress has been reported yet.</p>}{external.safeRationale && <p>{external.safeRationale}</p>}<div className="page-actions">{external.canRetry && <button className="button secondary" type="button" onClick={() => void tasksClient.retryExternal(id).then(load).catch(setError)}>Retry external delivery</button>}{external.canCancel && <button className="button quiet" type="button" onClick={() => void tasksClient.cancelExternal(id).then(load).catch(setError)}>Cancel external delivery</button>}{external.fallbackAvailable && external.canRetry && <button className="button quiet" type="button" onClick={() => void tasksClient.fallbackExternal(id, { accepted: false, reason: "Parent routed this delivery to human review." }).then(load).catch(setError)}>Route to human review</button>}</div></section>}
    {artifact && <ArtifactInspectPanel state={artifact} />}
    {timeline && <div className="inspect-layout">
      <section className="inspect-timeline" aria-label="Verification timeline">
        {timeline.entries.length === 0 ? <div className="empty"><h2>No dialogue evidence yet</h2><p>The student has not produced a durable question or answer for this occurrence.</p></div> : timeline.entries.map((entry) => <Entry key={entry.id} entry={entry} />)}
      </section>
      <aside className="inspect-panel" aria-label="Inspect summary">
        <p className="eyebrow">Policy / provenance</p>
        <dl>
          <div><dt>Status</dt><dd>{artifact ? artifact.status.replaceAll("_", " ") : timeline.status.replaceAll("_", " ")}</dd></div>
          <div><dt>Accepted</dt><dd>{artifact?.evaluation ? `${artifact.evaluation.criteria.filter((criterion) => criterion.status === "accepted").length} of ${artifact.evaluation.criteria.filter((criterion) => criterion.required).length}` : `${timeline.acceptedCount} of ${timeline.requiredCount}`}</dd></div>
          <div><dt>Provider</dt><dd>{artifact?.evaluation?.provider ?? timeline.provider ?? "—"}</dd></div>
          <div><dt>Policy</dt><dd>{artifact?.evaluation?.policyVersion ?? timeline.policyVersion ?? "—"}</dd></div>
        </dl>
        {timeline.overrides.length > 0 && <section aria-label="Audited overrides">{timeline.overrides.map((row) => <article className="inspect-override" key={row.id}><p className="system-label">{row.accepted ? "Accepted override" : "Rejected override"}</p><p>{row.reason}</p><p className="meta">{row.actorId} · {formatWhen(row.createdAt)}</p></article>)}</section>}
        <form className="inspect-override-form" onSubmit={override}>
          <p className="system-label">Human review fallback · append-only override</p>
          <label className="field" htmlFor="override-decision"><span>Decision</span><select id="override-decision" name="decision" className="input" aria-label="Override decision" value={accepted ? "accept" : "reject"} onChange={(event) => setAccepted(event.target.value === "accept")}><option value="accept">Accept requirement</option><option value="reject">Reject requirement</option></select></label>
          <label className="field" htmlFor="override-reason"><span>Reason</span><textarea id="override-reason" name="reason" className="input" aria-label="Override reason" rows={4} value={reason} onChange={(event) => setReason(event.target.value)} required /></label>
          <button className="button" type="submit" disabled={busy || Boolean(validateOverrideInput({ accepted, reason }))}>{busy ? "Recording…" : "Record override"}</button>
          <p className="meta">This is the current human-review fallback. It creates a separate audited decision and cannot edit student answers or model evaluations.</p>
        </form>
      </aside>
    </div>}
  </>;
}
