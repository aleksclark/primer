/**
 * Phase 0 content/data contract validator.
 * Fails non-zero on any broken claim invariant.
 *
 * Run: npm test  (or npm run validate)
 */
import {
  adoptionLadder,
  beachheadJobs,
  competitorCategories,
  founderProof,
  fundingPlan,
  instructionalLoop,
  investorData,
  marketLayers,
  pricingTiers,
  problemPoints,
  productState,
  researchClaims,
  sections,
  seedScorecard,
  sources,
  teachingExamples,
  teamPlan,
  type MarketLayer,
  type StatusLabel,
} from "../src/data/index.ts";

const STATUS_LABELS = new Set<StatusLabel>([
  "LIVE",
  "IN_DEVELOPMENT",
  "PLANNED",
  "HYPOTHESIS",
  "EVIDENCE_NEEDED",
]);

const FORBIDDEN_CLAIM_WORDS = ["proven", "validated", "traction"] as const;

/** Contexts where a forbidden token is explicitly allowed (negation / meta). */
const FORBIDDEN_WORD_ALLOWLIST: RegExp[] = [
  /\bunproven\b/i,
  /\bnot\s+yet\s+proven\b/i,
  /\bnot\s+proven\b/i,
  /\bwithout\s+being\s+proven\b/i,
  /\bto\s+be\s+proven\b/i,
  /\bremain(?:s)?\s+to\s+be\s+proven\b/i,
  /\bnot\s+yet\s+validated\b/i,
  /\bunvalidated\b/i,
  /\bnot\s+validated\b/i,
  /\bto\s+be\s+validated\b/i,
  /\bwithout\s+.*\btraction\b/i,
  /\bno\s+traction\b/i,
  /\bnot\s+.*\btraction\b/i,
  /\bforbidden words.*\b(?:proven|validated|traction)\b/i,
  /\bdo not describe it as customer demand\b/i,
  /\bdo not use\b.*\b(?:proven|validated|traction)\b/i,
];

interface CheckResult {
  name: string;
  ok: boolean;
  details: string[];
}

const results: CheckResult[] = [];

function check(name: string, fn: () => string[]): void {
  try {
    const details = fn();
    results.push({ name, ok: details.length === 0, details });
  } catch (err) {
    results.push({
      name,
      ok: false,
      details: [err instanceof Error ? err.message : String(err)],
    });
  }
}

function collectStrings(value: unknown, path = ""): { path: string; text: string }[] {
  if (value == null) return [];
  if (typeof value === "string") return [{ path, text: value }];
  if (typeof value === "number" || typeof value === "boolean") return [];
  if (Array.isArray(value)) {
    return value.flatMap((item, i) => collectStrings(item, `${path}[${i}]`));
  }
  if (typeof value === "object") {
    return Object.entries(value as Record<string, unknown>).flatMap(([k, v]) =>
      collectStrings(v, path ? `${path}.${k}` : k),
    );
  }
  return [];
}

function isAllowedForbiddenContext(text: string, word: string): boolean {
  const lower = text.toLowerCase();
  const idx = lower.indexOf(word);
  if (idx < 0) return true;

  // If the match is part of an allowlisted phrase, accept the whole string.
  for (const re of FORBIDDEN_WORD_ALLOWLIST) {
    if (re.test(text)) {
      // Still reject if another raw occurrence sits outside the allowlisted phrase.
      const allowedSpans: [number, number][] = [];
      const global = new RegExp(re.source, re.flags.includes("g") ? re.flags : `${re.flags}g`);
      let m: RegExpExecArray | null;
      while ((m = global.exec(text)) !== null) {
        allowedSpans.push([m.index, m.index + m[0].length]);
      }
      const wordRe = new RegExp(`\\b${word}\\b`, "gi");
      let wm: RegExpExecArray | null;
      while ((wm = wordRe.exec(lower)) !== null) {
        const start = wm.index;
        const end = start + word.length;
        const covered = allowedSpans.some(([a, b]) => start >= a && end <= b);
        // "unproven" contains "proven" as substring — require word boundary on the forbidden token.
        // For "proven" inside "unproven", \\bproven\\b does not match because of the leading "un".
        if (!covered && word === "proven") {
          // double-check: if the char before is a letter, it is a longer word (e.g. unproven)
          const before = start > 0 ? lower[start - 1] : " ";
          if (/[a-z]/.test(before)) continue;
        }
        if (!covered) return false;
      }
      return true;
    }
  }

  // Bare word boundary check for proven inside unproven without allowlist hit
  if (word === "proven") {
    const re = /\bproven\b/i;
    if (!re.test(text)) return true; // only inside longer words like unproven
  }

  return false;
}

