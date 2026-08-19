import { useEffect, useState } from "react";
import { BookOpen, Compass, FolderKanban, LogOut, Moon, Settings2, Sun } from "lucide-react";
import { currentSession } from "./api/client";

type Theme = "dark" | "light";

const nav = [
  { label: "Explore", icon: Compass, text: "Curricula" },
  { label: "Operate", icon: FolderKanban, text: "Plans" },
  { label: "Configure", icon: Settings2, text: "Workspace" },
];

export default function App() {
  const [theme, setTheme] = useState<Theme>("dark");
  const [subject, setSubject] = useState("Loading session…");

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    currentSession().then(({ data }) => {
      if (data && "subjectRef" in data) setSubject(String(data.subjectRef));
      else setSubject("Sign in to continue");
    }).catch(() => setSubject("Sign in to continue"));
  }, [theme]);

  return <div className="studio-shell">
    <aside className="rail" aria-label="Primary navigation">
      <div className="brand"><BookOpen aria-hidden size={18} /><span>Curriculum<br /><strong>Studio</strong></span></div>
      <nav>{nav.map(({ label, icon: Icon, text }, index) => <a className={index === 0 ? "active" : ""} href={`#${text.toLowerCase()}`} key={label}><Icon size={16} aria-hidden /><span>{label}</span><small>{text}</small></a>)}</nav>
      <div className="rail-foot"><span className="eyebrow">Workspace</span><strong>Choose a workspace</strong><button type="button" className="plain-button"><LogOut size={14} aria-hidden /> Sign out</button></div>
    </aside>
    <main className="main">
      <header className="topbar"><div><span className="eyebrow">Explore / Curriculum library</span><h1>Plan with purpose.</h1></div><button className="icon-button" type="button" aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`} onClick={() => setTheme(theme === "dark" ? "light" : "dark")}>{theme === "dark" ? <Sun size={17} /> : <Moon size={17} />}</button></header>
      <section className="context-rule"><span>AUTHENTICATED SESSION</span><code>{subject}</code><span className="status"><i /> BFF session active</span></section>
      <section className="intro"><div><span className="eyebrow">Curriculum Studio</span><h2>Make the next right plan.</h2><p>Explore standards, shape a draft, and keep the parent’s judgment at the center of every decision.</p></div><button className="primary" type="button">Create curriculum <span aria-hidden>↗</span></button></section>
      <section className="empty-state" aria-labelledby="empty-title"><div className="empty-mark">01</div><div><span className="eyebrow">No workspace selected</span><h3 id="empty-title">Your curriculum library starts here.</h3><p>Choose a workspace from the shell to see curricula. Lists are searched and paged by the Studio API; the browser never becomes the source of truth.</p><button className="secondary" type="button">Choose workspace</button></div></section>
      <footer className="footer"><span>Studio shell · dark-first Editorial Instrument</span><span>Bearer tokens stay server-side · host-only session cookie</span></footer>
    </main>
  </div>;
}
