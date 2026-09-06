import { FormEvent, useEffect, useState } from "react";
import { BookOpen, Compass, FolderKanban, LogIn, Moon, Settings2, Sun } from "lucide-react";
import { createCurriculum, createPlanNode, currentSession, createRevision, downloadExport, exportRevision, getRevisionGraph, listCurricula, listRevisions, publishRevision, validateRevision } from "./api/client";
import type { ExportFormat } from "./api/client";

type Theme = "dark" | "light";
type Workspace = { workspaceId: string; workspaceName: string; role: string };
type Curriculum = { id: string; name: string; description?: string; status: string; updatedAt?: string };
type Revision = { id: string; state: string; revisionNumber?: number };

const nav = [
  { label: "Explore", icon: Compass, text: "Curricula" },
  { label: "Operate", icon: FolderKanban, text: "Plans" },
  { label: "Configure", icon: Settings2, text: "Workspace" },
];

export default function App() {
  const [theme, setTheme] = useState<Theme>("dark");
  const [subject, setSubject] = useState("Loading session…");
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [curricula, setCurricula] = useState<Curriculum[]>([]);
  const [query, setQuery] = useState("");
  const [name, setName] = useState("");
  const [message, setMessage] = useState("");
  const [selected, setSelected] = useState<Curriculum | null>(null);
  const [revisions, setRevisions] = useState<Revision[]>([]);
  const [revision, setRevision] = useState<Revision | null>(null);
  const [exportFormat, setExportFormat] = useState<ExportFormat>("markdown");
  const [exporting, setExporting] = useState(false);
  const [projectName, setProjectName] = useState("Chicken coop");
  const [projectPhases, setProjectPhases] = useState("Design, Build, Present");
  const [projectNodes, setProjectNodes] = useState<{ title: string; phases: string }[]>([]);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    currentSession().then(async ({ data }) => {
      if (!data || !("subjectRef" in data)) { setSubject("Sign in to continue"); return; }
      setSubject(String(data.subjectRef));
      const memberships = ("memberships" in data ? data.memberships : []) ?? [];
      const first = memberships[0];
      if (!first) { setMessage("Choose or create a workspace to begin."); return; }
      const selected = { workspaceId: String(first.workspaceId), workspaceName: String(first.workspaceName), role: String(first.role) };
      setWorkspace(selected);
      const result = await listCurricula(selected.workspaceId);
      if (result.data && "items" in result.data) setCurricula(result.data.items as Curriculum[]);
    }).catch(() => setSubject("Sign in to continue"));
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!workspace || !name.trim()) return;
    setMessage("Creating draft…");
    const result = await createCurriculum(workspace.workspaceId, name.trim());
    if (result.data && "id" in result.data) {
      setCurricula((items) => [result.data as Curriculum, ...items]);
      setName(""); setMessage("Draft curriculum created.");
    } else setMessage("The Studio API could not create that curriculum.");
  }

  async function openCurriculum(item: Curriculum) {
    setSelected(item); setMessage("Loading plan drafts…");
    const result = await listRevisions(item.id);
    if (result.data && "items" in result.data) { const items = result.data.items as Revision[]; setRevisions(items); const next = items[0] ?? null; setRevision(next); setMessage(items.length ? "Draft ready to configure." : "No draft revision yet."); if (next) await loadProjects(next.id); }
  }

  async function newDraft() {
    if (!selected) return; const result = await createRevision(selected.id); if (result.data && "id" in result.data) { const next = result.data as Revision; setRevisions((items) => [next, ...items]); setRevision(next); setMessage("Draft revision created."); await loadProjects(next.id); }
  }
  async function revisionAction(action: "validate" | "publish") {
    if (!revision) return;
    if (action === "validate") await validateRevision(revision.id);
    if (action === "publish") await publishRevision(revision.id);
    setMessage(action === "validate" ? "Validation report saved." : "Revision published.");
  }
  async function loadProjects(revisionId: string) {
    const result = await getRevisionGraph(revisionId);
    if (result.data && "nodes" in result.data) {
      const nodes = (result.data.nodes as { kind?: string; title?: string; attributes?: Record<string, string> }[])
        .filter((node) => node.kind === "project")
        .map((node) => ({ title: String(node.title ?? "Untitled project"), phases: String(node.attributes?.phaseNames ?? node.attributes?.phases ?? "") }));
      setProjectNodes(nodes);
    }
  }
  async function addProject(event: FormEvent) {
    event.preventDefault();
    if (!revision || !projectName.trim()) return;
    const names = projectPhases.split(",").map((part) => part.trim()).filter(Boolean);
    const ids = names.map((name) => name.toLowerCase().replace(/\s+/g, "-"));
    const result = await createPlanNode(revision.id, {
      kind: "project",
      title: projectName.trim(),
      body: "Multi-subject project blueprint",
      attributes: { phases: ids.join(","), phaseNames: names.join("|") },
    });
    if (result.data && "id" in result.data) {
      setMessage(`Saved project ${projectName.trim()} with ${names.length} ordered phases.`);
      await loadProjects(revision.id);
    } else setMessage("The Studio API could not save that project blueprint.");
  }
  async function exportPlan() {
    if (!revision || exporting) return;
    setExporting(true); setMessage("Rendering and storing export…");
    try {
      const result = await exportRevision(revision.id, exportFormat);
      if (!result.data || result.data.status !== "ready") {
        setMessage(result.data?.errorMessage || result.error?.detail || "Export failed. No download is ready.");
        return;
      }
      const download = await downloadExport(result.data.id);
      if (!download.data) { setMessage("Export stored, but the download failed. Please try again."); return; }
      const url = URL.createObjectURL(download.data);
      const link = document.createElement("a");
      const extensions: Record<ExportFormat, string> = { markdown: "md", pdf: "pdf", docx: "docx", csv_coverage: "csv", json_bundle: "json", ical: "ics" };
      link.href = url; link.download = `${result.data.id}.${extensions[exportFormat]}`;
      document.body.appendChild(link); link.click(); link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      setMessage("Export ready and downloaded.");
    } catch { setMessage("Export or download failed. Please try again."); }
    finally { setExporting(false); }
  }

  async function search(value: string) {
    setQuery(value);
    if (!workspace) return;
    const result = await listCurricula(workspace.workspaceId, value);
    if (result.data && "items" in result.data) setCurricula(result.data.items as Curriculum[]);
  }

  return <div className="studio-shell">
    <aside className="rail" aria-label="Primary navigation">
      <div className="brand"><BookOpen aria-hidden size={18} /><span>Curriculum<br /><strong>Studio</strong></span></div>
      <nav>{nav.map(({ label, icon: Icon, text }, index) => <a className={index === 0 ? "active" : ""} href={`#${text.toLowerCase()}`} key={label}><Icon size={16} aria-hidden /><span>{label}</span><small>{text}</small></a>)}</nav>
      <div className="rail-foot"><span className="eyebrow">Workspace</span><strong>{workspace?.workspaceName ?? "No workspace selected"}</strong><button type="button" className="plain-button"><LogIn size={14} aria-hidden /> Sign in / switch</button></div>
    </aside>
    <main className="main">
      <header className="topbar"><div><span className="eyebrow">Explore / Curriculum library</span><h1>Plan with purpose.</h1></div><button className="icon-button" type="button" aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`} onClick={() => setTheme(theme === "dark" ? "light" : "dark")}>{theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}</button></header>
      <section className="context-rule"><span>AUTHENTICATED SESSION</span><code>{subject}</code><span className="status"><i /> BFF session active</span></section>
      <section className="intro"><div><span className="eyebrow">{workspace?.workspaceName ?? "Curriculum Studio"}</span><h2>Make the next right plan.</h2><p>Explore standards, shape a draft, and keep the parent’s judgment at the center of every decision.</p></div></section>
      <section className="library" aria-labelledby="library-title"><div className="section-heading"><div><span className="eyebrow">Explore</span><h3 id="library-title">Curriculum library</h3></div><input aria-label="Search curricula" placeholder="Search by name" value={query} onChange={(event) => search(event.target.value)} /></div>
        <form className="create-form" onSubmit={submit}><label htmlFor="curriculum-name">New curriculum brief</label><input id="curriculum-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="e.g. Grade 6 mathematics" /><button className="primary" type="submit">Create draft <span aria-hidden>↗</span></button></form>
        {message && <p className="feedback" role="status">{message}</p>}
        {curricula.length === 0 ? <div className="empty-state"><div className="empty-mark">01</div><div><span className="eyebrow">No curricula found</span><h3>Start with a brief.</h3><p>Create a draft above. The server owns search, pagination, and durable identity; this view never filters a bulk client-side collection.</p></div></div> : <div className="table-wrap"><table><thead><tr><th>Name</th><th>Status</th><th>Updated</th><th>ID</th></tr></thead><tbody>{curricula.map((item) => <tr key={item.id} onClick={() => openCurriculum(item)}><th scope="row">{item.name}</th><td><span className="status-text">● {item.status}</span></td><td>{item.updatedAt ? new Date(item.updatedAt).toLocaleDateString() : "—"}</td><td><code>{item.id}</code></td></tr>)}</tbody></table></div>}
        {selected && <div className="plan-panel"><div><span className="eyebrow">Configure / {selected.name}</span><h3>{revision ? `Revision ${revision.revisionNumber ?? "draft"}` : "No revision"}</h3><p>{message}</p></div><div className="plan-actions"><button className="secondary" type="button" onClick={newDraft}>New draft</button>{revision && <><button className="secondary" type="button" onClick={() => revisionAction("validate")}>Validate</button><button className="primary" type="button" onClick={() => revisionAction("publish")}>Publish</button><select className="secondary" aria-label="Export format" value={exportFormat} disabled={exporting} onChange={(event) => setExportFormat(event.target.value as ExportFormat)}><option value="markdown">Markdown</option><option value="pdf">PDF</option><option value="docx">DOCX</option><option value="csv_coverage">CSV coverage</option><option value="json_bundle">JSON download subset</option><option value="ical">iCal schedule</option></select><button className="plain-button" type="button" disabled={exporting} onClick={exportPlan}>{exporting ? "Exporting…" : "Export and download"}</button></>}</div></div>}
        {selected && revision && <section className="project-designer" aria-labelledby="project-designer-title"><div className="section-heading"><div><span className="eyebrow">Configure / Projects</span><h3 id="project-designer-title">Project designer</h3></div></div><form className="create-form" onSubmit={addProject}><label htmlFor="project-name">Project name</label><input id="project-name" value={projectName} onChange={(event) => setProjectName(event.target.value)} placeholder="e.g. Chicken coop" /><label htmlFor="project-phases">Ordered phases</label><input id="project-phases" value={projectPhases} onChange={(event) => setProjectPhases(event.target.value)} placeholder="Design, Build, Present" /><button className="secondary" type="submit">Save blueprint</button></form>{projectNodes.length > 0 && <ul className="project-list">{projectNodes.map((node) => <li key={node.title}><strong>{node.title}</strong><span>{node.phases || "No phases yet"}</span></li>)}</ul>}</section>}
      </section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Bearer tokens stay server-side · host-only session cookie</span></footer>
    </main>
  </div>;
}
