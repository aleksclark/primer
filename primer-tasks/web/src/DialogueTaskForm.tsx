import { DIALOGUE_CONFIG_SCHEMA, parseDialogueConfig, type DialogueConfig, type Task } from "@primer-tasks/client";

type Requirements = NonNullable<Task["requirements"]>;
const fields = DIALOGUE_CONFIG_SCHEMA.properties;

export function newDialogueConfig(): DialogueConfig {
  return { sourceText: "", learningFocus: "", rubric: [""], requiredQuestions: fields.requiredQuestions.const,
    retentionPolicy: fields.retentionPolicy.const, allowedFollowUps: fields.allowedFollowUps.minimum,
    maxAttempts: fields.maxAttempts.minimum, maxTurns: fields.maxTurns.minimum };
}

/** Replace only the explicitly edited requirement. IDs and unrelated payloads
 * survive a revision, including methods this editor does not understand. */
export function replaceRequirement(requirements: Requirements, index: number, next: Requirements[number]): Requirements {
  return requirements.map((requirement, i) => i === index ? { ...next, id: requirement.id } : requirement);
}
export function dialogueFormValid(requirements: Requirements): boolean {
  return requirements.length > 0 && requirements.every(requirement => requirement.kind !== DIALOGUE_CONFIG_SCHEMA["x-manifest"].kind || parseDialogueConfig(requirement.config) !== null);
}

export default function DialogueTaskForm({ requirements, onChange, disabled }: {
  requirements: Requirements; onChange: (requirements: Requirements) => void; disabled: boolean;
}) {
  const update = (index: number, config: DialogueConfig) => onChange(replaceRequirement(requirements, index, { ...requirements[index], config: { ...config } }));
  return <section className="verification-config" aria-label="Verification requirements">
    <h3>How work is checked</h3>
    <p>Every requirement stays attached to this task version. Already assigned work keeps its original checks.</p>
    {requirements.map((requirement, index) => {
      // Partial edits remain typed configuration; the generated validator gates
      // saving. Unknown existing payloads are not cast into editable controls.
      const config = readEditableConfig(requirement.config);
      const dialogue = requirement.kind === DIALOGUE_CONFIG_SCHEMA["x-manifest"].kind;
      return <fieldset key={index} disabled={disabled} className="verification-requirement">
        <legend>Requirement {index + 1} · {dialogue ? "Reading dialogue" : requirement.kind === "parent_approval" ? "Parent approval" : "Other verification"}</legend>
        {requirement.kind === "parent_approval" && <>
          <p>Your student starts the task and submits it for your approval.</p>
          <button type="button" className="button secondary" onClick={() => {
            if (!window.confirm("Replace this parent-approval requirement with reading dialogue? Other requirements and assigned work will not change.")) return;
            const next = { ...DIALOGUE_CONFIG_SCHEMA["x-manifest"], id: requirement.id, config: { ...newDialogueConfig() } };
            onChange(replaceRequirement(requirements, index, next));
          }}>Use reading dialogue instead</button>
        </>}
        {dialogue && config && <DialogueFields config={config} index={index} onChange={next => update(index, next)} />}
        {dialogue && !config && <p className="notice error" role="alert">This configuration cannot be edited safely here. Its original payload has been retained; do not replace it with a default.</p>}
        {!dialogue && requirement.kind !== "parent_approval" && <p>This verification method is not editable here. Its settings are kept unchanged.</p>}
        <details><summary>Requirement identity</summary><p className="secondary-cell">{requirement.id || "Assigned when saved"} · configuration version {requirement.configVersion}</p></details>
      </fieldset>;
    })}
    <button className="button secondary" type="button" disabled={disabled} onClick={() => onChange([...requirements, {
      ...DIALOGUE_CONFIG_SCHEMA["x-manifest"], id: crypto.randomUUID(), config: { ...newDialogueConfig() },
    }])}>Add reading dialogue requirement</button>
  </section>;
}

