import { FormEvent, useEffect, useRef, useState } from "react";
import { BookOpen, Compass, FolderKanban, Moon, Settings2, Sun } from "lucide-react";
import { createCatalogStandard, createCurriculum, createPlanEdge, createPlanNode, createResource, currentSession, createRevision, deletePlanNode, downloadExport, exportRevision, getRevisionGraph, importStandardsCatalog, listCatalogStandards, listCurricula, listMaterializedItems, listRevisions, listStandardsCatalogs, materializeRevision, publishRevision, validateRevision } from "./api/client";
import type { ExportFormat } from "./api/client";
import type { Model } from './api/collaboration';
import { problem } from './api/collaboration';
import CollaborationPane from './collaboration/CollaborationPane';
import { TemplateCreate, WorkspacePolicy } from './collaboration/WorkspaceControls';
import { canAuthor, sameWorkspace } from './collaboration/common';
import './styles/collaboration.css';

type Theme = "dark" | "light";
type Workspace = Model<'MembershipView'>;
type Curriculum = Model<'Curriculum'>;
type Revision = Model<'PlanRevision'>;
type GraphNode = Model<'PlanNode'>;
type ProjectPhase = { id: string; name: string; position?: number; offScreen?: boolean; activities?: { kind: string; title: string }[] };

const nav = [
  { label: "Explore", icon: Compass, text: "Curricula" },
  { label: "Operate", icon: FolderKanban, text: "Plans" },
  { label: "Configure", icon: Settings2, text: "Workspace" },
];

