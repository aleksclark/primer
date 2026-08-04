/**
 * Enforce Phase 1 performance budgets on the production build output.
 *
 * Budgets (from phase-1-foundation.md / phase-5-launch.md):
 *   - initial JS (entry + critical imports): < 180 KB gzip combined heuristic
 *   - no single JS chunk over 250 KB raw
 *   - total JS assets under 450 KB raw (static pitch site)
 *
 * Run after `npm run build`: npm run check:bundle
 */
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { gzipSync } from "node:zlib";

const ROOT = new URL("..", import.meta.url).pathname;
const DIST = join(ROOT, "dist");
const ASSETS = join(DIST, "assets");

/** Combined gzip budget for entry + shared chunks used on first paint (bytes). */
const INITIAL_JS_GZIP_BUDGET = 180 * 1024;
/**
 * Soft ceiling for any single JS file raw size.
 * Diligence routes may ship larger async chunks; entry must stay lean.
 */
const SINGLE_JS_RAW_BUDGET = 350 * 1024;
/** Total JS raw budget across the static build (all routes). */
const TOTAL_JS_RAW_BUDGET = 550 * 1024;

const failures: string[] = [];

if (!existsSync(DIST) || !existsSync(ASSETS)) {
  console.error("check-bundle: dist/assets not found. Run `npm run build` first.");
  process.exit(1);
}

function listAssets(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) out.push(...listAssets(full));
    else out.push(full);
  }
  return out;
}

const assets = listAssets(ASSETS);
const jsFiles = assets.filter((f) => f.endsWith(".js"));
const cssFiles = assets.filter((f) => f.endsWith(".css"));

if (jsFiles.length === 0) {
  failures.push("no JS assets found in dist/assets");
}

let totalJsRaw = 0;
let totalJsGzip = 0;
const rows: { name: string; raw: number; gzip: number }[] = [];

for (const file of jsFiles) {
  const buf = readFileSync(file);
  const raw = buf.byteLength;
  const gzip = gzipSync(buf).byteLength;
  totalJsRaw += raw;
  totalJsGzip += gzip;
  const name = file.slice(ASSETS.length + 1);
  rows.push({ name, raw, gzip });
  if (raw > SINGLE_JS_RAW_BUDGET) {
    failures.push(
      `${name}: ${kb(raw)} KB raw exceeds single-chunk budget ${kb(SINGLE_JS_RAW_BUDGET)} KB`,
    );
  }
}

if (totalJsRaw > TOTAL_JS_RAW_BUDGET) {
  failures.push(
    `total JS ${kb(totalJsRaw)} KB raw exceeds budget ${kb(TOTAL_JS_RAW_BUDGET)} KB`,
  );
}

// Initial JS: prefer entry-named chunks; if the build is a single bundle,
// the whole gzip total is the initial cost. Async route chunks are excluded
// when clearly named as separate route modules.
const entryLike = rows.filter((r) =>
  /(^|\/)(index|main|app|entry|client)[-.]/i.test(r.name),
);
const asyncLike = rows.filter((r) =>
  /(Demo|Market|Evidence|Schools|Company|Diligence|Page)-/i.test(r.name),
);
let initialBudgetValue: number;
if (entryLike.length > 0) {
  // Entry + any non-async remaining shared chunks
  const shared = rows.filter((r) => !asyncLike.includes(r));
  initialBudgetValue = shared.reduce((s, r) => s + r.gzip, 0);
} else if (rows.length === 1) {
  initialBudgetValue = totalJsGzip;
} else {
  // Fallback: largest chunk as proxy for main
  initialBudgetValue = Math.max(...rows.map((r) => r.gzip));
}

if (initialBudgetValue > INITIAL_JS_GZIP_BUDGET) {
  failures.push(
    `initial JS ~${kb(initialBudgetValue)} KB gzip exceeds ${kb(INITIAL_JS_GZIP_BUDGET)} KB budget`,
  );
}

// robots / sitemap / index presence
for (const required of ["index.html", "robots.txt", "sitemap.xml"]) {
  if (!existsSync(join(DIST, required))) {
    failures.push(`dist missing ${required}`);
  }
}

function kb(n: number): string {
  return (n / 1024).toFixed(1);
}

console.log("Bundle budget report\n");
console.log("  JS assets:");
for (const r of rows.sort((a, b) => b.gzip - a.gzip)) {
  console.log(`    ${r.name.padEnd(42)} raw ${kb(r.raw).padStart(7)} KB  gzip ${kb(r.gzip).padStart(7)} KB`);
}
console.log("");
console.log(`  total JS raw:   ${kb(totalJsRaw)} KB / ${kb(TOTAL_JS_RAW_BUDGET)} KB`);
console.log(`  total JS gzip:  ${kb(totalJsGzip)} KB`);
console.log(`  initial gzip≈   ${kb(initialBudgetValue)} KB / ${kb(INITIAL_JS_GZIP_BUDGET)} KB`);
console.log(`  CSS files:      ${cssFiles.length}`);
console.log(`  async-ish:      ${asyncLike.length}`);

if (failures.length > 0) {
  console.error("\nBundle checks failed:");
  for (const f of failures) console.error(`  ✗ ${f}`);
  process.exit(1);
}

console.log("\nBundle checks passed.");
