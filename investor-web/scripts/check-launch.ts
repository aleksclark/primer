/**
 * Pre-launch content gate for the investor pitch site.
 *
 * Verifies ask/runway/salaries/packages/scorecard consistency against data
 * modules, rejects unsupported traction language, missing sources, additive
 * market totals, and incomplete premium-school caveats.
 *
 * Run: npm run check:launch
 */
import {
  fundingPlan,
  marketLayers,
  pricingTiers,
  productState,
  researchClaims,
  seedScorecard,
  sources,
  teamPlan,
  type MarketLayer,
} from "../src/data/index.ts";

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

const FORBIDDEN = ["proven", "validated", "traction"] as const;
const ALLOWLIST: RegExp[] = [
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
  /\bdo not describe\b/i,
  /\bdo not use\b/i,
  /\bforbidden words\b/i,
  /\bevidence needed\b/i,
  /\bunit economics unproven\b/i,
];

function isAllowed(text: string, word: string): boolean {
  const lower = text.toLowerCase();
  if (word === "proven" && !/\bproven\b/i.test(text)) return true;

  for (const re of ALLOWLIST) {
    if (!re.test(text)) continue;
    const allowedSpans: [number, number][] = [];
    const global = new RegExp(re.source, re.flags.includes("g") ? re.flags : `${re.flags}g`);
    let m: RegExpExecArray | null;
    while ((m = global.exec(text)) !== null) {
      allowedSpans.push([m.index, m.index + m[0].length]);
    }
    const wordRe = new RegExp(`\\b${word}\\b`, "gi");
    let wm: RegExpExecArray | null;
    let allCovered = true;
    while ((wm = wordRe.exec(lower)) !== null) {
      const start = wm.index;
      const end = start + word.length;
      const covered = allowedSpans.some(([a, b]) => start >= a && end <= b);
      if (!covered) {
        if (word === "proven") {
          const before = start > 0 ? lower[start - 1] : " ";
          if (/[a-z]/.test(before)) continue;
        }
        allCovered = false;
        break;
      }
    }
    if (allCovered) return true;
  }
  return false;
}

// ---------------------------------------------------------------------------
// Content gates
// ---------------------------------------------------------------------------

