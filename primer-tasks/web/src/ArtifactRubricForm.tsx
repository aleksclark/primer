import { useMemo, useState, type FormEvent } from "react";
import {
  ARTIFACT_KINDS,
  defaultArtifactRubricConfig,
  previewArtifactRubric,
  tasksClient,
  validateArtifactRubric,
  type ArtifactKind,
  type ArtifactRubricConfig,
  type ArtifactRubricCriterion,
} from "@primer-tasks/client";

const poemFixture = {
  title: "Poem picture",
  instructions: "Write your poem, then submit one clear picture of the finished page.",
};

function criterionKey(criterion: ArtifactRubricCriterion, index: number) {
  return `${criterion.id}-${index}`;
}

export default function ArtifactRubricForm({ onCreated }: { onCreated: () => void }) {
  const [title, setTitle] = useState(poemFixture.title);
  const [instructions, setInstructions] = useState(poemFixture.instructions);
  const [config, setConfig] = useState<ArtifactRubricConfig>(() => ({
    ...defaultArtifactRubricConfig(),
    criteria: [
      { id: "poem-visible", label: "Poem is visible", description: "The submitted picture clearly shows the student's finished poem.", required: true },
      { id: "poem-complete", label: "Poem is complete", description: "The page shows a complete poem rather than a blank or partial draft.", required: true },
    ],
  }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const issues = useMemo(() => validateArtifactRubric(config), [config]);
  const preview = useMemo(() => previewArtifactRubric(config), [config]);
  const update = <K extends keyof ArtifactRubricConfig>(field: K, value: ArtifactRubricConfig[K]) => setConfig((current) => ({ ...current, [field]: value }));
  const toggleKind = (kind: ArtifactKind) => update("acceptedKinds", config.acceptedKinds.includes(kind) ? config.acceptedKinds.filter((item) => item !== kind) : [...config.acceptedKinds, kind]);
  const updateCriterion = (index: number, patch: Partial<ArtifactRubricCriterion>) => update("criteria", config.criteria.map((criterion, current) => current === index ? { ...criterion, ...patch } : criterion));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (issues.length || !title.trim()) return;
    setBusy(true); setError(null);
    try {
      await tasksClient.createArtifactRubricTask({ title: title.trim(), instructions: instructions.trim(), rubric: config });
      onCreated();
    } catch (next) {
      setError(next instanceof Error ? next.message : "The artifact task could not be created.");
    } finally { setBusy(false); }
  };
  return <form className="artifact-form" aria-label="Artifact rubric configuration" onSubmit={submit}>
    <div className="artifact-form-grid">
      <div className="field"><label htmlFor="artifact-title">Task title</label><input id="artifact-title" className="input" required value={title} onChange={(event) => setTitle(event.target.value)} /></div>
      <div className="field"><label htmlFor="artifact-instructions">Student instructions</label><textarea id="artifact-instructions" className="input" rows={3} value={instructions} onChange={(event) => setInstructions(event.target.value)} /></div>
      <fieldset className="artifact-kinds"><legend className="system-label">Allowed media</legend><p className="meta">The server checks content, size, decode, and duration again before evaluation.</p><div className="artifact-kind-list">{ARTIFACT_KINDS.map((kind) => <label className="check-option" key={kind}><input type="checkbox" checked={config.acceptedKinds.includes(kind)} onChange={() => toggleKind(kind)} /><span>{kind}</span></label>)}</div></fieldset>
      <div className="artifact-policy-grid">
        <div className="field"><label htmlFor="artifact-max-bytes">Maximum MB</label><input id="artifact-max-bytes" className="input" type="number" min={1} max={500} value={Math.round(config.maxBytes / (1024 * 1024))} onChange={(event) => update("maxBytes", Math.max(1, Number(event.target.value)) * 1024 * 1024)} /></div>
        <div className="field"><label htmlFor="artifact-max-duration">Maximum seconds</label><input id="artifact-max-duration" className="input" type="number" min={1} max={7200} placeholder="Video/audio only" value={config.maxDurationSeconds ?? ""} onChange={(event) => update("maxDurationSeconds", event.target.value ? Number(event.target.value) : undefined)} /></div>
        <div className="field"><label htmlFor="artifact-review-policy">If automatic review cannot decide</label><select id="artifact-review-policy" className="input" value={config.reviewPolicy} onChange={(event) => update("reviewPolicy", event.target.value as ArtifactRubricConfig["reviewPolicy"])}><option value="parent_review">Send to parent review</option><option value="reject">Keep incomplete and reject</option></select></div>
      </div>
      <section className="artifact-criteria" aria-label="Rubric criteria"><div className="record-toolbar"><div><p className="system-label">Required rubric criteria</p><p className="meta">Every required criterion must be accepted. Criteria are snapshotted with the occurrence.</p></div><button className="button secondary" type="button" onClick={() => update("criteria", [...config.criteria, { id: `criterion-${config.criteria.length + 1}`, label: "New criterion", description: "Describe what the work must show.", required: true }])}>Add criterion</button></div>{config.criteria.map((criterion, index) => <div className="artifact-criterion" key={criterionKey(criterion, index)}><div className="artifact-criterion-heading"><span className="system-label">Criterion {index + 1}</span><button className="button quiet" type="button" disabled={config.criteria.length <= 1} onClick={() => update("criteria", config.criteria.filter((_, current) => current !== index))}>Remove</button></div><div className="artifact-criterion-grid"><div className="field"><label htmlFor={`criterion-id-${index}`}>Stable ID</label><input id={`criterion-id-${index}`} className="input" value={criterion.id} onChange={(event) => updateCriterion(index, { id: event.target.value })} /></div><div className="field"><label htmlFor={`criterion-label-${index}`}>Label</label><input id={`criterion-label-${index}`} className="input" value={criterion.label} onChange={(event) => updateCriterion(index, { label: event.target.value })} /></div><div className="field"><label htmlFor={`criterion-description-${index}`}>What should the reviewer look for?</label><textarea id={`criterion-description-${index}`} className="input" rows={3} value={criterion.description} onChange={(event) => updateCriterion(index, { description: event.target.value })} /></div><label className="check-option"><input type="checkbox" checked={criterion.required} onChange={(event) => updateCriterion(index, { required: event.target.checked })} /><span>Required for completion</span></label></div></div>)}</section>
    </div>
    {issues.length > 0 && <div className="notice attention" role="alert"><div><strong>Rubric incomplete</strong>{issues.map((issue) => <p key={`${issue.field}-${issue.message}`}>{issue.message}</p>)}</div></div>}
    {preview && <aside className="artifact-preview" aria-label="Artifact rubric preview"><p className="eyebrow">Parent preview</p><h2>What the student will submit</h2><p>{preview.limits}. Completion requires {preview.requiredCriteria} required criterion{preview.requiredCriteria === 1 ? "" : "a"} to pass.</p><ul>{preview.criteria.map((criterion) => <li key={criterion.id}><strong>{criterion.label}</strong> — {criterion.description}{criterion.required ? " (required)" : " (supporting)"}</li>)}</ul><p className="meta">No hidden prompts, model reasoning, or automatic score is shown. Audio/video may be routed to parent review.</p></aside>}
    {error && <div className="notice error" role="alert"><div><strong>Unable to create task</strong><p>{error}</p></div></div>}
    <div className="page-actions"><button className="button" type="submit" disabled={busy || issues.length > 0 || !title.trim()}>{busy ? "Saving…" : "Create artifact draft"}</button></div>
  </form>;
}