// ---------------------------------------------------------------------------
// Checks
// ---------------------------------------------------------------------------

check("package exports are complete", () => {
  const errors: string[] = [];
  const required: (keyof typeof investorData)[] = [
    "productState",
    "pricingTiers",
    "marketLayers",
    "seedScorecard",
    "researchClaims",
    "competitorCategories",
    "founderProof",
    "fundingPlan",
    "sources",
    "sections",
    "instructionalLoop",
    "teachingExamples",
    "adoptionLadder",
    "problemPoints",
    "beachheadJobs",
    "teamPlan",
  ];
  for (const key of required) {
    const value = investorData[key];
    if (value == null) errors.push(`missing investorData.${key}`);
    if (Array.isArray(value) && value.length === 0) errors.push(`empty array: ${key}`);
  }
  return errors;
});

check("status labels match System C vocabulary", () => {
  const errors: string[] = [];
  const statuses = collectStrings(investorData)
    .filter(({ path }) => path.endsWith(".status") || path === "status")
    .map(({ path, text }) => ({ path, text }));

  for (const { path, text } of statuses) {
    if (!STATUS_LABELS.has(text as StatusLabel)) {
      errors.push(`${path}: invalid status "${text}"`);
    }
  }
  return errors;
});

check("source IDs are unique", () => {
  const seen = new Map<string, number>();
  for (const s of sources) {
    seen.set(s.id, (seen.get(s.id) ?? 0) + 1);
  }
  return [...seen.entries()]
    .filter(([, n]) => n > 1)
    .map(([id, n]) => `duplicate source id "${id}" (${n} times)`);
});

check("source records have required fields", () => {
  const errors: string[] = [];
  for (const s of sources) {
    if (!s.id) errors.push("source missing id");
    if (!s.title) errors.push(`${s.id}: missing title`);
    if (!s.organization) errors.push(`${s.id}: missing organization`);
    if (!s.url) errors.push(`${s.id}: missing url`);
    if (!s.publicationDate) errors.push(`${s.id}: missing publicationDate`);
    if (!s.accessedDate) errors.push(`${s.id}: missing accessedDate`);
    if (!s.qualityTier) errors.push(`${s.id}: missing qualityTier`);
  }
  return errors;
});

check("cited sourceIds resolve", () => {
  const known = new Set(sources.map((s) => s.id));
  const errors: string[] = [];
  const refPaths: { id: string; path: string }[] = [];

  for (const m of marketLayers) {
    for (const id of m.sourceIds) refPaths.push({ id, path: `marketLayers.${m.id}` });
  }
  for (const m of seedScorecard) {
    for (const id of m.sourceIds) refPaths.push({ id, path: `seedScorecard.${m.id}` });
  }
  for (const r of researchClaims) {
    for (const id of r.sourceIds) refPaths.push({ id, path: `researchClaims.${r.id}` });
  }
  for (const c of competitorCategories) {
    for (const id of c.sourceIds) refPaths.push({ id, path: `competitorCategories.${c.id}` });
  }
  for (const f of founderProof) {
    for (const id of f.sourceIds) refPaths.push({ id, path: `founderProof.${f.id}` });
  }
  for (const id of fundingPlan.sourceIds) {
    refPaths.push({ id, path: "fundingPlan" });
  }
  for (const s of sections) {
    for (const id of s.citationIds) refPaths.push({ id, path: `sections.${s.id}` });
  }

  for (const { id, path } of refPaths) {
    if (!known.has(id)) errors.push(`${path} cites unknown source "${id}"`);
  }
  return errors;
});

