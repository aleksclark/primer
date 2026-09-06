import { FormEvent, useEffect, useState } from "react";
import { BookOpen, Compass, FolderKanban, LogIn, Moon, Settings2, Sun } from "lucide-react";
import { createCurriculum, createPlanEdge, createPlanNode, createResource, currentSession, createRevision, downloadExport, exportRevision, getRevisionGraph, importStandardsCatalog, listCatalogStandards, listCurricula, listMaterializedItems, listRevisions, listStandardsCatalogs, materializeRevision, publishRevision, validateRevision } from "./api/client";
import type { ExportFormat } from "./api/client";

type Theme = "dark" | "light";
type Workspace = { workspaceId: string; workspaceName: string; role: string };
type Curriculum = { id: string; name: string; description?: string; status: string; updatedAt?: string };
type Revision = { id: string; state: string; revisionNumber?: number };
type GraphNode = { id: string; kind: string; title: string; attributes?: Record<string, string> };
type GraphEdge = { kind: string; fromNodeId: string; toNodeId: string; note?: string };
type ProjectPhase = { id: string; name: string; position?: number; offScreen?: boolean; activities?: { kind: string; title: string }[] };

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
  const [phaseDesign, setPhaseDesign] = useState("Design");
  const [phaseBuild, setPhaseBuild] = useState("Build");
  const [offScreen, setOffScreen] = useState(true);
  const [targetOutcome, setTargetOutcome] = useState("Scale drawings");
  const [priorOutcome, setPriorOutcome] = useState("Load paths");
  const [stretchOutcome, setStretchOutcome] = useState("Cost estimate");
  const [toolName, setToolName] = useState("Circular saw");
  const [targetStandard, setTargetStandard] = useState("MATH.6.G");
  const [priorStandard, setPriorStandard] = useState("SCI.6.PS");
  const [stretchStandard, setStretchStandard] = useState("MATH.6.RP");
  const [projectNodes, setProjectNodes] = useState<{ id: string; title: string; phases: ProjectPhase[] }[]>([]);
  const [selectedProject, setSelectedProject] = useState("");
  const [selectedPhase, setSelectedPhase] = useState("");
  const [phaseItems, setPhaseItems] = useState<{ kind: string; title: string }[]>([]);


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
      const nodes = (result.data.nodes as GraphNode[]).filter((node) => node.kind === "project").map((node) => {
        let phases: ProjectPhase[] = [];
        try { phases = JSON.parse(String(node.attributes?.phasesJSON ?? "[]")); } catch { phases = []; }
        return { id: node.id, title: node.title, phases };
      });
      setProjectNodes(nodes);
      if (!selectedProject && nodes[0]) {
        setSelectedProject(nodes[0].id);
        setSelectedPhase(nodes[0].phases[0]?.id ?? "");
      }
    }
  }
  function slug(value: string) { return value.trim().toLowerCase().replace(/\s+/g, "-"); }
  async function ensureProjectStandards(workspaceId: string, codes: string[]) {
    const unique = [...new Set(codes.map((code) => code.trim()).filter(Boolean))];
    if (unique.length === 0) return [] as string[];
    const listed = await listStandardsCatalogs(workspaceId);
    const catalogs = listed.data && "items" in listed.data ? listed.data.items as { id: string }[] : [];
    let catalogId = catalogs[0]?.id ?? "";
    if (!catalogId) {
      const created = await importStandardsCatalog(workspaceId, {
        source: "custom",
        title: "Project standards",
        standards: unique.map((code) => ({ code, source: "custom", description: code })),
      });
      if (!created.data || !("id" in created.data)) return [];
      catalogId = String(created.data.id);
    }
    const page = await listCatalogStandards(catalogId);
    const existing = new Set((page.data && "items" in page.data ? page.data.items as { code: string }[] : []).map((item) => item.code));
    const missing = unique.filter((code) => !existing.has(code));
    if (missing.length) {
      const created = await importStandardsCatalog(workspaceId, {
        source: "custom",
        title: "Project standards",
        standards: missing.map((code) => ({ code, source: "custom", description: code })),
      });
      if (created.data && "id" in created.data) catalogId = String(created.data.id);
    }
    const refreshed = await listCatalogStandards(catalogId);
    const available = new Set((refreshed.data && "items" in refreshed.data ? refreshed.data.items as { code: string }[] : []).map((item) => item.code));
    return unique.filter((code) => available.has(code));
  }
  async function addProject(event: FormEvent) {
    event.preventDefault();
    if (!revision || !workspace || !projectName.trim()) return;
    const phases: ProjectPhase[] = [
      { id: slug(phaseDesign) || "design", name: phaseDesign.trim() || "Design", position: 1 },
      { id: slug(phaseBuild) || "build", name: phaseBuild.trim() || "Build", position: 2, offScreen, activities: offScreen ? [{ kind: "off_screen", title: `${phaseBuild.trim() || "Build"} off-screen task` }] : [] },
    ];
    const project = await createPlanNode(revision.id, { kind: "project", title: projectName.trim(), body: "Multi-subject project blueprint", attributes: { phasesJSON: JSON.stringify(phases) } });
    if (!project.data || !("id" in project.data)) { setMessage("The Studio API could not save that project blueprint."); return; }
    const roles = [
      { title: targetOutcome, role: "target", standard: targetStandard, evidenceKind: "portfolio", evidenceDescription: "photo essay" },
      { title: priorOutcome, role: "prior", standard: priorStandard },
      { title: stretchOutcome, role: "stretch", standard: stretchStandard },
    ];
    const mapped = await ensureProjectStandards(workspace.workspaceId, roles.map((entry) => entry.standard));
    if (mapped.length === 0) { setMessage("Create or import a standards catalog before saving a project blueprint."); return; }
    for (const entry of roles) {
      if (!entry.title.trim()) continue;
      const code = entry.standard.trim();
      const outcome = await createPlanNode(revision.id, {
        kind: "outcome",
        title: entry.title.trim(),
        standardCodes: code ? [code] : [],
        attributes: entry.evidenceKind ? { evidenceKind: entry.evidenceKind, evidenceDescription: entry.evidenceDescription ?? "" } : {},
      });
      if (outcome.data && "id" in outcome.data) await createPlanEdge(revision.id, { kind: "parent_child", fromNodeId: String(project.data.id), toNodeId: String(outcome.data.id), note: entry.role });
    }
    if (toolName.trim()) {
      const tool = await createResource(workspace.workspaceId, { kind: "tool", title: toolName.trim() });
      if (tool.data && "id" in tool.data) await createPlanEdge(revision.id, { kind: "uses_resource", fromNodeId: String(project.data.id), toNodeId: String(tool.data.id), note: "required" });
    }
    setSelectedProject(String(project.data.id));
    setSelectedPhase(phases[1]?.id ?? phases[0]?.id ?? "");
    setMessage(`Saved ${projectName.trim()} with target/prior/stretch outcomes.`);
    await loadProjects(revision.id);
  }
  async function materializePhase() {
    if (!revision || !selectedProject || !selectedPhase) return;
    const result = await materializeRevision(revision.id, { window: { availableMinutes: 45 }, attributes: { projectId: selectedProject, projectPhaseId: selectedPhase } });
    if (!result.data || !("id" in result.data)) { setMessage("Phase materialization failed. Publish the revision first."); return; }
    const items = await listMaterializedItems(String(result.data.id));
    const next = items.data && "items" in items.data ? (items.data.items as { kind: string; title: string }[]) : [];
    setPhaseItems(next);
    setMessage(`Materialized phase ${selectedPhase} into ${next.length} items.`);
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
        {selected && revision && <section className="project-designer" aria-labelledby="project-designer-title"><div className="section-heading"><div><span className="eyebrow">Configure / Projects</span><h3 id="project-designer-title">Project designer</h3></div></div><form className="create-form project-form" onSubmit={addProject}><label htmlFor="project-name">Project name</label><input id="project-name" value={projectName} onChange={(event) => setProjectName(event.target.value)} /><label htmlFor="phase-design">Design phase</label><input id="phase-design" value={phaseDesign} onChange={(event) => setPhaseDesign(event.target.value)} /><label htmlFor="phase-build">Build phase</label><input id="phase-build" value={phaseBuild} onChange={(event) => setPhaseBuild(event.target.value)} /><label htmlFor="target-outcome">Target outcome</label><input id="target-outcome" value={targetOutcome} onChange={(event) => setTargetOutcome(event.target.value)} /><label htmlFor="target-standard">Target standard</label><input id="target-standard" value={targetStandard} onChange={(event) => setTargetStandard(event.target.value)} /><label htmlFor="prior-outcome">Prior outcome</label><input id="prior-outcome" value={priorOutcome} onChange={(event) => setPriorOutcome(event.target.value)} /><label htmlFor="prior-standard">Prior standard</label><input id="prior-standard" value={priorStandard} onChange={(event) => setPriorStandard(event.target.value)} /><label htmlFor="stretch-outcome">Stretch outcome</label><input id="stretch-outcome" value={stretchOutcome} onChange={(event) => setStretchOutcome(event.target.value)} /><label htmlFor="stretch-standard">Stretch standard</label><input id="stretch-standard" value={stretchStandard} onChange={(event) => setStretchStandard(event.target.value)} /><label htmlFor="tool-name">Required tool</label><input id="tool-name" value={toolName} onChange={(event) => setToolName(event.target.value)} /><label className="plain-button" htmlFor="off-screen"><input id="off-screen" type="checkbox" checked={offScreen} onChange={(event) => setOffScreen(event.target.checked)} /> Off-screen build task</label><button className="secondary" type="submit">Save blueprint</button></form>{projectNodes.length > 0 && <div className="project-operate"><label htmlFor="operate-project">Operate phase</label><select id="operate-project" className="secondary" value={selectedProject} onChange={(event) => { setSelectedProject(event.target.value); const next = projectNodes.find((node) => node.id === event.target.value); setSelectedPhase(next?.phases[0]?.id ?? ""); }}>{projectNodes.map((node) => <option key={node.id} value={node.id}>{node.title}</option>)}</select><label htmlFor="operate-phase">Phase</label><select id="operate-phase" className="secondary" value={selectedPhase} onChange={(event) => setSelectedPhase(event.target.value)}>{(projectNodes.find((node) => node.id === selectedProject)?.phases ?? []).map((phase) => <option key={phase.id} value={phase.id}>{phase.name}{phase.offScreen ? " (off-screen)" : ""}</option>)}</select><button className="primary" type="button" onClick={materializePhase}>Materialize phase</button></div>}{phaseItems.length > 0 && <ul className="project-list">{phaseItems.map((item) => <li key={item.title}><strong>{item.title}</strong><span>{item.kind}</span></li>)}</ul>}</section>}
      </section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Bearer tokens stay server-side · host-only session cookie</span></footer>
    </main>
  </div>;
}
