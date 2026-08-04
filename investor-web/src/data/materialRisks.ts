import type { StatusLabel } from "./types";

/** Public material risks — no private contracts, security internals, or cap table. */

export interface MaterialRisk {
  id: string;
  order: number;
  title: string;
  summary: string;
  mitigation: string;
  status: StatusLabel;
}

export interface DiligenceIndexEntry {
  id: string;
  title: string;
  href: string;
  summary: string;
  updated: string;
  access: "public" | "gated_placeholder";
}

export const DILIGENCE_UPDATED = "2026-08-04";

export const materialRisks: MaterialRisk[] = [
  {
    id: "early-use",
    order: 1,
    title: "Very early product use",
    summary:
      "Approximately one week of family discovery use is insufficient for retention or outcome evidence.",
    mitigation:
      "Label all current use as discovery. Seed scorecard currents stay NOT YET MEASURED until observed.",
    status: "LIVE",
  },
  {
    id: "solo-founder",
    order: 2,
    title: "Solo-founder concentration",
    summary:
      "Curriculum, safety, privacy, accessibility, and product judgment currently rest with Aleks.",
    mitigation:
      "18-month plan hires an educator/product owner and a product/platform builder; role ownership is explicit.",
    status: "LIVE",
  },
  {
    id: "founder-bias",
    order: 3,
    title: "Founder-bias risk",
    summary:
      "Risk of building the product the founder wants rather than one with broad demand.",
    mitigation:
      "Paid retention, independent learning progression, and design-partner evidence are the gates — not founder preference.",
    status: "HYPOTHESIS",
  },
  {
    id: "autonomous-assessment",
    order: 4,
    title: "Autonomous assessment risk",
    summary: "Mastery evaluation may fail silently or be gamed.",
    mitigation:
      "Measurement-validity stage before efficacy claims; blind expert agreement; anomaly checks; human review for consequential decisions.",
    status: "PLANNED",
  },
  {
    id: "inference-economics",
    order: 5,
    title: "Inference economics",
    summary: "Usage variance may undermine the $50 Base price hypothesis.",
    mitigation:
      "Metered allowances, percentile COGS reporting, and seed floors on median and p95 COGS before scaling acquisition.",
    status: "EVIDENCE_NEEDED",
  },
  {
    id: "consumer-acquisition",
    order: 6,
    title: "Consumer acquisition",
    summary: "Homeschool and parent channels are fragmented and trust-heavy.",
    mitigation:
      "Family-first wedge, ESA/microschool aggregators later, sales/marketing spend only after retention evidence.",
    status: "HYPOTHESIS",
  },
  {
    id: "institutional-burden",
    order: 7,
    title: "Institutional burden",
    summary:
      "Supplementary entry lowers but does not remove privacy, security, accessibility, and procurement requirements.",
    mitigation:
      "Explicit school-readiness gates; no broad district selling before DPA, accessibility, and SSO/roster path.",
    status: "PLANNED",
  },
  {
    id: "incumbent-bundling",
    order: 8,
    title: "Incumbent bundling",
    summary: "Generic AI and incumbent LMS platforms can add tutoring features.",
    mitigation:
      "Compete on adult-directed longitudinal planning, cross-subject habits, mastery evidence, and projects — not chat novelty.",
    status: "HYPOTHESIS",
  },
  {
    id: "evidence-transfer",
    order: 9,
    title: "Evidence transfer",
    summary: "Human tutoring research does not establish LLM efficacy.",
    mitigation:
      "Research justifies mechanism hypothesis only. Company proof plan and null-result policy are public.",
    status: "LIVE",
  },
  {
    id: "brand-collision",
    order: 10,
    title: "Brand collision",
    summary:
      "An existing Primer microschool company overlaps in name, customer, and positioning.",
    mitigation:
      "Trademark and positioning diligence before broad launch marketing; clarify instructional-LMS identity.",
    status: "HYPOTHESIS",
  },
  {
    id: "compact-team",
    order: 11,
    title: "Compact-team execution",
    summary:
      "Plan concentrates responsibility in three senior roles. Hiring quality and succession coverage matter.",
    mitigation:
      "Milestone-tied hiring, explicit ownership lists, and contingency in the $3.5M working ask.",
    status: "HYPOTHESIS",
  },
  {
    id: "data-retention",
    order: 12,
    title: "Data retention policy gap",
    summary:
      "Permanent LMS retention conflicts with data-minimization and institutional deletion expectations.",
    mitigation:
      "Revise to purpose-limited retention by record class before launch; document in compliance roadmap.",
    status: "IN_DEVELOPMENT",
  },
  {
    id: "beta-training",
    order: 13,
    title: "Anonymized beta training",
    summary:
      "Any future training programme needs rigorous consent, deidentification, and legal review.",
    mitigation:
      "Default: no training on identifiable student data. Any future programme is opt-in, withdrawable, and does not gate core access.",
    status: "PLANNED",
  },
];

export const diligenceIndex: DiligenceIndexEntry[] = [
  {
    id: "product-state",
    title: "Product state",
    href: "/demo",
    summary:
      "Live surfaces, target instructional experience, and capability inventory with status labels.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "market-model",
    title: "Market model",
    href: "/market",
    summary:
      "Source-year tables, Base/Core/Premier layers, contracting models, price comparisons, and overlap methodology.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "evidence-register",
    title: "Evidence and research basis",
    href: "/evidence",
    summary:
      "Tutoring/mastery/adaptive research, Bloom caveat, measurement plan, thresholds, and null-result policy.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "schools-compliance",
    title: "Schools and compliance roadmap",
    href: "/schools",
    summary:
      "Family-to-school motion, LTI/Clever/OneRoster path, Blackbaud/Veracross strategy, privacy and accessibility gates.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "company",
    title: "Company, founder, and team",
    href: "/company",
    summary: "Founder timeline, eBackpack proof, current team, 18-month roles, and references process.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "funding-plan",
    title: "Funding plan (public summary)",
    href: "/#round",
    summary:
      "Working $3.5M pre-seed / 18 months. Role- and milestone-based use of funds on the primary page. No cap table publicly.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "seed-economics",
    title: "Seed-readiness economics",
    href: "/#round",
    summary:
      "Scorecard floors and targets (learners, retention, COGS, CAC payback, learning). Currents NOT YET MEASURED.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "material-risks",
    title: "Material risks",
    href: "/diligence#risks",
    summary: "Thirteen public material risks with mitigations. No private incident detail.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "source-register",
    title: "Source register",
    href: "/evidence#source-register",
    summary: "Full citation list with quality tiers, vintages, and URLs.",
    updated: DILIGENCE_UPDATED,
    access: "public",
  },
  {
    id: "data-room",
    title: "Gated data room",
    href: "/diligence#data-room",
    summary:
      "Placeholder only. Private financial model detail, security architecture, contracts, family data, and cap table are not on this site.",
    updated: DILIGENCE_UPDATED,
    access: "gated_placeholder",
  },
];

export const publicPrivateBoundary = [
  "Public: product status, market equations, research claims, school roadmap, founder timeline, funding headline, seed scorecard definitions, material risks, sources.",
  "Not public: private contracts, family or student data, detailed security architecture, subprocessors beyond high-level posture, cap table, option grants, or live credentials.",
  "Gated data room (when opened under NDA): full financial model, architecture diagrams appropriate for diligence, reference intros, and employment verification — still no unnecessary student data.",
];