check("market ceiling arithmetic", () => {
  const errors: string[] = [];
  const expected: { id: string; learners: number; arpu: number; ceiling: number }[] = [
    { id: "base-homeschool-nheri-mid", learners: 3.4e6, arpu: 600, ceiling: 2.04e9 },
    { id: "core-public-grades-5-8", learners: 14.61e6, arpu: 1200, ceiling: 17.532e9 },
    { id: "premier-nais", learners: 631_672, arpu: 3600, ceiling: 2_274_019_200 },
    { id: "premier-private-15k-plus", learners: 1_159_670, arpu: 3600, ceiling: 4_174_812_000 },
  ];

  for (const exp of expected) {
    const layer = marketLayers.find((l) => l.id === exp.id);
    if (!layer) {
      errors.push(`missing required market layer ${exp.id}`);
      continue;
    }
    if (layer.learners !== exp.learners) {
      errors.push(`${exp.id}: learners ${layer.learners} !== ${exp.learners}`);
    }
    if (layer.annualRevenuePerLearner !== exp.arpu) {
      errors.push(
        `${exp.id}: annualRevenuePerLearner ${layer.annualRevenuePerLearner} !== ${exp.arpu}`,
      );
    }
    const product = layer.learners * layer.annualRevenuePerLearner;
    if (product !== exp.ceiling) {
      errors.push(
        `${exp.id}: learners*arpu = ${product} !== expected ${exp.ceiling}`,
      );
    }
    if (layer.modeledCeiling !== exp.ceiling) {
      errors.push(
        `${exp.id}: modeledCeiling ${layer.modeledCeiling} !== ${exp.ceiling}`,
      );
    }
    if (layer.modeledCeiling !== product) {
      errors.push(
        `${exp.id}: modeledCeiling ${layer.modeledCeiling} !== learners*arpu ${product}`,
      );
    }
  }

  // Every layer must be internally consistent
  for (const layer of marketLayers) {
    const product = layer.learners * layer.annualRevenuePerLearner;
    if (layer.modeledCeiling !== product) {
      errors.push(
        `${layer.id}: modeledCeiling ${layer.modeledCeiling} !== ${layer.learners}*${layer.annualRevenuePerLearner} (${product})`,
      );
    }
  }

  return errors;
});

check("no market layer is additive", () => {
  const errors: string[] = [];
  for (const layer of marketLayers) {
    // Runtime guard — type says false but data could be wrong-cast
    if ((layer as MarketLayer).additive !== false) {
      errors.push(`${layer.id}: additive must be false, got ${String(layer.additive)}`);
    }
    if (!layer.overlapGroup) {
      errors.push(`${layer.id}: missing overlapGroup`);
    }
  }
  return errors;
});

check("pricing tiers include Base $50, Core $100, Premier $300", () => {
  const errors: string[] = [];
  const required = [
    { id: "base", monthly: 50, annual: 600 },
    { id: "core", monthly: 100, annual: 1200 },
    { id: "premier", monthly: 300, annual: 3600 },
  ] as const;

  for (const exp of required) {
    const tier = pricingTiers.find((t) => t.id === exp.id);
    if (!tier) {
      errors.push(`missing pricing tier ${exp.id}`);
      continue;
    }
    if (tier.monthlyPriceUsd !== exp.monthly) {
      errors.push(`${exp.id}: monthly ${tier.monthlyPriceUsd} !== ${exp.monthly}`);
    }
    if (tier.annualRevenuePerLearnerUsd !== exp.annual) {
      errors.push(`${exp.id}: annual ${tier.annualRevenuePerLearnerUsd} !== ${exp.annual}`);
    }
    if (tier.monthlyPriceUsd * 12 !== tier.annualRevenuePerLearnerUsd) {
      errors.push(`${exp.id}: monthly*12 !== annual`);
    }
  }
  return errors;
});

