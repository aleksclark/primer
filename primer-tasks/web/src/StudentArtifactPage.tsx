import { useEffect, useMemo, useState, type ChangeEvent } from "react";
import { useNavigate } from "react-router-dom";
import {
  createArtifactClient,
  newArtifactIdempotencyKey,
  sha256Hex,
  tasksClient,
  validateArtifactFile,
  type ArtifactClientSnapshot,
  type ArtifactStudentState,
  type Occurrence,
} from "@primer-tasks/client";

type UploadState = "ready" | "selected" | "uploading" | "queued" | "loading" | "evaluating" | "review" | "rejected" | "complete" | "error";

function statusCopy(status: UploadState) {
  return {
    ready: ["Ready to submit", "Choose one allowed file, review it, then finalize the upload."],
    selected: ["Ready to upload", "Review the preview and filename before sending it."],
    uploading: ["Uploading", "Your file is moving to the server. Keep this page open."],
    queued: ["Queued", "The server saved your submission and queued a safe review."],
    loading: ["Loading", "The reviewer is preparing the authorized artifact derivative."],
    evaluating: ["Evaluating", "The reviewer is checking the rubric criteria. No chat is opened."],
    review: ["Parent review", "Automatic review cannot decide this media safely. Your parent can inspect it."],
    rejected: ["Try again", "This submission did not meet every required criterion. Your work is still saved."],
    complete: ["Complete", "Every required criterion was accepted. This task is complete."],
    error: ["Could not finish", "The upload or review did not finish. You can retry without changing the prior submission."],
  }[status];
}

function effectiveStatus(state: ArtifactStudentState, socket: ArtifactClientSnapshot, uploadState: UploadState): UploadState {
  if (["uploading", "selected"].includes(uploadState)) return uploadState;
  if (state.evaluation?.status === "complete" || state.status === "complete") return "complete";
  if (state.evaluation?.status === "review" || state.status === "review") return "review";
  if (state.evaluation?.status === "rejected" || state.status === "rejected") return "rejected";
  const progress = [...socket.events].reverse().find((event) => event.kind === "progress");
  if (progress?.phase === "evaluating") return "evaluating";
  if (progress?.phase === "loading") return "loading";
  if (progress?.phase === "queued") return "queued";
  if (socket.connectionState === "offline" && state.submissions.length) return "queued";
  if (state.status === "evaluating") return "evaluating";
  return uploadState;
}

function progressPercent(uploadState: UploadState, loaded: number, total: number) {
  if (uploadState !== "uploading") return undefined;
  return total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : 0;
}

function accepts(config: ArtifactStudentState["config"]) {
  const values: string[] = [];
  if (config.acceptedKinds.includes("image")) values.push("image/*");
  if (config.acceptedKinds.includes("video")) values.push("video/*");
  if (config.acceptedKinds.includes("audio")) values.push("audio/*");
  return values.join(",");
}

async function mediaDurationMs(file: File): Promise<number> {
  if (!file.type.startsWith("audio/") && !file.type.startsWith("video/")) return 0;
  const source = URL.createObjectURL(file);
  try {
    const media = document.createElement(file.type.startsWith("audio/") ? "audio" : "video");
    media.preload = "metadata";
    media.src = source;
    await new Promise<void>((resolve, reject) => { media.onloadedmetadata = () => resolve(); media.onerror = () => reject(new Error("The media duration could not be read.")); });
    if (!Number.isFinite(media.duration) || media.duration <= 0) throw new Error("The media duration could not be read.");
    return Math.ceil(media.duration * 1000);
  } finally { URL.revokeObjectURL(source); }
}

