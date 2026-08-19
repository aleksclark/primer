import { FormEvent, useEffect, useState } from "react";
import { BookOpen, Compass, FolderKanban, LogIn, Moon, Settings2, Sun } from "lucide-react";
import { createCurriculum, currentSession, createRevision, exportRevision, listCurricula, listRevisions, publishRevision, validateRevision } from "./api/client";

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
    if (result.data && "items" in result.data) { const items = result.data.items as Revision[]; setRevisions(items); setRevision(items[0] ?? null); setMessage(items.length ? "Draft ready to configure." : "No draft revision yet."); }
  }

  async function newDraft() {
    if (!selected) return; const result = await createRevision(selected.id); if (result.data && "id" in result.data) { const next = result.data as Revision; setRevisions((items) => [next, ...items]); setRevision(next); setMessage("Draft revision created."); }
  }
  async function revisionAction(action: "validate" | "publish" | "markdown" | "pdf") {
    if (!revision) return;
    if (action === "validate") await validateRevision(revision.id);
    if (action === "publish") await publishRevision(revision.id);
    if (action === "markdown" || action === "pdf") await exportRevision(revision.id, action);
    setMessage(action === "validate" ? "Validation report saved." : action === "publish" ? "Revision published." : `Preparing ${action} export.`);
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
        {selected && <div className="plan-panel"><div><span className="eyebrow">Configure / {selected.name}</span><h3>{revision ? `Revision ${revision.revisionNumber ?? "draft"}` : "No revision"}</h3><p>{message}</p></div><div className="plan-actions"><button className="secondary" type="button" onClick={newDraft}>New draft</button>{revision && <><button className="secondary" type="button" onClick={() => revisionAction("validate")}>Validate</button><button className="primary" type="button" onClick={() => revisionAction("publish")}>Publish</button><button className="plain-button" type="button" onClick={() => revisionAction("markdown")}>Export MD</button><button className="plain-button" type="button" onClick={() => revisionAction("pdf")}>Export PDF</button></>}</div></div>}
      </section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Bearer tokens stay server-side · host-only session cookie</span></footer>
    </main>
  </div>;
}