check("funding plan is $3.5M / 18 months", () => {
  const errors: string[] = [];
  if (fundingPlan.amountUsd !== 3_500_000) {
    errors.push(`amountUsd ${fundingPlan.amountUsd} !== 3500000`);
  }
  if (fundingPlan.runwayMonths !== 18) {
    errors.push(`runwayMonths ${fundingPlan.runwayMonths} !== 18`);
  }
  if (fundingPlan.annualCashSalaryUsd !== 300_000) {
    errors.push(`annualCashSalaryUsd ${fundingPlan.annualCashSalaryUsd} !== 300000`);
  }
  if (fundingPlan.teamSize !== 3) {
    errors.push(`teamSize ${fundingPlan.teamSize} !== 3`);
  }
  if (fundingPlan.contingencyRate !== 0.2) {
    errors.push(`contingencyRate ${fundingPlan.contingencyRate} !== 0.2`);
  }
  if (fundingPlan.lineItems.length === 0) {
    errors.push("funding plan has no line items");
  }
  if (fundingPlan.milestones.length === 0) {
    errors.push("funding plan has no milestones");
  }
  // people cost: 3 * 300k * 1.5 * 1.3 + 9k equipment = 1,350,000 * 1.3 + 9,000 = 1,764,000
  const expectedPeople = 3 * 300_000 * 1.5 * 1.3 + 9_000;
  if (fundingPlan.peopleCost18MoUsd !== expectedPeople) {
    errors.push(
      `peopleCost18MoUsd ${fundingPlan.peopleCost18MoUsd} !== expected ${expectedPeople}`,
    );
  }
  return errors;
});

check("LIVE products require artifact links", () => {
  const errors: string[] = [];
  for (const p of productState) {
    if (p.status === "LIVE") {
      if (!p.artifactUrl || p.artifactUrl.trim() === "") {
        errors.push(`${p.id}: LIVE product missing artifactUrl`);
      }
      if (!p.artifactLabel || p.artifactLabel.trim() === "") {
        errors.push(`${p.id}: LIVE product missing artifactLabel`);
      }
    }
  }
  return errors;
});

check("product state covers LIVE / IN_DEVELOPMENT / PLANNED", () => {
  const errors: string[] = [];
  const statuses = new Set(productState.map((p) => p.status));
  for (const required of ["LIVE", "IN_DEVELOPMENT", "PLANNED"] as const) {
    if (!statuses.has(required)) errors.push(`no product with status ${required}`);
  }
  return errors;
});

check("seed scorecard metrics have definition, source, caveat; unknowns are NOT_YET_MEASURED", () => {
  const errors: string[] = [];
  if (seedScorecard.length === 0) errors.push("scorecard is empty");

  for (const m of seedScorecard) {
    if (!m.definition || m.definition.trim() === "") {
      errors.push(`${m.id}: missing definition`);
    }
    if (!m.caveat || m.caveat.trim() === "") {
      errors.push(`${m.id}: missing caveat`);
    }
    if (!m.sourceIds || m.sourceIds.length === 0) {
      errors.push(`${m.id}: missing sourceIds`);
    }
    if (m.current !== "NOT_YET_MEASURED" && typeof m.current !== "number") {
      errors.push(`${m.id}: current must be number or NOT_YET_MEASURED`);
    }
    if (typeof m.floor !== "number" || typeof m.target !== "number") {
      errors.push(`${m.id}: floor/target must be numbers`);
    }
    // Directional consistency: for higher_is_better, target >= floor
    if (m.direction === "higher_is_better" && m.target < m.floor) {
      errors.push(`${m.id}: target ${m.target} < floor ${m.floor} for higher_is_better`);
    }
    if (m.direction === "lower_is_better" && m.target > m.floor) {
      errors.push(`${m.id}: target ${m.target} > floor ${m.floor} for lower_is_better`);
    }
  }

  const requiredIds = [
    "paying-learners",
    "arr-run-rate",
    "retention-12-week",
    "gross-margin-post-credit",
    "cogs-per-learner",
    "cac-payback",
    "learning-outcome",
  ];
  for (const id of requiredIds) {
    if (!seedScorecard.some((m) => m.id === id)) {
      errors.push(`missing required scorecard metric ${id}`);
    }
  }
  return errors;
});

