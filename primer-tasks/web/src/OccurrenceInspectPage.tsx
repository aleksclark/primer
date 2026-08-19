import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  tasksClient,
  validateOverrideInput,
  type InspectEntry,
  type InspectTimeline,
} from "@primer-tasks/client";

function formatWhen(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
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
  const [error, setError] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [accepted, setAccepted] = useState(true);
  const [busy, setBusy] = useState(false);
  const load = useCallback(() => {
    setError(null);
    tasksClient.inspectOccurrence(id).then(setTimeline).catch((next) => setError(next instanceof Error ? next.message : "Unable to inspect this occurrence."));
  }, [id]);
  useEffect(load, [load]);
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
      setTimeline(await tasksClient.overrideOccurrence(id, { accepted, reason: reason.trim() }));
      setReason("");
    } catch (next) {
      setError(next instanceof Error ? next.message : "The override could not be recorded.");
    } finally {
      setBusy(false);
    }
  };
  return <>
    <header className="page-header"><div><p className="eyebrow">Parent workspace / Inspect</p><h1>Occurrence inspect</h1><p>Questions, answers, evaluations, and overrides stay append-only. Prior evidence is never rewritten.</p></div><div className="page-actions"><button className="button secondary" type="button" onClick={() => navigate("/parent/occurrences")}>Back to occurrences</button></div></header>
    {error && <div className="notice error" role="alert"><div><strong>Inspect problem</strong><p>{error}</p><button className="button quiet" type="button" onClick={load}>Try again</button></div></div>}
    {!timeline && !error && <div className="notice" role="status"><p>Loading the inspect timeline…</p></div>}
    {timeline && <div className="inspect-layout">
      <section className="inspect-timeline" aria-label="Verification timeline">
        {timeline.entries.length === 0 ? <div className="empty"><h2>No dialogue evidence yet</h2><p>The student has not produced a durable question or answer for this occurrence.</p></div> : timeline.entries.map((entry) => <Entry key={entry.id} entry={entry} />)}
      </section>
      <aside className="inspect-panel" aria-label="Inspect summary">
        <p className="eyebrow">Policy / provenance</p>
        <dl>
          <div><dt>Status</dt><dd>{timeline.status.replaceAll("_", " ")}</dd></div>
          <div><dt>Accepted</dt><dd>{timeline.acceptedCount} of {timeline.requiredCount}</dd></div>
          <div><dt>Provider</dt><dd>{timeline.provider ?? "—"}</dd></div>
          <div><dt>Policy</dt><dd>{timeline.policyVersion ?? "—"}</dd></div>
        </dl>
        {timeline.overrides.length > 0 && <section aria-label="Audited overrides">{timeline.overrides.map((row) => <article className="inspect-override" key={row.id}><p className="system-label">{row.accepted ? "Accepted override" : "Rejected override"}</p><p>{row.reason}</p><p className="meta">{row.actorId} · {formatWhen(row.createdAt)}</p></article>)}</section>}
        <form className="inspect-override-form" onSubmit={override}>
          <p className="system-label">Append-only override</p>
          <label className="field"><span>Decision</span><select className="input" aria-label="Override decision" value={accepted ? "accept" : "reject"} onChange={(event) => setAccepted(event.target.value === "accept")}><option value="accept">Accept requirement</option><option value="reject">Reject requirement</option></select></label>
          <label className="field"><span>Reason</span><textarea className="input" aria-label="Override reason" rows={4} value={reason} onChange={(event) => setReason(event.target.value)} required /></label>
          <button className="button" type="submit" disabled={busy || Boolean(validateOverrideInput({ accepted, reason }))}>{busy ? "Recording…" : "Record override"}</button>
          <p className="meta">This creates a separate audited decision. It cannot edit student answers or model evaluations.</p>
        </form>
      </aside>
    </div>}
  </>;
}
