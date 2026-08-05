import type { FundingPlan } from "./types";

/**
 * Working pre-seed: $3.5M / 18 months.
 * Bottom-up people + non-payroll + 20% contingency.
 *
 * Base case (founder + 2):
 *   people $1,764,000 + non-payroll ~$1,000,000 = $2,764,000
 *   + 20% contingency $552,800 ≈ $3.32M → working ask $3.5M
 */
export const fundingPlan: FundingPlan = {
  roundName: "Pre-seed",
  instrument: "Working target — terms not finalized",
  amountUsd: 3_500_000,
  runwayMonths: 18,
  teamSize: 3,
  annualCashSalaryUsd: 300_000,
  employmentLoadRate: 0.3,
  contingencyRate: 0.2,
  peopleCost18MoUsd: 1_764_000,
  nonPayrollBaseUsd: 1_000_000,
  contingencyUsd: 552_800,
  totalUsd: 3_500_000,
  compensationAnnotation:
    "Cash-forward compensation recruits staff-level owners for product, education, and platform outcomes. Each role budgeted at $300,000 annual cash salary plus strong benefits (~30% employment load). Financing model shows salary, benefits, equity, and capitalization as one plan.",
  lineItems: [
    {
      id: "people-founder-plus-two",
      category: "People (founder + 2 at $300k cash + 30% load + equipment)",
      leanUsd: 1_176_000,
      fullUsd: 1_764_000,
      driver: "Headcount × $300k annual cash × 1.5 years × 1.30 employment load + setup",
    },
    {
      id: "model-inference",
      category: "Model inference and evaluation",
      leanUsd: 150_000,
      fullUsd: 450_000,
      driver: "active learners × sessions × calls × tokens × model mix",
    },
    {
      id: "cloud-media",
      category: "Cloud, media, storage, observability",
      leanUsd: 100_000,
      fullUsd: 300_000,
      driver: "environments, uptime target, video/content usage",
    },
    {
      id: "hardware",
      category: "Devices, development hardware, test lab",
      leanUsd: 40_000,
      fullUsd: 120_000,
      driver: "workstations, mobile devices, supervised-station prototypes",
    },
    {
      id: "curriculum-research",
      category: "Curriculum, assessment, and research",
      leanUsd: 100_000,
      fullUsd: 300_000,
      driver: "external reviewers, instruments, study support, content licensing",
    },
    {
      id: "compliance",
      category: "Privacy, security, accessibility, legal",
      leanUsd: 125_000,
      fullUsd: 300_000,
      driver: "counsel, policies, penetration testing, VPAT, compliance preparation",
    },
    {
      id: "discovery-pilots",
      category: "Customer discovery and pilot support",
      leanUsd: 75_000,
      fullUsd: 200_000,
      driver: "recruitment, travel, support, conventions, pilot operations",
    },
    {
      id: "admin-ops",
      category: "Software, insurance, accounting, administration",
      leanUsd: 75_000,
      fullUsd: 175_000,
      driver: "recurring operating stack and corporate requirements",
    },
  ],
  milestones: [
    "Paid remediation workflow from syllabus ingestion through independent reassessment",
    "Curriculum planning, assessment evaluation, and standards/mastery records",
    "At least 1,000 and preferably 1,500 paying learners",
    "$600K–$900K ARR run rate at the Base $50 monthly price",
    "At least 75% and preferably 85%+ 12-week paid retention",
    "Mastery-scoring agreement and state-aligned learning progression on independent measures",
    "At least 60% and preferably 70%+ post-credit gross margin",
    "≤$20 and preferably ≤$15 COGS per learner per month",
    "Sub-eight-month and preferably five-month CAC payback",
    "Observed inference, infrastructure, onboarding, and support economics by usage percentile",
    "Controlled Core $100 conversion tests",
    "At least 20 paid Premier $300 families or one elite-school design partner",
    "LTI and Clever pilot readiness after family proof",
  ],
  status: "HYPOTHESIS",
  sourceIds: ["funding-plan-internal", "seed-readiness-internal"],
  publicationBlockers: [
    "Quantified customers, revenue, retention, and learning outcomes",
    "Exact hiring timing and option grants",
    "Total financing + option-pool dilution under proposed instrument",
    "Next-round proof points and financing terms",
  ],
};