export default function App() {
  const selectionRequest=useRef(0);const projectRequest=useRef(0);const searchRequest=useRef(0);
  const [theme, setTheme] = useState<Theme>("dark");
  const [subject, setSubject] = useState("Loading session…");
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [memberships,setMemberships]=useState<Workspace[]>([]);
  const [curricula, setCurricula] = useState<Curriculum[]>([]);
  const [query, setQuery] = useState("");
  const [name, setName] = useState("");
  const [message, setMessage] = useState("");
  const [selected, setSelected] = useState<Curriculum | null>(null);
  const [, setRevisions] = useState<Revision[]>([]);
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
      const request=selectionRequest.current;
      if (!data || !("subjectRef" in data)) { setSubject("Sign in to continue"); return; }
      setSubject(String(data.subjectRef));
      const memberships = ("memberships" in data ? data.memberships : []) ?? [];
      const first = memberships[0];
      if (!first) { setMessage("Choose or create a workspace to begin."); return; }
      const selected = first;
      setMemberships(memberships);
      setWorkspace(selected);
      const result = await listCurricula(selected.workspaceId);
      if(request!==selectionRequest.current)return;
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
    const request=++selectionRequest.current;projectRequest.current++;
    setSelected(item); setRevision(null); setRevisions([]); setProjectNodes([]); setSelectedProject(''); setMessage("Loading plan drafts…");
    try {const result = await listRevisions(item.id);
    if(request!==selectionRequest.current)return;
    if(result.error)throw new Error(problem(result.error));
    if (result.data) { const items = result.data.items; setRevisions(items); const next = items[0] ?? null; setRevision(next); setMessage(items.length ? "Draft ready to configure." : "No draft revision yet."); if (next) await loadProjects(next.id); }
    }catch(e){if(request===selectionRequest.current)setMessage(problem(e))}
  }

  async function newDraft() {
    if (!selected) return;const request=selectionRequest.current;
    try { const result = await createRevision(selected.id); if(request!==selectionRequest.current)return; if(result.error)throw new Error(problem(result.error)); if (result.data) { const next = result.data; setRevisions((items) => [next, ...items]); setRevision(next); setMessage("Draft revision created."); await loadProjects(next.id); } } catch(e){setMessage(problem(e))}
  }
  async function revisionAction(action: "validate" | "publish") {
    if (!revision) return;const request=selectionRequest.current;
    if(action==='publish'&&!window.confirm(`Publish ${revision.title}? This freezes the revision.`))return;
    try {
      if(action==='validate'){const result=await validateRevision(revision.id);if(request!==selectionRequest.current)return;if(result.error)throw new Error(problem(result.error));setMessage(result.data?.passed?'Validation passed.':`Validation findings: ${result.data?.findings.map(f=>f.message).join('; ')||'report unavailable'}`)}
      else {const result=await publishRevision(revision.id);if(request!==selectionRequest.current)return;if(result.error)throw new Error(problem(result.error));if(result.data){setRevision(result.data);setMessage('Revision published.')}}
    }catch(e){setMessage(problem(e))}
  }
  async function loadProjects(revisionId: string) {
    const request=++projectRequest.current;
    const result = await getRevisionGraph(revisionId);
    if(request!==projectRequest.current)return;
    if (result.data && "nodes" in result.data) {
      const nodes = (result.data.nodes as GraphNode[]).filter((node) => node.kind === "project").map((node) => {
        let phases: ProjectPhase[] = [];
        try { phases = JSON.parse(String(node.attributes?.phasesJSON ?? "[]")); } catch { phases = []; }
        return { id: node.id, title: node.title, phases };
      });
      setProjectNodes(nodes);
      if (nodes[0]) {
        setSelectedProject(nodes[0].id);
        setSelectedPhase(nodes[0].phases[0]?.id ?? "");
      }
    }
  }
  function slug(value: string) { return value.trim().toLowerCase().replace(/\s+/g, "-"); }
  async function findProjectStandardsCatalog(workspaceId: string) {
    for (let offset = 0; ;) {
      const listed = await listStandardsCatalogs(workspaceId, offset, 100);
      const page = listed.data && "items" in listed.data ? listed.data.items as { id: string; title?: string }[] : [];
      const match = page.find((item) => item.title === "Project standards");
      if (match) return match.id;
      const total = listed.data && "totalCount" in listed.data ? Number(listed.data.totalCount) : page.length;
      offset += page.length;
      if (page.length === 0 || offset >= total) return page[0]?.id ?? "";
    }
  }
  async function catalogHasCode(catalogId: string, code: string) {
    const exact = await listCatalogStandards(catalogId, 0, 100, code);
    const hits = exact.data && "items" in exact.data ? exact.data.items as { code: string }[] : [];
    if (hits.some((item) => item.code === code)) return true;
    for (let offset = 0; ;) {
      const listed = await listCatalogStandards(catalogId, offset, 100);
      const page = listed.data && "items" in listed.data ? listed.data.items as { code: string }[] : [];
      if (page.some((item) => item.code === code)) return true;
      const total = listed.data && "totalCount" in listed.data ? Number(listed.data.totalCount) : page.length;
      offset += page.length;
      if (page.length === 0 || offset >= total) return false;
    }
  }
  async function ensureProjectStandards(workspaceId: string, codes: string[]) {
    const needed = codes.map((code) => code.trim());
    if (needed.some((code) => !code)) return { codes: [] as string[], error: "Each outcome needs a standard code." };
    const unique = [...new Set(needed)];
    let catalogId = await findProjectStandardsCatalog(workspaceId);
    if (!catalogId) {
      const created = await importStandardsCatalog(workspaceId, {
        source: "custom",
        title: "Project standards",
        standards: unique.map((code) => ({ code, source: "custom", description: code })),
      });
      if (!created.data || !("id" in created.data)) return { codes: [] as string[], error: "Could not create a Project standards catalog." };
      catalogId = String(created.data.id);
    }
    for (const code of unique) {
      if (await catalogHasCode(catalogId, code)) continue;
      const created = await createCatalogStandard(catalogId, { code, source: "custom", description: code });
      if (created.data && "id" in created.data) continue;
      if (created.error && /already exists/i.test(String(created.error.detail ?? created.error.title ?? "")) && await catalogHasCode(catalogId, code)) continue;
      return { codes: [] as string[], error: `Could not add standard ${code} to the existing catalog.` };
    }
    const missing: string[] = [];
    for (const code of unique) {
      if (!(await catalogHasCode(catalogId, code))) missing.push(code);
    }
    if (missing.length) return { codes: [] as string[], error: `Missing standards: ${missing.join(", ")}.` };
    return { codes: unique, error: "" };
  }
  async function rollbackCreatedNodes(revisionId: string, ids: string[]) {
    for (const id of [...ids].reverse()) {
      for (let attempt = 0; attempt < 3; attempt++) {
        const deleted = await deletePlanNode(revisionId, id);
        if (!deleted.error || deleted.response?.status === 404) break;
        if (attempt === 2) return false;
      }
    }
    return true;
  }
  async function addProject(event: FormEvent) {
    event.preventDefault();
    if (!revision || !workspace || !projectName.trim()) return;
    const roles = [
      { title: targetOutcome, role: "target", standard: targetStandard, evidenceKind: "portfolio", evidenceDescription: "photo essay" },
      { title: priorOutcome, role: "prior", standard: priorStandard },
      { title: stretchOutcome, role: "stretch", standard: stretchStandard },
    ].filter((entry) => entry.title.trim());
    if (roles.some((entry) => !entry.standard.trim())) {
      setMessage("Every outcome needs a mapped standard before the project can be saved.");
      return;
    }
    const mapped = await ensureProjectStandards(workspace.workspaceId, roles.map((entry) => entry.standard));
    if (mapped.error || roles.some((entry) => !mapped.codes.includes(entry.standard.trim()))) {
      setMessage(mapped.error || "Every outcome needs a mapped standard before the project can be saved.");
      return;
    }
    const phases: ProjectPhase[] = [
      { id: slug(phaseDesign) || "design", name: phaseDesign.trim() || "Design", position: 1 },
      { id: slug(phaseBuild) || "build", name: phaseBuild.trim() || "Build", position: 2, offScreen, activities: offScreen ? [{ kind: "off_screen", title: `${phaseBuild.trim() || "Build"} off-screen task` }] : [] },
    ];
    const project = await createPlanNode(revision.id, { kind: "project", title: projectName.trim(), body: "Multi-subject project blueprint", attributes: { phasesJSON: JSON.stringify(phases) } });
    if (!project.data || !("id" in project.data)) { setMessage("The Studio API could not save that project blueprint."); return; }
    const createdIds = [String(project.data.id)];
    try {
      for (const entry of roles) {
        const code = entry.standard.trim();
        const outcome = await createPlanNode(revision.id, {
          kind: "outcome",
          title: entry.title.trim(),
          standardCodes: [code],
          attributes: entry.evidenceKind ? { evidenceKind: entry.evidenceKind, evidenceDescription: entry.evidenceDescription ?? "" } : {},
        });
        if (!outcome.data || !("id" in outcome.data)) throw new Error(`Could not map outcome ${entry.title.trim()} to ${code}.`);
        createdIds.push(String(outcome.data.id));
        const edge = await createPlanEdge(revision.id, { kind: "parent_child", fromNodeId: String(project.data.id), toNodeId: String(outcome.data.id), note: entry.role });
        if (edge.error) throw new Error(`Could not attach ${entry.role} outcome ${entry.title.trim()}.`);
      }
      if (toolName.trim()) {
        const tool = await createResource(workspace.workspaceId, { kind: "tool", title: toolName.trim() });
        if (!tool.data || !("id" in tool.data)) throw new Error("Could not save the required tool.");
        const edge = await createPlanEdge(revision.id, { kind: "uses_resource", fromNodeId: String(project.data.id), toNodeId: String(tool.data.id), note: "required" });
        if (edge.error) throw new Error("Could not attach the required tool.");
      }
    } catch (cause) {
      const removed = await rollbackCreatedNodes(revision.id, createdIds);
      setMessage(removed
        ? (cause instanceof Error ? cause.message : "Project mapping failed; the incomplete blueprint was removed.")
        : "Project mapping failed and the incomplete blueprint could not be removed. Retry before publishing.");
      return;
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
    const request=++searchRequest.current;const scope=selectionRequest.current;
    setQuery(value);
    if (!workspace) return;
    const result = await listCurricula(workspace.workspaceId, value);
    if(request!==searchRequest.current||scope!==selectionRequest.current)return;
    if (result.data && "items" in result.data) setCurricula(result.data.items as Curriculum[]);
  }

  const ownSelected=!!workspace&&!!selected&&sameWorkspace(workspace.workspaceId,selected.workspaceId);
  const editableSelection=ownSelected&&!!workspace&&canAuthor(workspace.role);
  async function switchWorkspace(id:string){
    const next=memberships.find(m=>m.workspaceId===id);if(!next)return;
    const request=++selectionRequest.current;projectRequest.current++;
    setWorkspace(next);setSelected(null);setRevision(null);setRevisions([]);setCurricula([]);setQuery('');
    try{const result=await listCurricula(next.workspaceId);if(request!==selectionRequest.current)return;if(result.error)throw new Error(problem(result.error));if(result.data)setCurricula(result.data.items)}catch(e){setMessage(problem(e))}
  }
  return <div className="studio-shell">
    <aside className="rail" aria-label="Primary navigation">
      <div className="brand"><BookOpen aria-hidden size={18} /><span>Curriculum<br /><strong>Studio</strong></span></div>
      <nav>{nav.map(({ label, icon: Icon, text }, index) => <a className={index === 0 ? "active" : ""} href={`#${text.toLowerCase()}`} key={label}><Icon size={16} aria-hidden /><span>{label}</span><small>{text}</small></a>)}</nav>
      <div className="rail-foot"><span className="eyebrow">Workspace</span><strong>{workspace?.workspaceName ?? "No workspace selected"}</strong><small>Session access is provided by the host.</small></div>
    </aside>
    <main className="main">
      <header className="topbar"><div><span className="eyebrow">Explore / Curriculum library</span><h1>Plan with purpose.</h1></div><button className="icon-button" type="button" aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`} onClick={() => setTheme(theme === "dark" ? "light" : "dark")}>{theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}</button></header>
      <section className="context-rule"><span>AUTHENTICATED SESSION</span><code>{subject}</code><span className="status">{workspace ? 'Validated workspace membership' : 'No validated membership'}</span></section>
      <section className="intro"><div><span className="eyebrow">{workspace?.workspaceName ?? "Curriculum Studio"}</span><h2>Make the next right plan.</h2><p>Explore standards, shape a draft, and keep the parent’s judgment at the center of every decision.</p></div></section>
      {workspace&&<section className="collab-section"><label htmlFor="studio-workspace">Active workspace</label><select id="studio-workspace" value={workspace.workspaceId} onChange={e=>void switchWorkspace(e.target.value)}>{memberships.map(m=><option key={m.workspaceId} value={m.workspaceId}>{m.workspaceName} · {m.role}</option>)}</select><TemplateCreate key={workspace.workspaceId} workspaceId={workspace.workspaceId} role={workspace.role} onCreated={cur=>{setCurricula(items=>[cur,...items]);void openCurriculum(cur)}}/><WorkspacePolicy key={`policy-${workspace.workspaceId}`} workspaceId={workspace.workspaceId} role={workspace.role}/></section>}
      <section className="library" aria-labelledby="library-title"><div className="section-heading"><div><span className="eyebrow">Explore</span><h3 id="library-title">Curriculum library</h3></div><input id="curriculum-search" name="curriculumSearch" aria-label="Search curricula" placeholder="Search by name" value={query} onChange={(event) => search(event.target.value)} /></div>
        <form className="create-form" onSubmit={submit}><label htmlFor="curriculum-name">New curriculum brief</label><input id="curriculum-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="e.g. Grade 6 mathematics" /><button className="primary" type="submit" disabled={!workspace||!canAuthor(workspace.role)}>Create draft <span aria-hidden>↗</span></button></form>
        {message && <p className="feedback" role="status">{message}</p>}
        {curricula.length === 0 ? <div className="empty-state"><div className="empty-mark">01</div><div><span className="eyebrow">No curricula found</span><h3>Start with a brief.</h3><p>Create a draft above. The server owns search, pagination, and durable identity; this view never filters a bulk client-side collection.</p></div></div> : <div className="table-wrap"><table><thead><tr><th>Name</th><th>Status</th><th>Updated</th><th>ID</th></tr></thead><tbody>{curricula.map((item) => <tr key={item.id} onClick={() => openCurriculum(item)}><th scope="row"><button className="plain-button curriculum-open" type="button" onClick={e=>{e.stopPropagation();void openCurriculum(item)}}>{item.name}</button></th><td><span className="status-text">● {item.status}</span></td><td>{item.updatedAt ? new Date(item.updatedAt).toLocaleDateString() : "—"}</td><td><code>{item.id}</code></td></tr>)}</tbody></table></div>}
        {selected && <div className="plan-panel"><div><span className="eyebrow">Configure / {selected.name}</span><h3>{revision ? `Revision ${revision.revisionNumber ?? "draft"}` : "No revision"}</h3><p>{message}</p></div><div className="plan-actions"><button className="secondary" type="button" onClick={newDraft} disabled={!editableSelection}>New draft</button>{revision && <><button className="secondary" type="button" onClick={() => revisionAction("validate")} disabled={!ownSelected}>Validate</button><button className="primary" type="button" onClick={() => revisionAction("publish")} disabled={!editableSelection||revision.state!=='draft'}>Publish</button><select id="export-format" name="exportFormat" className="secondary" aria-label="Export format" value={exportFormat} disabled={exporting} onChange={(event) => setExportFormat(event.target.value as ExportFormat)}><option value="markdown">Markdown</option><option value="pdf">PDF</option><option value="docx">DOCX</option><option value="csv_coverage">CSV coverage</option><option value="json_bundle">JSON download subset</option><option value="ical">iCal schedule</option></select><button className="plain-button" type="button" disabled={exporting||!editableSelection} onClick={exportPlan}>{exporting ? "Exporting…" : "Export and download"}</button></>}</div></div>}
        {selected&&revision&&workspace&&<CollaborationPane key={selected.id} workspaceId={workspace.workspaceId} role={workspace.role} curriculum={selected} revision={revision} onRevision={next=>{selectionRequest.current++;setRevision(next);setProjectNodes([]);setSelectedProject('');void loadProjects(next.id)}}/>}
        {editableSelection && selected && revision && <section className="project-designer" aria-labelledby="project-designer-title"><div className="section-heading"><div><span className="eyebrow">Configure / Projects</span><h3 id="project-designer-title">Project designer</h3></div></div><form className="create-form project-form" onSubmit={addProject}><label htmlFor="project-name">Project name</label><input id="project-name" value={projectName} onChange={(event) => setProjectName(event.target.value)} /><label htmlFor="phase-design">Design phase</label><input id="phase-design" value={phaseDesign} onChange={(event) => setPhaseDesign(event.target.value)} /><label htmlFor="phase-build">Build phase</label><input id="phase-build" value={phaseBuild} onChange={(event) => setPhaseBuild(event.target.value)} /><label htmlFor="target-outcome">Target outcome</label><input id="target-outcome" value={targetOutcome} onChange={(event) => setTargetOutcome(event.target.value)} /><label htmlFor="target-standard">Target standard</label><input id="target-standard" value={targetStandard} onChange={(event) => setTargetStandard(event.target.value)} /><label htmlFor="prior-outcome">Prior outcome</label><input id="prior-outcome" value={priorOutcome} onChange={(event) => setPriorOutcome(event.target.value)} /><label htmlFor="prior-standard">Prior standard</label><input id="prior-standard" value={priorStandard} onChange={(event) => setPriorStandard(event.target.value)} /><label htmlFor="stretch-outcome">Stretch outcome</label><input id="stretch-outcome" value={stretchOutcome} onChange={(event) => setStretchOutcome(event.target.value)} /><label htmlFor="stretch-standard">Stretch standard</label><input id="stretch-standard" value={stretchStandard} onChange={(event) => setStretchStandard(event.target.value)} /><label htmlFor="tool-name">Required tool</label><input id="tool-name" value={toolName} onChange={(event) => setToolName(event.target.value)} /><label className="plain-button" htmlFor="off-screen"><input id="off-screen" type="checkbox" checked={offScreen} onChange={(event) => setOffScreen(event.target.checked)} /> Off-screen build task</label><button className="secondary" type="submit">Save blueprint</button></form>{projectNodes.length > 0 && <div className="project-operate"><label htmlFor="operate-project">Operate phase</label><select id="operate-project" className="secondary" value={selectedProject} onChange={(event) => { setSelectedProject(event.target.value); const next = projectNodes.find((node) => node.id === event.target.value); setSelectedPhase(next?.phases[0]?.id ?? ""); }}>{projectNodes.map((node) => <option key={node.id} value={node.id}>{node.title}</option>)}</select><label htmlFor="operate-phase">Phase</label><select id="operate-phase" className="secondary" value={selectedPhase} onChange={(event) => setSelectedPhase(event.target.value)}>{(projectNodes.find((node) => node.id === selectedProject)?.phases ?? []).map((phase) => <option key={phase.id} value={phase.id}>{phase.name}{phase.offScreen ? " (off-screen)" : ""}</option>)}</select><button className="primary" type="button" onClick={materializePhase}>Materialize phase</button></div>}{phaseItems.length > 0 && <ul className="project-list">{phaseItems.map((item) => <li key={item.title}><strong>{item.title}</strong><span>{item.kind}</span></li>)}</ul>}</section>}
      </section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Authoring API · Studio-local workspace authority</span></footer>
    </main>
  </div>;
}
