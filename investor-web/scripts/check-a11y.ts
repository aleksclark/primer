/**
 * Lightweight accessibility static gate for investor-web.
 *
 * Complements manual keyboard / screen-reader passes documented in LAUNCH.md.
 * Checks authored source for landmarks, skip link, focus styles, reduced motion,
 * and common anti-patterns. Does not replace axe or NVDA.
 *
 * Run: npm run check:a11y
 */
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const ROOT = new URL("..", import.meta.url).pathname;
const SRC = join(ROOT, "src");
const failures: string[] = [];
const notes: string[] = [];

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) out.push(...walk(full));
    else if (/\.(tsx|ts|css)$/.test(name)) out.push(full);
  }
  return out;
}

// Skip link + main landmark
const skip = join(SRC, "components/SkipLink.tsx");
const app = join(SRC, "App.tsx");
const css = join(SRC, "styles/index.css");

if (!existsSync(skip)) failures.push("missing SkipLink component");
else {
  const text = readFileSync(skip, "utf8");
  if (!/href=["']#main-content["']/.test(text) && !/main-content/.test(text)) {
    failures.push("SkipLink must target #main-content");
  }
}

if (!existsSync(app)) failures.push("missing App.tsx");
else {
  const text = readFileSync(app, "utf8");
  if (!/id=["']main-content["']/.test(text)) {
    failures.push('App shell missing main id="main-content"');
  }
  if (!/<main\b/.test(text)) failures.push("App shell missing <main> landmark");
  if (!/SkipLink/.test(text)) failures.push("App shell must render SkipLink");
  if (!/SiteHeader/.test(text)) failures.push("App shell must render SiteHeader");
  if (!/SiteFooter/.test(text)) failures.push("App shell must render SiteFooter");
  if (!/RouteAnnouncer/.test(text)) failures.push("App shell must render RouteAnnouncer");
}

if (!existsSync(css)) failures.push("missing styles/index.css");
else {
  const text = readFileSync(css, "utf8");
  if (!/:focus-visible/.test(text)) {
    failures.push("CSS missing :focus-visible styles");
  }
  if (!/prefers-reduced-motion/.test(text)) {
    failures.push("CSS missing prefers-reduced-motion rules");
  }
  if (!/\.sr-only/.test(text)) {
    failures.push("CSS missing .sr-only utility");
  }
  if (!/@media print/.test(text)) {
    failures.push("CSS missing print stylesheet");
  }
}

// Route titles for screen reader / document title parity
const announcer = join(SRC, "components/RouteAnnouncer.tsx");
const siteMeta = join(SRC, "lib/siteMeta.ts");
if (!existsSync(announcer)) failures.push("missing RouteAnnouncer");
else {
  const text = readFileSync(announcer, "utf8");
  if (!/aria-live/.test(text)) failures.push("RouteAnnouncer missing aria-live");
  if (!/usePageMeta|metaForPath/.test(text)) {
    failures.push("RouteAnnouncer must sync document meta via usePageMeta or metaForPath");
  }
}
if (!existsSync(siteMeta)) failures.push("missing lib/siteMeta.ts route metadata");
else {
  const text = readFileSync(siteMeta, "utf8");
  const requiredPaths = ["/", "/demo", "/market", "/evidence", "/schools", "/company", "/diligence"];
  for (const p of requiredPaths) {
    if (!text.includes(`"${p}"`)) {
      failures.push(`siteMeta missing route meta for ${p}`);
    }
  }
  if (!/shouldNoIndex/.test(text)) failures.push("siteMeta missing shouldNoIndex helper");
}

// Interactive explorers should expose accessible names / live regions where stepped
const demo = join(SRC, "components/explorers/DemoExplorer.tsx");
if (existsSync(demo)) {
  const text = readFileSync(demo, "utf8");
  if (!/aria-live/.test(text)) {
    failures.push("DemoExplorer should expose aria-live for step changes");
  }
  if (!/role=["']tablist["']/.test(text) && !/aria-label/.test(text)) {
    failures.push("DemoExplorer missing tablist or labelled controls");
  }
}

// Scan for common a11y footguns in authored TSX
for (const file of walk(SRC)) {
  if (!file.endsWith(".tsx")) continue;
  const rel = relative(ROOT, file);
  const lines = readFileSync(file, "utf8").split("\n");
  lines.forEach((line, i) => {
    const trimmed = line.trim();
    if (trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*")) return;

    // <img> without alt
    if (/<img\b/.test(line) && !/\balt=/.test(line)) {
      failures.push(`${rel}:${i + 1}: <img> missing alt attribute`);
    }
    // positive tabindex
    if (/tabIndex=\{?[1-9]/.test(line) || /tabindex=["'][1-9]/.test(line)) {
      failures.push(`${rel}:${i + 1}: positive tabIndex disrupts focus order`);
    }
    // click handler on non-interactive element without role/button
    if (
      /onClick=\{/.test(line) &&
      /<(div|span)\b/.test(line) &&
      !/role=/.test(line) &&
      !/button/.test(line)
    ) {
      notes.push(`${rel}:${i + 1}: onClick on non-interactive element (review)`);
    }
    // icon-only buttons should have aria-label (heuristic)
    if (/<button[^>]*>\s*<\/button>/.test(line) && !/aria-label/.test(line)) {
      failures.push(`${rel}:${i + 1}: empty button without aria-label`);
    }
  });
}

// Manual checklist reminder (always printed; not a failure)
const manualChecklist = [
  "Keyboard-only full flow (header, theme, explorers, contact)",
  "NVDA or VoiceOver pass on home + one diligence route",
  "Heading/landmark audit on each route",
  "Color contrast in dark and light themes",
  "Chart/table text equivalence for explorers",
  "Zoom/reflow at 200% and 400%",
];

if (failures.length > 0) {
  console.error("Accessibility static checks failed:\n");
  for (const f of failures) console.error(`  ✗ ${f}`);
  if (notes.length) {
    console.error("\nNotes:");
    for (const n of notes) console.error(`  · ${n}`);
  }
  process.exit(1);
}

console.log("Accessibility static checks passed.");
console.log(`  scanned ${walk(SRC).length} source files`);
if (notes.length) {
  console.log("  review notes:");
  for (const n of notes) console.log(`    · ${n}`);
}
console.log("\nManual a11y checklist (not automated):");
for (const item of manualChecklist) console.log(`  □ ${item}`);