check("research claims include tutoring meta-analysis 0.288 SD", () => {
  const errors: string[] = [];
  const meta = researchClaims.find((r) => r.id === "tutoring-meta-2024");
  if (!meta) {
    errors.push("missing tutoring-meta-2024 claim");
  } else if (meta.effectSizeSd !== 0.288) {
    errors.push(`tutoring meta effectSizeSd ${meta.effectSizeSd} !== 0.288`);
  }
  const primer = researchClaims.find((r) => r.id === "primer-efficacy");
  if (!primer || primer.status !== "EVIDENCE_NEEDED") {
    errors.push("primer-efficacy must exist with status EVIDENCE_NEEDED");
  }
  for (const r of researchClaims) {
    if (!r.safeClaim) errors.push(`${r.id}: missing safeClaim`);
    if (!r.caveat) errors.push(`${r.id}: missing caveat`);
    if (!r.sourceIds?.length) errors.push(`${r.id}: missing sourceIds`);
  }
  return errors;
});

check("sections manifest is complete", () => {
  const errors: string[] = [];
  const required = [
    "thesis",
    "problem",
    "product",
    "how-it-teaches",
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
    const s = sections.find((x) => x.id === id);
    if (!s) {
      errors.push(`missing section ${id}`);
      continue;
    }
    if (!s.navLabel) errors.push(`${id}: missing navLabel`);
    if (!s.headline) errors.push(`${id}: missing headline`);
    if (!s.body) errors.push(`${id}: missing body`);
    if (!s.artifactRequirement) errors.push(`${id}: missing artifactRequirement`);
    if (!STATUS_LABELS.has(s.status)) errors.push(`${id}: bad status`);
  }
  return errors;
});

check("no unsupported claim words outside qualified contexts", () => {
  const errors: string[] = [];
  const strings = collectStrings(investorData);

  for (const { path, text } of strings) {
    // Skip structural IDs / enums
    if (path.endsWith(".id") || path.endsWith(".status") || path.endsWith(".qualityTier")) {
      continue;
    }
    if (path.endsWith(".direction") || path.endsWith(".package") || path.endsWith(".proofType")) {
      continue;
    }
    if (path.includes("publicationBlocker") || path.includes("inlineCaveat")) {
      // blockers may mention the forbidden words as things to avoid
      if (/forbidden|do not|EVIDENCE NEEDED|not yet|unproven|no traction/i.test(text)) {
        continue;
      }
    }

    const lower = text.toLowerCase();
    for (const word of FORBIDDEN_CLAIM_WORDS) {
      if (!lower.includes(word)) continue;
      if (isAllowedForbiddenContext(text, word)) continue;
      errors.push(`${path}: unsupported word "${word}" in: ${text.slice(0, 120)}`);
    }
  }
  return errors;
});

check("founder proof includes eBackpack scale facts", () => {
  const errors: string[] = [];
  const eb = founderProof.find((e) => e.id === "ebackpack-lms");
  if (!eb) {
    errors.push("missing ebackpack-lms event");
    return errors;
  }
  const blob = JSON.stringify(eb).toLowerCase();
  if (!blob.includes("500,000") && !blob.includes("half a million")) {
    errors.push("ebackpack event missing half-million students fact");
  }
  if (!blob.includes("60,000")) {
    errors.push("ebackpack event missing 60,000 req/min fact");
  }
  if (!blob.includes("99.9")) {
    errors.push("ebackpack event missing 99.9% uptime fact");
  }
  return errors;
});

