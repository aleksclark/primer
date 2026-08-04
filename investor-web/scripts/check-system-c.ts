/**
 * System C style gate for investor-web authored styles.
 * Fails if nonzero border-radius or box-shadow appear outside generated tokens.
 *
 * Run: npm run check:system-c
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const ROOT = new URL("..", import.meta.url).pathname;
const SRC = join(ROOT, "src");

const failures: string[] = [];

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    const st = statSync(full);
    if (st.isDirectory()) out.push(...walk(full));
    else if (/\.(css|tsx|ts)$/.test(name)) out.push(full);
  }
  return out;
}

const radiusRe = /border-radius\s*:\s*(?!0(?:px|rem|em|%)?\s*;?)(?!var\(--primer-radius)/i;
const shadowRe = /box-shadow\s*:\s*(?!none)/i;
const rawHexInCss = /(?<!var\(--primer[^)]*)#(?:[0-9a-fA-F]{3,8})\b/;

// Only flag hex in .css files under src (generated primer.css is imported, not authored here).
for (const file of walk(SRC)) {
  const rel = relative(ROOT, file);
  const text = readFileSync(file, "utf8");
  const lines = text.split("\n");

  lines.forEach((line, i) => {
    const trimmed = line.trim();
    if (trimmed.startsWith("//") || trimmed.startsWith("/*") || trimmed.startsWith("*")) return;

    if (radiusRe.test(line) && !/border-radius\s*:\s*0/.test(line) && !/var\(--primer-radius/.test(line)) {
      // allow "0" and "0px"
      if (!/border-radius\s*:\s*0(px)?\s*;?/.test(line)) {
        failures.push(`${rel}:${i + 1}: nonzero border-radius → ${trimmed}`);
      }
    }
    if (shadowRe.test(line)) {
      failures.push(`${rel}:${i + 1}: box-shadow → ${trimmed}`);
    }
    if (file.endsWith(".css") && rawHexInCss.test(line) && !line.includes("primer")) {
      // Allow nothing — investor CSS must use tokens only.
      if (/#[0-9a-fA-F]{3,8}/.test(line)) {
        failures.push(`${rel}:${i + 1}: raw hex in authored CSS → ${trimmed}`);
      }
    }
  });
}

// Landmark smoke: required section ids must exist in sections data.
const sectionsPath = join(SRC, "data/sections.ts");
const sectionsText = readFileSync(sectionsPath, "utf8");
const required = [
  "thesis",
  "problem",
  "product",
  "proof",
  "market",
  "go-to-market",
  "competition",
  "founder",
  "current-state",
  "round",
  "contact",
];
for (const id of required) {
  if (!sectionsText.includes(`id: "${id}"`)) {
    failures.push(`sections.ts: missing required section id "${id}"`);
  }
}

// Component presence
const requiredComponents = [
  "SiteHeader.tsx",
  "SiteFooter.tsx",
  "SectionFrame.tsx",
  "SystemLabel.tsx",
  "StateBadge.tsx",
  "MetricBlock.tsx",
  "RuledCard.tsx",
  "SourceLink.tsx",
  "CaveatNote.tsx",
  "PrimaryButton.tsx",
  "TextLink.tsx",
  "ThemeToggle.tsx",
  "SkipLink.tsx",
  "MobileSectionNav.tsx",
  "InvestorCTA.tsx",
];
for (const name of requiredComponents) {
  try {
    statSync(join(SRC, "components", name));
  } catch {
    failures.push(`missing component: components/${name}`);
  }
}

if (failures.length > 0) {
  console.error("System C checks failed:\n");
  for (const f of failures) console.error(`  ✗ ${f}`);
  process.exit(1);
}

console.log("System C checks passed.");
console.log(`  scanned ${walk(SRC).length} source files`);
console.log(`  required sections: ${required.length}`);
console.log(`  required components: ${requiredComponents.length}`);