function FilePreview({ file }: { file: File }) {
  const [source, setSource] = useState<string | null>(null);
  useEffect(() => {
    const next = URL.createObjectURL(file);
    setSource(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  if (!source) return null;
  if (file.type.startsWith("image/")) return <img className="artifact-file-preview" src={source} alt="Selected work preview" />;
  if (file.type.startsWith("video/")) return <video className="artifact-file-preview" src={source} controls preload="metadata" aria-label="Selected video preview" />;
  if (file.type.startsWith("audio/")) return <audio src={source} controls aria-label="Selected audio preview" />;
  return null;
}

function ProgressIndicator({ state, loaded, total }: { state: UploadState; loaded: number; total: number }) {
  const [label, detail] = statusCopy(state);
  const percent = progressPercent(state, loaded, total);
  return <div className={`artifact-state artifact-state-${state}`} role="status" aria-live="polite"><div><strong>{label}</strong><p>{detail}</p></div>{percent !== undefined && <div className="artifact-upload-progress"><progress max={100} value={percent}>{percent}%</progress><span>{percent}%</span></div>}</div>;
}

export default function StudentArtifactPage({ occurrence, initialState }: { occurrence: Occurrence; initialState: ArtifactStudentState }) {
  const navigate = useNavigate();
  const [state, setState] = useState(initialState);
  const [file, setFile] = useState<File | null>(null);
  const [uploadState, setUploadState] = useState<UploadState>(initialState.status === "ready" ? "ready" : initialState.status as UploadState);
  const [loaded, setLoaded] = useState(0);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [socket, setSocket] = useState<ArtifactClientSnapshot>({ connectionState: "idle", cursor: 0, events: [] });
  const activeSubmission = state.submissions.find((item) => item.id === state.activeSubmissionId) ?? state.submissions.at(-1);
  const artifactClient = useMemo(() => activeSubmission ? createArtifactClient({ occurrenceId: occurrence.id, submissionId: activeSubmission.id }) : null, [activeSubmission?.id, occurrence.id]);
  useEffect(() => {
    if (!artifactClient) return;
    const unsubscribe = artifactClient.subscribe(setSocket);
    artifactClient.connect();
    return () => { unsubscribe(); artifactClient.disconnect(); };
  }, [artifactClient]);
  const status = effectiveStatus(state, socket, uploadState);
  const [label] = statusCopy(status);
  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const next = event.target.files?.[0] ?? null;
    setError(null);
    if (!next) return;
    const invalid = validateArtifactFile(next, state.config);
    if (invalid) { setFile(null); setError(invalid); setUploadState("error"); return; }
    setFile(next); setLoaded(0); setTotal(next.size); setUploadState("selected");
  };
  const upload = async () => {
    if (!file || status === "uploading") return;
    setError(null); setLoaded(0); setTotal(file.size); setUploadState("uploading");
    try {
      const digest = await sha256Hex(file);
      const kind = file.type.startsWith("image/") ? "image" : file.type.startsWith("video/") ? "video" : "audio";
      const reservation = await tasksClient.reserveArtifact(occurrence.id, { kind, mediaType: file.type, sizeBytes: file.size, digest, idempotencyKey: newArtifactIdempotencyKey() });
      await tasksClient.uploadArtifact(reservation, file, { onProgress: (current, maximum) => { setLoaded(current); setTotal(maximum); } });
      const durationMs = await mediaDurationMs(file);
      const next = await tasksClient.finalizeArtifact(occurrence.id, { artifactId: reservation.artifactId, idempotencyKey: reservation.idempotencyKey, digest, sizeBytes: file.size, mediaType: file.type, durationMs, filename: file.name });
      setState(next); setUploadState(next.status === "complete" ? "complete" : "queued");
    } catch (next) {
      setUploadState("error");
      setError(next instanceof Error ? next.message : "The upload could not be completed.");
    }
  };
  const retryReview = async () => {
    setError(null);
    try { setState(await tasksClient.retryArtifactEvaluation(occurrence.id)); setUploadState("queued"); } catch (next) { setError(next instanceof Error ? next.message : "The review could not be retried."); }
  };
  const startOver = () => { setFile(null); setError(null); setLoaded(0); setTotal(0); setUploadState("ready"); };
  return <>
    <header className="page-header"><div><p className="eyebrow">Student workspace / Submit + Learn</p><h1>{occurrence.title}</h1><p>{occurrence.instructions}</p></div><div className="page-actions"><span className={`status ${socket.connectionState === "connected" ? "active" : socket.connectionState === "offline" ? "attention" : ""}`} role="status">{socket.connectionState === "connected" ? "Connected" : socket.connectionState === "reconnecting" ? "Reconnecting" : socket.connectionState === "offline" ? "Offline" : "No live review"}</span><button className="button secondary" type="button" onClick={() => navigate("/student")}>Back to today</button></div></header>
    <ProgressIndicator state={status} loaded={loaded} total={total} />
    {error && <div className="notice error" role="alert"><div><strong>Submission problem</strong><p>{error}</p><button className="button quiet" type="button" onClick={startOver}>Choose another file</button></div></div>}
    <div className="artifact-layout">
      <section className="artifact-submit-card" aria-label="Submit media evidence"><p className="eyebrow">Evidence upload</p><h2>Choose your work</h2><p className="meta">Allowed: {state.config.acceptedKinds.join(", ")} · up to {Math.round(state.config.maxBytes / (1024 * 1024))} MB. The server checks the file again before review.</p><label className="artifact-file-input"><span>{file ? "Replace selected file" : "Choose a file"}</span><input type="file" accept={accepts(state.config)} onChange={choose} disabled={status === "uploading" || status === "complete"} /></label>{file && <div className="artifact-selected-file"><FilePreview file={file} /><dl><div><dt>Filename</dt><dd>{file.name}</dd></div><div><dt>Type</dt><dd>{file.type || "Unknown"}</dd></div><div><dt>Size</dt><dd>{Math.ceil(file.size / 1024)} KB</dd></div></dl></div>}{file && status === "selected" && <button className="button" type="button" onClick={() => void upload()}>Finalize upload</button>}{(status === "rejected" || status === "review" || status === "error") && <div className="page-actions"><button className="button secondary" type="button" onClick={startOver}>Submit a new file</button>{status === "error" && activeSubmission && <button className="button quiet" type="button" onClick={() => void retryReview()}>Retry review</button>}</div>}</section>
      <aside className="artifact-rubric-card" aria-label="Task rubric"><p className="eyebrow">What will be checked</p><ul>{state.config.criteria.map((criterion) => <li key={criterion.id}><strong>{criterion.label}</strong><span>{criterion.description}</span>{criterion.required && <small>Required</small>}</li>)}</ul><p className="meta">A parent may review audio/video when automatic evaluation is not supported. Nothing passes silently.</p></aside>
    </div>
    {state.evaluation && <section className="artifact-feedback" aria-label="Rubric feedback"><div className="record-toolbar"><div><p className="system-label">Review result</p><p>{label}</p></div>{state.evaluation.rubricRevision && <span className="meta">Rubric revision {state.evaluation.rubricRevision}</span>}</div><div className="artifact-criteria-results">{state.evaluation.criteria.map((criterion) => <article className={`artifact-criterion-result artifact-result-${criterion.status}`} key={criterion.id}><span className="status">{criterion.status}</span><strong>{criterion.criterionId}</strong>{criterion.feedback && <p>{criterion.feedback}</p>}{criterion.evidence && <p className="meta">Evidence: {criterion.evidence}</p>}</article>)}</div>{state.evaluation.nextStep && <p className="artifact-next-step">{state.evaluation.nextStep}</p>}</section>}
    {status === "complete" && <div className="notice complete" role="status"><div><strong>✓ Task complete</strong><p>All required rubric criteria were accepted by the server.</p></div></div>}
  </>;
}

