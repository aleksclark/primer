import { useEffect, useState, type FormEvent } from "react";
import { tasksClient, type ExternalVerifier } from "@primer-tasks/client";

export default function ExternalTaskForm({ onCreated }: { onCreated: () => void }) {
  const [verifiers, setVerifiers] = useState<ExternalVerifier[]>([]);
  const [verifierId, setVerifierId] = useState("");
  const [capability, setCapability] = useState("");
  const [schemaVersion, setSchemaVersion] = useState("");
  const [title, setTitle] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => { tasksClient.listExternalVerifiers().then((page) => { setVerifiers(page.items ?? []); if (page.items?.[0]) { setVerifierId(page.items[0].id); setCapability((page.items[0].capabilities ?? [])[0] ?? ""); setSchemaVersion((page.items[0].schemaVersions ?? [])[0] ?? ""); } }).catch((next) => setError(next instanceof Error ? next.message : "Verifier catalog unavailable.")); }, []);
  const selected = verifiers.find((item) => item.id === verifierId);
  const submit = async (event: FormEvent) => {
    event.preventDefault(); if (!title.trim() || !verifierId || !capability || !schemaVersion) return;
    setBusy(true); setError(null);
    try { await tasksClient.createTask({ title: title.trim(), instructions: "Submit the requested response for external verification.", requirements: [{ id: "external-callback", kind: "external_callback", configVersion: 1, config: { verifierId, capability, schemaVersion, options: {} }, interaction: "external", executor: "external" }] }); setTitle(""); onCreated(); }
    catch (next) { setError(next instanceof Error ? next.message : "The external task could not be created."); }
    finally { setBusy(false); }
  };
  return <form className="record-form" onSubmit={submit} aria-label="External verifier task configuration"><div className="field"><label htmlFor="external-title">Task title</label><input id="external-title" className="input" value={title} onChange={(event) => setTitle(event.target.value)} placeholder="Explain the design" required /></div><div className="field"><label htmlFor="external-verifier">Administrator verifier</label><select id="external-verifier" className="input" value={verifierId} onChange={(event) => { const next = verifiers.find((item) => item.id === event.target.value); setVerifierId(event.target.value); setCapability((next?.capabilities ?? [])[0] ?? ""); setSchemaVersion((next?.schemaVersions ?? [])[0] ?? ""); }}><option value="">Choose verifier</option>{verifiers.filter((item) => item.active).map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}</select></div>{selected && <p className="meta">Capabilities: {(selected.capabilities ?? []).join(", ")} · schemas: {(selected.schemaVersions ?? []).join(", ")} · no endpoint or secret is exposed here.</p>}<div className="page-actions"><input className="input" aria-label="Verifier capability" value={capability} onChange={(event) => setCapability(event.target.value)} placeholder="Capability" required /><input className="input" aria-label="Verifier schema version" value={schemaVersion} onChange={(event) => setSchemaVersion(event.target.value)} placeholder="Schema version" required /><button className="button" type="submit" disabled={busy || !title.trim() || !verifierId}>{busy ? "Saving…" : "Create external task"}</button></div>{error && <p className="notice error" role="alert">{error}</p>}</form>;
}
