import { useCallback, useEffect, useState } from "react";
import { tasksClient } from "@primer-tasks/client";

function formatStartedAt(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

/** The health view only reports the server-owned health contract. */
export default function HealthPage() {
  const [health, setHealth] = useState<Awaited<ReturnType<typeof tasksClient.health>> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    tasksClient.health().then(setHealth).catch((next) => setError(next instanceof Error ? next.message : "Unable to read Tasks service health.")).finally(() => setLoading(false));
  }, []);
  useEffect(() => { load(); }, [load]);

  return <>
    <header className="page-header">
      <div><p className="eyebrow">Parent workspace / Operate</p><h1>Tasks service health</h1><p>This is the server-owned health contract, including safe external-verifier queue and security counters. No endpoints, credentials, signatures, or payloads are exposed.</p></div>
      <div className="page-actions"><button className="button secondary" type="button" onClick={load} disabled={loading}>{loading ? "Refreshing…" : "Refresh"}</button></div>
    </header>
    {error && <div className="notice error" role="alert"><div><strong>Health unavailable</strong><p>{error}</p><button className="button quiet" type="button" onClick={load}>Try again</button></div></div>}
    {health && <section className="record" aria-label="Tasks service health">
      <div className="record-toolbar"><div><p className="system-label">Service status</p><p className="meta">Read-only operational information</p></div><span className={`status ${health.status === "ok" ? "active" : "attention"}`}>{health.status}</span></div>
      <dl className="inspect-provenance" style={{ padding: 20 }}><div><dt>Model provider</dt><dd>{health.modelProvider}</dd></div><div><dt>Server started</dt><dd>{formatStartedAt(health.startedAt)}</dd></div></dl>
      {health.externalVerifier && <><p className="system-label">External verifier delivery</p><dl className="inspect-provenance" style={{ padding: 20 }}><div><dt>Queued</dt><dd>{health.externalVerifier.queued}</dd></div><div><dt>Running</dt><dd>{health.externalVerifier.running}</dd></div><div><dt>Waiting</dt><dd>{health.externalVerifier.waiting}</dd></div><div><dt>Retryable</dt><dd>{health.externalVerifier.retryable}</dd></div><div><dt>Terminal</dt><dd>{health.externalVerifier.terminal}</dd></div><div><dt>Dead letters</dt><dd>{health.externalVerifier.dead}</dd></div><div><dt>Attempts</dt><dd>{health.externalVerifier.attempts}</dd></div><div><dt>Queue age (seconds)</dt><dd>{health.externalVerifier.queueAgeSeconds.toFixed(1)}</dd></div><div><dt>Average delivery (seconds)</dt><dd>{health.externalVerifier.averageDeliverySeconds.toFixed(1)}</dd></div><div><dt>Security failures (last hour)</dt><dd>{health.externalVerifier.securityFailures}</dd></div></dl></>}
    </section>}
  </>;
}