check("dataset IDs are unique within each collection", () => {
  const errors: string[] = [];
  const groups: [string, { id: string }[]][] = [
    ["productState", productState],
    ["pricingTiers", pricingTiers],
    ["marketLayers", marketLayers],
    ["seedScorecard", seedScorecard],
    ["researchClaims", researchClaims],
    ["competitorCategories", competitorCategories],
    ["founderProof", founderProof],
    ["sections", sections],
    ["fundingLineItems", fundingPlan.lineItems],
    ["instructionalLoop", instructionalLoop],
    ["teachingExamples", teachingExamples],
    ["adoptionLadder", adoptionLadder],
    ["problemPoints", problemPoints],
    ["beachheadJobs", beachheadJobs],
    ["teamPlan", teamPlan],
  ];
  for (const [name, rows] of groups) {
    const seen = new Map<string, number>();
    for (const row of rows) {
      seen.set(row.id, (seen.get(row.id) ?? 0) + 1);
    }
    for (const [id, n] of seen) {
      if (n > 1) errors.push(`${name}: duplicate id "${id}"`);
    }
  }
  return errors;
});

check("instructional loop has ordered nodes with status badges", () => {
  const errors: string[] = [];
  if (instructionalLoop.length < 6) {
    errors.push(`expected >= 6 loop nodes, got ${instructionalLoop.length}`);
  }
  const required = [
    "adult-direction",
    "planner",
    "specialist-tutor",
    "habit-checks",
    "revision-loop",
    "evidence-record",
    "transfer-project",
    "adult-exception-review",
  ];
  for (const id of required) {
    if (!instructionalLoop.some((n) => n.id === id)) {
      errors.push(`missing loop node ${id}`);
    }
  }
  for (const node of instructionalLoop) {
    if (!node.name || !node.summary) errors.push(`${node.id}: missing name/summary`);
    if (!["LIVE", "IN_DEVELOPMENT", "PLANNED"].includes(node.status)) {
      errors.push(`${node.id}: invalid product status ${node.status}`);
    }
  }
  // LLM must not be a central/named node
  for (const node of instructionalLoop) {
    if (/\bllm\b|\blanguage model\b/i.test(`${node.name} ${node.summary}`)) {
      errors.push(`${node.id}: LLM must not be a loop node`);
    }
  }
  return errors;
});

check("teaching examples cover 4–6 ruled mechanisms", () => {
  const errors: string[] = [];
  if (teachingExamples.length < 4 || teachingExamples.length > 6) {
    errors.push(`expected 4–6 teaching examples, got ${teachingExamples.length}`);
  }
  for (const ex of teachingExamples) {
    if (!ex.title || !ex.principle || !ex.example) {
      errors.push(`${ex.id}: missing title/principle/example`);
    }
  }
  return errors;
});

check("adoption ladder has family-to-institution rungs", () => {
  const errors: string[] = [];
  if (adoptionLadder.length < 5) {
    errors.push(`expected >= 5 adoption rungs, got ${adoptionLadder.length}`);
  }
  const required = [
    "family-base",
    "family-core",
    "esa-microschool",
    "premier-elite-support",
    "lti-clever-pilot",
    "tuition-embedded",
  ];
  for (const id of required) {
    if (!adoptionLadder.some((r) => r.id === id)) errors.push(`missing rung ${id}`);
  }
  for (const rung of adoptionLadder) {
    if (!rung.buyer || !rung.proof || !rung.integration) {
      errors.push(`${rung.id}: missing buyer/proof/integration`);
    }
  }
  return errors;
});

check("problem points and beachhead jobs are present", () => {
  const errors: string[] = [];
  if (problemPoints.length < 4) errors.push("expected >= 4 problem points");
  if (!problemPoints.some((p) => p.side === "founder-example")) {
    errors.push("missing founder-example problem point");
  }
  if (beachheadJobs.length < 2) errors.push("expected >= 2 beachhead jobs");
  if (teamPlan.length < 3) errors.push("expected founder + 2 planned roles");
  return errors;
});

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

let failed = 0;
for (const r of results) {
  if (r.ok) {
    console.log(`  PASS  ${r.name}`);
  } else {
    failed += 1;
    console.error(`  FAIL  ${r.name}`);
    for (const d of r.details) console.error(`         - ${d}`);
  }
}

console.log("");
console.log(
  failed === 0
    ? `All ${results.length} checks passed.`
    : `${failed}/${results.length} checks failed.`,
);

process.exit(failed === 0 ? 0 : 1);