// Editable drafts may contain empty fields, but never unknown fields or a
// published plan. Validate a filled probe through the actual generated guard;
// then retain the draft values with the same field types, without a DTO cast.
export function readEditableConfig(value: unknown): DialogueConfig | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const probe = { ...value };
  if (!("learningFocus" in value) || typeof value.learningFocus !== "string" || !("rubric" in value) || !Array.isArray(value.rubric) || !value.rubric.every(item => typeof item === "string")) return null;
  const sourceText = "sourceText" in value && typeof value.sourceText === "string" ? value.sourceText : undefined;
  if (!("allowedFollowUps" in value) || typeof value.allowedFollowUps !== "number" || !("maxAttempts" in value) || typeof value.maxAttempts !== "number" || !("maxTurns" in value) || typeof value.maxTurns !== "number") return null;
  const parsed = parseDialogueConfig({ ...probe, ...(sourceText !== undefined ? { sourceText: "Draft" } : {}), learningFocus: "Draft", rubric: ["Draft"], allowedFollowUps: fields.allowedFollowUps.minimum, maxAttempts: fields.maxAttempts.minimum, maxTurns: fields.maxTurns.minimum });
  if (!parsed) return null;
  return { ...parsed, learningFocus: value.learningFocus, rubric: value.rubric, allowedFollowUps: value.allowedFollowUps, maxAttempts: value.maxAttempts, maxTurns: value.maxTurns, ...(sourceText !== undefined ? { sourceText } : {}) };
}

function DialogueFields({ config, index, onChange }: { config: DialogueConfig; index: number; onChange: (config: DialogueConfig) => void }) {
  const prefix = `dialogue-${index}`;
  const valid = parseDialogueConfig(config) !== null;
  return <>
    <label className="field">Source type<select className="input" value={config.sourceRef ? "curated" : "inline"} onChange={event => {
      const { sourceText: _text, sourceRef: _ref, ...rest } = config;
      onChange(event.target.value === "curated" ? { ...rest, sourceRef: fields.sourceRef.const } : { ...rest, sourceText: "" });
    }}><option value="inline">Parent-authored passage</option><option value="curated">Curated chapter · {fields.sourceRef.const}</option></select></label>
    {config.sourceRef ? <p>The published task binds the supported curated chapter’s exact version and hash. Arbitrary remote references are not supported.</p> :
      <label className="field">Parent-authored source<textarea className="input" rows={6} required value={config.sourceText ?? ""} onChange={event => onChange({ ...config, sourceText: event.target.value })} aria-describedby={`${prefix}-source-help`} /></label>}
    <p id={`${prefix}-source-help`} className="meta">Inline source limit: {fields.sourceText["x-maxBytes"]} UTF-8 bytes. Source and rubric are parent-only; they are not student-supplied policy.</p>
    <label className="field">Learning focus<input className="input" required maxLength={fields.learningFocus.maxLength} value={config.learningFocus} onChange={event => onChange({ ...config, learningFocus: event.target.value })} /></label>
    <label className="field">Acceptance criteria · one per line<textarea className="input" required rows={4} value={config.rubric.join("\n")} onChange={event => onChange({ ...config, rubric: event.target.value.split("\n") })} /></label>
    <p className="meta">{fields.rubric.minItems}–{fields.rubric.maxItems} distinct criteria, up to {fields.rubric.items.maxLength} characters each.</p>
    <details><summary>Finite follow-up and retry policy</summary><div className="dialogue-policy">
      <label className="field">Follow-ups per question<input className="input" type="number" required min={fields.allowedFollowUps.minimum} max={fields.allowedFollowUps.maximum} value={config.allowedFollowUps} onChange={event => onChange({ ...config, allowedFollowUps: Number(event.target.value) })} /></label>
      <label className="field">Maximum attempts<input className="input" type="number" required min={fields.maxAttempts.minimum} max={fields.maxAttempts.maximum} value={config.maxAttempts} onChange={event => onChange({ ...config, maxAttempts: Number(event.target.value) })} /></label>
      <label className="field">Maximum turns<input className="input" type="number" required min={fields.maxTurns.minimum} max={fields.maxTurns.maximum} value={config.maxTurns} onChange={event => onChange({ ...config, maxTurns: Number(event.target.value) })} /></label>
    </div></details>
    <aside className="dialogue-preview" aria-label={`Requirement ${index + 1} preview`}>
      <p className="system-label">Published behavior preview</p>
      <p>Exactly {config.requiredQuestions} distinct, bound questions must receive accepted evaluations. Other required work must also be satisfied before this task completes.</p>
      <p>Up to {config.allowedFollowUps} follow-ups per question, {config.maxTurns} turns per attempt, and {config.maxAttempts} attempts. A parent reviews exhausted attempts; no failure invents success.</p>
      <p>Evidence is retained. Answers, evaluations and overrides are append-only. There is no automatic deletion or retention-days setting.</p>
    </aside>
    {!valid && <p className="notice error" role="alert">Complete the source, focus and distinct criteria within the published limits before saving.</p>}
  </>;
}
