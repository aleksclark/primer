import { FormEvent, useEffect, useState } from "react";
import { BookOpen, Compass, FolderKanban, LogIn, Moon, Settings2, Sun } from "lucide-react";
import { createCurriculum, currentSession, listCurricula } from "./api/client";

type Theme = "dark" | "light";
type Workspace = { workspaceId: string; workspaceName: string; role: string };
type Curriculum = { id: string; name: string; description?: string; status: string; updatedAt?: string };

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
        {curricula.length === 0 ? <div className="empty-state"><div className="empty-mark">01</div><div><span className="eyebrow">No curricula found</span><h3>Start with a brief.</h3><p>Create a draft above. The server owns search, pagination, and durable identity; this view never filters a bulk client-side collection.</p></div></div> : <div className="table-wrap"><table><thead><tr><th>Name</th><th>Status</th><th>Updated</th><th>ID</th></tr></thead><tbody>{curricula.map((item) => <tr key={item.id}><th scope="row">{item.name}</th><td><span className="status-text">● {item.status}</span></td><td>{item.updatedAt ? new Date(item.updatedAt).toLocaleDateString() : "—"}</td><td><code>{item.id}</code></td></tr>)}</tbody></table></div>}
      </section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Bearer tokens stay server-side · host-only session cookie</span></footer>
    </main>
  </div>;
}
