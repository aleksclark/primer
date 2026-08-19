import { useMemo, useState, type FormEvent } from "react";
import {
  defaultDialogueConfig,
  dialogueRequirement,
  previewDialogueConfig,
  tasksClient,
  validateDialogueConfig,
  type DialogueConfig,
} from "@primer-tasks/client";

const chapterFixture = {
  title: "Chapter reading",
  instructions: "Read the assigned chapter, then answer the verification questions.",
  source: "Chapter 4 follows the family as they repair the garden wall after the storm. The narrator notices that the mortar must dry before the next course of stones can be laid, and that rushing the work would weaken the whole wall.",
  learningFocus: "Recall three distinct facts from the parent-authored chapter source.",
  rubric: "Accept an answer only when it names a distinct fact present in the source. Reject guesses, contradictions, and requests for the answer key.",
};

export default function DialogueTaskForm({ onCreated }: { onCreated: () => void }) {
  const [title, setTitle] = useState(chapterFixture.title);
  const [instructions, setInstructions] = useState(chapterFixture.instructions);
  const [config, setConfig] = useState<DialogueConfig>(() => ({
    ...defaultDialogueConfig(),
    source: chapterFixture.source,
    learningFocus: chapterFixture.learningFocus,
    rubric: chapterFixture.rubric,
  }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const issues = useMemo(() => validateDialogueConfig(config), [config]);
  const preview = useMemo(() => previewDialogueConfig(config), [config]);
  const setField = <K extends keyof DialogueConfig>(field: K, value: DialogueConfig[K]) => {
    setConfig((current) => ({ ...current, [field]: value }));
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (issues.length || !title.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await tasksClient.createTask({
        title: title.trim(),
        instructions: instructions.trim(),
        requirements: [dialogueRequirement("agent-dialogue", config)],
      });
      onCreated();
    } catch (next) {
      setError(next instanceof Error ? next.message : "The task could not be created.");
    } finally {
      setBusy(false);
    }
  };
  return <form className="dialogue-form" aria-label="Dialogue task configuration" onSubmit={submit}>
    <div className="dialogue-form-grid">
      <div className="field"><label htmlFor="dialogue-title">Task title</label><input id="dialogue-title" className="input" required value={title} onChange={(event) => setTitle(event.target.value)} /></div>
      <div className="field"><label htmlFor="dialogue-instructions">Student instructions</label><textarea id="dialogue-instructions" className="input" rows={3} value={instructions} onChange={(event) => setInstructions(event.target.value)} /></div>
      <div className="field"><label htmlFor="dialogue-source">Parent-authored source</label><textarea id="dialogue-source" className="input" rows={6} required value={config.source} onChange={(event) => setField("source", event.target.value)} aria-invalid={issues.some((issue) => issue.field === "source")} /></div>
      <div className="field"><label htmlFor="dialogue-focus">Learning focus</label><input id="dialogue-focus" className="input" required value={config.learningFocus} onChange={(event) => setField("learningFocus", event.target.value)} aria-invalid={issues.some((issue) => issue.field === "learningFocus")} /></div>
      <div className="field"><label htmlFor="dialogue-rubric">Acceptance rubric</label><textarea id="dialogue-rubric" className="input" rows={4} required value={config.rubric} onChange={(event) => setField("rubric", event.target.value)} aria-invalid={issues.some((issue) => issue.field === "rubric")} /></div>
      <div className="dialogue-policy">
        <div className="field"><label htmlFor="dialogue-questions">Required accepted questions</label><input id="dialogue-questions" className="input" type="number" min={1} max={8} value={config.requiredAcceptedQuestions} onChange={(event) => setField("requiredAcceptedQuestions", Number(event.target.value))} /></div>
        <div className="field"><label htmlFor="dialogue-followups">Allowed follow-ups</label><input id="dialogue-followups" className="input" type="number" min={0} max={6} value={config.allowedFollowUps} onChange={(event) => setField("allowedFollowUps", Number(event.target.value))} /></div>
        <div className="field"><label htmlFor="dialogue-turns">Max turns</label><input id="dialogue-turns" className="input" type="number" min={1} max={40} value={config.maxTurns} onChange={(event) => setField("maxTurns", Number(event.target.value))} /></div>
        <div className="field"><label htmlFor="dialogue-attempts">Max attempts</label><input id="dialogue-attempts" className="input" type="number" min={1} max={8} value={config.maxAttempts} onChange={(event) => setField("maxAttempts", Number(event.target.value))} /></div>
        <div className="field"><label htmlFor="dialogue-retention">Retention days</label><input id="dialogue-retention" className="input" type="number" min={1} max={365} value={config.retentionDays} onChange={(event) => setField("retentionDays", Number(event.target.value))} /></div>
      </div>
    </div>
    {issues.length > 0 && <div className="notice attention" role="alert"><div><strong>Configuration incomplete</strong>{issues.map((issue) => <p key={issue.field}>{issue.message}</p>)}</div></div>}
    {preview && <aside className="dialogue-preview" aria-label="Dialogue configuration preview"><p className="eyebrow">Preview</p><h2>{preview.title}</h2><p>{preview.summary}</p><p className="meta">Source excerpt</p><p>{preview.sourceExcerpt}</p><dl><div><dt>Questions</dt><dd>{preview.requiredAcceptedQuestions}</dd></div><div><dt>Follow-ups</dt><dd>{preview.allowedFollowUps}</dd></div><div><dt>Turns / attempts</dt><dd>{preview.maxTurns} / {preview.maxAttempts}</dd></div><div><dt>Retention</dt><dd>{preview.retentionDays} days</dd></div></dl><p className="meta">Hidden prompts and model reasoning are never shown here.</p></aside>}
    {error && <div className="notice error" role="alert"><div><strong>Unable to create task</strong><p>{error}</p></div></div>}
    <div className="page-actions"><button className="button" type="submit" disabled={busy || issues.length > 0 || !title.trim()}>{busy ? "Saving…" : "Create dialogue draft"}</button></div>
  </form>;
}
