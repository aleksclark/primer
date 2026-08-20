import { useEffect, useState, type FormEvent } from "react";
import { tasksClient, type ExternalState, type Occurrence } from "@primer-tasks/client";

function nextIdempotencyKey() {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  return `external-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export default function ExternalVerifierPage({ occurrence, onRefresh }: { occurrence: Occurrence; onRefresh: () => void }) {
  const [state, setState] = useState<ExternalState | null>(null);
  const [answer, setAnswer] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const load = () => tasksClient.studentExternalState(occurrence.id).then(setState).catch((next) => setError(next instanceof Error ? next.message : "External verification state is unavailable."));
  useEffect(() => { void load(); }, [occurrence.id]);
  useEffect(() => {
    if (!state || ["completed", "accepted", "rejected", "canceled"].includes(state.status)) return;
    const timer = window.setInterval(() => { void load(); }, 2500);
    return () => window.clearInterval(timer);
  }, [state?.status, occurrence.id]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!answer.trim()) return;
    setBusy(true); setError(null);
    try {
      await tasksClient.submitExternal(occurrence.id, { idempotencyKey: nextIdempotencyKey(), publicPayload: { response: answer.trim() } });
      setAnswer(""); await load(); onRefresh();
    } catch (next) { setError(next instanceof Error ? next.message : "The response could not be submitted."); }
    finally { setBusy(false); }
  };
  if (!state) return <section className="pair-card" aria-label="External verifier"><p className="system-label">External verification</p><p>{error ?? "Loading the verifier state…"}</p></section>;
  return <section className="pair-card" aria-label="External verifier progress">
    <div className="record-toolbar"><div><p className="system-label">External verification · {state.source}</p><h2 style={{ margin: "4px 0 0" }}>{state.status.replaceAll("_", " ")}</h2></div><span className={`status ${state.status === "completed" ? "active" : "attention"}`}>{state.status}</span></div>
    {error && <p className="notice error" role="alert">{error}</p>}
    {state.progress && state.progress.length > 0 && <ol aria-label="Safe verifier progress">{state.progress.map((item, index) => <li key={`${String(item.sequence ?? index)}-${index}`}>{String(item.message ?? item.status ?? "Verification update")}</li>)}</ol>}
    {state.safeRationale && <p>{state.safeRationale}</p>}
    {!state.attemptId || state.status === "completed" || state.status === "accepted" || state.status === "rejected" ? null : <form onSubmit={submit} style={{ display: "grid", gap: 12 }}><label className="field" htmlFor="external-response"><span>Your response</span><textarea id="external-response" className="input" rows={5} value={answer} onChange={(event) => setAnswer(event.target.value)} /></label><button className="button" type="submit" disabled={busy || !answer.trim()}>{busy ? "Submitting…" : "Submit for external verification"}</button></form>}
    {state.canRetry && <button className="button secondary" type="button" onClick={() => void tasksClient.retryExternal(occurrence.id).then(load).catch((next) => setError(next instanceof Error ? next.message : "Retry unavailable."))}>Retry verification</button>}
    <button className="button quiet" type="button" onClick={() => void load()}>Refresh state</button>
  </section>;
}