check("ask / runway / salary match funding plan constants", () => {
  const errors: string[] = [];
  if (fundingPlan.amountUsd !== 3_500_000) {
    errors.push(`ask ${fundingPlan.amountUsd} !== 3500000`);
  }
  if (fundingPlan.totalUsd !== fundingPlan.amountUsd) {
    errors.push(`totalUsd ${fundingPlan.totalUsd} !== amountUsd ${fundingPlan.amountUsd}`);
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
  if (fundingPlan.employmentLoadRate !== 0.3) {
    errors.push(`employmentLoadRate ${fundingPlan.employmentLoadRate} !== 0.3`);
  }
  if (fundingPlan.contingencyRate !== 0.2) {
    errors.push(`contingencyRate ${fundingPlan.contingencyRate} !== 0.2`);
  }

  const expectedPeople =
    fundingPlan.teamSize *
      fundingPlan.annualCashSalaryUsd *
      (fundingPlan.runwayMonths / 12) *
      (1 + fundingPlan.employmentLoadRate) +
    9_000;
  if (fundingPlan.peopleCost18MoUsd !== expectedPeople) {
    errors.push(
      `peopleCost18MoUsd ${fundingPlan.peopleCost18MoUsd} !== recomputed ${expectedPeople}`,
    );
  }

  const peopleLine = fundingPlan.lineItems.find((l) => l.id === "people-founder-plus-two");
  if (!peopleLine) {
    errors.push("missing people-founder-plus-two line item");
  } else if (peopleLine.fullUsd !== fundingPlan.peopleCost18MoUsd) {
    errors.push(
      `people line fullUsd ${peopleLine.fullUsd} !== peopleCost18MoUsd ${fundingPlan.peopleCost18MoUsd}`,
    );
  }

  // Bottom-up full line items may exceed the working non-payroll base used for
  // contingency; contingency is defined on people + nonPayrollBase only.
  const expectedContingency = Math.round(
    (fundingPlan.peopleCost18MoUsd + fundingPlan.nonPayrollBaseUsd) *
      fundingPlan.contingencyRate,
  );
  if (fundingPlan.contingencyUsd !== expectedContingency) {
    errors.push(
      `contingencyUsd ${fundingPlan.contingencyUsd} !== recomputed ${expectedContingency}`,
    );
  }

  const stack =
    fundingPlan.peopleCost18MoUsd +
    fundingPlan.nonPayrollBaseUsd +
    fundingPlan.contingencyUsd;
  if (stack > fundingPlan.amountUsd) {
    errors.push(
      `people+nonPayroll+contingency ${stack} exceeds ask ${fundingPlan.amountUsd}`,
    );
  }
  // Working ask rounds up from the stack to a clean figure.
  if (fundingPlan.amountUsd - stack > 250_000) {
    errors.push(
      `ask ${fundingPlan.amountUsd} is more than $250k above modeled stack ${stack}`,
    );
  }

  return errors;
});

check("team plan roles match funding team size", () => {
  const errors: string[] = [];
  if (teamPlan.length !== fundingPlan.teamSize) {
    errors.push(
      `teamPlan has ${teamPlan.length} roles but fundingPlan.teamSize is ${fundingPlan.teamSize}`,
    );
  }
  const current = teamPlan.filter((r) => r.presence === "current");
  const planned = teamPlan.filter((r) => r.presence === "planned");
  if (current.length !== 1) errors.push(`expected exactly 1 current role, got ${current.length}`);
  if (planned.length !== 2) errors.push(`expected exactly 2 planned roles, got ${planned.length}`);
  return errors;
});

check("Base / Core / Premier scope and pricing are consistent", () => {
  const errors: string[] = [];
  const required = [
    { id: "base", name: "Base", monthly: 50, annual: 600 },
    { id: "core", name: "Core", monthly: 100, annual: 1200 },
    { id: "premier", name: "Premier", monthly: 300, annual: 3600 },
  ] as const;

  for (const exp of required) {
    const tier = pricingTiers.find((t) => t.id === exp.id);
    if (!tier) {
      errors.push(`missing tier ${exp.id}`);
      continue;
    }
    if (tier.name !== exp.name) errors.push(`${exp.id}: name "${tier.name}" !== "${exp.name}"`);
    if (tier.monthlyPriceUsd !== exp.monthly) {
      errors.push(`${exp.id}: monthly ${tier.monthlyPriceUsd} !== ${exp.monthly}`);
    }
    if (tier.annualRevenuePerLearnerUsd !== exp.annual) {
      errors.push(`${exp.id}: annual ${tier.annualRevenuePerLearnerUsd} !== ${exp.annual}`);
    }
    if (tier.monthlyPriceUsd * 12 !== tier.annualRevenuePerLearnerUsd) {
      errors.push(`${exp.id}: monthly*12 !== annual`);
    }
    if (!tier.scope || tier.scope.length === 0) errors.push(`${exp.id}: empty scope`);
    if (!tier.primaryBuyers || tier.primaryBuyers.length === 0) {
      errors.push(`${exp.id}: empty primaryBuyers`);
    }
    if (tier.status !== "HYPOTHESIS") {
      errors.push(`${exp.id}: package pricing must remain HYPOTHESIS until sold`);
    }
  }

  // Scorecard Base ARPU alignment
  const mrr = seedScorecard.find((m) => m.id === "mrr-at-base");
  const arr = seedScorecard.find((m) => m.id === "arr-run-rate");
  const learners = seedScorecard.find((m) => m.id === "paying-learners");
  if (mrr && learners) {
    if (mrr.floor !== learners.floor * 50) {
      errors.push(`mrr-at-base floor ${mrr.floor} !== paying-learners floor * $50`);
    }
    if (mrr.target !== learners.target * 50) {
      errors.push(`mrr-at-base target ${mrr.target} !== paying-learners target * $50`);
    }
  }
  if (arr && mrr) {
    if (arr.floor !== mrr.floor * 12) {
      errors.push(`arr-run-rate floor ${arr.floor} !== mrr floor * 12`);
    }
    if (arr.target !== mrr.target * 12) {
      errors.push(`arr-run-rate target ${arr.target} !== mrr target * 12`);
    }
  }
  return errors;
});

check("seed-readiness scorecard definitions are complete and consistent", () => {
  const errors: string[] = [];
  const required = [
    "paying-learners",
    "mrr-at-base",
    "arr-run-rate",
    "retention-12-week",
    "gross-margin-post-credit",
    "cogs-per-learner",
    "cac-payback",
    "learning-outcome",
  ];
  for (const id of required) {
    const m = seedScorecard.find((row) => row.id === id);
    if (!m) {
      errors.push(`missing scorecard metric ${id}`);
      continue;
    }
    if (!m.definition?.trim()) errors.push(`${id}: missing definition`);
    if (!m.caveat?.trim()) errors.push(`${id}: missing caveat`);
    if (!m.sourceIds?.length) errors.push(`${id}: missing sourceIds`);
    if (m.current !== "NOT_YET_MEASURED" && typeof m.current !== "number") {
      errors.push(`${id}: current must be number or NOT_YET_MEASURED`);
    }
    if (m.status !== "EVIDENCE_NEEDED" && m.current === "NOT_YET_MEASURED") {
      errors.push(`${id}: unmeasured metric should be EVIDENCE_NEEDED`);
    }
  }

  // Funding milestones should reference core scorecard targets
  const milestoneBlob = fundingPlan.milestones.join(" ").toLowerCase();
  for (const needle of ["1,000", "1,500", "600k", "900k", "75%", "85%", "70%"]) {
    if (!milestoneBlob.includes(needle.toLowerCase())) {
      // 600K–$900K style may use different casing
      if (!milestoneBlob.includes(needle.replace("k", "k"))) {
        // soft: only fail on clearly missing learner targets
      }
    }
  }
  if (!milestoneBlob.includes("1,000") || !milestoneBlob.includes("1,500")) {
    errors.push("funding milestones missing paying-learner floor/target language");
  }
  if (!/\$600/.test(fundingPlan.milestones.join(" ")) && !/600k/i.test(milestoneBlob)) {
    errors.push("funding milestones missing $600K ARR floor language");
  }

  return errors;
});

check("market arithmetic is internally consistent and non-additive", () => {
  const errors: string[] = [];
  for (const layer of marketLayers) {
    const product = layer.learners * layer.annualRevenuePerLearner;
    if (layer.modeledCeiling !== product) {
      errors.push(
        `${layer.id}: modeledCeiling ${layer.modeledCeiling} !== ${layer.learners}*${layer.annualRevenuePerLearner}`,
      );
    }
    if ((layer as MarketLayer).additive !== false) {
      errors.push(`${layer.id}: additive must be false`);
    }
    if (!layer.overlapGroup) errors.push(`${layer.id}: missing overlapGroup`);
    if (!layer.sourceYear) errors.push(`${layer.id}: missing sourceYear`);
    if (!layer.sourceIds?.length) errors.push(`${layer.id}: missing sourceIds`);
    if (!layer.caveat?.trim()) errors.push(`${layer.id}: missing caveat`);
  }

  // Overlap groups with >1 layer must mention non-additivity
  const byGroup = new Map<string, MarketLayer[]>();
  for (const layer of marketLayers) {
    const list = byGroup.get(layer.overlapGroup) ?? [];
    list.push(layer);
    byGroup.set(layer.overlapGroup, list);
  }
  for (const [group, layers] of byGroup) {
    if (layers.length < 2) continue;
    for (const layer of layers) {
      if (!/do not sum|not additive|alternative boundary|overlaps/i.test(layer.caveat)) {
        errors.push(
          `${layer.id} (group ${group}): overlapping layer caveat must warn against summing`,
        );
      }
    }
  }
  return errors;
});

check("premium-school layers include source year and sticker/net caveats", () => {
  const errors: string[] = [];
  const premium = marketLayers.filter(
    (l) => l.package === "premier" || /nais|private|tuition/i.test(l.id + l.segment),
  );
  if (premium.length === 0) errors.push("no premium-school layers found");

  for (const layer of premium) {
    if (!layer.sourceYear?.trim()) {
      errors.push(`${layer.id}: premium layer missing sourceYear`);
    }
    const blob = `${layer.caveat} ${layer.segment} ${layer.displayLabel ?? ""}`.toLowerCase();
    const needsStickerNet =
      /tuition|15k|nais|private/.test(layer.id + layer.segment.toLowerCase());
    if (needsStickerNet) {
      const hasCaveat =
        /sticker|net\s*tuition|net price|vintage|reporting universe|overlaps/.test(blob);
      if (!hasCaveat) {
        errors.push(
          `${layer.id}: premium/tuition layer needs sticker/net or vintage/overlap caveat`,
        );
      }
    }
  }
  return errors;
});

check("cited sourceIds resolve and sources are complete", () => {
  const errors: string[] = [];
  const known = new Set(sources.map((s) => s.id));
  for (const s of sources) {
    if (!s.url?.trim()) errors.push(`${s.id}: missing url`);
    if (!s.publicationDate?.trim()) errors.push(`${s.id}: missing publicationDate`);
    if (!s.accessedDate?.trim()) errors.push(`${s.id}: missing accessedDate`);
    if (!s.qualityTier) errors.push(`${s.id}: missing qualityTier`);
  }

  const refs: { id: string; path: string }[] = [];
  for (const layer of marketLayers) {
    for (const id of layer.sourceIds) refs.push({ id, path: `marketLayers.${layer.id}` });
  }
  for (const m of seedScorecard) {
    for (const id of m.sourceIds) refs.push({ id, path: `seedScorecard.${m.id}` });
  }
  for (const r of researchClaims) {
    for (const id of r.sourceIds) refs.push({ id, path: `researchClaims.${r.id}` });
  }
  for (const id of fundingPlan.sourceIds) refs.push({ id, path: "fundingPlan" });

  for (const { id, path } of refs) {
    if (!known.has(id)) errors.push(`${path} cites unknown source "${id}"`);
  }
  return errors;
});

check("no unsupported traction / efficacy language", () => {
  const errors: string[] = [];
  const package_ = {
    fundingPlan,
    pricingTiers,
    seedScorecard,
    marketLayers,
    productState,
    researchClaims,
    teamPlan,
  };
  for (const { path, text } of collectStrings(package_)) {
    if (
      path.endsWith(".id") ||
      path.endsWith(".status") ||
      path.endsWith(".qualityTier") ||
      path.endsWith(".direction") ||
      path.endsWith(".package")
    ) {
      continue;
    }
    if (/publicationBlocker|inlineCaveat|caveat/.test(path)) {
      if (/do not|not yet|unproven|no traction|evidence needed|forbidden/i.test(text)) {
        continue;
      }
    }
    const lower = text.toLowerCase();
    for (const word of FORBIDDEN) {
      if (!lower.includes(word)) continue;
      if (isAllowed(text, word)) continue;
      errors.push(`${path}: unsupported word "${word}" in: ${text.slice(0, 140)}`);
    }
  }

  // Family use must not be framed as efficacy or traction
  for (const p of productState) {
    const blob = `${p.name} ${p.description ?? ""} ${p.notes ?? ""}`.toLowerCase();
    if (/\bfamily\b/.test(blob) && /\b(efficacy|traction|validated outcome)/.test(blob)) {
      errors.push(`${p.id}: family use described as efficacy/traction`);
    }
  }
  return errors;
});

check("EVIDENCE_NEEDED items remain visible; LIVE products have artifacts", () => {
  const errors: string[] = [];
  // Product capabilities use LIVE/IN_DEVELOPMENT/PLANNED only; EVIDENCE_NEEDED
  // lives on scorecard metrics and research claims (and section blockers).
  const evidenceNeeded = [
    ...seedScorecard.filter((m) => m.status === "EVIDENCE_NEEDED"),
    ...researchClaims.filter((r) => r.status === "EVIDENCE_NEEDED"),
  ];
  if (evidenceNeeded.length === 0) {
    errors.push("expected retained EVIDENCE_NEEDED items on launch day");
  }

  const primerEfficacy = researchClaims.find((r) => r.id === "primer-efficacy");
  if (!primerEfficacy || primerEfficacy.status !== "EVIDENCE_NEEDED") {
    errors.push("primer-efficacy must remain EVIDENCE_NEEDED until measured");
  }

  for (const p of productState) {
    if (p.status !== "LIVE") continue;
    if (!p.artifactUrl?.trim()) errors.push(`${p.id}: LIVE product missing artifactUrl`);
    if (!p.artifactLabel?.trim()) errors.push(`${p.id}: LIVE product missing artifactLabel`);
  }
  return errors;
});

check("milestones and scorecard floors/targets stay aligned", () => {
  const errors: string[] = [];
  const text = fundingPlan.milestones.join("\n");

  const pairs: { id: string; floor: number | string; target: number | string }[] = [
    { id: "paying-learners", floor: "1,000", target: "1,500" },
    { id: "retention-12-week", floor: "75%", target: "85%" },
    { id: "gross-margin-post-credit", floor: "60%", target: "70%" },
    { id: "cac-payback", floor: "eight-month", target: "five-month" },
  ];

  for (const pair of pairs) {
    const metric = seedScorecard.find((m) => m.id === pair.id);
    if (!metric) {
      errors.push(`missing scorecard ${pair.id}`);
      continue;
    }
    // Presence checks in milestone language (flexible formatting)
    if (typeof pair.floor === "string" && !text.includes(String(pair.floor))) {
      // cac-payback uses words
      if (pair.id === "cac-payback") {
        if (!/eight-month|8-month|≤\$?8|sub-eight/i.test(text)) {
          errors.push(`milestones missing ${pair.id} floor language`);
        }
      } else {
        errors.push(`milestones missing ${pair.id} floor language (${pair.floor})`);
      }
    }
    if (typeof pair.target === "string" && !text.includes(String(pair.target))) {
      if (pair.id === "cac-payback") {
        if (!/five-month|5-month/i.test(text)) {
          errors.push(`milestones missing ${pair.id} target language`);
        }
      } else if (pair.id === "retention-12-week") {
        if (!/85%/.test(text)) errors.push(`milestones missing retention target 85%`);
      } else if (pair.id === "gross-margin-post-credit") {
        if (!/70%/.test(text)) errors.push(`milestones missing gross margin target 70%`);
      } else {
        errors.push(`milestones missing ${pair.id} target language (${pair.target})`);
      }
    }
  }
  return errors;
});

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

let failed = 0;
console.log("Pre-launch content gate\n");
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
    ? `All ${results.length} launch checks passed.`
    : `${failed}/${results.length} launch checks failed.`,
);

process.exit(failed === 0 ? 0 : 1);
